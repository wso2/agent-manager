// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package modhost

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
)

// externalFile is the name every external schema file has: a three-digit
// version, a description and the store's own extension.
var externalFile = regexp.MustCompile(`^[0-9]{3}_[a-z0-9_]+\.[a-z0-9]+$`)

// externalCheckTimeout bounds the install and enable check of an external store.
const externalCheckTimeout = 10 * time.Second

// externalMigrateTimeout bounds one module's external schema migration,
// waiting for the lease included.
const externalMigrateTimeout = 5 * time.Minute

// externalLock is the core lease row that serialises external schema migrations.
const externalLock = "modhost.external_schema"

// externalLockTTL outlives a migration bounded by externalMigrateTimeout.
const externalLockTTL = externalMigrateTimeout + time.Minute

// externalLockPoll is how often a migration waiting for the lease retries.
const externalLockPoll = time.Second

// moduleConfig reads a module's settings, as Runtime.Config does.
func moduleConfig(name string) func(key string) string {
	return func(key string) string { return config.Value(config.ModuleConfigKey(name, key)) }
}

// externalSchema returns the module's external schema, or nil with the reason
// when it has no store configured.
func (h *Host) externalSchema(name string, apply bool) (modkit.ExternalSchema, string, error) {
	em, ok := h.mods[name].(modkit.ExternalMigrator)
	if !ok {
		return nil, "no external store", nil
	}
	s, err := em.ExternalSchema(moduleConfig(name), apply)
	switch {
	case errors.Is(err, modkit.ErrNotConfigured):
		return nil, err.Error(), nil
	case err != nil:
		return nil, "", fmt.Errorf("module %s external schema: %w", name, err)
	case s == nil:
		return nil, "not configured", nil
	}
	return s, "", nil
}

// loadExternal reads an external schema's files in lexical order; two files
// may not share a version.
func loadExternal(module string, s modkit.ExternalSchema) ([]migration, error) {
	fsys := s.Files()
	if fsys == nil {
		return nil, nil
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("module %s %s schema: %w", module, s.Store(), err)
	}
	var out []migration
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !externalFile.MatchString(e.Name()) {
			return nil, fmt.Errorf("module %s %s schema file %s: files are named NNN_description.<ext>", module, s.Store(), e.Name())
		}
		body, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("module %s %s schema: %w", module, s.Store(), err)
		}
		sum := sha256.Sum256(body)
		out = append(out, migration{
			Version:  strings.TrimSuffix(e.Name(), path.Ext(e.Name())),
			Checksum: hex.EncodeToString(sum[:]),
			body:     body,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	for i := 1; i < len(out); i++ {
		if out[i].Version == out[i-1].Version {
			return nil, fmt.Errorf("module %s %s schema: more than one file is version %s", module, s.Store(), out[i].Version)
		}
	}
	return out, nil
}

// migrateExternal applies a module's pending external schema files, then
// settles the store. With no store configured it logs that and does nothing.
func (h *Host) migrateExternal(ctx context.Context, name string) (*ExternalResult, error) {
	s, skipped, err := h.externalSchema(name, true)
	if err != nil {
		return nil, err
	}
	if s == nil {
		h.log.Info("modhost:skipping external schema", "module", name, "reason", skipped)
		return &ExternalResult{Skipped: skipped}, nil
	}
	ms, err := loadExternal(name, s)
	if err != nil {
		return nil, err
	}
	res := &ExternalResult{Store: s.Store(), Total: len(ms)}
	what := fmt.Sprintf("module %s %s schema", name, s.Store())
	ctx, cancel := context.WithTimeout(ctx, externalMigrateTimeout)
	defer cancel()
	// A lease, not a held transaction, serialises migrators, so the store's
	// calls never hold the core database's write lock.
	owner, err := h.takeExternalLock(ctx, name, s.Store())
	if err != nil {
		return res, err
	}
	defer h.releaseExternalLock(ctx, owner)
	applied, err := s.Applied(ctx)
	if err != nil {
		return res, fmt.Errorf("%s: reading what was applied: %w", what, err)
	}
	todo, err := pendingFrom(what, applied, ms)
	if err != nil {
		return res, err
	}
	for _, m := range todo {
		h.log.Info("modhost:applying external schema", "module", name, "store", s.Store(), "version", m.Version)
		if err := s.Apply(ctx, m.Version, m.Checksum, m.body); err != nil {
			return res, fmt.Errorf("%s %s: %w", what, m.Version, err)
		}
		res.Applied++
	}
	if err := s.Settle(ctx); err != nil {
		return res, fmt.Errorf("%s: %w", what, err)
	}
	return res, nil
}

// takeExternalLock takes the external schema lease for a new owner, retrying
// while another run holds it until ctx ends, and returns the owner.
func (h *Host) takeExternalLock(ctx context.Context, module, store string) (string, error) {
	owner, err := externalLockOwner()
	if err != nil {
		return "", err
	}
	for {
		ok, err := dbx.TryLock(ctx, h.db, "locks", externalLock, owner, dbx.Now(), externalLockTTL)
		if err != nil && ctx.Err() == nil {
			return "", fmt.Errorf("taking the external schema lease: %w", err)
		}
		if ok {
			return owner, nil
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("the lease for applying %s's %s schema is still held, by another amp migrate or one that stopped (a lease expires %s after it is taken); try again: %w", module, store, externalLockTTL, ctx.Err())
		case <-time.After(externalLockPoll):
		}
	}
}

// releaseExternalLock frees the lease, on a context of its own since ctx may
// have ended.
func (h *Host) releaseExternalLock(ctx context.Context, owner string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), externalCheckTimeout)
	defer cancel()
	if err := dbx.Unlock(ctx, h.db, "locks", externalLock, owner); err != nil {
		h.log.Warn("modhost:releasing the external schema lease", "error", err)
	}
}

// externalLockOwner names one migration run: host, pid and a random suffix.
func externalLockOwner() (string, error) {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%d/%s", host, os.Getpid(), hex.EncodeToString(suffix)), nil
}

// externalPlanReady runs externalReady for each module installing name would
// activate, before the lifecycle transaction, so a slow store never holds the
// generation lock. It returns the modules it checked.
func (h *Host) externalPlanReady(ctx context.Context, name string, withDeps bool) (map[string]bool, error) {
	rows, err := loadRows(h.db.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	states := h.effectiveStates(rows)
	if states[name] == StateActive {
		// The transaction refuses it.
		return map[string]bool{}, nil
	}
	steps, err := Plan(name, h.manifests, states, withDeps)
	if err != nil {
		return nil, err
	}
	checked := make(map[string]bool, len(steps))
	for _, s := range steps {
		if err := h.externalReady(ctx, s.Module); err != nil {
			return nil, err
		}
		checked[s.Module] = true
	}
	return checked, nil
}

// externalUnready runs externalReady for each active module not in loaded
// with a configured external store, and returns those that failed it.
func (h *Host) externalUnready(ctx context.Context, states map[string]string, loaded []string) map[string]error {
	out := map[string]error{}
	for name, st := range states {
		if st != StateActive || slices.Contains(loaded, name) {
			continue
		}
		if s, _, err := h.externalSchema(name, false); err == nil && s == nil {
			continue
		}
		if err := h.externalReady(ctx, name); err != nil {
			out[name] = err
		}
	}
	return out
}

// holdEvents adds each module of left to v.held for the event types its
// manifest subscribes to.
func (h *Host) holdEvents(v *view, left map[string]error) {
	for name := range left {
		for _, e := range h.manifests[name].Subscribes {
			v.held[e] = append(v.held[e], name)
		}
	}
}

// without is states with the modules of drop left out.
func without(states map[string]string, drop map[string]error) map[string]string {
	if len(drop) == 0 {
		return states
	}
	out := make(map[string]string, len(states))
	for n, s := range states {
		if _, ok := drop[n]; !ok {
			out[n] = s
		}
	}
	return out
}

// externalReady refuses a module whose external store is configured but does
// not hold every schema file of this binary. With no store configured it
// warns that the store was not checked. It runs outside the lifecycle
// transaction.
func (h *Host) externalReady(ctx context.Context, name string) error {
	s, skipped, err := h.externalSchema(name, false)
	if err != nil {
		return err
	}
	if s == nil {
		if _, ok := h.mods[name].(modkit.ExternalMigrator); ok {
			h.log.Warn("modhost:external store not checked", "module", name, "reason", skipped)
		}
		return nil
	}
	p, readErr, err := externalPending(ctx, name, s)
	if readErr != nil {
		return &ResolveError{
			Msg:  fmt.Sprintf("cannot read %s's %s schema: %v", name, s.Store(), readErr),
			Hint: "check that " + s.Store() + " is reachable with the module's configured login",
		}
	}
	if err != nil {
		return err
	}
	if len(p) > 0 {
		return &ResolveError{
			Msg:  fmt.Sprintf("%s has %d %s schema file(s) not applied", name, len(p), s.Store()),
			Hint: "run `amp migrate` with this binary and the module's " + s.Store() + " migrator login first; the schema is created at migrate time, not at install",
		}
	}
	return nil
}

// externalPending returns this binary's schema files that s lacks; readErr is
// set when s can't be read.
func externalPending(ctx context.Context, name string, s modkit.ExternalSchema) ([]migration, error, error) {
	ms, err := loadExternal(name, s)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, externalCheckTimeout)
	defer cancel()
	applied, err := s.Applied(ctx)
	if err != nil {
		return nil, err, nil
	}
	p, err := pendingFrom(fmt.Sprintf("module %s %s schema", name, s.Store()), applied, ms)
	return p, nil, err
}

// ErrExternalBehind is CheckExternalSchema's error when an active module's
// store lacks schema files of this binary.
var ErrExternalBehind = errors.New("external schemas are behind this binary")

// CheckExternalSchema reports the active modules whose configured external
// store lacks schema files of this binary. A store it can't read is logged,
// not reported: boot leaves that module out and retries it.
func (h *Host) CheckExternalSchema(ctx context.Context) error {
	rows, err := loadRows(h.db.WithContext(ctx))
	if err != nil {
		return err
	}
	var behind []string
	for _, name := range h.names() {
		if rows[name].State != StateActive {
			continue
		}
		s, _, err := h.externalSchema(name, false)
		if err != nil {
			return err
		}
		if s == nil {
			continue
		}
		p, readErr, err := externalPending(ctx, name, s)
		switch {
		case readErr != nil:
			h.log.Warn("modhost:cannot read the external schema; not waiting for it", "module", name, "store", s.Store(), "error", readErr)
		case err != nil:
			return err
		case len(p) > 0:
			behind = append(behind, fmt.Sprintf("%s: %d %s file(s)", name, len(p), s.Store()))
		}
	}
	if len(behind) > 0 {
		return fmt.Errorf("%w (%s); run `amp migrate` with each store's migrator login", ErrExternalBehind, strings.Join(behind, ", "))
	}
	return nil
}

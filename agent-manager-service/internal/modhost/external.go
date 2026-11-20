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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
)

// externalFile is the name every external schema file has: a three-digit
// version, a description and the store's own extension.
var externalFile = regexp.MustCompile(`^[0-9]{3}_[a-z0-9_]+\.[a-z0-9]+$`)

// externalCheckTimeout bounds the install and enable check of an external store.
const externalCheckTimeout = 10 * time.Second

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

// loadExternal reads an external schema's files in lexical order.
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
	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialise with other migrators, as the SQL migrations do.
		if err := dbx.LockRow(tx, "module_generation", "value", "id = ?", 1); err != nil {
			return err
		}
		applied, err := s.Applied(ctx)
		if err != nil {
			return fmt.Errorf("%s: reading what was applied: %w", what, err)
		}
		todo, err := pendingFrom(what, applied, ms)
		if err != nil {
			return err
		}
		for _, m := range todo {
			h.log.Info("modhost:applying external schema", "module", name, "store", s.Store(), "version", m.Version)
			if err := s.Apply(ctx, m.Version, m.Checksum, m.body); err != nil {
				return fmt.Errorf("%s %s: %w", what, m.Version, err)
			}
			res.Applied++
		}
		if err := s.Settle(ctx); err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		return nil
	})
	return res, err
}

// externalReady refuses a module whose external store is configured but does
// not hold every schema file of this binary. With no store configured there
// is nothing to check.
func (h *Host) externalReady(ctx context.Context, name string) error {
	s, _, err := h.externalSchema(name, false)
	if err != nil || s == nil {
		return err
	}
	ms, err := loadExternal(name, s)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, externalCheckTimeout)
	defer cancel()
	applied, err := s.Applied(ctx)
	if err != nil {
		return &ResolveError{
			Msg:  fmt.Sprintf("cannot read %s's %s schema: %v", name, s.Store(), err),
			Hint: "check that " + s.Store() + " is reachable with the module's configured login",
		}
	}
	p, err := pendingFrom(fmt.Sprintf("module %s %s schema", name, s.Store()), applied, ms)
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

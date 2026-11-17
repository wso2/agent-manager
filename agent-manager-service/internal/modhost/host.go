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

// Package modhost is the module host: it keeps track of which compiled-in
// modules are installed, runs their lifecycle (install, enable, disable,
// uninstall) and their migrations, and holds an immutable snapshot of what the
// active modules registered. Lifecycle changes bump a generation counter in
// the database; every replica polls it and swaps in a new snapshot, so a
// change needs no restart.
package modhost

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// DefaultPollInterval is how often a replica checks the module generation for
// lifecycle changes made elsewhere.
const DefaultPollInterval = 2 * time.Second

// Host is the module host. One per process.
type Host struct {
	db     *gorm.DB
	driver string
	log    *slog.Logger

	mods      map[string]modkit.Module
	manifests map[string]modkit.Manifest

	// PollInterval is how often this replica checks for changes made
	// elsewhere. Set it before Boot; zero means DefaultPollInterval.
	PollInterval time.Duration

	reloadMu sync.Mutex // one Reload at a time (poller and local changes)

	mu     sync.RWMutex
	gen    int64
	states map[string]string
	dbVer  map[string]string
	v      *view
}

// view is an immutable snapshot of what the active modules registered.
// Reload builds a new one and swaps it in, so readers never see a half-built set.
type view struct {
	active []string
	caps   map[string]any
	capBy  map[string]string
}

func newView() *view {
	return &view{caps: map[string]any{}, capBy: map[string]string{}}
}

// New creates the host for the modules compiled into this binary. driver is
// the database driver name ("sqlite" or "postgres").
func New(db *gorm.DB, driver string, log *slog.Logger) *Host {
	return NewWith(db, driver, log, modkit.Registered())
}

// NewWith creates a host for an explicit set of modules; tests use it to stand
// in for a different binary.
func NewWith(db *gorm.DB, driver string, log *slog.Logger, mods []modkit.Module) *Host {
	h := &Host{
		db: db, driver: driver, log: log,
		mods: map[string]modkit.Module{}, manifests: map[string]modkit.Manifest{},
		states: map[string]string{}, dbVer: map[string]string{}, v: newView(),
	}
	for _, m := range mods {
		mf := m.Manifest()
		h.mods[mf.Name] = m
		h.manifests[mf.Name] = mf
	}
	return h
}

func (h *Host) names() []string {
	out := make([]string, 0, len(h.mods))
	for n := range h.mods {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (h *Host) current() *view {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.v
}

// Generation is the lifecycle generation this replica has loaded.
func (h *Host) Generation() int64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.gen
}

// Active lists the active modules in activation order.
func (h *Host) Active() []string {
	return append([]string(nil), h.current().active...)
}

// Boot checks that the database is migrated for this binary, loads the active
// set, and follows lifecycle changes made by other processes until ctx ends.
// It never changes the schema: that is Migrate's job.
func (h *Host) Boot(ctx context.Context) error {
	if err := h.CheckSchema(ctx); err != nil {
		return err
	}
	if err := h.Reload(ctx); err != nil {
		return err
	}
	go h.follow(ctx)
	return nil
}

type moduleRow struct {
	Name    string
	Version string
	State   string
}

func loadRows(db *gorm.DB) (map[string]moduleRow, error) {
	var rows []moduleRow
	if err := db.Raw(`SELECT name, version, state FROM modules`).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("reading modules: %w", err)
	}
	out := make(map[string]moduleRow, len(rows))
	for _, r := range rows {
		out[r.Name] = r
	}
	return out, nil
}

// effectiveStates is the recorded state of every module, with built-in
// modules always active.
func (h *Host) effectiveStates(rows map[string]moduleRow) map[string]string {
	out := make(map[string]string, len(rows))
	for n, r := range rows {
		out[n] = r.State
	}
	for n, mf := range h.manifests {
		if mf.Builtin {
			out[n] = StateActive
		}
	}
	return out
}

func readGeneration(db *gorm.DB) (int64, error) {
	var g int64
	if err := db.Raw(`SELECT value FROM module_generation WHERE id = 1`).Scan(&g).Error; err != nil {
		return 0, fmt.Errorf("reading module generation: %w", err)
	}
	return g, nil
}

// bump moves the generation on inside tx. Other replicas see the new value
// on their next poll, once tx commits. Taken first in a lifecycle change, the
// row write also serialises concurrent changes on either database.
func bump(tx *gorm.DB) error {
	return tx.Exec(`UPDATE module_generation SET value = value + 1 WHERE id = 1`).Error
}

// follow polls the generation and reloads when another process has changed
// the module set. A single-row read per interval works on every database,
// and a replica that missed changes catches up on its next tick.
func (h *Host) follow(ctx context.Context) {
	every := h.PollInterval
	if every <= 0 {
		every = DefaultPollInterval
	}
	t := time.NewTicker(every)
	defer t.Stop()
	failing := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		g, err := readGeneration(h.db.WithContext(ctx))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if !failing {
				h.log.Warn("modhost:cannot read the module generation; keeping the current module set", "error", err)
				failing = true
			}
			continue
		}
		if failing {
			h.log.Info("modhost:module generation readable again")
			failing = false
		}
		if g <= h.Generation() {
			continue
		}
		h.log.Info("modhost:module set changed elsewhere", "generation", g)
		if err := h.Reload(ctx); err != nil {
			h.log.Error("modhost:reload failed", "error", err)
		}
	}
}

// Reload activates the active modules into a new snapshot and swaps it in.
func (h *Host) Reload(ctx context.Context) error {
	h.reloadMu.Lock()
	defer h.reloadMu.Unlock()
	db := h.db.WithContext(ctx)
	gen, err := readGeneration(db)
	if err != nil {
		return err
	}
	rows, err := loadRows(db)
	if err != nil {
		return err
	}
	states := h.effectiveStates(rows)
	var active []string
	for name, s := range states {
		if s != StateActive {
			continue
		}
		if _, ok := h.mods[name]; !ok {
			h.log.Warn("modhost:module is active in the database but not compiled into this binary; skipping", "module", name)
			continue
		}
		active = append(active, name)
	}
	v := newView()
	for _, name := range ActivationOrder(active, h.manifests) {
		if err := h.activate(ctx, name, v); err != nil {
			h.log.Error("modhost:module failed to activate", "module", name, "error", err)
			continue
		}
		v.active = append(v.active, name)
	}
	h.mu.Lock()
	h.v, h.gen, h.states = v, gen, states
	h.dbVer = make(map[string]string, len(rows))
	for n, r := range rows {
		h.dbVer[n] = r.Version
	}
	h.mu.Unlock()
	h.log.Info("modhost:modules loaded", "generation", gen, "active", v.active)
	return nil
}

// activate runs one module's Activate against the snapshot being built. A
// module whose required capabilities are not all provided, or that uses a
// Runtime feature this core does not implement yet, is left inactive.
func (h *Host) activate(ctx context.Context, name string, v *view) error {
	mf := h.manifests[name]
	if mf.CoreAPI != modkit.CoreAPIVersion {
		return fmt.Errorf("targets core API v%d, this core implements v%d", mf.CoreAPI, modkit.CoreAPIVersion)
	}
	for _, c := range mf.Requires {
		if _, ok := v.caps[c]; !ok {
			return fmt.Errorf("required capability %s is not provided by an active module", c)
		}
	}
	rt := h.runtime(name, v)
	rt.building.Store(true)
	err := h.mods[name].Activate(ctx, rt)
	rt.building.Store(false)
	if err != nil {
		return err
	}
	if err := rt.err(); err != nil {
		// Drop what the module registered before it failed.
		for c, by := range v.capBy {
			if by == name {
				delete(v.caps, c)
				delete(v.capBy, c)
			}
		}
		return err
	}
	return nil
}

// ---- lifecycle ----

// change runs fn in one transaction that first bumps the generation, then
// reloads this replica. Other replicas reload on their next poll. Lifecycle
// changes are data changes only, never DDL.
func (h *Host) change(ctx context.Context, fn func(tx *gorm.DB, states map[string]string) error) error {
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := bump(tx); err != nil {
			return err
		}
		rows, err := loadRows(tx)
		if err != nil {
			return err
		}
		return fn(tx, h.effectiveStates(rows))
	})
	if err != nil {
		return err
	}
	return h.Reload(ctx)
}

func (h *Host) refuseBuiltin(name string) error {
	if h.manifests[name].Builtin {
		return &ResolveError{Msg: name + " is built in: it is always active"}
	}
	return nil
}

func setState(tx *gorm.DB, name, version, state string) error {
	now := time.Now().UTC()
	return tx.Exec(`INSERT INTO modules (name, version, state, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (name) DO UPDATE SET version = excluded.version, state = excluded.state, updated_at = excluded.updated_at`,
		name, version, state, now, now).Error
}

// Install installs a module (and, with withDeps, the providers of its missing
// required capabilities). It returns the steps it carried out.
func (h *Host) Install(ctx context.Context, name string, withDeps bool) ([]Step, error) {
	var steps []Step
	err := h.change(ctx, func(tx *gorm.DB, states map[string]string) error {
		if err := h.refuseBuiltin(name); err != nil {
			return err
		}
		if states[name] == StateActive {
			return &ResolveError{Msg: name + " is already installed and active"}
		}
		var err error
		if steps, err = Plan(name, h.manifests, states, withDeps); err != nil {
			return err
		}
		for _, s := range steps {
			if err := h.schemaReady(tx, s.Module); err != nil {
				return err
			}
			if err := setState(tx, s.Module, h.manifests[s.Module].Version, StateActive); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Catch-up: a module installed late rebuilds its view of existing core state.
	for _, s := range steps {
		inst, ok := h.mods[s.Module].(modkit.Installer)
		if !ok || s.Action != "install" {
			continue
		}
		if err := inst.OnInstall(ctx, h.runtime(s.Module, h.current())); err != nil {
			h.log.Error("modhost:module catch-up failed", "module", s.Module, "error", err)
		}
	}
	return steps, nil
}

// Enable re-activates a disabled module.
func (h *Host) Enable(ctx context.Context, name string) error {
	return h.change(ctx, func(tx *gorm.DB, states map[string]string) error {
		if err := h.refuseBuiltin(name); err != nil {
			return err
		}
		if states[name] != StateDisabled {
			return &ResolveError{Msg: fmt.Sprintf("%s is %s, not disabled", name, stateOf(name, states))}
		}
		if _, err := Plan(name, h.manifests, states, false); err != nil {
			return err
		}
		if err := h.schemaReady(tx, name); err != nil {
			return err
		}
		return setState(tx, name, h.manifests[name].Version, StateActive)
	})
}

// Disable deactivates a module and keeps its data.
func (h *Host) Disable(ctx context.Context, name string) error {
	return h.change(ctx, func(tx *gorm.DB, states map[string]string) error {
		if err := h.refuseBuiltin(name); err != nil {
			return err
		}
		if states[name] != StateActive {
			return &ResolveError{Msg: fmt.Sprintf("%s is %s, not active", name, stateOf(name, states))}
		}
		if deps := Dependents(name, h.manifests, states, false); len(deps) > 0 {
			return &ResolveError{Msg: fmt.Sprintf("%s is required by %v", name, deps), Hint: "disable those first"}
		}
		return tx.Exec(`UPDATE modules SET state = ?, updated_at = ? WHERE name = ?`, StateDisabled, time.Now().UTC(), name).Error
	})
}

// Uninstall marks a module uninstalled. Its tables and data stay, because
// they belong to the binary's schema, so installing it again picks up where it
// left off.
func (h *Host) Uninstall(ctx context.Context, name string) error {
	return h.change(ctx, func(tx *gorm.DB, states map[string]string) error {
		if err := h.refuseBuiltin(name); err != nil {
			return err
		}
		if st := stateOf(name, states); st == StateAvailable || st == StateUninstalled {
			return &ResolveError{Msg: name + " is not installed"}
		}
		if deps := Dependents(name, h.manifests, states, true); len(deps) > 0 {
			return &ResolveError{Msg: fmt.Sprintf("%s is required by %v", name, deps), Hint: "uninstall those first"}
		}
		return tx.Exec(`UPDATE modules SET state = ?, updated_at = ? WHERE name = ?`, StateUninstalled, time.Now().UTC(), name).Error
	})
}

// ---- listing ----

// ModuleInfo describes one compiled-in module.
type ModuleInfo struct {
	modkit.Manifest
	State            string   `json:"state"`
	InstalledVersion string   `json:"installedVersion,omitempty"`
	RequiredBy       []string `json:"requiredBy,omitempty"`
	// Loaded is true when the module is active and activated in this process.
	Loaded bool `json:"loaded"`
}

// List describes every module compiled into the binary, as of the last Reload.
func (h *Host) List() []ModuleInfo {
	h.mu.RLock()
	states, dbVer, v := h.states, h.dbVer, h.v
	h.mu.RUnlock()
	loaded := map[string]bool{}
	for _, n := range v.active {
		loaded[n] = true
	}
	out := make([]ModuleInfo, 0, len(h.mods))
	for _, name := range h.names() {
		out = append(out, ModuleInfo{
			Manifest: h.manifests[name], State: stateOf(name, states), InstalledVersion: dbVer[name],
			RequiredBy: Dependents(name, h.manifests, states, true), Loaded: loaded[name],
		})
	}
	return out
}

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
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db_migrations/schema"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// DefaultPollInterval is how often a replica checks the module generation for
// lifecycle changes made elsewhere.
const DefaultPollInterval = 2 * time.Second

// maxRetryInterval caps the backoff between attempts to activate a module
// that failed to activate.
const maxRetryInterval = time.Minute

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

	// EventRetryBase is the first retry delay of a failed module event
	// delivery; zero means DefaultEventRetryBase. Set it before Boot.
	EventRetryBase time.Duration
	// EventDeliveryTimeout bounds one module event delivery; zero means
	// DefaultEventDeliveryTimeout. Set it before Boot.
	EventDeliveryTimeout time.Duration

	// WorkerDrainTimeout is how long a stopping module worker gets to return;
	// zero means DefaultWorkerDrainTimeout. WorkerRetryBase is the first delay
	// before a failed worker restarts; zero means DefaultWorkerRetryBase. Set
	// them before Boot.
	WorkerDrainTimeout time.Duration
	WorkerRetryBase    time.Duration

	// Passive hosts track module state without activating anything, for
	// commands that change module state on behalf of running servers.
	// Install and Enable still trial-activate the module set they produce.
	Passive bool

	// NoWorkers activates modules without starting their background workers.
	// For tests: a module's workers act on every organization in the database,
	// so on a database shared by tests running in parallel they would change
	// other tests' rows mid-test. Set it before Boot.
	NoWorkers bool

	// warned holds modules already reported as active but not compiled in.
	warned sync.Map

	reloadMu sync.Mutex // one Reload at a time (poller and local changes)

	mu     sync.RWMutex
	gen    int64
	states map[string]string
	dbVer  map[string]string
	v      *view
	// failed holds the active modules that did not activate in the current
	// snapshot, with the reason.
	failed map[string]error
	// d holds the core services the server attached.
	d Deps

	// workMu guards the running module workers. runCtx is Boot's ctx, which
	// workers run under; nil before Boot.
	workMu  sync.Mutex
	runCtx  context.Context
	running map[string]*moduleWorkers
}

// view is an immutable snapshot of what the active modules registered.
// Reload builds a new one and swaps it in, so readers never see a half-built set.
type view struct {
	active []string
	caps   map[string]any
	capBy  map[string]string

	perms *rbac.Registry
	// public and internal hold each active module's routes.
	public   map[string]*http.ServeMux
	internal map[string]*http.ServeMux
	// hooks are by resource and phase, in activation order.
	hooks map[string]map[modkit.Phase][]hookEntry
	subs  map[string][]subEntry
	// workers are each active module's background workers.
	workers map[string][]workerEntry
	// at holds every HandleInternalAt route, served by internalAt.
	at         []Route
	internalAt *http.ServeMux
	// tools are the active modules' MCP tools, in activation order.
	tools []toolEntry
	// routes are each active module's public routes, as registered.
	routes map[string][]Route
}

func newView() *view {
	return &view{
		caps: map[string]any{}, capBy: map[string]string{}, perms: rbac.NewRegistry(),
		public: map[string]*http.ServeMux{}, internal: map[string]*http.ServeMux{},
		hooks: map[string]map[modkit.Phase][]hookEntry{}, subs: map[string][]subEntry{},
		workers: map[string][]workerEntry{}, routes: map[string][]Route{},
	}
}

// Capability returns what an active module provides under a capability id.
// The core resolves capabilities through it (see coreext.Lookup).
func (h *Host) Capability(name string) (any, bool) {
	impl, ok := h.current().caps[name]
	return impl, ok
}

// Permissions is the permission registry of the current module set.
func (h *Host) Permissions() *rbac.Registry {
	return h.current().perms
}

// New creates the host for the modules compiled into this binary. driver is
// the database driver name ("sqlite" or "postgres").
func New(db *gorm.DB, driver string, log *slog.Logger) *Host {
	return NewWith(db, driver, log, modkit.Registered())
}

// NewWith creates a host for an explicit set of modules; tests use it to stand
// in for a different binary. Like modkit.Register, it panics on module names
// that are invalid or would share tables.
func NewWith(db *gorm.DB, driver string, log *slog.Logger, mods []modkit.Module) *Host {
	h := &Host{
		db: db, driver: driver, log: log,
		mods: map[string]modkit.Module{}, manifests: map[string]modkit.Manifest{},
		states: map[string]string{}, dbVer: map[string]string{}, v: newView(),
		running: map[string]*moduleWorkers{},
	}
	for _, m := range mods {
		mf := m.Manifest()
		if err := modkit.CheckName(mf.Name, h.names()); err != nil {
			panic(err.Error())
		}
		if err := checkReserved(mf); err != nil {
			panic(err.Error())
		}
		h.mods[mf.Name] = m
		h.manifests[mf.Name] = mf
	}
	return h
}

// checkReserved refuses a module named after a core permission resource.
func checkReserved(mf modkit.Manifest) error {
	if slices.Contains(rbac.ReservedResources(), mf.Name) {
		return fmt.Errorf("modhost: module name %q is reserved for the core's %s:* permissions", mf.Name, mf.Name)
	}
	for _, c := range mf.Provides {
		if isCoreCapability(c) {
			return fmt.Errorf("modhost: module %q provides %s, which the core provides", mf.Name, c)
		}
	}
	return nil
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
	if !h.Passive {
		h.workMu.Lock()
		h.runCtx = ctx
		h.workMu.Unlock()
	}
	if err := h.Reload(ctx); err != nil {
		return err
	}
	go h.follow(ctx)
	if !h.Passive {
		go h.drain(ctx)
	}
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

// effectiveStates is the recorded state of every module.
func (h *Host) effectiveStates(rows map[string]moduleRow) map[string]string {
	out := make(map[string]string, len(rows))
	for n, r := range rows {
		out[n] = r.State
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
// and a replica that missed changes catches up on its next tick. Any change
// reloads, not only an increase, so a restored or recreated database is
// picked up too. While an active module has failed to activate, follow also
// retries it with backoff.
func (h *Host) follow(ctx context.Context) {
	every := h.PollInterval
	if every <= 0 {
		every = DefaultPollInterval
	}
	t := time.NewTicker(every)
	defer t.Stop()
	failing := false
	backoff := every
	var retryAt time.Time
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
		switch {
		case g != h.Generation():
			h.log.Info("modhost:module set changed elsewhere", "generation", g)
		case h.unloaded() && !time.Now().Before(retryAt):
			h.log.Info("modhost:retrying modules that failed to activate")
		default:
			continue
		}
		if err := h.Reload(ctx); err != nil {
			h.log.Error("modhost:reload failed", "error", err)
		}
		if h.unloaded() {
			retryAt = time.Now().Add(backoff)
			backoff = min(2*backoff, maxRetryInterval)
		} else {
			backoff = every
		}
	}
}

// unloaded reports whether an active module failed to activate.
func (h *Host) unloaded() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.failed) > 0
}

// Reload activates the active modules into a new snapshot and swaps it in. A
// passive host only reloads the recorded states.
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
	v, failed := newView(), map[string]error{}
	if !h.Passive {
		// A module not yet loaded whose external store is unusable is left
		// out and retried; a loaded one stays.
		unready := h.externalUnready(ctx, states, h.current().active)
		v, failed = h.buildView(ctx, without(states, unready))
		for name, err := range unready {
			failed[name] = err
		}
		for name, err := range failed {
			h.log.Error("modhost:module failed to activate", "module", name, "error", err)
		}
	}
	h.mu.Lock()
	h.v, h.gen, h.states, h.failed = v, gen, states, failed
	if !h.Passive {
		rbac.SetCurrent(v.perms)
	}
	h.dbVer = make(map[string]string, len(rows))
	for n, r := range rows {
		h.dbVer[n] = r.Version
	}
	h.mu.Unlock()
	if !h.Passive {
		h.log.Info("modhost:modules loaded", "generation", gen, "active", v.active)
		h.publishCatalogs(v)
		h.syncWorkers(v)
	}
	return nil
}

// buildView activates the modules active in states into a new snapshot, and
// returns the active modules that did not activate, with the reason.
func (h *Host) buildView(ctx context.Context, states map[string]string) (*view, map[string]error) {
	var active []string
	for name, s := range states {
		if s != StateActive {
			continue
		}
		if _, ok := h.mods[name]; !ok {
			if _, seen := h.warned.LoadOrStore(name, true); !seen {
				h.log.Warn("modhost:module is active in the database but not compiled into this binary; skipping", "module", name)
			}
			continue
		}
		active = append(active, name)
	}
	v, failed := h.seedCore(newView()), map[string]error{}
	order, cycles := ActivationOrder(active, h.manifests)
	for _, c := range cycles {
		for _, name := range c[:len(c)-1] {
			failed[name] = fmt.Errorf("dependency cycle: %s", strings.Join(c, " -> "))
		}
	}
	for _, name := range order {
		if err := h.activate(ctx, name, v); err != nil {
			failed[name] = err
			continue
		}
		v.active = append(v.active, name)
	}
	return v, failed
}

// activate runs one module's Activate against the snapshot being built. A
// module whose required capabilities are not all provided, or that uses a
// Runtime feature this core does not implement yet, is left inactive.
func (h *Host) activate(ctx context.Context, name string, v *view) error {
	mf := h.manifests[name]
	if mf.CoreAPI != modkit.CoreAPIVersion {
		return fmt.Errorf("targets core API v%d, this core implements v%d", mf.CoreAPI, modkit.CoreAPIVersion)
	}
	if err := checkWorkloads(mf); err != nil {
		return err
	}
	for _, c := range mf.Requires {
		if _, ok := v.caps[c]; !ok && !isCoreCapability(c) {
			return fmt.Errorf("required capability %s is not provided by an active module", c)
		}
	}
	rt := h.runtime(name, v)
	rt.building.Store(true)
	err := h.mods[name].Activate(ctx, rt)
	rt.building.Store(false)
	if err == nil {
		err = rt.err()
	}
	if err == nil {
		err = h.commit(rt, v)
	}
	if err != nil {
		// Drop what the module registered before it failed, so no consumer
		// activates against it.
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

// commit adds what a module registered during a successful Activate to the
// snapshot: its permissions, routes, hooks and subscriptions. Nothing is
// added unless all of it is accepted.
func (h *Host) commit(rt *runtime, v *view) error {
	name := rt.name
	if err := checkManifest(rt.mf); err != nil {
		return err
	}
	if err := declare(v.perms.Clone(), rt.mf); err != nil {
		return err
	}
	if err := checkEndpoints(rt.mf, rt.internal); err != nil {
		return err
	}
	d := h.deps()
	var pub, internal *http.ServeMux
	if d.Mount != nil && len(rt.routes) > 0 {
		var err error
		if pub, err = mountPublic(d.Mount, rt.routes); err != nil {
			return err
		}
	}
	if len(rt.internal) > 0 || len(rt.webhooks) > 0 {
		var err error
		if internal, err = h.mountInternal(name, rt.internal, rt.webhooks, d.MountInternal); err != nil {
			return err
		}
	}
	var at *http.ServeMux
	if len(rt.at) > 0 {
		var err error
		if at, err = mountInternalAt(d.MountInternal, append(slices.Clone(v.at), rt.at...)); err != nil {
			return err
		}
	}
	for _, t := range rt.tools {
		for _, other := range v.tools {
			if other.tool.Name == t.tool.Name {
				return fmt.Errorf("MCP tool %s is already added by %s", t.tool.Name, other.module)
			}
		}
	}
	if err := declare(v.perms, rt.mf); err != nil {
		return err
	}
	if at != nil {
		v.at, v.internalAt = append(v.at, rt.at...), at
	}
	v.tools = append(v.tools, rt.tools...)
	if len(rt.routes) > 0 {
		v.routes[name] = rt.routes
	}
	if pub != nil {
		v.public[name] = pub
	}
	if internal != nil {
		v.internal[name] = internal
	}
	for _, e := range rt.hooks {
		if v.hooks[e.resource] == nil {
			v.hooks[e.resource] = map[modkit.Phase][]hookEntry{}
		}
		v.hooks[e.resource][e.phase] = append(v.hooks[e.resource][e.phase], e)
	}
	for _, s := range rt.subs {
		v.subs[s.event] = append(v.subs[s.event], s)
	}
	if len(rt.workers) > 0 {
		v.workers[name] = rt.workers
	}
	return nil
}

// trial activates the module set next would produce and reports the first
// of targets that would not activate, so a lifecycle change that leaves a
// module recorded active but not running is refused instead of committed. It
// leaves out the modules of left, as Reload does; a target left out fails.
func (h *Host) trial(ctx context.Context, next map[string]string, targets []string, left map[string]error) (*view, error) {
	v, failed := h.buildView(ctx, without(next, left))
	for _, name := range targets {
		err := failed[name]
		if why, out := left[name]; out {
			err = why
		}
		if err != nil {
			re := &ResolveError{Msg: fmt.Sprintf("%s failed to activate: %v", name, err)}
			if len(left) > 0 {
				names := slices.Sorted(maps.Keys(left))
				re.Hint = strings.Join(names, ", ") + " left out until their store is usable"
			}
			return nil, re
		}
	}
	// Nor may it stop a module that runs now, as a newcomer claiming an MCP
	// tool name or an internal path it holds would.
	for _, name := range h.current().active {
		if err := failed[name]; err != nil {
			return nil, &ResolveError{Msg: fmt.Sprintf("%s would stop %s: %v", strings.Join(targets, ", "), name, err)}
		}
	}
	return v, nil
}

func withActive(states map[string]string, names ...string) map[string]string {
	out := make(map[string]string, len(states)+len(names))
	for n, s := range states {
		out[n] = s
	}
	for _, n := range names {
		out[n] = StateActive
	}
	return out
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

// setState records a module's version and state, and reports whether that
// changed anything. Callers hold the generation row lock, so the read and the
// write cannot race another lifecycle change.
func setState(tx *gorm.DB, name, version, state string) (bool, error) {
	now := dbx.Now()
	var n int64
	if err := tx.Raw(`SELECT count(*) FROM modules WHERE name = ?`, name).Scan(&n).Error; err != nil {
		return false, err
	}
	if n == 0 {
		err := tx.Exec(`INSERT INTO modules (name, version, state, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			name, version, state, now, now).Error
		return err == nil, err
	}
	r := tx.Exec(`UPDATE modules SET version = ?, state = ?, updated_at = ? WHERE name = ? AND (version <> ? OR state <> ?)`,
		version, state, now, name, version, state)
	return r.RowsAffected > 0, r.Error
}

// Install installs a module (and, with withDeps, the providers of its missing
// required capabilities). It returns the steps it carried out. Nothing is
// committed unless every installed module activates. A failed catch-up
// (OnInstall) is returned with the steps: the modules stay installed.
func (h *Host) Install(ctx context.Context, name string, withDeps bool) ([]Step, error) {
	return h.install(ctx, name, withDeps, true)
}

// install is Install. With checkStores false it does not check the external
// stores of the modules it installs, as the installs at boot do: a module
// whose store is not ready is then left out and retried, not refused.
func (h *Host) install(ctx context.Context, name string, withDeps, checkStores bool) ([]Step, error) {
	checked, left, err := h.externalPlanReady(ctx, name, withDeps, checkStores)
	if err != nil {
		return nil, err
	}
	var steps []Step
	var trial *view
	err = h.change(ctx, func(tx *gorm.DB, states map[string]string) error {
		if states[name] == StateActive {
			return &ResolveError{Msg: name + " is already installed and active"}
		}
		var err error
		if steps, err = Plan(name, h.manifests, states, withDeps); err != nil {
			return err
		}
		for _, s := range steps {
			if !checked[s.Module] {
				return &ResolveError{Msg: "the modules to install changed while " + name + " was being checked", Hint: "try again"}
			}
			if err := h.schemaReady(tx, s.Module); err != nil {
				return err
			}
			if _, err := setState(tx, s.Module, h.manifests[s.Module].Version, StateActive); err != nil {
				return err
			}
		}
		targets := make([]string, 0, len(steps))
		for _, s := range steps {
			targets = append(targets, s.Module)
		}
		trial, err = h.trial(ctx, withActive(states, targets...), targets, left)
		return err
	})
	if err != nil {
		return nil, err
	}
	// Catch-up: a module installed late rebuilds its view of existing core state.
	var errs []error
	for _, s := range steps {
		inst, ok := h.mods[s.Module].(modkit.Installer)
		if !ok || s.Action != "install" {
			continue
		}
		rt := h.runtime(s.Module, trial)
		rt.pinned = true
		if err := inst.OnInstall(ctx, rt); err != nil {
			errs = append(errs, fmt.Errorf("%s is installed, but its catch-up failed: %w", s.Module, err))
		}
	}
	return steps, errors.Join(errs...)
}

// Enable re-activates a disabled module.
func (h *Host) Enable(ctx context.Context, name string) error {
	if err := h.externalReady(ctx, name); err != nil {
		return err
	}
	left, err := h.leftOut(ctx)
	if err != nil {
		return err
	}
	return h.change(ctx, func(tx *gorm.DB, states map[string]string) error {
		if states[name] != StateDisabled {
			return &ResolveError{Msg: fmt.Sprintf("%s is %s, not disabled", name, stateOf(name, states))}
		}
		if _, err := Plan(name, h.manifests, states, false); err != nil {
			return err
		}
		if err := h.schemaReady(tx, name); err != nil {
			return err
		}
		if err := dropDeliveries(tx, name, dbx.Now().Add(-EventRetention)); err != nil {
			return err
		}
		if _, err := setState(tx, name, h.manifests[name].Version, StateActive); err != nil {
			return err
		}
		_, err := h.trial(ctx, withActive(states, name), []string{name}, left)
		return err
	})
}

// Disable deactivates a module and keeps its data.
func (h *Host) Disable(ctx context.Context, name string) error {
	return h.change(ctx, func(tx *gorm.DB, states map[string]string) error {
		if states[name] != StateActive {
			return &ResolveError{Msg: fmt.Sprintf("%s is %s, not active", name, stateOf(name, states))}
		}
		if deps := Dependents(name, h.manifests, states, false); len(deps) > 0 {
			return &ResolveError{Msg: fmt.Sprintf("%s is required by %v", name, deps), Hint: "disable those first"}
		}
		// Deliveries already queued for the module stay while it is
		// disabled, and expire after EventRetention, so enabling it later
		// replays no more than that.
		if err := dropDeliveries(tx, name, dbx.Now().Add(-EventRetention)); err != nil {
			return err
		}
		return tx.Exec(`UPDATE modules SET state = ?, updated_at = ? WHERE name = ?`, StateDisabled, dbx.Now(), name).Error
	})
}

// Uninstall marks a module uninstalled. Without purge its tables and data
// stay, because they belong to the binary's schema, so installing it again
// picks up where it left off. With purge its tables are emptied, its secrets
// deleted and its workload credentials revoked, and it is forgotten: the
// tables themselves stay, so installing it again needs no migration and its
// OnInstall restores any reference data. An uninstalled module can still be
// purged.
func (h *Host) Uninstall(ctx context.Context, name string, purge bool) error {
	var revoked []string
	err := h.change(ctx, func(tx *gorm.DB, states map[string]string) error {
		if st := stateOf(name, states); st == StateAvailable || (st == StateUninstalled && !purge) {
			return &ResolveError{Msg: name + " is not installed"}
		}
		if deps := Dependents(name, h.manifests, states, true); len(deps) > 0 {
			return &ResolveError{Msg: fmt.Sprintf("%s is required by %v", name, deps), Hint: "uninstall those first"}
		}
		// An uninstalled module gets no more events.
		if err := dropDeliveries(tx, name, time.Time{}); err != nil {
			return err
		}
		if !purge {
			return tx.Exec(`UPDATE modules SET state = ?, updated_at = ? WHERE name = ?`, StateUninstalled, dbx.Now(), name).Error
		}
		var err error
		if revoked, err = h.purge(tx, name); err != nil {
			return err
		}
		return tx.Exec(`DELETE FROM modules WHERE name = ?`, name).Error
	})
	recordRevoked(ctx, name, revoked, err)
	return err
}

// purge empties a module's tables, deletes its secrets and revokes its
// workload credentials inside tx, and returns the revoked credential ids.
func (h *Host) purge(tx *gorm.DB, name string) ([]string, error) {
	tables, err := moduleTables(tx, h.driver, name)
	if err != nil {
		return nil, err
	}
	if len(tables) > 0 {
		if h.driver == "postgres" {
			// One statement, so foreign keys between the module's tables hold.
			if err := tx.Exec(`TRUNCATE TABLE ` + strings.Join(tables, ", ")).Error; err != nil {
				return nil, fmt.Errorf("emptying %s's tables: %w", name, err)
			}
		} else {
			if err := tx.Exec(`PRAGMA defer_foreign_keys = ON`).Error; err != nil {
				return nil, err
			}
			for _, t := range tables {
				if err := tx.Exec(`DELETE FROM ` + t).Error; err != nil {
					return nil, fmt.Errorf("emptying %s: %w", t, err)
				}
			}
		}
	}
	// Releasing needs no vault: external values are destroyed by the secret job runner after tx commits, so the
	// CLI and the server purge the same way.
	if err := repositories.ReleaseSecretsOfKind(tx, "mod:"+name); err != nil {
		return nil, fmt.Errorf("deleting %s's secrets: %w", name, err)
	}
	return revokeAllTx(tx, name)
}

// moduleTables lists a module's tables (mod_<name>_*), quoted for SQL.
func moduleTables(tx *gorm.DB, driver, module string) ([]string, error) {
	pattern := strings.ReplaceAll(schema.ModuleTablePrefix(module), "_", `\_`) + "%"
	q := `SELECT name FROM sqlite_master WHERE type = 'table' AND name LIKE ? ESCAPE '\' ORDER BY name`
	if driver == "postgres" {
		q = `SELECT tablename FROM pg_tables WHERE schemaname = current_schema() AND tablename LIKE ? ESCAPE '\' ORDER BY tablename`
	}
	var names []string
	if err := tx.Raw(q, pattern).Scan(&names).Error; err != nil {
		return nil, fmt.Errorf("listing %s's tables: %w", module, err)
	}
	for i, n := range names {
		names[i] = `"` + n + `"`
	}
	return names, nil
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

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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// runtime is the modkit.Runtime a module gets for one activation.
type runtime struct {
	h    *Host
	name string
	mf   modkit.Manifest
	v    *view
	log  *slog.Logger

	// building is true while Activate runs, when v is not yet the current snapshot.
	building atomic.Bool
	// pinned runtimes resolve capabilities from v, the snapshot they were
	// made for, rather than the current one.
	pinned bool

	mu   sync.Mutex
	errs []error
	// What Activate registered, added to the snapshot only if it succeeds.
	routes   []Route
	internal []Route
	hooks    []hookEntry
	subs     []subEntry
}

var _ modkit.Runtime = (*runtime)(nil)

func (h *Host) runtime(name string, v *view) *runtime {
	return &runtime{h: h, name: name, mf: h.manifests[name], v: v, log: h.log.With("module", name)}
}

func (r *runtime) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errs = append(r.errs, err)
}

func (r *runtime) err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return errors.Join(r.errs...)
}

// registering reports whether registrations are allowed now, and records an
// error when they are not: a module registers only from Activate.
func (r *runtime) registering(what string) bool {
	if !r.building.Load() {
		r.fail(fmt.Errorf("%s outside Activate", what))
		return false
	}
	return true
}

func (r *runtime) Name() string         { return r.name }
func (r *runtime) DB() *gorm.DB         { return r.h.db }
func (r *runtime) Logger() *slog.Logger { return r.log }

func (r *runtime) Core() modkit.CoreAPI {
	if d := r.h.deps(); d.Core != nil {
		return d.Core
	}
	return unimplementedCore{}
}

var routeMethods = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}

func (r *runtime) checkRoute(kind, method, path string, h http.HandlerFunc) error {
	if !slices.Contains(routeMethods, method) {
		return fmt.Errorf("%s %s %s: method must be one of %v", kind, method, path, routeMethods)
	}
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, " \t") {
		return fmt.Errorf("%s %s %q: path must start with / and hold no spaces", kind, method, path)
	}
	if h == nil {
		return fmt.Errorf("%s %s %s: nil handler", kind, method, path)
	}
	return nil
}

func (r *runtime) Handle(method, path, perm string, h http.HandlerFunc) {
	if !r.registering("Handle") {
		return
	}
	if err := r.checkRoute("Handle", method, path, h); err != nil {
		r.fail(err)
		return
	}
	full := rbac.Permission("")
	if perm != "" {
		var err error
		if full, err = r.h.permission(r.name, perm); err != nil {
			r.fail(fmt.Errorf("Handle %s %s: %w", method, path, err))
			return
		}
	} else if method != http.MethodGet {
		// Every state-changing route names a permission, as in the core: it
		// is what the route's audit action is named after.
		r.fail(fmt.Errorf("Handle %s %s: a %s route needs a permission", method, path, method))
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes = append(r.routes, Route{Module: r.name, Method: method, Path: path, Permission: full, Handler: h})
}

func (r *runtime) HandleInternal(method, path, scope string, h http.HandlerFunc) {
	if !r.registering("HandleInternal") {
		return
	}
	if err := r.checkRoute("HandleInternal", method, path, h); err != nil {
		r.fail(err)
		return
	}
	if scope == "" {
		r.fail(fmt.Errorf("HandleInternal %s %s: a workload route needs a scope", method, path))
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.internal = append(r.internal, Route{Module: r.name, Method: method, Path: path, Scope: scope, Handler: h})
}

func (r *runtime) Hook(resource string, phase modkit.Phase, h modkit.HookFunc) {
	if !r.registering("Hook") {
		return
	}
	if resource != modkit.ResourceAgent && resource != modkit.ResourceEnvironment {
		r.fail(fmt.Errorf("Hook: resource %q takes no hooks (want %s or %s)", resource, modkit.ResourceAgent, modkit.ResourceEnvironment))
		return
	}
	if phase != modkit.Validate && phase != modkit.Apply && phase != modkit.Finalize {
		r.fail(fmt.Errorf("Hook: unknown phase %q", phase))
		return
	}
	if h == nil {
		r.fail(fmt.Errorf("Hook %s/%s: nil hook", resource, phase))
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks = append(r.hooks, hookEntry{module: r.name, resource: resource, phase: phase, fn: h})
}

func (r *runtime) Subscribe(eventType string, h modkit.EventHandler) {
	if !r.registering("Subscribe") {
		return
	}
	if !r.h.knownEvent(eventType) {
		r.fail(fmt.Errorf("Subscribe: no core event or module manifest declares %q", eventType))
		return
	}
	if h == nil {
		r.fail(fmt.Errorf("Subscribe %s: nil handler", eventType))
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subs = append(r.subs, subEntry{module: r.name, event: eventType, fn: h})
}

func (r *runtime) Publish(ctx context.Context, eventType string, data any) error {
	return r.PublishTx(ctx, nil, eventType, data)
}

func (r *runtime) PublishTx(ctx context.Context, tx *gorm.DB, eventType string, data any) error {
	if !slices.Contains(r.mf.Events, eventType) {
		return fmt.Errorf("modkit: %s publishes %s, which its manifest does not list", r.name, eventType)
	}
	return r.h.publish(ctx, tx, r.name, eventType, data)
}

func (r *runtime) Can(ctx context.Context, perm string) bool {
	full, err := r.h.permission(r.name, perm)
	if err != nil {
		r.log.Warn("modhost:Can asked about an unknown permission", "permission", perm)
		return false
	}
	return jwtassertion.HoldsAnyScope(ctx, full)
}

func (r *runtime) IssueToken(ctx context.Context, req modkit.TokenRequest) (string, error) {
	return r.h.issueToken(ctx, r.name, req)
}

func (r *runtime) IssueCredential(ctx context.Context, req modkit.CredentialRequest) (modkit.Credential, error) {
	return r.h.issueCredential(ctx, r.name, req)
}

func (r *runtime) RevokeCredential(ctx context.Context, id string) error {
	return r.h.revokeCredential(ctx, r.name, id)
}

func (r *runtime) Provide(capability string, impl any) {
	if !slices.Contains(r.mf.Provides, capability) {
		r.fail(fmt.Errorf("provides %s, which its manifest does not list", capability))
		return
	}
	if !r.registering("Provide " + capability) {
		return
	}
	if by, ok := r.v.capBy[capability]; ok && by != r.name {
		r.fail(fmt.Errorf("provides %s, already provided by %s", capability, by))
		return
	}
	r.v.caps[capability] = impl
	r.v.capBy[capability] = r.name
}

// Capability resolves against the snapshot current at the time of the call,
// so a provider disabled since the caller activated is no longer returned.
func (r *runtime) Capability(name string) (any, bool) {
	if !slices.Contains(r.mf.Requires, name) && !slices.Contains(r.mf.Optional, name) {
		r.log.Warn("modhost:capability is not listed in Requires or Optional", "capability", name)
		return nil, false
	}
	v := r.h.current()
	if r.pinned || r.building.Load() {
		v = r.v
	}
	impl, ok := v.caps[name]
	return impl, ok
}

type unimplementedCore struct{}

func (unimplementedCore) GetOrg(context.Context, string) (modkit.Org, error) {
	return modkit.Org{}, modkit.ErrNotImplemented
}

func (unimplementedCore) ListProjects(context.Context, string) ([]modkit.Project, error) {
	return nil, modkit.ErrNotImplemented
}

func (unimplementedCore) GetProject(context.Context, string, string) (modkit.Project, error) {
	return modkit.Project{}, modkit.ErrNotImplemented
}

func (unimplementedCore) ListAgents(context.Context, string, string) ([]modkit.Agent, error) {
	return nil, modkit.ErrNotImplemented
}

func (unimplementedCore) GetAgent(context.Context, string, string) (modkit.Agent, error) {
	return modkit.Agent{}, modkit.ErrNotImplemented
}

func (unimplementedCore) ListEnvironments(context.Context, string) ([]modkit.Environment, error) {
	return nil, modkit.ErrNotImplemented
}

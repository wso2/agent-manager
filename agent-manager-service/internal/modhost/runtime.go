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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/growthanalytics"
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
	workers  []workerEntry
	at       []Route
	tools    []toolEntry
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

func (r *runtime) Config(key string) string {
	return config.Value(config.ModuleConfigKey(r.name, key))
}

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

// options resolves a route's options against the manifest: the audit action
// it names and the webhook event it emits must be the module's own.
func (r *runtime) options(rt *Route, opts []modkit.RouteOption) error {
	o := modkit.Options(opts)
	if o.AuditAction != "" {
		full, err := r.h.auditAction(r.name, o.AuditAction)
		if err != nil {
			return err
		}
		rt.AuditAction, rt.AuditRead = full, o.AuditRead
	}
	if o.AuditCoalesce < 0 {
		return fmt.Errorf("negative audit coalesce window %s", o.AuditCoalesce)
	}
	rt.AuditCoalesce = o.AuditCoalesce
	rt.AuditFailClosed = o.AuditFailClosed
	if o.Event != "" {
		e, ok := r.mf.Event(o.Event)
		if !ok || e.Scope == "" {
			return fmt.Errorf("emits %s, which the manifest does not list as an event with a scope", o.Event)
		}
		rt.Event, rt.EventScope, rt.EventWithoutData = o.Event, e.Scope, o.EventWithoutData
	}
	if (len(o.AlsoRequires) > 0 || len(o.AnyPermission) > 0 || o.PlatformAdmin) && (rt.Scope != "" || rt.At != "") {
		return errors.New("AlsoRequires, AnyPermission and PlatformAdmin apply to public routes only")
	}
	for _, list := range []struct {
		names []string
		into  *[]rbac.Permission
	}{{o.AlsoRequires, &rt.AlsoRequires}, {o.AnyPermission, &rt.AnyOf}} {
		for _, n := range list.names {
			full, err := r.h.permission(r.name, n)
			if err != nil {
				return err
			}
			*list.into = append(*list.into, full)
		}
	}
	if len(rt.AnyOf) > 0 && (rt.Permission != "" || len(rt.AlsoRequires) > 0) {
		return errors.New("AnyPermission is for a route registered with no permission")
	}
	if o.PlatformAdmin && strings.Contains(rt.Path, "{orgName}") {
		return errors.New("a PlatformAdmin route reads across organizations and must not name one")
	}
	rt.PlatformAdmin = o.PlatformAdmin
	if o.AllowOrgOverride && (rt.Scope != "" || rt.At != "" || !strings.Contains(rt.Path, "{orgName}")) {
		return errors.New("AllowOrgOverride applies to public routes whose path names {orgName}")
	}
	rt.AllowOrgOverride = o.AllowOrgOverride
	return nil
}

func (r *runtime) Handle(method, path, perm string, h http.HandlerFunc, opts ...modkit.RouteOption) {
	if !r.registering("Handle") {
		return
	}
	if err := r.checkRoute("Handle", method, path, h); err != nil {
		r.fail(err)
		return
	}
	if path == "/internal" || strings.HasPrefix(path, "/internal/") {
		r.fail(fmt.Errorf("Handle %s %s: /m/%s/internal belongs to the module's workload routes (HandleInternal)", method, path, r.name))
		return
	}
	full := rbac.Permission("")
	if perm != "" {
		var err error
		if full, err = r.h.permission(r.name, perm); err != nil {
			r.fail(fmt.Errorf("Handle %s %s: %w", method, path, err))
			return
		}
	}
	route := Route{Module: r.name, Method: method, Path: path, Permission: full, Handler: h}
	if err := r.options(&route, opts); err != nil {
		r.fail(fmt.Errorf("Handle %s %s: %w", method, path, err))
		return
	}
	if full == "" && method != http.MethodGet && (len(route.AnyOf) == 0 || route.AuditAction == "") {
		// Every state-changing route names a permission, as in the core: it
		// is what the route's audit action is named after. One that takes any
		// of several names its action with modkit.Audit.
		r.fail(fmt.Errorf("Handle %s %s: a %s route needs a permission (or AnyPermission with modkit.Audit)", method, path, method))
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes = append(r.routes, route)
}

func (r *runtime) HandleInternal(method, path, scope string, h http.HandlerFunc, opts ...modkit.RouteOption) {
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
	route := Route{Module: r.name, Method: method, Path: path, Scope: scope, Handler: h}
	if err := r.options(&route, opts); err != nil {
		r.fail(fmt.Errorf("HandleInternal %s %s: %w", method, path, err))
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.internal = append(r.internal, route)
}

// internalAtPrefix is the internal API's prefix; HandleInternalAt paths live
// below it.
const internalAtPrefix = "/api/internal/v1"

// internalAtReserved are the core's own paths below the internal prefix.
var internalAtReserved = []string{"/m", "/workloads", "/health"}

func (r *runtime) HandleInternalAt(method, absPath string, h http.HandlerFunc, opts ...modkit.RouteOption) {
	if !r.registering("HandleInternalAt") {
		return
	}
	rel, ok := strings.CutPrefix(absPath, internalAtPrefix)
	if !ok || !strings.HasPrefix(rel, "/") || rel == "/" {
		r.fail(fmt.Errorf("HandleInternalAt %s %s: the path must be below %s/", method, absPath, internalAtPrefix))
		return
	}
	if err := r.checkRoute("HandleInternalAt", method, rel, h); err != nil {
		r.fail(err)
		return
	}
	for _, p := range internalAtReserved {
		if rel == p || strings.HasPrefix(rel, p+"/") {
			r.fail(fmt.Errorf("HandleInternalAt %s %s: %s%s belongs to the core", method, absPath, internalAtPrefix, p))
			return
		}
	}
	route := Route{Module: r.name, Method: method, Path: rel, At: rel, Handler: h}
	if err := r.options(&route, opts); err != nil {
		r.fail(fmt.Errorf("HandleInternalAt %s %s: %w", method, absPath, err))
		return
	}
	if method != http.MethodGet && route.AuditAction == "" {
		// Nothing else names it: the route has no permission.
		r.fail(fmt.Errorf("HandleInternalAt %s %s: a %s route names its audit action with modkit.Audit", method, absPath, method))
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.at = append(r.at, route)
}

var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func (r *runtime) MCPTool(tool modkit.MCPTool, perm string, h modkit.MCPToolHandler, opts ...modkit.RouteOption) {
	if !r.registering("MCPTool") {
		return
	}
	if !toolNamePattern.MatchString(tool.Name) || len(tool.Name) > 64 {
		r.fail(fmt.Errorf("MCPTool %q: the name must be lowercase letters, digits and underscores, starting with a letter, at most 64 characters", tool.Name))
		return
	}
	if tool.Description == "" || h == nil {
		r.fail(fmt.Errorf("MCPTool %s: a tool needs a description and a handler", tool.Name))
		return
	}
	if len(tool.InputSchema) > 0 {
		var schema map[string]any
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil || schema["type"] != "object" {
			r.fail(fmt.Errorf("MCPTool %s: the input schema must be a JSON Schema object of type object", tool.Name))
			return
		}
	}
	if perm == "" {
		r.fail(fmt.Errorf("MCPTool %s: a tool needs a permission", tool.Name))
		return
	}
	full, err := r.h.permission(r.name, perm)
	if err != nil {
		r.fail(fmt.Errorf("MCPTool %s: %w", tool.Name, err))
		return
	}
	route := Route{}
	if err := r.options(&route, opts); err != nil {
		r.fail(fmt.Errorf("MCPTool %s: %w", tool.Name, err))
		return
	}
	if route.Event != "" {
		r.fail(fmt.Errorf("MCPTool %s: tools emit no webhook events", tool.Name))
		return
	}
	action := route.AuditAction
	if action == "" {
		action = string(full)
	}
	r.mu.Lock()
	twice := slices.ContainsFunc(r.tools, func(t toolEntry) bool { return t.tool.Name == tool.Name })
	if !twice {
		r.tools = append(r.tools, toolEntry{module: r.name, tool: tool, perm: full, action: action, auditRead: route.AuditRead, fn: h})
	}
	r.mu.Unlock()
	if twice {
		r.fail(fmt.Errorf("MCPTool %s registered twice", tool.Name))
	}
}

func (r *runtime) Secrets() modkit.Secrets { return moduleSecrets{h: r.h, module: r.name} }

func (r *runtime) Hook(resource string, phase modkit.Phase, h modkit.HookFunc) {
	if !r.registering("Hook") {
		return
	}
	if resource != modkit.ResourceAgent && resource != modkit.ResourceEnvironment && resource != modkit.ResourceProject &&
		resource != modkit.ResourceEventDestination {
		r.fail(fmt.Errorf("Hook: resource %q takes no hooks (want %s, %s, %s or %s)", resource,
			modkit.ResourceAgent, modkit.ResourceEnvironment, modkit.ResourceProject, modkit.ResourceEventDestination))
		return
	}
	if phase != modkit.Validate && phase != modkit.Apply && phase != modkit.Finalize {
		r.fail(fmt.Errorf("Hook: unknown phase %q", phase))
		return
	}
	if resource == modkit.ResourceProject && phase != modkit.Finalize {
		r.fail(fmt.Errorf("Hook: projects take only %s hooks", modkit.Finalize))
		return
	}
	if resource == modkit.ResourceEventDestination && phase != modkit.Validate {
		r.fail(fmt.Errorf("Hook: event destinations take only %s hooks", modkit.Validate))
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

func (r *runtime) Go(name string, fn func(ctx context.Context) error) {
	if !r.registering("Go") {
		return
	}
	if name == "" || strings.ContainsAny(name, " \t") {
		r.fail(fmt.Errorf("Go: worker name %q must be one word", name))
		return
	}
	if fn == nil {
		r.fail(fmt.Errorf("Go %s: nil worker", name))
		return
	}
	r.mu.Lock()
	twice := slices.ContainsFunc(r.workers, func(w workerEntry) bool { return w.name == name })
	if !twice {
		r.workers = append(r.workers, workerEntry{module: r.name, name: name, fn: fn})
	}
	r.mu.Unlock()
	if twice {
		r.fail(fmt.Errorf("Go: worker %s registered twice", name))
	}
}

func (r *runtime) Publish(ctx context.Context, eventType string, data any) error {
	return r.PublishTx(ctx, nil, eventType, data)
}

func (r *runtime) PublishTx(ctx context.Context, tx *gorm.DB, eventType string, data any) error {
	if _, ok := r.mf.Event(eventType); !ok {
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

func (r *runtime) Track(ctx context.Context, event string, props map[string]any) {
	growthanalytics.Mark(ctx, event, props)
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

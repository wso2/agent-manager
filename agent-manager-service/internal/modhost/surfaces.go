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
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/coreext"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// Route is a route a module registered.
type Route struct {
	Module string
	Method string
	Path   string
	// Permission guards a public route; empty means any signed-in user.
	Permission rbac.Permission
	// Scope guards an internal route.
	Scope string
	// At is the fixed path of a route mounted with HandleInternalAt, below
	// /api/internal/v1; the route authenticates its callers itself.
	At      string
	Handler http.HandlerFunc

	// AuditAction, when set, is the full name of the route's audit action;
	// otherwise the permission names it. AuditRead records a read route.
	AuditAction   string
	AuditRead     bool
	AuditCoalesce time.Duration
	// Event, of EventScope, is the webhook event the route emits after a 2xx
	// response.
	Event            string
	EventScope       string
	EventWithoutData bool
}

// Pattern is the route's net/http pattern: /m/<module><path> on the API port,
// or /m/<module>/internal<path> on the internal server.
func (r Route) Pattern() string {
	if r.At != "" {
		return r.Method + " " + r.At
	}
	if r.Scope != "" {
		return r.Method + " /m/" + r.Module + "/internal" + r.Path
	}
	return r.Method + " /m/" + r.Module + r.Path
}

type hookEntry struct {
	module   string
	resource string
	phase    modkit.Phase
	fn       modkit.HookFunc
}

type subEntry struct {
	module string
	event  string
	fn     modkit.EventHandler
}

// Tokens signs and verifies workload tokens with the core's signing keys.
type Tokens interface {
	IssueWorkload(w modkit.Workload, ttl time.Duration) (token string, expiresAt time.Time, err error)
	VerifyWorkload(token string) (modkit.Workload, error)
}

// Deps is what the host needs from the core to serve modules. The server
// attaches them before Boot; commands that only change module state run
// without them.
type Deps struct {
	// Mount registers a module's public route on mux behind the core's
	// route middleware (permission check, audit, org resolution). It may
	// panic on a route the middleware refuses.
	Mount func(mux *http.ServeMux, r Route)
	// MountInternal registers a module's HandleInternalAt route on mux
	// behind the core's internal route middleware (audit). It may panic on a
	// route the middleware refuses.
	MountInternal func(mux *http.ServeMux, r Route)
	Core          modkit.CoreAPI
	Tokens        Tokens
	// Secrets is the core's secret store.
	Secrets SecretStore
	// InternalURL is the origin workloads reach the internal server at,
	// for the endpoints a heartbeat returns; empty means the origin each
	// heartbeat arrived on.
	InternalURL string
}

// Attach supplies the core services modules use. Call it before Boot.
func (h *Host) Attach(d Deps) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.d = d
}

func (h *Host) deps() Deps {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.d
}

var _ coreext.Extensions = (*Host)(nil)

// ---- permissions ----

// permission resolves a permission a module names: one of its own, by its
// manifest name, or a core permission (including those of built-in modules).
func (h *Host) permission(module, name string) (rbac.Permission, error) {
	mf := h.manifests[module]
	for _, p := range mf.Permissions {
		if p.Name == name {
			return permissionName(mf, name), nil
		}
	}
	p := rbac.Permission(name)
	if slices.Contains(rbac.CorePermissions, p) {
		return p, nil
	}
	for _, other := range h.manifests {
		if !other.Builtin {
			continue
		}
		for _, q := range other.Permissions {
			if other.PermissionName(q.Name) == name {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("permission %q is neither declared in the manifest nor a core permission", name)
}

// permissionName is the full name of a module permission (see
// modkit.Manifest.PermissionName).
func permissionName(mf modkit.Manifest, name string) rbac.Permission {
	return rbac.Permission(mf.PermissionName(name))
}

// A permission is <action> or <resource>.<action>.
var permissionNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*(\.[a-z][a-z0-9-]*)?$`)

// declare adds a module's permissions and role grants to the registry being
// built for a snapshot.
func declare(reg *rbac.Registry, mf modkit.Manifest) error {
	perms := make([]rbac.Permission, 0, len(mf.Permissions))
	for _, p := range mf.Permissions {
		written := mf.Builtin && strings.Contains(p.Name, ":")
		if written && strings.ContainsAny(p.Name, " \t") {
			return fmt.Errorf("permission name %q is not valid", p.Name)
		}
		// No colon, so an identity provider that builds scopes as
		// <resource>:<action> gives "<module>:<name>" with the module as the
		// resource.
		if !written && !permissionNamePattern.MatchString(p.Name) {
			return fmt.Errorf("permission name %q must be <action> or <resource>.<action>, each lowercase letters, digits and dashes, starting with a letter", p.Name)
		}
		perms = append(perms, permissionName(mf, p.Name))
	}
	grants := make(map[string][]rbac.Permission, len(mf.RoleGrants))
	for role, names := range mf.RoleGrants {
		for _, n := range names {
			grants[role] = append(grants[role], permissionName(mf, n))
		}
	}
	return reg.Add(mf.Name, perms, grants)
}

// ---- routes ----

func writeNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"message":"Not found"}`))
}

// moduleOf returns the module named by a path of the form /m/<module>/...
func moduleOf(path string) string {
	rest, ok := strings.CutPrefix(path, "/m/")
	if !ok {
		return ""
	}
	name, _, _ := strings.Cut(rest, "/")
	return name
}

// ServeAPI serves module routes, mounted at /m/ on the API port, behind
// sign-in. It resolves the module against the current
// snapshot on every request, so a disabled module's routes are gone at once.
func (h *Host) ServeAPI() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux := h.current().public[moduleOf(r.URL.Path)]
		if mux == nil {
			writeNotFound(w)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// ServeInternalAt serves the routes modules mount at fixed paths with
// HandleInternalAt, below /api/internal/v1 on the internal server. Like the
// other module routes, they are looked up in the current snapshot on every
// request, so they are gone as soon as their module is disabled.
func (h *Host) ServeInternalAt() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux := h.current().internalAt
		if mux == nil {
			writeNotFound(w)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// mountInternalAt builds the mux of every HandleInternalAt route in a
// snapshot. Two modules claiming one pattern refuse the second.
func mountInternalAt(mount func(*http.ServeMux, Route), routes []Route) (mux *http.ServeMux, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("route refused: %v", p)
		}
	}()
	mux = http.NewServeMux()
	for _, rt := range routes {
		if mount != nil {
			mount(mux, rt)
		} else {
			mux.HandleFunc(rt.Pattern(), rt.Handler)
		}
	}
	return mux, nil
}

// ServeInternal serves module workload routes, mounted at /m/ on the internal
// server: /m/<module>/internal/... A compiled-in module that is not active
// answers 401 module_inactive, which tells its workloads to stand by.
func (h *Host) ServeInternal() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := moduleOf(r.URL.Path)
		v := h.current()
		mux := v.internal[name]
		if mux == nil {
			if _, compiled := h.mods[name]; compiled && !slices.Contains(v.active, name) {
				writeWorkloadError(w, http.StatusUnauthorized, WorkloadError{
					Error:   ErrModuleInactive,
					Message: name + " is not active; stand by and retry",
				})
				return
			}
			writeNotFound(w)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// withOrg puts the caller's organization on a public module request.
func withOrg(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c := jwtassertion.GetTokenClaims(r.Context()); c != nil && c.OuId != "" {
			r = r.WithContext(modkit.WithOrgID(r.Context(), c.OuId))
		}
		next(w, r)
	}
}

// mountPublic builds a module's public mux. The core middleware panics on a
// route it cannot serve; that fails the module's activation.
func mountPublic(mount func(*http.ServeMux, Route), routes []Route) (mux *http.ServeMux, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("route refused: %v", p)
		}
	}()
	mux = http.NewServeMux()
	for _, rt := range routes {
		rt.Handler = withOrg(rt.Handler)
		mount(mux, rt)
	}
	return mux, nil
}

// mountInternal builds a module's workload mux: every route requires a
// workload token issued for this module that carries the route's scope.
func (h *Host) mountInternal(module string, routes []Route) (mux *http.ServeMux, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("route refused: %v", p)
		}
	}()
	mux = http.NewServeMux()
	for _, rt := range routes {
		mux.HandleFunc(rt.Pattern(), h.requireWorkload(module, rt.Scope, rt.Handler))
	}
	return mux, nil
}

func (h *Host) requireWorkload(module, scope string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokens := h.deps().Tokens
		raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tokens == nil || !ok || raw == "" {
			writeWorkloadError(w, http.StatusUnauthorized, WorkloadError{Error: ErrInvalidToken, Message: "a workload token is required"})
			return
		}
		wl, err := tokens.VerifyWorkload(strings.TrimSpace(raw))
		if err != nil {
			h.log.Debug("modhost:rejected workload token", "module", module, "error", err)
			writeWorkloadError(w, http.StatusUnauthorized, WorkloadError{Error: ErrInvalidToken, Message: "invalid workload token"})
			return
		}
		if wl.Module != module || !slices.Contains(wl.Scopes, scope) {
			writeWorkloadError(w, http.StatusForbidden, WorkloadError{
				Error:   ErrInsufficientScope,
				Message: "the workload token does not grant this route",
			})
			return
		}
		ctx := modkit.WithWorkload(r.Context(), wl)
		if wl.OrgID != "" {
			ctx = modkit.WithOrgID(ctx, wl.OrgID)
		}
		next(w, r.WithContext(ctx))
	}
}

// ---- workload tokens ----

const (
	// DefaultWorkloadTokenTTL is the lifetime of a workload token when the
	// request names none.
	DefaultWorkloadTokenTTL = 15 * time.Minute
	// MaxWorkloadTokenTTL caps the lifetime of a workload token.
	MaxWorkloadTokenTTL = time.Hour
)

func tokenTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return DefaultWorkloadTokenTTL
	}
	return min(ttl, MaxWorkloadTokenTTL)
}

func checkScopes(scopes []string) error {
	if len(scopes) == 0 {
		return errors.New("modkit: a workload token needs at least one scope")
	}
	for _, s := range scopes {
		if s == "" || strings.ContainsAny(s, " \t") {
			return fmt.Errorf("modkit: scope %q is not valid", s)
		}
	}
	return nil
}

func (h *Host) issueToken(ctx context.Context, module string, req modkit.TokenRequest) (string, error) {
	tokens := h.deps().Tokens
	if tokens == nil {
		return "", modkit.ErrNotImplemented
	}
	if req.Subject == "" {
		return "", errors.New("modkit: a workload token needs a subject")
	}
	if err := checkScopes(req.Scopes); err != nil {
		return "", err
	}
	org := req.OrgID
	if org == "" {
		org = modkit.OrgID(ctx)
	}
	tok, _, err := tokens.IssueWorkload(modkit.Workload{
		Module: module, Subject: req.Subject, OrgID: org, Scopes: req.Scopes, Claims: req.Claims,
	}, tokenTTL(req.TTL))
	return tok, err
}

// ---- console UI ----

// ModuleUI is an active module's console UI, with the permissions it names
// resolved to full names.
type ModuleUI struct {
	Name    string
	Version string
	Builtin bool
	Pages   []UIEntry
	Nav     []UIEntry
	Slots   []UIEntry
}

// UIEntry is one page, nav item or slot contribution. Permission is empty when
// any signed-in user may see it.
type UIEntry struct {
	// ID is the page id, the nav item's page, or the slot.
	ID         string
	Label      string
	Component  string
	Permission rbac.Permission
}

// UI lists the console UI of the active modules, in activation order. An
// entry naming a permission the module cannot use is left out and logged.
func (h *Host) UI() []ModuleUI {
	v := h.current()
	out := make([]ModuleUI, 0, len(v.active))
	for _, name := range v.active {
		mf := h.manifests[name]
		if mf.UI == nil {
			continue
		}
		m := ModuleUI{Name: name, Version: mf.Version, Builtin: mf.Builtin}
		add := func(list *[]UIEntry, e UIEntry, perm string) {
			if perm != "" {
				p, err := h.permission(name, perm)
				if err != nil {
					h.log.Warn("module UI entry left out", "module", name, "entry", e.ID, "error", err)
					return
				}
				e.Permission = p
			}
			*list = append(*list, e)
		}
		for _, p := range mf.UI.Pages {
			add(&m.Pages, UIEntry{ID: p.ID}, p.Permission)
		}
		for _, n := range mf.UI.Nav {
			add(&m.Nav, UIEntry{ID: n.Page, Label: n.Label}, n.Permission)
		}
		for _, s := range mf.UI.Slots {
			add(&m.Slots, UIEntry{ID: s.Slot, Component: s.Component}, s.Permission)
		}
		out = append(out, m)
	}
	return out
}

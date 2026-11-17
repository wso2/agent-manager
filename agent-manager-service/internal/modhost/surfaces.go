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
	Scope   string
	Handler http.HandlerFunc
}

// Pattern is the route's net/http pattern on the API mux, below /api/v1 or
// /api/internal/v1.
func (r Route) Pattern() string {
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
	Mount  func(mux *http.ServeMux, r Route)
	Core   modkit.CoreAPI
	Tokens Tokens
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
			if q.Name == name {
				return p, nil
			}
		}
	}
	return "", fmt.Errorf("permission %q is neither declared in the manifest nor a core permission", name)
}

// permissionName is the full name of a module permission: "<module>:<name>",
// or the name as written for a built-in module.
func permissionName(mf modkit.Manifest, name string) rbac.Permission {
	if mf.Builtin {
		return rbac.Permission(name)
	}
	return rbac.Permission(mf.Name + ":" + name)
}

var permissionNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// declare adds a module's permissions and role grants to the registry being
// built for a snapshot.
func declare(reg *rbac.Registry, mf modkit.Manifest) error {
	perms := make([]rbac.Permission, 0, len(mf.Permissions))
	for _, p := range mf.Permissions {
		if mf.Builtin && (p.Name == "" || strings.ContainsAny(p.Name, " \t")) {
			return fmt.Errorf("permission name %q is not valid", p.Name)
		}
		// One segment, so an identity provider that builds scopes as
		// <resource>:<action> gives "<module>:<name>" with the module as the
		// resource.
		if !mf.Builtin && !permissionNamePattern.MatchString(p.Name) {
			return fmt.Errorf("permission name %q must be lowercase letters, digits and dashes, starting with a letter", p.Name)
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

// ServeAPI serves module routes, mounted at /m/ on the API mux (below
// /api/v1, behind sign-in). It resolves the module against the current
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

// ServeInternal serves module workload routes, mounted at /m/ on the internal
// API mux (below /api/internal/v1).
func (h *Host) ServeInternal() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux := h.current().internal[moduleOf(r.URL.Path)]
		if mux == nil {
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

func writeUnauthorized(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
	}
	w.WriteHeader(status)
	body, _ := json.Marshal(map[string]string{"message": msg})
	_, _ = w.Write(body)
}

func (h *Host) requireWorkload(module, scope string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokens := h.deps().Tokens
		raw, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if tokens == nil || !ok || raw == "" {
			writeUnauthorized(w, http.StatusUnauthorized, "a workload token is required")
			return
		}
		wl, err := tokens.VerifyWorkload(strings.TrimSpace(raw))
		if err != nil {
			h.log.Debug("modhost:rejected workload token", "module", module, "error", err)
			writeUnauthorized(w, http.StatusUnauthorized, "invalid workload token")
			return
		}
		if wl.Module != module || !slices.Contains(wl.Scopes, scope) {
			writeUnauthorized(w, http.StatusForbidden, "the workload token does not grant this route")
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

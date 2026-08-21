// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package middleware

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const DefaultOrgName = "default"

const orgNamePlaceholder = "{" + utils.PathParamOrgName + "}"

// RouteRegistrar wraps an http.ServeMux and an OrgResolver to provide route
// registration helpers that automatically apply org validation + resolution on
// any pattern containing {orgName}.
//
// It is also the audit trail's coverage guarantee. Every route in the public
// API is registered here, and registration already carries the two things an
// audit record needs — the route pattern and the permission that gates it — so
// wrapping here audits the whole API without touching a single route file.
type RouteRegistrar struct {
	mux         *http.ServeMux
	orgResolver OrgResolver
	auditor     audit.Recorder
	// surface distinguishes the internal gateway server from the public API so
	// its records are not mistaken for user-driven traffic.
	surface audit.Surface
	// routes is a registration ledger. It exists for the coverage test, which
	// walks it to assert that no mutating route shipped unaudited; it is never
	// consulted at request time.
	routes []audit.RouteMeta
}

// NewRouteRegistrar creates a RouteRegistrar backed by the given mux and resolver.
// A nil auditor disables audit recording for routes registered through it.
func NewRouteRegistrar(mux *http.ServeMux, resolver OrgResolver, auditor audit.Recorder) *RouteRegistrar {
	return newRouteRegistrar(mux, resolver, auditor, audit.SurfaceAPI)
}

// NewInternalRouteRegistrar creates a registrar for the gateway-facing internal
// server.
//
// That server has no JWT middleware — gateways authenticate with an api-key
// header checked inside each handler — so its routes carry no permission and
// no org placeholder, and the registrar applies neither. What it does apply is
// the audit wrapper and the route ledger, which is the point: registering here
// rather than on a bare mux is what puts these routes under the same coverage
// test as the public API.
func NewInternalRouteRegistrar(mux *http.ServeMux, auditor audit.Recorder) *RouteRegistrar {
	return newRouteRegistrar(mux, nil, auditor, audit.SurfaceInternal)
}

func newRouteRegistrar(
	mux *http.ServeMux, resolver OrgResolver, auditor audit.Recorder, surface audit.Surface,
) *RouteRegistrar {
	if auditor == nil {
		auditor = audit.NewNoopRecorder()
	}
	return &RouteRegistrar{mux: mux, orgResolver: resolver, auditor: auditor, surface: surface}
}

// Routes returns the audit metadata for every registered route. Used by the
// coverage test to detect a new mutating endpoint that is not audited.
func (rr *RouteRegistrar) Routes() []audit.RouteMeta {
	out := make([]audit.RouteMeta, len(rr.routes))
	copy(out, rr.routes)
	return out
}

// register applies the common wrapper chain and installs the route.
//
// authz is applied by the caller-supplied function so that the differences
// between the permission variants stay visible at each call site rather than
// hiding behind a flag. Order, innermost first: path-param validation, authz,
// org resolution, audit. Audit is outermost so it observes the 400 from
// validation and the 403 from authz as well as the handler's own response.
func (rr *RouteRegistrar) register(
	pattern string,
	perms []rbac.Permission,
	authz func(http.HandlerFunc) http.HandlerFunc,
	handler http.HandlerFunc,
) {
	params := extractPathParams(pattern)
	if len(params) > 0 {
		handler = WithPathParamValidation(handler, params...)
	}
	if authz != nil {
		handler = authz(handler)
	}
	if strings.Contains(pattern, orgNamePlaceholder) {
		handler = RequireOrgMatch(rr.orgResolver)(handler)
	}

	meta := audit.NewRouteMetaForSurface(pattern, params, perms, rr.surface)
	rr.routes = append(rr.routes, meta)
	handler = WithAudit(rr.auditor, meta)(handler)
	// Outermost, so the completion record reports the status actually returned
	// — including the 400 from path-param validation and the 403 from authz.
	handler = WithRequestLog(meta)(handler)

	rr.mux.HandleFunc(pattern, handler)
}

func (rr *RouteRegistrar) HandleFuncWithValidation(pattern string, handler http.HandlerFunc) {
	rr.register(pattern, nil, nil, handler)
}

func (rr *RouteRegistrar) HandleFuncWithValidationAndAuthz(pattern string, perm rbac.Permission, handler http.HandlerFunc) {
	rr.register(pattern, []rbac.Permission{perm}, RequirePermission(perm), handler)
}

// HandleFuncWithValidationAndAuthzAllowRootOU is identical to
// HandleFuncWithValidationAndAuthz except it also bypasses the scope check for
// a client-credentials token issued to the configured root/admin OU. Org
// resolution is unchanged from the normal path: under the token-trust model
// the org always comes from the token, so this does NOT let a root-OU token
// act on a different org named in the path — see RequireOrgMatchAllowRootOU /
// RequirePermissionAllowRootOU. Use only for system-client bootstrap endpoints.
func (rr *RouteRegistrar) HandleFuncWithValidationAndAuthzAllowRootOU(pattern string, perm rbac.Permission, handler http.HandlerFunc) {
	// The root-OU bypass admits a token regardless of its scopes, so the audit
	// record for these routes needs to show that the normal check did not decide
	// the outcome. RequirePermissionAllowRootOU marks the request when it takes
	// that path; see auditRootOUBypass in authorization.go.
	rr.registerRootOU(pattern, []rbac.Permission{perm}, RequirePermissionAllowRootOU(perm), handler)
}

func (rr *RouteRegistrar) HandleFuncWithValidationAndAnyAuthz(pattern string, handler http.HandlerFunc, perms ...rbac.Permission) {
	rr.register(pattern, perms, RequireAnyPermission(perms...), handler)
}

func (rr *RouteRegistrar) HandleFuncWithValidationAndDynamicAuthz(pattern string, resolver PermissionResolver, handler http.HandlerFunc) {
	// The permission is resolved per request, so none is known at registration
	// time. The resolved one is recorded on the event by RequireDynamicPermission.
	rr.register(pattern, nil, RequireDynamicPermission(resolver), handler)
}

// registerRootOU mirrors register but uses the root-OU org resolution variant.
func (rr *RouteRegistrar) registerRootOU(
	pattern string,
	perms []rbac.Permission,
	authz func(http.HandlerFunc) http.HandlerFunc,
	handler http.HandlerFunc,
) {
	params := extractPathParams(pattern)
	if len(params) > 0 {
		handler = WithPathParamValidation(handler, params...)
	}
	handler = authz(handler)
	if strings.Contains(pattern, orgNamePlaceholder) {
		handler = RequireOrgMatchAllowRootOU(rr.orgResolver)(handler)
	}

	meta := audit.NewRouteMetaForSurface(pattern, params, perms, rr.surface)
	rr.routes = append(rr.routes, meta)
	handler = WithAudit(rr.auditor, meta)(handler)
	// Outermost, so the completion record reports the status actually returned
	// — including the 400 from path-param validation and the 403 from authz.
	handler = WithRequestLog(meta)(handler)

	rr.mux.HandleFunc(pattern, handler)
}

// WithPathParamValidation wraps a handler and validates required path parameters
// This runs after route matching, so r.PathValue() works correctly
func WithPathParamValidation(handler http.HandlerFunc, requiredParams ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Validate each required parameter
		for _, paramName := range requiredParams {
			value := r.PathValue(paramName)
			if strings.TrimSpace(value) == "" {
				utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing required path parameter: "+paramName)
				return
			}
		}

		// All validations passed, call the original handler
		handler(w, r)
	}
}

// extractPathParams extracts parameter names from a route pattern
// Example: "GET /orgs/{orgName}/projects/{projName}" -> ["orgName", "projName"]
func extractPathParams(pattern string) []string {
	re := regexp.MustCompile(`\{([^}]+)\}`)
	matches := re.FindAllStringSubmatch(pattern, -1)

	params := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			paramName := strings.TrimSpace(match[1])
			params = append(params, paramName)
		}
	}

	return params
}

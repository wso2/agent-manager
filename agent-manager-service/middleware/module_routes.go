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

package middleware

import (
	"net/http"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// ModuleRoutePolicy is what a module route declares besides its permission:
// its audit policy and the webhook event it emits.
type ModuleRoutePolicy struct {
	Audit audit.RoutePolicy
	// Event, of EventScope, is the webhook event emitted after a 2xx
	// response, without the response body when EventWithoutData is set.
	Event            string
	EventScope       events.Scope
	EventWithoutData bool
}

// HandleModuleRoute registers a module's route with the same middleware as a
// core route. Module routes are always scoped to the caller's organization,
// taken from the token, whether or not the pattern names one. An empty perm
// lets any signed-in member of the organization in; the module host allows
// that only on reads, so every audited route still names its action.
func (rr *RouteRegistrar) HandleModuleRoute(pattern string, perm rbac.Permission, policy ModuleRoutePolicy, handler http.HandlerFunc) {
	var perms []rbac.Permission
	var authz func(http.HandlerFunc) http.HandlerFunc
	if perm != "" {
		perms = []rbac.Permission{perm}
		authz = RequirePermission(perm)
	}
	params := extractPathParams(pattern)
	meta := audit.NewRouteMetaWithPolicy(pattern, params, perms, rr.surface, policy.Audit)
	handler = WithDefaultEnvironment(handler)
	handler = withRouteEvent(rr.events, meta, policy.Event, policy.EventScope, policy.EventWithoutData)(handler)
	if len(params) > 0 {
		handler = WithPathParamValidation(handler, params...)
	}
	if authz != nil {
		handler = authz(handler)
	}
	handler = resolveOrgFromToken()(handler)
	if strings.Contains(pattern, orgNamePlaceholder) {
		handler = RequireOrgMatch(rr.orgResolver)(handler)
	}
	rr.install(pattern, meta, handler)
}

// HandleModuleInternalRoute registers a module's route at a fixed path on the
// internal server (HandleInternalAt). It authenticates its callers itself, so
// only the audit wrapper applies.
func (rr *RouteRegistrar) HandleModuleInternalRoute(pattern string, policy ModuleRoutePolicy, handler http.HandlerFunc) {
	meta := audit.NewRouteMetaWithPolicy(pattern, extractPathParams(pattern), nil, rr.surface, policy.Audit)
	rr.install(pattern, meta, handler)
}

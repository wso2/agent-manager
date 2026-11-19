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

package gwapi

import (
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// coreRouter serves the gateway's routes on the core's route registrar, for
// tests that exercise them without a module host. It tracks nothing, and sets
// modkit.OrgID from the resolved org as the module host does.
type coreRouter struct{ *middleware.RouteRegistrar }

func (coreRouter) Track(_ string, _ map[string]any, h http.HandlerFunc) http.HandlerFunc { return h }

func (c coreRouter) HandleFuncWithValidationAndAuthz(pattern string, perm rbac.Permission, h http.HandlerFunc) {
	c.RouteRegistrar.HandleFuncWithValidationAndAuthz(pattern, perm, withOrgID(h))
}

func (c coreRouter) HandleFuncWithValidationAndAllAuthz(pattern string, h http.HandlerFunc, perms ...rbac.Permission) {
	c.RouteRegistrar.HandleFuncWithValidationAndAllAuthz(pattern, withOrgID(h), perms...)
}

func (c coreRouter) HandleFuncWithValidationAndAnyAuthz(pattern string, h http.HandlerFunc, perms ...rbac.Permission) {
	c.RouteRegistrar.HandleFuncWithValidationAndAnyAuthz(pattern, withOrgID(h), perms...)
}

func (c coreRouter) HandleFuncWithValidationAndPlatformAdminAuthz(pattern string, perm rbac.Permission, h http.HandlerFunc) {
	c.RouteRegistrar.HandleFuncWithValidationAndPlatformAdminAuthz(pattern, perm, withOrgID(h))
}

func withOrgID(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if org, ok := middleware.GetResolvedOrg(r.Context()); ok {
			r = r.WithContext(modkit.WithOrgID(r.Context(), org.OUID))
		}
		h(w, r)
	}
}

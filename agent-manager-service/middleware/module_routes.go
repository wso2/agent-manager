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

	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// HandleModuleRoute registers a module's route with the same middleware as a
// core route. Module routes are always scoped to the caller's organization,
// taken from the token, whether or not the pattern names one. An empty perm
// lets any signed-in member of the organization in; the module host allows
// that only on reads, so every audited route still names its action.
func (rr *RouteRegistrar) HandleModuleRoute(pattern string, perm rbac.Permission, handler http.HandlerFunc) {
	var perms []rbac.Permission
	var authz func(http.HandlerFunc) http.HandlerFunc
	if perm != "" {
		perms = []rbac.Permission{perm}
		authz = RequirePermission(perm)
	}
	rr.registerWithOrgOverride(pattern, perms, authz, resolveOrgFromToken(), handler)
}

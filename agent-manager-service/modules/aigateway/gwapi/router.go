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
	"regexp"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtrack"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// Router takes the gateway's public routes. NewModuleRouter mounts them on the
// module runtime at /m/ai-gateway.
type Router interface {
	HandleFuncWithValidationAndAuthz(pattern string, perm string, handler http.HandlerFunc)
	HandleFuncWithValidationAndAllAuthz(pattern string, handler http.HandlerFunc, perms ...string)
	HandleFuncWithValidationAndAnyAuthz(pattern string, handler http.HandlerFunc, perms ...string)
	HandleFuncWithValidationAndPlatformAdminAuthz(pattern string, perm string, handler http.HandlerFunc)
	// Track reports the requests handler serves as feature usage.
	Track(feature string, dimensions map[string]any, handler http.HandlerFunc) http.HandlerFunc
}

// InternalRouter takes the gateway's controller-facing routes, whose handlers
// authenticate the controller themselves.
type InternalRouter interface {
	HandleFuncWithValidation(pattern string, handler http.HandlerFunc)
}

// NewModuleRouter mounts routes with rt.Handle, relative to /m/ai-gateway,
// with the audit actions and webhook events routeOptions declares.
func NewModuleRouter(rt modkit.Runtime) Router { return moduleRouter{rt} }

// NewModuleInternalRouter mounts routes with rt.HandleInternalAt at their
// paths below /api/internal/v1, where the released gateway controllers call
// them (until they can register as workloads, wso2/agent-manager#2493).
func NewModuleInternalRouter(rt modkit.Runtime) InternalRouter { return moduleInternalRouter{rt} }

type moduleRouter struct{ rt modkit.Runtime }

func (m moduleRouter) HandleFuncWithValidationAndAuthz(pattern string, perm string, h http.HandlerFunc) {
	m.handle(pattern, perm, h)
}

func (m moduleRouter) HandleFuncWithValidationAndAllAuthz(pattern string, h http.HandlerFunc, perms ...string) {
	also := make([]string, 0, len(perms))
	for _, p := range perms[1:] {
		also = append(also, short(p))
	}
	m.handle(pattern, perms[0], h, modkit.AlsoRequires(also...))
}

func (m moduleRouter) HandleFuncWithValidationAndAnyAuthz(pattern string, h http.HandlerFunc, perms ...string) {
	anyOf := make([]string, 0, len(perms))
	for _, p := range perms {
		anyOf = append(anyOf, short(p))
	}
	m.handle(pattern, "", h, modkit.AnyPermission(anyOf...))
}

func (m moduleRouter) HandleFuncWithValidationAndPlatformAdminAuthz(pattern string, perm string, h http.HandlerFunc) {
	m.handle(pattern, perm, h, modkit.PlatformAdmin())
}

func (m moduleRouter) Track(feature string, dimensions map[string]any, h http.HandlerFunc) http.HandlerFunc {
	return gwtrack.Wrap(m.rt.Track, feature, dimensions, h)
}

func (m moduleRouter) handle(pattern string, perm string, h http.HandlerFunc, opts ...modkit.RouteOption) {
	method, path, _ := strings.Cut(pattern, " ")
	m.rt.Handle(method, path, short(perm), h, append(opts, routeOptions[pattern]...)...)
}

type moduleInternalRouter struct{ rt modkit.Runtime }

func (m moduleInternalRouter) HandleFuncWithValidation(pattern string, h http.HandlerFunc) {
	method, path, _ := strings.Cut(pattern, " ")
	if params := pathParams(path); len(params) > 0 {
		h = requirePathParams(h, params)
	}
	m.rt.HandleInternalAt(method, internalBase+path, h, internalRouteOptions[pattern]...)
}

// internalBase is where the gateway controllers call the control plane.
const internalBase = "/api/internal/v1"

// short is a permission's name as the module runtime takes it: the module's
// own without its prefix, a core one as it is.
func short(p string) string {
	return strings.TrimPrefix(string(p), gwperm.Module+":")
}

// requirePathParams answers 400 when a path parameter of the route is blank.
func requirePathParams(h http.HandlerFunc, params []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		for _, p := range params {
			if strings.TrimSpace(r.PathValue(p)) == "" {
				httpx.WriteErrorResponse(w, http.StatusBadRequest, "Missing required path parameter: "+p)
				return
			}
		}
		h(w, r)
	}
}

var pathParam = regexp.MustCompile(`\{([^}]+)\}`)

func pathParams(path string) []string {
	var out []string
	for _, m := range pathParam.FindAllStringSubmatch(path, -1) {
		out = append(out, m[1])
	}
	return out
}

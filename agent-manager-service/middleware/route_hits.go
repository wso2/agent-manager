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
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
)

// coreModule names the routes the core registers in the route-hit log.
const coreModule = "core"

var routeHitLog atomic.Bool

// SetRouteHitLog turns the route-hit log on or off (LOG_ROUTE_HITS). It is
// called once at startup, before any route is registered. When on, every
// route registered through a RouteRegistrar is logged once ("route
// registered") and on every request ("route hit"), with its pattern, module
// and response status; never the request path, query, headers or body. The
// e2e suite reads it to report which routes it covers.
func SetRouteHitLog(on bool) {
	routeHitLog.Store(on)
}

// withRouteHitLog logs the route on registration and wraps handler to log
// each request to it, when the route-hit log is on.
func withRouteHitLog(meta audit.RouteMeta, module string, handler http.HandlerFunc) http.HandlerFunc {
	if !routeHitLog.Load() {
		return handler
	}
	if module == "" {
		module = coreModule
	}
	slog.Info("route registered", "pattern", meta.Pattern, "module", module, "surface", string(meta.Surface))
	return func(w http.ResponseWriter, r *http.Request) {
		rec := newResponseRecorder(w)
		defer func() {
			// A panic becomes the recoverer's 500.
			panicked := recover()
			if panicked != nil {
				rec.setStatus(http.StatusInternalServerError)
			}
			slog.Info("route hit", "pattern", meta.Pattern, "module", module,
				"surface", string(meta.Surface), "status", rec.Status())
			if panicked != nil {
				panic(panicked)
			}
		}()
		handler(rec, r)
	}
}

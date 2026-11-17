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

package api

import (
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/apicatalog"
	"github.com/wso2/agent-manager/agent-manager-service/apidocs"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/docs"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
)

// ActiveModules lists the modules the module host has active.
type ActiveModules interface {
	Active() []string
}

// registerAPICatalogRoutes serves the REST API's description: the OpenAPI
// spec of the core and the active modules, as YAML and JSON, and the
// reference page that renders it.
//
// Like /api/v1/config they need no token: the spec is the API's public
// shape, with no data or settings in it, and a client reads it to learn
// how to authenticate. The spec routes take no method in the pattern so the
// CORS preflight reaches the CORS middleware (see registerConfigRoutes).
func registerAPICatalogRoutes(mux *http.ServeMux, modules ActiveModules) {
	catalog := apicatalog.New(docs.OpenAPI)
	corsWrap := middleware.CORS(config.GetConfig().CORSAllowedOrigin)
	spec := func(contentType string, body func(*apicatalog.Spec) []byte) http.Handler {
		return corsWrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var active []string
			if modules != nil {
				active = modules.Active()
			}
			s, err := catalog.For(active)
			if err != nil {
				logger.GetLogger(r.Context()).Error("failed to render the API spec", "error", err)
				http.Error(w, "failed to render the API spec", http.StatusInternalServerError)
				return
			}
			// Enabling or disabling a module changes the spec, so it is
			// revalidated rather than cached.
			w.Header().Set("Content-Type", contentType)
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(body(s))
		}))
	}
	mux.Handle("/api/v1/openapi.yaml", spec("application/yaml", func(s *apicatalog.Spec) []byte { return s.YAML }))
	mux.Handle("/api/v1/openapi.json", spec("application/json", func(s *apicatalog.Spec) []byte { return s.JSON }))

	mux.Handle("GET /api/docs", apidocs.Page())
	mux.Handle("GET /api/docs/{file}", apidocs.Files())
}

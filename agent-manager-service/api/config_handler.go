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
	"encoding/json"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/growthanalytics"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// registerConfigRoutes registers the unauthenticated service-configuration
// discovery endpoint. It is registered without a method in the pattern
// (rather than "GET /api/v1/config") because the Go 1.22 ServeMux would
// otherwise auto-reject OPTIONS preflight requests with a 405 before the
// CORS middleware ever runs, breaking cross-origin GETs from the console/CLI.
// Method enforcement is done inside the handler instead.
func registerConfigRoutes(mux *http.ServeMux) {
	corsWrap := middleware.CORS(config.GetConfig().CORSAllowedOrigin)
	mux.Handle("/api/v1/config", corsWrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		cfg := config.GetConfig()
		// Served from the service's own CONSOLE_ANALYTICS_ENABLED so that flag
		// is the single switch for the whole path: the console asks here
		// whether to report at all, rather than carrying a second flag of its
		// own that could disagree with this one.
		consoleAnalytics := growthanalytics.ConsoleEnabled(cfg.GrowthAnalytics)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(spec.ConfigResponse{
			ObserverBaseUrl:         cfg.Observer.PublicURL,
			ConsoleAnalyticsEnabled: &consoleAnalytics,
		}); err != nil {
			logger.GetLogger(r.Context()).Error("failed to encode config response", "error", err)
		}
	})))
}

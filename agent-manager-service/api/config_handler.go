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
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

func registerConfigRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/config", func(w http.ResponseWriter, r *http.Request) {
		cfg := config.GetConfig()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(spec.ConfigResponse{
			TraceObserverBaseUrl: cfg.TraceObserver.URL,
		}); err != nil {
			logger.GetLogger(r.Context()).Error("failed to encode config response", "error", err)
		}
	})
}

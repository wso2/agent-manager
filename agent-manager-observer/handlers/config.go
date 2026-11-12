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

package handlers

import (
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-observer/config"
)

// ObservabilityConfigResponse is the body of GET /api/v1/observability/config.
type ObservabilityConfigResponse struct {
	Traces TracesConfigResponse `json:"traces"`
}

// TracesConfigResponse says whether traces are served, and by which backend.
type TracesConfigResponse struct {
	Enabled bool   `json:"enabled"`
	Backend string `json:"backend"`
}

// GetObservabilityConfig handles GET /api/v1/observability/config for cfg's trace backend.
func GetObservabilityConfig(cfg config.TracesConfig) http.HandlerFunc {
	body := ObservabilityConfigResponse{Traces: TracesConfigResponse{
		Enabled: cfg.Backend != config.TraceBackendOff,
		Backend: cfg.Backend,
	}}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		writeJSON(w, http.StatusOK, body)
	}
}

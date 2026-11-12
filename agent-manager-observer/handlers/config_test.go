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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-observer/config"
)

func TestGetObservabilityConfig(t *testing.T) {
	for backend, enabled := range map[string]bool{config.TraceBackendOpenChoreo: true, config.TraceBackendOff: false} {
		rec := httptest.NewRecorder()
		GetObservabilityConfig(config.TracesConfig{Backend: backend})(rec,
			httptest.NewRequest(http.MethodGet, "/api/v1/observability/config", nil))

		assertStatus(t, rec, http.StatusOK)
		var body ObservabilityConfigResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		want := TracesConfigResponse{Enabled: enabled, Backend: backend}
		if body.Traces != want {
			t.Errorf("%s: traces = %+v, want %+v", backend, body.Traces, want)
		}
	}
}

func TestGetObservabilityConfig_RejectsNonGet(t *testing.T) {
	rec := httptest.NewRecorder()
	GetObservabilityConfig(config.TracesConfig{Backend: config.TraceBackendOff})(rec,
		httptest.NewRequest(http.MethodPost, "/api/v1/observability/config", nil))
	assertStatus(t, rec, http.StatusMethodNotAllowed)
}

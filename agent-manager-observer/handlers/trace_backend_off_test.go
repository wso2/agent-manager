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

	"github.com/wso2/agent-manager/agent-manager-observer/controllers"
	"github.com/wso2/agent-manager/agent-manager-observer/store/off"
)

func TestTraceRoutes_BackendOff(t *testing.T) {
	h := NewHandler(controllers.NewTracingController(off.Store{}), nil)
	tests := []struct {
		name string
		path string
		call func(http.ResponseWriter, *http.Request)
	}{
		{"list", "/api/v1/traces?" + baseParams(), h.GetTraceOverviews},
		{"filtered list", "/api/v1/traces?" + baseParams() + "&status=error", h.GetTraceOverviews},
		{"export", "/api/v1/traces/export?" + baseParams(), h.ExportTraces},
		{"filtered export", "/api/v1/traces/export?" + baseParams() + "&status=error", h.ExportTraces},
		{"spans", "/api/v1/traces/trace-1/spans?" + baseParams(), h.GetTraceSpans},
		{"span detail", "/api/v1/traces/trace-1/spans/span-1", h.GetSpanDetail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tt.call(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			assertStatus(t, rec, http.StatusServiceUnavailable)
			var body ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body.Error != ErrCodeObservabilityDisabled {
				t.Errorf("error = %q, want %q", body.Error, ErrCodeObservabilityDisabled)
			}
		})
	}
}

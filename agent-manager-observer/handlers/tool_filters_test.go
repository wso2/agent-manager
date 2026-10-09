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
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-observer/controllers"
)

// tool and toolError parse; toolError is "true", or empty or "false" for unset.
func TestParseTraceFilters_Tool(t *testing.T) {
	tests := []struct {
		query   string
		want    controllers.TraceFilters
		wantErr string
	}{
		{query: "", want: controllers.TraceFilters{}},
		{query: "tool=search_web", want: controllers.TraceFilters{Tool: "search_web"}},
		{query: "toolError=true", want: controllers.TraceFilters{ToolError: true}},
		{query: "tool=search_web&toolError=true", want: controllers.TraceFilters{Tool: "search_web", ToolError: true}},
		{query: "toolError=false", want: controllers.TraceFilters{}},
		{query: "toolError=", want: controllers.TraceFilters{}},
		{query: "tool=", want: controllers.TraceFilters{}},
		{query: "toolError=yes", wantErr: "toolError"},
		{query: "toolError=TRUE", wantErr: "toolError"},
		{query: "toolError=1", wantErr: "toolError"},
		{query: "tool=" + strings.Repeat("a", 257), wantErr: "tool must be at most 256 characters"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			q, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}
			got, err := parseTraceFilters(q)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("filters = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// An invalid toolError or an over-long tool is a 400 on the list and the export,
// and a valid or unset one is accepted.
func TestTraceEndpoints_ToolFilters(t *testing.T) {
	endpoints := map[string]func(*Handler) http.HandlerFunc{
		"/api/v1/traces":        func(h *Handler) http.HandlerFunc { return h.GetTraceOverviews },
		"/api/v1/traces/export": func(h *Handler) http.HandlerFunc { return h.ExportTraces },
	}
	tests := []struct {
		name  string
		query string
		want  int
	}{
		{name: "toolError=yes", query: "&toolError=yes", want: http.StatusBadRequest},
		{name: "toolError=TRUE", query: "&toolError=TRUE", want: http.StatusBadRequest},
		{name: "tool over 256 characters", query: "&tool=" + strings.Repeat("a", 257), want: http.StatusBadRequest},
		{name: "toolError=true", query: "&toolError=true", want: http.StatusOK},
		{name: "toolError=false", query: "&toolError=false", want: http.StatusOK},
		{name: "tool and toolError", query: "&tool=search&toolError=true", want: http.StatusOK},
		{name: "tool at 256 characters", query: "&tool=" + strings.Repeat("a", 256), want: http.StatusOK},
	}
	for path, handler := range endpoints {
		for _, tt := range tests {
			t.Run(path+" "+tt.name, func(t *testing.T) {
				h := NewHandler(controllers.NewTracingController(&fakeObserverClient{}), nil)
				r := httptest.NewRequest(http.MethodGet, path+"?"+baseParams()+tt.query, nil)
				rec := httptest.NewRecorder()
				handler(h)(rec, r)

				assertStatus(t, rec, tt.want)
				if tt.want == http.StatusBadRequest && strings.Contains(rec.Body.String(), strings.Repeat("a", 257)) {
					t.Error("body echoes the rejected value")
				}
			})
		}
	}
}

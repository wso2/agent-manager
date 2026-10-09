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

// include=mcpServers sets MCPServers, alone or beside the others; the name is case-sensitive.
func TestParseInclude_MCPServers(t *testing.T) {
	tests := []struct {
		raw     []string
		want    controllers.Include
		wantErr bool
	}{
		{raw: []string{"mcpServers"}, want: controllers.Include{MCPServers: true}},
		{raw: []string{"models,tools,mcpServers"}, want: controllers.Include{Models: true, Tools: true, MCPServers: true}},
		{raw: []string{"tools", "mcpServers"}, want: controllers.Include{Tools: true, MCPServers: true}},
		{raw: []string{"mcpservers"}, wantErr: true},
	}
	for _, tt := range tests {
		got, err := parseInclude(tt.raw)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseInclude(%q) error = %v, wantErr %t", tt.raw, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("parseInclude(%q) = %+v, want %+v", tt.raw, got, tt.want)
		}
	}
}

// mcpServer parses as a plain string, capped at 256 characters.
func TestParseTraceFilters_MCPServer(t *testing.T) {
	tests := []struct {
		query   string
		want    controllers.TraceFilters
		wantErr string
	}{
		{query: "mcpServer=github", want: controllers.TraceFilters{MCPServer: "github"}},
		{query: "mcpServer=", want: controllers.TraceFilters{}},
		{query: "mcpServer=github&toolError=true", want: controllers.TraceFilters{MCPServer: "github", ToolError: true}},
		{query: "mcpServer=" + strings.Repeat("a", 257), wantErr: "mcpServer must be at most 256 characters"},
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

// An over-long mcpServer is a 400 on the list and the export; a valid one is accepted.
func TestTraceEndpoints_MCPServerFilter(t *testing.T) {
	endpoints := map[string]func(*Handler) http.HandlerFunc{
		"/api/v1/traces":        func(h *Handler) http.HandlerFunc { return h.GetTraceOverviews },
		"/api/v1/traces/export": func(h *Handler) http.HandlerFunc { return h.ExportTraces },
	}
	tests := []struct {
		name  string
		query string
		want  int
	}{
		{name: "mcpServer over 256 characters", query: "&mcpServer=" + strings.Repeat("a", 257), want: http.StatusBadRequest},
		{name: "mcpServer", query: "&mcpServer=github", want: http.StatusOK},
		{name: "mcpServer at 256 characters", query: "&mcpServer=" + strings.Repeat("a", 256), want: http.StatusOK},
	}
	for path, handler := range endpoints {
		for _, tt := range tests {
			t.Run(path+" "+tt.name, func(t *testing.T) {
				h := NewHandler(controllers.NewTracingController(&fakeObserverClient{}), nil)
				r := httptest.NewRequest(http.MethodGet, path+"?"+baseParams()+tt.query, nil)
				rec := httptest.NewRecorder()
				handler(h)(rec, r)

				assertStatus(t, rec, tt.want)
			})
		}
	}
}

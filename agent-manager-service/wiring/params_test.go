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

package wiring

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/config"
)

func TestProvideAuthMiddlewareAdvertisesResourceSpecificMetadata(t *testing.T) {
	middleware := ProvideAuthMiddleware(config.Config{
		AuthHeader:      "Authorization",
		ServerPublicURL: "https://api.example.com/",
	})
	handler := middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "REST API", path: "/api/v1/projects", want: "https://api.example.com/.well-known/oauth-protected-resource"},
		{name: "MCP", path: "/mcp", want: "https://api.example.com/.well-known/oauth-protected-resource/mcp"},
		{name: "MCP subpath", path: "/mcp/", want: "https://api.example.com/.well-known/oauth-protected-resource/mcp"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", rec.Code)
			}
			challenge := rec.Header().Get("WWW-Authenticate")
			if !strings.Contains(challenge, `resource_metadata="`+tc.want+`"`) {
				t.Fatalf("expected challenge to advertise %q, got %q", tc.want, challenge)
			}
		})
	}
}

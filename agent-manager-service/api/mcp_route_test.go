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
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/mcp"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
)

// TestMCPRoute_InvalidJWT_PreservesRequestLoggingContext verifies that an invalid JWT
// on the root-level MCP route emits a validation failure log with request logging
// context (correlation_id and path) intact.
func TestMCPRoute_InvalidJWT_PreservesRequestLoggingContext(t *testing.T) {
	var buf bytes.Buffer
	origDefault := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origDefault) })
	testLogger := slog.New(slog.NewJSONHandler(&buf, nil))
	slog.SetDefault(testLogger)

	authMiddleware := jwtassertion.JWTAuthMiddleware("Authorization", "")
	mcpMiddleware := buildMCPMiddleware(authMiddleware, nil)

	mux := http.NewServeMux()
	mcp.RegisterRoute(mux, mcp.Dependencies{}, mcpMiddleware)

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer invalid.jwt.token")
	req.Header.Set(middleware.CorrelationIDHeader, "test-mcp-corr-id")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	var found bool
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry["msg"] == "JWT validation failed" {
			found = true
			if entry["correlation_id"] != "test-mcp-corr-id" {
				t.Errorf("expected correlation_id 'test-mcp-corr-id', got %v", entry["correlation_id"])
			}
			if entry["path"] != "/mcp" {
				t.Errorf("expected path '/mcp', got %v", entry["path"])
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected 'JWT validation failed' log entry, got logs: %s", buf.String())
	}
}

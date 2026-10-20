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

package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// TestWithLoggerPathParams_EnrichesLogger confirms that WithLoggerPathParams extracts
// configured path parameters and injects them as structured attributes on the request-scoped logger.
func TestWithLoggerPathParams_EnrichesLogger(t *testing.T) {
	var buf bytes.Buffer
	baseLogger := slog.New(slog.NewJSONHandler(&buf, nil)).With("correlation_id", "test-corr-id")

	handler := WithLoggerPathParams(func(w http.ResponseWriter, r *http.Request) {
		l := logger.GetLogger(r.Context())
		l.Info("handler called")
		w.WriteHeader(http.StatusOK)
	}, utils.PathParamAgentName, utils.PathParamProjName, utils.PathParamOrgName)

	req := httptest.NewRequest(http.MethodGet, "/orgs/my-org/projects/my-proj/agents/my-agent", nil)
	req.SetPathValue(utils.PathParamAgentName, "my-agent")
	req.SetPathValue(utils.PathParamProjName, "my-proj")
	req.SetPathValue(utils.PathParamOrgName, "my-org") // routing-only param, should NOT be in logger
	req = req.WithContext(logger.WithLogger(req.Context(), baseLogger))

	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log output: %v, raw: %s", err, buf.String())
	}

	if entry["correlation_id"] != "test-corr-id" {
		t.Errorf("expected correlation_id preserved, got %v", entry["correlation_id"])
	}
	if entry["agent_name"] != "my-agent" {
		t.Errorf("expected agent_name to be 'my-agent', got %v", entry["agent_name"])
	}
	if entry["project_name"] != "my-proj" {
		t.Errorf("expected project_name to be 'my-proj', got %v", entry["project_name"])
	}
	if _, exists := entry["orgName"]; exists {
		t.Errorf("orgName should not be added to logger attributes, got %v", entry["orgName"])
	}
}

// TestRequireOrgMatch_EnrichesLoggerWithOrgID confirms that RequireOrgMatch enriches
// the context logger with the org_id (claims.OuId) from validated token claims.
func TestRequireOrgMatch_EnrichesLoggerWithOrgID(t *testing.T) {
	var buf bytes.Buffer
	baseLogger := slog.New(slog.NewJSONHandler(&buf, nil)).With("correlation_id", "test-corr-id")

	claims := &jwtassertion.TokenClaims{Sub: "user-a", OuId: "ou-uuid-456", OuHandle: "test-org"}

	next := func(w http.ResponseWriter, r *http.Request) {
		l := logger.GetLogger(r.Context())
		l.Info("downstream handler")
		w.WriteHeader(http.StatusOK)
	}

	req := httptest.NewRequest(http.MethodGet, "/orgs/test-org/agents", nil)
	ctx := logger.WithLogger(req.Context(), baseLogger)
	ctx = jwtassertion.ContextWithTokenClaims(ctx, claims)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	RequireOrgMatch(stubResolver{})(next)(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log output: %v, raw: %s", err, buf.String())
	}

	if entry["correlation_id"] != "test-corr-id" {
		t.Errorf("expected correlation_id preserved, got %v", entry["correlation_id"])
	}
	if entry["org_id"] != "ou-uuid-456" {
		t.Errorf("expected org_id to be 'ou-uuid-456', got %v", entry["org_id"])
	}
}

// TestLoggerPropagation_FullStack confirms that correlation_id, path parameters, and
// org_id are all preserved downstream in the context logger across the middleware stack.
func TestLoggerPropagation_FullStack(t *testing.T) {
	var buf bytes.Buffer
	baseLogger := slog.New(slog.NewJSONHandler(&buf, nil)).With(
		"correlation_id", "corr-abc",
		"method", "GET",
		"path", "/orgs/acme/projects/core/agents/support",
	)

	claims := &jwtassertion.TokenClaims{Sub: "user-1", OuId: "ou-999", OuHandle: "acme"}

	mux := http.NewServeMux()
	rr := NewRouteRegistrar(mux, stubResolver{}, nil)
	rr.HandleFuncWithValidation("GET /orgs/{orgName}/projects/{projName}/agents/{agentName}", func(w http.ResponseWriter, r *http.Request) {
		l := logger.GetLogger(r.Context())
		l.Info("business logic executed")
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/orgs/acme/projects/core/agents/support", nil)
	ctx := logger.WithLogger(req.Context(), baseLogger)
	ctx = jwtassertion.ContextWithTokenClaims(ctx, claims)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("failed to parse log output: %v, raw: %s", err, buf.String())
	}

	// Verify all attributes are intact in the same logger
	expected := map[string]string{
		"correlation_id": "corr-abc",
		"method":         "GET",
		"path":           "/orgs/acme/projects/core/agents/support",
		"project_name":   "core",
		"agent_name":     "support",
		"org_id":         "ou-999",
	}

	for k, v := range expected {
		if entry[k] != v {
			t.Errorf("expected %s=%q, got %v", k, v, entry[k])
		}
	}
}

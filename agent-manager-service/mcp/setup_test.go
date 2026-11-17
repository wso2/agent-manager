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

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/modelcontextprotocol/go-sdk/auth"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
)

// testAuthMiddleware mimics jwtassertion.NewMockMiddleware — it pins fixed
// claims on every request's context, standing in for our production JWT
// middleware — but also sets Sub, since claimsTokenVerifier rejects an empty
// sub (see mcp/tokeninfo.go). NewMockMiddleware itself doesn't expose a way
// to set Sub, so this test builds its own equivalent rather than changing
// that shared helper.
func testAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := &jwtassertion.TokenClaims{
			Sub:   "test-user-id",
			Scope: "test-scopes",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}
		ctx := jwtassertion.ContextWithTokenClaimsAndScope(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// bearerInjectingTransport adds a fixed Authorization header to every
// outgoing request. claimsTokenVerifier ignores the raw token value — the
// real identity comes from the claims our auth middleware already validated
// and placed on the request context — so any non-empty bearer token
// satisfies auth.RequireBearerToken's extraction step.
type bearerInjectingTransport struct{}

func (bearerInjectingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer test-raw-token")
	return http.DefaultTransport.RoundTrip(req)
}

// TestFullHTTPChainDeliversPerRequestTokenInfo exercises the exact middleware
// nesting RegisterRoute installs — authMiddleware(auth.RequireBearerToken(
// claimsTokenVerifier, nil)(handler)) — over a real HTTP round trip (not the
// in-memory transport package tools tests use), and verifies that a tool
// handler observes CallToolRequest.Extra.TokenInfo populated from the claims
// testAuthMiddleware puts on the request context.
func TestFullHTTPChainDeliversPerRequestTokenInfo(t *testing.T) {
	var gotTokenInfo *auth.TokenInfo

	server := gomcp.NewServer(&gomcp.Implementation{Name: "test-full-chain", Version: "0.0.1"}, nil)
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "probe_tool",
		Description: "records the TokenInfo observed on the incoming CallToolRequest",
	}, func(_ context.Context, req *gomcp.CallToolRequest, _ struct{}) (*gomcp.CallToolResult, any, error) {
		if req.Extra != nil {
			gotTokenInfo = req.Extra.TokenInfo
		}
		return &gomcp.CallToolResult{}, nil, nil
	})

	streamableHandler := newStreamableHandler(server)

	// Mirrors mcp/setup.go RegisterRoute: the SDK's bearer-token middleware
	// runs INSIDE our auth middleware.
	wrapped := testAuthMiddleware(auth.RequireBearerToken(claimsTokenVerifier, nil)(streamableHandler))

	httpServer := httptest.NewServer(wrapped)
	t.Cleanup(httpServer.Close)

	client := gomcp.NewClient(&gomcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	transport := &gomcp.StreamableClientTransport{
		Endpoint:   httpServer.URL,
		HTTPClient: &http.Client{Transport: bearerInjectingTransport{}},
	}
	session, err := client.Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatalf("client.Connect failed: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	result, err := session.CallTool(context.Background(), &gomcp.CallToolParams{
		Name:      "probe_tool",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error result: %+v", result.Content)
	}

	if gotTokenInfo == nil {
		t.Fatal("tool handler observed no TokenInfo on CallToolRequest.Extra")
	}
	if len(gotTokenInfo.Scopes) == 0 {
		t.Errorf("TokenInfo.Scopes = %v, want the mock middleware's \"test-scopes\"", gotTokenInfo.Scopes)
	}
	if gotTokenInfo.Expiration.IsZero() {
		t.Error("TokenInfo.Expiration is zero, want the mock middleware's future expiry")
	}
}

// TestSessionSurvivesRequestsLandingOnDifferentReplicas stands two independent
// handlers up as two replicas and alternates requests between them, the way a
// plain load balancer does. A client must be able to initialize on one and
// keep working on the other; with a stateful handler the second request would
// get 404 "session not found".
func TestSessionSurvivesRequestsLandingOnDifferentReplicas(t *testing.T) {
	replicas := []http.Handler{
		testAuthMiddleware(auth.RequireBearerToken(claimsTokenVerifier, nil)(NewHTTPServer(nil))),
		testAuthMiddleware(auth.RequireBearerToken(claimsTokenVerifier, nil)(NewHTTPServer(nil))),
	}
	var next atomic.Uint32
	loadBalancer := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		replicas[next.Add(1)%uint32(len(replicas))].ServeHTTP(w, r)
	})

	httpServer := httptest.NewServer(loadBalancer)
	t.Cleanup(httpServer.Close)

	client := gomcp.NewClient(&gomcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	transport := &gomcp.StreamableClientTransport{
		Endpoint:   httpServer.URL,
		HTTPClient: &http.Client{Transport: bearerInjectingTransport{}},
	}
	session, err := client.Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatalf("client.Connect failed: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	for i := range 4 {
		if _, err := session.ListTools(context.Background(), nil); err != nil {
			t.Fatalf("ListTools #%d failed: %v", i+1, err)
		}
	}
}

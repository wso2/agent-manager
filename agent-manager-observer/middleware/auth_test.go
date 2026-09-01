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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2/agent-manager/agent-manager-observer/config"
)

// devToken builds an unsigned-verification JWT (HS256, arbitrary key) carrying
// the given expiry. validateLocalDev (IsLocalDevEnv mode) parses claims
// without verifying the signature, so this avoids needing a JWKS server.
func devToken(t *testing.T, expiresAt time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{"sub": "test-user"}
	if !expiresAt.IsZero() {
		claims["exp"] = expiresAt.Unix()
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte("k"))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

func TestJWTAuth_WWWAuthenticateChallenge(t *testing.T) {
	const resourceMetadataURL = "https://traces.amp.example.com/.well-known/oauth-protected-resource"

	tests := []struct {
		name                string
		cfg                 config.AuthConfig
		authHeader          string
		setAuthHeader       bool
		wantStatus          int
		wantWWWAuthenticate string
	}{
		{
			name:                "missing header, no public URL configured",
			cfg:                 config.AuthConfig{IsLocalDevEnv: true},
			setAuthHeader:       false,
			wantStatus:          http.StatusUnauthorized,
			wantWWWAuthenticate: `Bearer realm="agent-manager-observer"`,
		},
		{
			name:                "missing header, public URL configured",
			cfg:                 config.AuthConfig{IsLocalDevEnv: true, ServerPublicURL: "https://traces.amp.example.com"},
			setAuthHeader:       false,
			wantStatus:          http.StatusUnauthorized,
			wantWWWAuthenticate: `Bearer realm="agent-manager-observer", resource_metadata="` + resourceMetadataURL + `"`,
		},
		{
			name:                "non-Bearer scheme carries no error code",
			cfg:                 config.AuthConfig{IsLocalDevEnv: true, ServerPublicURL: "https://traces.amp.example.com"},
			authHeader:          "Basic dXNlcjpwYXNz",
			setAuthHeader:       true,
			wantStatus:          http.StatusUnauthorized,
			wantWWWAuthenticate: `Bearer realm="agent-manager-observer", resource_metadata="` + resourceMetadataURL + `"`,
		},
		{
			name:                "invalid token carries error=invalid_token",
			cfg:                 config.AuthConfig{IsLocalDevEnv: true, ServerPublicURL: "https://traces.amp.example.com"},
			authHeader:          "Bearer not-a-valid-jwt",
			setAuthHeader:       true,
			wantStatus:          http.StatusUnauthorized,
			wantWWWAuthenticate: `Bearer realm="agent-manager-observer", error="invalid_token", resource_metadata="` + resourceMetadataURL + `"`,
		},
		{
			name:                "expired token carries error=invalid_token",
			cfg:                 config.AuthConfig{IsLocalDevEnv: true, ServerPublicURL: "https://traces.amp.example.com"},
			authHeader:          "", // filled in below with a real expired token
			setAuthHeader:       true,
			wantStatus:          http.StatusUnauthorized,
			wantWWWAuthenticate: `Bearer realm="agent-manager-observer", error="invalid_token", resource_metadata="` + resourceMetadataURL + `"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			handler := JWTAuth(tc.cfg)(passThroughHandler(&called))

			r := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
			if tc.setAuthHeader {
				authHeader := tc.authHeader
				if authHeader == "" {
					authHeader = "Bearer " + devToken(t, time.Now().Add(-time.Hour))
				}
				r.Header.Set("Authorization", authHeader)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, r)

			if rec.Code != tc.wantStatus {
				t.Errorf("expected status %d, got %d (body: %s)", tc.wantStatus, rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("WWW-Authenticate"); got != tc.wantWWWAuthenticate {
				t.Errorf("expected WWW-Authenticate %q, got %q", tc.wantWWWAuthenticate, got)
			}
			if called {
				t.Error("expected next handler not to be called")
			}
		})
	}
}

func TestJWTAuth_ValidTokenNoChallengeHeader(t *testing.T) {
	cfg := config.AuthConfig{IsLocalDevEnv: true, ServerPublicURL: "https://traces.amp.example.com"}
	called := false
	handler := JWTAuth(cfg)(passThroughHandler(&called))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	r.Header.Set("Authorization", "Bearer "+devToken(t, time.Now().Add(time.Hour)))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if !called {
		t.Error("expected next handler to be called for a valid token")
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "" {
		t.Errorf("expected no WWW-Authenticate header on success, got %q", got)
	}
}

func TestJWTAuthForMCP_AdvertisesPathSpecificMetadata(t *testing.T) {
	cfg := config.AuthConfig{IsLocalDevEnv: true, ServerPublicURL: "https://traces.amp.example.com/"}
	handler := JWTAuthForMCP(cfg)(passThroughHandler(new(bool)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", nil))

	want := `Bearer realm="agent-manager-observer", resource_metadata="https://traces.amp.example.com/.well-known/oauth-protected-resource/mcp"`
	if got := rec.Header().Get("WWW-Authenticate"); got != want {
		t.Fatalf("expected WWW-Authenticate %q, got %q", want, got)
	}
}

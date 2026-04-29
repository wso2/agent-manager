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

package jwtassertion

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildBearerChallenge(t *testing.T) {
	tests := []struct {
		name                string
		resourceMetadataURL string
		errorCode           string
		want                string
	}{
		{
			name:                "realm only",
			resourceMetadataURL: "",
			errorCode:           "",
			want:                `Bearer realm="agent-manager"`,
		},
		{
			name:                "with error code",
			resourceMetadataURL: "",
			errorCode:           "invalid_token",
			want:                `Bearer realm="agent-manager", error="invalid_token"`,
		},
		{
			name:                "with resource metadata URL",
			resourceMetadataURL: "https://am.example.com/.well-known/oauth-protected-resource",
			errorCode:           "",
			want:                `Bearer realm="agent-manager", resource_metadata="https://am.example.com/.well-known/oauth-protected-resource"`,
		},
		{
			name:                "with error and resource metadata URL",
			resourceMetadataURL: "https://am.example.com/.well-known/oauth-protected-resource",
			errorCode:           "invalid_token",
			want:                `Bearer realm="agent-manager", error="invalid_token", resource_metadata="https://am.example.com/.well-known/oauth-protected-resource"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildBearerChallenge(tt.resourceMetadataURL, tt.errorCode)
			if got != tt.want {
				t.Errorf("buildBearerChallenge() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJWTAuthMiddleware_MissingHeader_WithURL(t *testing.T) {
	metadataURL := "https://am.example.com/.well-known/oauth-protected-resource"
	handler := JWTAuthMiddleware("Authorization", metadataURL)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called when Authorization header is missing")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	want := `Bearer realm="agent-manager", resource_metadata="https://am.example.com/.well-known/oauth-protected-resource"`
	if got := rec.Header().Get("WWW-Authenticate"); got != want {
		t.Errorf("WWW-Authenticate = %q, want %q", got, want)
	}
}

func TestJWTAuthMiddleware_MissingHeader_NoURL(t *testing.T) {
	handler := JWTAuthMiddleware("Authorization", "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called when Authorization header is missing")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	want := `Bearer realm="agent-manager"`
	if got := rec.Header().Get("WWW-Authenticate"); got != want {
		t.Errorf("WWW-Authenticate = %q, want %q", got, want)
	}
}

func TestJWTAuthMiddleware_InvalidJWT_WithURL(t *testing.T) {
	metadataURL := "https://am.example.com/.well-known/oauth-protected-resource"
	handler := JWTAuthMiddleware("Authorization", metadataURL)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for invalid JWT")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer invalid.jwt.token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	want := `Bearer realm="agent-manager", error="invalid_token", resource_metadata="https://am.example.com/.well-known/oauth-protected-resource"`
	if got := rec.Header().Get("WWW-Authenticate"); got != want {
		t.Errorf("WWW-Authenticate = %q, want %q", got, want)
	}
}

func TestJWTAuthMiddleware_InvalidJWT_NoURL(t *testing.T) {
	handler := JWTAuthMiddleware("Authorization", "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for invalid JWT")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer invalid.jwt.token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	want := `Bearer realm="agent-manager", error="invalid_token"`
	if got := rec.Header().Get("WWW-Authenticate"); got != want {
		t.Errorf("WWW-Authenticate = %q, want %q", got, want)
	}
}

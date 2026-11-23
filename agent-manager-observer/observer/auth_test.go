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

package observer

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAuthProviderUsesSuppliedBasicCredentials(t *testing.T) {
	clientID := "test-client"
	clientSecret := rand.Text()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, secret, ok := r.BasicAuth(); !ok || id != clientID || secret != clientSecret {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":120}`))
	}))
	defer server.Close()

	provider := NewAuthProvider(server.URL, clientID, clientSecret)
	token, err := provider.GetToken(t.Context())
	if err != nil || token != "test-token" {
		t.Fatalf("expected a token from the server using the supplied credentials, error: %v", err)
	}
}

// Credentials with reserved characters exercise the RFC 6749 section 2.3.1
// form-encoding of the client_secret_basic header.
const (
	specialClientID     = "client:id+with %special"
	specialClientSecret = "sec:ret+with %20 space"
)

type tokenRequest struct {
	basicID, basicSecret string
	hasBasic             bool
	formID, formSecret   string
}

func recordTokenRequest(t *testing.T, r *http.Request) tokenRequest {
	t.Helper()
	var got tokenRequest
	if header := r.Header.Get("Authorization"); header != "" {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(header, "Basic "))
		if err != nil {
			t.Fatalf("decode Authorization header: %v", err)
		}
		encID, encSecret, ok := strings.Cut(string(raw), ":")
		if !ok {
			t.Fatalf("Authorization header has no separator: %q", raw)
		}
		var errID, errSecret error
		got.basicID, errID = url.QueryUnescape(encID)
		got.basicSecret, errSecret = url.QueryUnescape(encSecret)
		if errID != nil || errSecret != nil {
			t.Fatalf("unescape Basic credentials: %v %v", errID, errSecret)
		}
		got.hasBasic = true
	}
	if err := r.ParseForm(); err != nil {
		t.Fatalf("parse form: %v", err)
	}
	got.formID = r.PostForm.Get("client_id")
	got.formSecret = r.PostForm.Get("client_secret")
	return got
}

func TestAuthProviderFormEncodesBasicCredentials(t *testing.T) {
	var got tokenRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = recordTokenRequest(t, r)
		_, _ = w.Write([]byte(`{"access_token":"test-token","expires_in":120}`))
	}))
	defer server.Close()

	token, err := NewAuthProvider(server.URL, specialClientID, specialClientSecret).GetToken(t.Context())
	if err != nil || token != "test-token" {
		t.Fatalf("GetToken() = %q, %v", token, err)
	}
	if !got.hasBasic || got.basicID != specialClientID || got.basicSecret != specialClientSecret {
		t.Fatalf("Basic credentials = %q/%q (present %v)", got.basicID, got.basicSecret, got.hasBasic)
	}
	if got.formID != "" || got.formSecret != "" {
		t.Fatalf("Basic request leaked credentials into the body: %q/%q", got.formID, got.formSecret)
	}
}

func TestAuthProviderFallsBackToPostCredentials(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"400 unauthorized_client", http.StatusBadRequest, `{"error":"unauthorized_client"}`},
		{"401 invalid_client", http.StatusUnauthorized, `{"error":"invalid_client"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var requests []tokenRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				req := recordTokenRequest(t, r)
				requests = append(requests, req)
				if req.hasBasic {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
					return
				}
				_, _ = w.Write([]byte(`{"access_token":"post-token","expires_in":120}`))
			}))
			defer server.Close()

			token, err := NewAuthProvider(server.URL, specialClientID, specialClientSecret).GetToken(t.Context())
			if err != nil || token != "post-token" {
				t.Fatalf("GetToken() = %q, %v", token, err)
			}
			if len(requests) != 2 {
				t.Fatalf("token requests = %d, want 2", len(requests))
			}
			post := requests[1]
			if post.hasBasic {
				t.Fatal("POST fallback still sent an Authorization header")
			}
			if post.formID != specialClientID || post.formSecret != specialClientSecret {
				t.Fatalf("POST credentials = %q/%q", post.formID, post.formSecret)
			}
		})
	}
}

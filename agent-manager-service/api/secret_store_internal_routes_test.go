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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
)

type fakeResolver struct {
	got []string
	err error
}

func (f *fakeResolver) Resolve(_ context.Context, ouID, name, key string) (string, error) {
	f.got = []string{ouID, name, key}
	if f.err != nil {
		return "", f.err
	}
	return "the-value", nil
}

func serveSecretStore(t *testing.T, resolver secretmanagersvc.SecretResolver, token, auth string) *httptest.ResponseRecorder {
	t.Helper()
	return serveSecretStorePath(t, resolver, token, auth, "/secrets/ou-1%2Fagent-secrets/API_KEY")
}

func serveSecretStorePath(t *testing.T, resolver secretmanagersvc.SecretResolver, token, auth, path string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	registerSecretStoreRoute(mux, resolver, token)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestSecretStoreRouteReturnsValue(t *testing.T) {
	r := &fakeResolver{}
	rec := serveSecretStore(t, r, "tok", "Bearer tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["value"] != "the-value" {
		t.Fatalf("body = %q (%v)", rec.Body.String(), err)
	}
	if r.got[0] != "ou-1" || r.got[1] != "agent-secrets" || r.got[2] != "API_KEY" {
		t.Fatalf("resolved %v", r.got)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("response may be cached")
	}
}

func TestSecretStoreRouteRejectsBadToken(t *testing.T) {
	for name, tc := range map[string]struct{ token, auth string }{
		"missing header": {"tok", ""},
		"wrong token":    {"tok", "Bearer nope"},
		"not bearer":     {"tok", "tok"},
		"store disabled": {"", "Bearer "},
	} {
		t.Run(name, func(t *testing.T) {
			r := &fakeResolver{}
			if rec := serveSecretStore(t, r, tc.token, tc.auth); rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			if r.got != nil {
				t.Fatal("resolver was called without a valid token")
			}
		})
	}
}

func TestSecretStoreRouteErrors(t *testing.T) {
	if rec := serveSecretStore(t, &fakeResolver{err: secretmanagersvc.ErrSecretNotFound}, "tok", "Bearer tok"); rec.Code != http.StatusNotFound {
		t.Fatalf("not found: status = %d", rec.Code)
	}
	if rec := serveSecretStore(t, &fakeResolver{err: errors.New("boom")}, "tok", "Bearer tok"); rec.Code != http.StatusInternalServerError {
		t.Fatalf("failure: status = %d", rec.Code)
	}
}

func TestSecretStoreRouteRejectsMalformedRef(t *testing.T) {
	for _, path := range []string{"/secrets/no-separator/API_KEY", "/secrets/%2Fname/API_KEY", "/secrets/ou-1%2F/API_KEY", "/secrets/ou-1/agent-secrets/API_KEY"} {
		r := &fakeResolver{}
		if rec := serveSecretStorePath(t, r, "tok", "Bearer tok", path); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404", path, rec.Code)
		}
		if r.got != nil {
			t.Fatalf("%s: resolver was called", path)
		}
	}
}

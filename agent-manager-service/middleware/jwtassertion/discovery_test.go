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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/config"
)

// discoveryServer serves an OpenID Connect discovery document at the well-known
// path, naming issuer (the server's own URL when empty) and jwksURI, and counts
// the requests for it.
func discoveryServer(t *testing.T, issuer, jwksURI string, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != openIDConfigurationPath {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		iss := issuer
		if iss == "" {
			iss = srv.URL
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"issuer": iss, "jwks_uri": jwksURI})
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func resetDiscovery(t *testing.T) {
	t.Helper()
	reset := func() {
		discoveredJWKSMu.Lock()
		discoveredJWKSURL = map[string]string{}
		discoveredJWKSMu.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

func TestKeySetURLPrefersTheConfiguredKeySet(t *testing.T) {
	resetDiscovery(t)
	got, err := keySetURL(config.KeyManagerConfigurations{JWKSUrl: " https://idp.example.com/keys ", DiscoveryIssuer: "https://unused"})
	if err != nil || got != "https://idp.example.com/keys" {
		t.Fatalf("keySetURL() = %q, %v", got, err)
	}
	if got, err := keySetURL(config.KeyManagerConfigurations{}); err != nil || got != "" {
		t.Fatalf("keySetURL(nothing configured) = %q, %v", got, err)
	}
}

func TestKeySetURLDiscoversAndRemembersTheKeySet(t *testing.T) {
	resetDiscovery(t)
	srv, hits := discoveryServer(t, "", "https://idp.example.com/jwks", http.StatusOK)
	km := config.KeyManagerConfigurations{DiscoveryIssuer: srv.URL}
	for range 2 {
		got, err := keySetURL(km)
		if err != nil || got != "https://idp.example.com/jwks" {
			t.Fatalf("keySetURL() = %q, %v", got, err)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("discovery document fetched %d times, want once", hits.Load())
	}
}

func TestKeySetURLRefusesABadDiscoveryDocument(t *testing.T) {
	cases := []struct {
		name            string
		issuer, jwksURI string
		status          int
	}{
		{"names another issuer", "https://other.example.com", "https://idp.example.com/jwks", http.StatusOK},
		{"has no jwks_uri", "", "", http.StatusOK},
		{"has a relative jwks_uri", "", "/jwks", http.StatusOK},
		{"is not served", "", "", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetDiscovery(t)
			srv, hits := discoveryServer(t, tc.issuer, tc.jwksURI, tc.status)
			km := config.KeyManagerConfigurations{DiscoveryIssuer: srv.URL}
			if _, err := keySetURL(km); !errors.Is(err, ErrKeySetUnavailable) {
				t.Fatalf("err = %v, want %v", err, ErrKeySetUnavailable)
			}
			// A failure is not remembered: the next token tries again.
			_, _ = keySetURL(km)
			if hits.Load() != 2 {
				t.Errorf("discovery document fetched %d times, want twice", hits.Load())
			}
		})
	}
}

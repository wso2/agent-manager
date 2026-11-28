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
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2/agent-manager/agent-manager-service/config"
)

const (
	jwksTestKid      = "test-key"
	jwksTestIssuer   = "https://idp.example.com"
	jwksTestAudience = "amp-console-client"
	jwksTestOrg      = "6f1e2c3d-0000-4000-8000-000000000001"
)

// withJWKS serves key's public half as a JWKS, points the key manager settings at it with
// jwksTestIssuer and audiences, and clears the JWKS cache, for one test.
func withJWKS(t *testing.T, key *rsa.PrivateKey, audiences ...string) {
	t.Helper()
	jwks := JWKS{Keys: []JSONWebKey{{
		Kty: "RSA", Kid: jwksTestKid, Use: "sig", Alg: "RS256",
		N: base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	t.Cleanup(srv.Close)

	cfg := config.GetConfig()
	orig := cfg.KeyManagerConfigurations
	t.Cleanup(func() { cfg.KeyManagerConfigurations = orig })
	cfg.KeyManagerConfigurations = config.KeyManagerConfigurations{
		JWKSUrl: srv.URL, Issuer: []string{jwksTestIssuer}, Audience: audiences,
	}
	resetJWKSCache(t)
}

func resetJWKSCache(t *testing.T) {
	t.Helper()
	reset := func() {
		jwksCacheMutex.Lock()
		jwksCache, jwksCacheTime = nil, time.Time{}
		jwksCacheMutex.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

func signTestToken(t *testing.T, key *rsa.PrivateKey, kid string, claims TokenClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func testClaims(issuer, audience string) TokenClaims {
	return TokenClaims{
		Sub: "alice", Scope: "amp:agent:read", OuId: jwksTestOrg, OuHandle: "acme",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
}

func TestValidateJWTWithJWKS(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	withJWKS(t, key, jwksTestAudience, "https://amp.example.com/*")

	t.Run("accepts a token signed by a key in the set", func(t *testing.T) {
		claims, err := validateJWTWithJWKS(signTestToken(t, key, jwksTestKid, testClaims(jwksTestIssuer, jwksTestAudience)))
		if err != nil {
			t.Fatal(err)
		}
		if claims.OuId != jwksTestOrg || claims.OuHandle != "acme" || claims.Scope != "amp:agent:read" {
			t.Errorf("claims = %+v", claims)
		}
	})

	t.Run("accepts an audience under an allowed prefix", func(t *testing.T) {
		token := signTestToken(t, key, jwksTestKid, testClaims(jwksTestIssuer, "https://amp.example.com/mcp"))
		if _, err := validateJWTWithJWKS(token); err != nil {
			t.Fatal(err)
		}
	})

	cases := []struct {
		name  string
		token string
		want  error
	}{
		{"refuses an issuer not allowed", signTestToken(t, key, jwksTestKid, testClaims("https://other.example.com", jwksTestAudience)), ErrBadIssuer},
		{"refuses an audience not allowed", signTestToken(t, key, jwksTestKid, testClaims(jwksTestIssuer, "someone-else")), ErrBadAudience},
		{"refuses a token without an audience", signTestToken(t, key, jwksTestKid, testClaims(jwksTestIssuer, "")), ErrBadAudience},
		{"refuses a kid not in the set", signTestToken(t, key, "unknown-key", testClaims(jwksTestIssuer, jwksTestAudience)), ErrUnknownKid},
		{"refuses a kid of an invalid form", signTestToken(t, key, "bad kid!", testClaims(jwksTestIssuer, jwksTestAudience)), ErrUnknownKid},
		{"refuses a token signed by another key", signTestToken(t, other, jwksTestKid, testClaims(jwksTestIssuer, jwksTestAudience)), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateJWTWithJWKS(tc.token)
			if err == nil {
				t.Fatal("token accepted")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestValidateJWTWithJWKSRefusesAnHMACToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	withJWKS(t, key, jwksTestAudience)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, testClaims(jwksTestIssuer, jwksTestAudience))
	token.Header["kid"] = jwksTestKid
	signed, err := token.SignedString([]byte("shared-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validateJWTWithJWKS(signed); !errors.Is(err, ErrBadSignature) {
		t.Errorf("err = %v, want %v", err, ErrBadSignature)
	}
}

func TestFetchJWKSReportsAnUnavailableSet(t *testing.T) {
	resetJWKSCache(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	if _, err := fetchJWKS(srv.URL); err == nil {
		t.Error("a 503 from the key set was accepted")
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	t.Cleanup(bad.Close)
	if _, err := fetchJWKS(bad.URL); err == nil {
		t.Error("a key set that is not JSON was accepted")
	}
}

func TestConvertJWKToPublicKeyRefusesBadEncoding(t *testing.T) {
	if _, err := convertJWKToPublicKey(&JSONWebKey{N: "!!", E: "AQAB"}); err == nil {
		t.Error("a modulus that is not base64url was accepted")
	}
	if _, err := convertJWKToPublicKey(&JSONWebKey{N: "AQAB", E: "!!"}); err == nil {
		t.Error("an exponent that is not base64url was accepted")
	}
}

func TestValidateAudienceRefusesWildcardConfiguration(t *testing.T) {
	for _, allowed := range [][]string{{"*"}, {"x", "*"}, nil} {
		if err := validateAudience(jwt.ClaimStrings{"x"}, allowed); !errors.Is(err, ErrAuthNotConfigured) {
			t.Errorf("allowed %v: err = %v, want %v", allowed, err, ErrAuthNotConfigured)
		}
	}
	if err := validateIssuer("x", nil); !errors.Is(err, ErrAuthNotConfigured) {
		t.Errorf("no issuers: err = %v, want %v", err, ErrAuthNotConfigured)
	}
}

func TestJWTContextRoundTrip(t *testing.T) {
	ctx := ContextWithJWT(t.Context(), "raw-token")
	if got := GetJWTFromContext(ctx); got != "raw-token" {
		t.Errorf("GetJWTFromContext() = %q", got)
	}
	if got := GetJWTFromContext(t.Context()); got != "" {
		t.Errorf("GetJWTFromContext(empty) = %q", got)
	}
}

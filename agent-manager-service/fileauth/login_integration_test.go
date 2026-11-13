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

// A token minted here has to survive the same gates as a Thunder token, and
// they fail independently and quietly: a wrong issuer, a missing audience, an
// absent ouHandle and an unprefixed scope all surface as a bare 401 or 403 with
// nothing naming the claim at fault. This test walks a real login token through
// the real middleware so that a change to any one of them fails here, by name,
// rather than in a browser.
package fileauth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/fileauth"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

const (
	testPassword = "correct-horse-battery-staple"
	testOrgUUID  = "01900000-0000-7000-8000-000000000001"
	testOrgName  = "default"
	testIssuer   = "amp-file-auth"
	testAudience = "urn:wso2:amp"
)

// auditableCtx returns a context carrying a recorder that discards events.
//
// A successful login mints a bearer token and refuses to do so unrecorded, so
// a bare context makes it fail by design. The equivalent helper in services/
// is package-private, hence the second copy. Tests that mean to assert the
// refusal itself pass a bare context and expect audit.ErrRecorderUnavailable.
func auditableCtx(t *testing.T) context.Context {
	t.Helper()
	return audit.WithRecorder(context.Background(), audit.NewNoopRecorder())
}

// testSigningKey returns one RSA key for the whole package.
//
// jwtassertion caches the fetched JWKS in a single package-level variable that
// is NOT keyed by URL (auth.go:647), so a second test server with a different
// key would be ignored for the cache TTL and every token would fail on its
// signature. One key per test binary also matches how the service is deployed.
var testSigningKey = sync.OnceValue(func() *rsa.PrivateKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("generate test signing key: " + err.Error())
	}
	return key
})

// harness is a full login-to-protected-route setup: a user file on disk, a
// freshly generated signing key, a JWKS endpoint serving its public half, and
// the service configured to use all three.
type harness struct {
	svc     fileauth.Service
	jwksSrv *httptest.Server
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	key := testSigningKey()
	dir := t.TempDir()

	keyPath := filepath.Join(dir, "private.pem")
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	usersPath := filepath.Join(dir, "users.yaml")
	usersYAML := fmt.Sprintf(`organization:
  handle: %s
  uuid: %s
users:
  - username: admin
    passwordHash: %q
    roles: [%q]
  - username: dev
    passwordHash: %q
    roles: [%q]
`, testOrgName, testOrgUUID, string(hash), rbac.RoleAdmin, string(hash), rbac.RoleDeveloper)
	if err := os.WriteFile(usersPath, []byte(usersYAML), 0o600); err != nil {
		t.Fatalf("write users: %v", err)
	}

	// The JWKS the middleware will fetch, in the shape AMS already publishes.
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"kid": "key-1",
				"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
			}},
		})
	}))
	t.Cleanup(jwks.Close)

	// The config is a process-wide singleton the middleware reads directly, so
	// the test mutates it in place and restores it, as the other middleware
	// tests do.
	cfg := config.GetConfig()
	origAuthMode, origFileAuth := cfg.AuthMode, cfg.FileAuth
	origSigning, origKeyManager := cfg.JWTSigning, cfg.KeyManagerConfigurations
	t.Cleanup(func() {
		cfg.AuthMode, cfg.FileAuth = origAuthMode, origFileAuth
		cfg.JWTSigning, cfg.KeyManagerConfigurations = origSigning, origKeyManager
	})

	cfg.AuthMode = config.AuthModeFile
	cfg.FileAuth = config.FileAuthConfig{
		UsersPath: usersPath,
		Issuer:    testIssuer,
		Audience:  testAudience,
		TokenTTL:  time.Hour,
	}
	cfg.JWTSigning = config.JWTSigningConfig{
		PrivateKeyPath: keyPath,
		ActiveKeyID:    "key-1",
		Issuer:         "agent-manager-service",
	}
	cfg.KeyManagerConfigurations = config.KeyManagerConfigurations{
		Issuer:   []string{testIssuer},
		Audience: []string{testAudience},
		JWKSUrl:  jwks.URL,
	}

	svc, err := fileauth.NewService(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return &harness{svc: svc, jwksSrv: jwks}
}

// protectedRoute is the real middleware stack a console request passes through:
// JWT validation, then org resolution from the token, then the scope check.
func protectedRoute(perm rbac.Permission) http.Handler {
	final := func(w http.ResponseWriter, r *http.Request) {
		org := middleware.OrgHandleFromRequest(r)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "ok org=%s", org)
	}
	guarded := middleware.RequirePermission(perm)(final)
	withOrg := middleware.RequireOrgMatch(nil)(guarded)
	return jwtassertion.JWTAuthMiddleware("Authorization", "")(http.HandlerFunc(withOrg))
}

func call(t *testing.T, token string, perm rbac.Permission) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/orgs/"+testOrgName+"/projects", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	protectedRoute(perm).ServeHTTP(rec, req)
	return rec.Code, strings.TrimSpace(rec.Body.String())
}

// TestLoginTokenPassesTheRealMiddleware is the end-to-end claim: a token this
// package mints is accepted by the same chain that accepts an IdP token.
func TestLoginTokenPassesTheRealMiddleware(t *testing.T) {
	h := newHarness(t)

	token, expiresIn, err := h.svc.Login(auditableCtx(t), "admin", testPassword)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if expiresIn <= 0 {
		t.Errorf("expiresIn = %d, want positive", expiresIn)
	}

	status, body := call(t, token, rbac.ProjectRead)
	if status != http.StatusOK {
		t.Fatalf("protected route: status %d, body %s", status, body)
	}
	if want := "ok org=" + testOrgName; body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
}

// TestRolesDecideScopes proves the role expansion is load-bearing: a Developer
// token is refused a permission only an Admin holds, on the same route.
func TestRolesDecideScopes(t *testing.T) {
	h := newHarness(t)

	adminOnly := rbac.OrgManageIDP
	if containsPerm(rbac.PredefinedRolePermissions[rbac.RoleDeveloper], adminOnly) {
		t.Fatalf("test premise broken: Developer now holds %s", adminOnly.Scope())
	}

	devToken, _, err := h.svc.Login(auditableCtx(t), "dev", testPassword)
	if err != nil {
		t.Fatalf("login dev: %v", err)
	}
	if status, body := call(t, devToken, adminOnly); status != http.StatusForbidden {
		t.Errorf("developer holding %s: status %d (%s), want 403", adminOnly.Scope(), status, body)
	}
	// The same token still passes a permission the Developer role does hold,
	// so the 403 above is about the scope and not about the token.
	if status, body := call(t, devToken, rbac.ProjectRead); status != http.StatusOK {
		t.Errorf("developer holding project:read: status %d (%s), want 200", status, body)
	}

	adminToken, _, err := h.svc.Login(auditableCtx(t), "admin", testPassword)
	if err != nil {
		t.Fatalf("login admin: %v", err)
	}
	if status, body := call(t, adminToken, adminOnly); status != http.StatusOK {
		t.Errorf("admin holding %s: status %d (%s), want 200", adminOnly.Scope(), status, body)
	}
}

// TestBadCredentialsAreIndistinguishable covers the username-oracle guard: an
// unknown user and a wrong password must return the same error.
func TestBadCredentialsAreIndistinguishable(t *testing.T) {
	h := newHarness(t)

	for _, tc := range []struct{ name, user, pass string }{
		{"unknown user", "nobody", testPassword},
		{"wrong password", "admin", "not-the-password"},
		{"empty password", "admin", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token, _, err := h.svc.Login(t.Context(), tc.user, tc.pass)
			if err == nil {
				t.Fatalf("login succeeded, want failure (token %q)", token)
			}
			if !errors.Is(err, fileauth.ErrInvalidCredentials) {
				t.Errorf("error = %v, want it to wrap ErrInvalidCredentials", err)
			}
		})
	}
}

// TestScopesCarryTheResourceServerPrefix pins the single detail that made the
// console's no-auth token fail: requireScopes matches rbac.Permission.Scope(),
// so a bare permission name is no permission at all.
func TestScopesCarryTheResourceServerPrefix(t *testing.T) {
	scopes := fileauth.ScopesForRoles([]string{rbac.RoleAdmin})
	if len(scopes) == 0 {
		t.Fatal("admin expanded to no scopes")
	}
	for _, s := range scopes {
		if !strings.HasPrefix(s, rbac.ResourceServer+":") {
			t.Errorf("scope %q lacks the %q prefix", s, rbac.ResourceServer+":")
		}
	}
	// Expansion must match the catalog exactly, not merely look prefixed.
	want := len(uniquePerms(rbac.PredefinedRolePermissions[rbac.RoleAdmin]))
	if len(scopes) != want {
		t.Errorf("admin expanded to %d scopes, want %d", len(scopes), want)
	}
}

func containsPerm(perms []rbac.Permission, want rbac.Permission) bool {
	for _, p := range perms {
		if p == want {
			return true
		}
	}
	return false
}

func uniquePerms(perms []rbac.Permission) map[rbac.Permission]bool {
	set := make(map[rbac.Permission]bool, len(perms))
	for _, p := range perms {
		set[p] = true
	}
	return set
}

// TestLoginRefusesToIssueATokenUnrecorded pins the fail-closed decision. A
// token that reached a user with no record of who got it is the gap auditing
// exists to close, so losing this guard should break a test rather than go
// unnoticed.
func TestLoginRefusesToIssueATokenUnrecorded(t *testing.T) {
	h := newHarness(t)

	token, _, err := h.svc.Login(context.Background(), "admin", testPassword)
	if !errors.Is(err, audit.ErrRecorderUnavailable) {
		t.Fatalf("error = %v, want audit.ErrRecorderUnavailable", err)
	}
	if token != "" {
		t.Errorf("a token was issued despite the refusal: %q", token)
	}
}

// TestRefusedLoginDoesNotNeedARecorder is the other half: a refusal issues no
// credential, so it must not depend on the audit path. If this starts failing,
// a recorder outage has become a way to tell a wrong password apart from an
// unknown user by watching which one errors differently.
func TestRefusedLoginDoesNotNeedARecorder(t *testing.T) {
	h := newHarness(t)

	for _, tc := range []struct{ name, user, pass string }{
		{"unknown user", "nobody", testPassword},
		{"wrong password", "admin", "not-the-password"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := h.svc.Login(context.Background(), tc.user, tc.pass)
			if !errors.Is(err, fileauth.ErrInvalidCredentials) {
				t.Errorf("error = %v, want ErrInvalidCredentials", err)
			}
		})
	}
}

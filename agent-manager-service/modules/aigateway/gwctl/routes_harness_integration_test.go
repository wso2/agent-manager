//go:build integration

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

package gwctl_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// These tests send requests through the whole server: the core's router and
// auth, the module host that mounts the gateway's routes, and the gateway
// wired on the test database. Each test gets an organization of its own, so
// tests see only their own rows.

// modulePrefix is where the module host serves the gateway's public routes.
const modulePrefix = "/m/ai-gateway"

// scopesHeader carries the scopes a test's token holds; without it the token
// holds every scope.
const scopesHeader = "X-Test-Scopes"

// routeAPI is the server for one test, signed in to the test's organization.
type routeAPI struct {
	t   *testing.T
	h   http.Handler
	db  *gorm.DB
	org string // the organization's id (the token's OU id)
	// handle is the organization's handle, used in /orgs/{orgName} paths.
	handle string
}

// newRouteAPI serves the whole server for a fresh organization.
func newRouteAPI(t *testing.T) *routeAPI {
	t.Helper()
	return newRouteAPIWith(t, apitestutils.CreateMockPlatformRepository())
}

// newRouteAPIWithRealCore is newRouteAPI with the core's platform repository
// on the test database, for tests that create or change core rows through
// the core's routes, which run the gateway's hooks.
func newRouteAPIWithRealCore(t *testing.T) *routeAPI {
	t.Helper()
	return newRouteAPIWith(t, repositories.NewPlatformRepo(db.GetDB()))
}

func newRouteAPIWith(t *testing.T, platform repositories.PlatformRepository) *routeAPI {
	t.Helper()
	gdb := db.GetDB()
	org := dbtest.Org(t, gdb)
	handle := "org-" + org[len(org)-12:]
	h := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{
		PlatformRepository: platform,
		SecretMgmtClient:   apitestutils.CreateMockSecretManagementClient(),
	}, tokenFor(org))
	require.NotNil(t, aigateway.Current(), "the gateway module is active")
	return &routeAPI{t: t, h: h, db: gdb, org: org, handle: handle}
}

// tokenFor signs every request in to org. The scopes come from scopesHeader,
// or are every scope when the header is absent.
func tokenFor(org string) jwtassertion.Middleware {
	all := allScopes()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scope := all
			if s, ok := r.Header[scopesHeader]; ok {
				scope = strings.Join(s, " ")
			}
			handle := "test-org"
			if parts := strings.Split(r.URL.Path, "/"); len(parts) > 0 {
				for i, p := range parts {
					if p == "orgs" && i+1 < len(parts) {
						handle = parts[i+1]
					}
				}
			}
			claims := &jwtassertion.TokenClaims{
				Sub: "route-test-user", Scope: scope, OuId: org, OuHandle: handle,
				RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
			}
			next.ServeHTTP(w, r.WithContext(jwtassertion.ContextWithTokenClaimsAndScope(r.Context(), claims)))
		})
	}
}

func allScopes() string {
	seen := map[string]bool{}
	var scopes []string
	add := func(p rbac.Permission) {
		if s := p.Scope(); !seen[s] {
			seen[s] = true
			scopes = append(scopes, s)
		}
	}
	for _, perm := range rbac.PredefinedRolePermissions[rbac.RoleAdmin] {
		add(perm)
	}
	for _, m := range modkit.Registered() {
		mf := m.Manifest()
		for _, p := range mf.Permissions {
			add(rbac.Permission(mf.PermissionName(p.Name)))
		}
	}
	return strings.Join(scopes, " ")
}

// scope is the token scope of one of the gateway's permissions.
func scope(perm string) string { return gwperm.Scope(perm) }

// reqOpt changes a request before it is sent.
type reqOpt func(*http.Request)

// withScopes sends the request with a token that holds only scopes.
func withScopes(scopes ...string) reqOpt {
	return func(r *http.Request) {
		r.Header.Del(scopesHeader)
		if len(scopes) == 0 {
			r.Header[scopesHeader] = []string{""}
		}
		for _, s := range scopes {
			r.Header.Add(scopesHeader, s)
		}
	}
}

// withHeader sets a request header.
func withHeader(k, v string) reqOpt { return func(r *http.Request) { r.Header.Set(k, v) } }

// do sends method path (relative to the module's prefix, with "{org}"
// replaced by the organization's handle) with body encoded as JSON, unless it
// is a string or []byte, which is sent as is.
func (a *routeAPI) do(method, path string, body any, opts ...reqOpt) *httptest.ResponseRecorder {
	a.t.Helper()
	return a.raw(method, modulePrefix+strings.ReplaceAll(path, "{org}", a.handle), body, opts...)
}

// raw is do without the module prefix, for the internal routes.
func (a *routeAPI) raw(method, path string, body any, opts ...reqOpt) *httptest.ResponseRecorder {
	a.t.Helper()
	var rd *bytes.Reader
	switch b := body.(type) {
	case nil:
		rd = bytes.NewReader(nil)
	case string:
		rd = bytes.NewReader([]byte(b))
	case []byte:
		rd = bytes.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		require.NoError(a.t, err)
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, o := range opts {
		o(req)
	}
	rec := httptest.NewRecorder()
	a.h.ServeHTTP(rec, req)
	return rec
}

// expect asserts the response's status and decodes its body into a map.
func expect(t *testing.T, rec *httptest.ResponseRecorder, status int) map[string]any {
	t.Helper()
	require.Equal(t, status, rec.Code, "body: %s", rec.Body.String())
	out := map[string]any{}
	if rec.Body.Len() > 0 && strings.HasPrefix(strings.TrimSpace(rec.Body.String()), "{") {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	}
	return out
}

// expectInto asserts the response's status and decodes its body into T.
func expectInto[T any](t *testing.T, rec *httptest.ResponseRecorder, status int) T {
	t.Helper()
	require.Equal(t, status, rec.Code, "body: %s", rec.Body.String())
	var out T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out), "body: %s", rec.Body.String())
	return out
}

// errorMessage is the message of an error response.
func errorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	body := map[string]any{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), "body: %s", rec.Body.String())
	for _, k := range []string{"message", "error", "detail"} {
		if s, ok := body[k].(string); ok && s != "" {
			return s
		}
	}
	return rec.Body.String()
}

// agent creates project/agent in the test's organization and returns the agent's id.
func (a *routeAPI) agent(project, name string) uuid.UUID {
	a.t.Helper()
	return dbtest.EnsureAgent(a.t, a.db, a.org, project, name)
}

// defaultEnv is the id of the organization's default environment.
func (a *routeAPI) defaultEnv() uuid.UUID {
	a.t.Helper()
	return dbtest.Environment(a.t, a.db, a.org)
}

// unique returns prefix with a short random suffix, for resource names.
func unique(prefix string) string { return prefix + "-" + uuid.NewString()[:8] }

// TestRouteHarnessServesTheGatewayForTheTokensOrg is the harness's own check:
// the token's organization scopes the data, and a missing scope is refused.
func TestRouteHarnessServesTheGatewayForTheTokensOrg(t *testing.T) {
	a := newRouteAPI(t)

	list := expect(t, a.do(http.MethodGet, "/orgs/{org}/gateways", nil), http.StatusOK)
	require.Contains(t, list, "gateways")
	require.EqualValues(t, 0, list["total"], "a fresh organization has no gateways")

	rec := a.do(http.MethodGet, "/orgs/{org}/gateways", nil, withScopes(scope(gwperm.LLMProviderRead)))
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}

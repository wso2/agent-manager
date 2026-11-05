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

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

const (
	overrideRootOUID     = "a5000000-0000-0000-0000-adminroot001"
	overrideRootHandle   = "admin-root"
	overrideTrustedAgent = "IPAAS_SERVICE"
)

// tokenFromHeaders is a stand-in for the JWT middleware: it mints claims for
// the OU and client named by test-only headers, carrying every scope, so the
// test is about the org override rather than the scope gate.
func tokenFromHeaders(t *testing.T) jwtassertion.Middleware {
	t.Helper()
	perms := rbac.PredefinedRolePermissions[rbac.RoleAdmin]
	scopes := make([]string, 0, len(perms))
	for _, p := range perms {
		scopes = append(scopes, p.Scope())
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := &jwtassertion.TokenClaims{
				Sub:      r.Header.Get("X-Test-Sub"),
				ClientID: r.Header.Get("X-Test-Client"),
				OuId:     r.Header.Get("X-Test-OUID"),
				OuHandle: r.Header.Get("X-Test-OUHandle"),
				Scope:    strings.Join(scopes, " "),
				RegisteredClaims: jwt.RegisteredClaims{
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
				},
			}
			ctx := jwtassertion.ContextWithTokenClaimsAndScope(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func TestProjectMCPProxy_OrgOverride(t *testing.T) {
	orgB := uuid.NewString()
	orgBHandle := "org-b-" + orgB[:8]
	userOUID := uuid.NewString()

	// The cloud resolver's shape: only the allow-listed client from the root OU
	// may impersonate, and only the org the header names.
	middleware.SetOrgOverrideResolver(func(r *http.Request) (string, bool) {
		claims := jwtassertion.GetTokenClaims(r.Context())
		if claims == nil || claims.OuId != overrideRootOUID || claims.ClientID != overrideTrustedAgent {
			return "", false
		}
		return middleware.ImpersonatedOrgHeader(r)
	})
	t.Cleanup(func() { middleware.SetOrgOverrideResolver(nil) })

	var lookups []string
	lookup := func(_ context.Context, ouID string) (string, error) {
		lookups = append(lookups, ouID)
		if ouID == orgB {
			return orgBHandle, nil
		}
		return "", fmt.Errorf("lookup of %s: %w", ouID, middleware.ErrOverrideOrgNotFound)
	}

	oc := apitestutils.CreateMockOpenChoreoClient()
	projectUUID := uuid.NewString()
	baseGetProject := oc.GetProjectFunc
	oc.GetProjectFunc = func(ctx context.Context, ns, name string) (*models.ProjectResponse, error) {
		p, err := baseGetProject(ctx, ns, name)
		if err != nil {
			return nil, err
		}
		p.UUID = projectUUID
		return p, nil
	}
	app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{
		OpenChoreoClient: oc,
		OUHandleLookup:   lookup,
	}, tokenFromHeaders(t))
	gdb := db.DB(context.Background())

	suffix := uuid.NewString()[:8]
	handle := "override-mcp-" + suffix
	t.Cleanup(func() {
		gdb.Exec(`DELETE FROM artifacts WHERE handle LIKE ?`, "%"+suffix)
	})

	type caller struct{ sub, client, ouID, ouHandle string }
	ip := caller{sub: overrideTrustedAgent, client: overrideTrustedAgent, ouID: overrideRootOUID, ouHandle: overrideRootHandle}
	user := caller{sub: "user-1", client: "AGENT_MANAGER_CONSOLE", ouID: userOUID, ouHandle: "user-org"}

	do := func(c caller, method, path string, body any, impersonate string) *httptest.ResponseRecorder {
		t.Helper()
		var reader *bytes.Reader
		if body != nil {
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			reader = bytes.NewReader(raw)
		} else {
			reader = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, "/api/v1"+path, reader)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-Sub", c.sub)
		req.Header.Set("X-Test-Client", c.client)
		req.Header.Set("X-Test-OUID", c.ouID)
		req.Header.Set("X-Test-OUHandle", c.ouHandle)
		if impersonate != "" {
			req.Header.Set(occlient.HeaderImpersonateOrg, impersonate)
		}
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		return rr
	}
	countProxy := func(ouID string) int64 {
		t.Helper()
		var n int64
		require.NoError(t, gdb.Raw(`SELECT count(*) FROM artifacts WHERE handle = ? AND ou_id = ?`, handle, ouID).Scan(&n).Error)
		return n
	}

	body := map[string]any{
		"name":    "Override MCP " + suffix,
		"context": "/override-" + suffix,
		"source":  map[string]string{"componentName": "orders-mcp", "endpointName": "mcp"},
	}

	t.Run("root-OU client impersonating org B stores the proxy under org B", func(t *testing.T) {
		rr := do(ip, http.MethodPut, "/orgs/"+orgBHandle+"/projects/p1/mcp-proxies/"+handle, body, orgB)
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		assert.Equal(t, int64(1), countProxy(orgB))
		assert.Zero(t, countProxy(overrideRootOUID), "nothing is written under the caller's own org")
		assert.Contains(t, lookups, orgB)

		rr = do(ip, http.MethodGet, "/orgs/"+orgBHandle+"/projects/p1/mcp-proxies/"+handle, nil, orgB)
		assert.Equal(t, http.StatusOK, rr.Code, "reads act on org B too")
		rr = do(ip, http.MethodGet, "/orgs/x/projects/p1/mcp-proxies/"+handle, nil, "")
		assert.Equal(t, http.StatusNotFound, rr.Code, "without the header the caller is back on its own org")
	})

	t.Run("an impersonating path must name the impersonated org", func(t *testing.T) {
		for _, pathOrg := range []string{"x", overrideRootHandle} {
			rr := do(ip, http.MethodGet, "/orgs/"+pathOrg+"/projects/p1/mcp-proxies/"+handle, nil, orgB)
			assert.Equal(t, http.StatusForbidden, rr.Code, pathOrg)
			assert.Contains(t, rr.Body.String(), "not the impersonated organization", pathOrg)
		}
	})

	t.Run("an org MCP route is not impersonated", func(t *testing.T) {
		orgBProxy := "org-b-proxy-" + suffix
		rootProxy := "root-proxy-" + suffix
		repo := repositories.NewMCPProxyRepo(gdb)
		require.NoError(t, repo.Create(context.Background(), gdb,
			&models.MCPProxy{Configuration: models.MCPProxyConfig{Name: "B " + suffix, Version: "v1"}},
			orgBProxy, "B "+suffix, "v1", orgB, nil, nil))
		require.NoError(t, repo.Create(context.Background(), gdb,
			&models.MCPProxy{Configuration: models.MCPProxyConfig{Name: "Root " + suffix, Version: "v1"}},
			rootProxy, "Root "+suffix, "v1", overrideRootOUID, nil, nil))

		rr := do(ip, http.MethodGet, "/orgs/x/mcp-proxies?limit=100", nil, orgB)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		var resp models.MCPProxyListResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		ids := map[string]bool{}
		for _, it := range resp.List {
			ids[*it.ID] = true
		}
		assert.True(t, ids[rootProxy], "the org route lists the token's org")
		assert.False(t, ids[orgBProxy], "the header does not switch an org route to org B")
	})

	t.Run("a user token sending the header is refused", func(t *testing.T) {
		other := "user-attempt-" + suffix
		rr := do(user, http.MethodPut, "/orgs/"+orgBHandle+"/projects/p1/mcp-proxies/"+other, body, orgB)
		assert.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
		assert.Contains(t, rr.Body.String(), "org impersonation not permitted")
		var n int64
		require.NoError(t, gdb.Raw(`SELECT count(*) FROM artifacts WHERE handle = ?`, other).Scan(&n).Error)
		assert.Zero(t, n)
	})

	t.Run("an unknown target org is refused", func(t *testing.T) {
		rr := do(ip, http.MethodGet, "/orgs/x/projects/p1/mcp-proxies", nil, uuid.NewString())
		assert.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
	})
}

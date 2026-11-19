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
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

const shadowScanTokenClient = "amp-api-client-test"

// claimsMiddleware puts the claims *current points at on every request, as
// the JWT middleware does for a verified token.
func claimsMiddleware(current **jwtassertion.TokenClaims) jwtassertion.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := jwtassertion.ContextWithTokenClaimsAndScope(r.Context(), *current)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func adminScopes() string {
	perms := rbac.PredefinedRolePermissions[rbac.RoleAdmin]
	scopes := make([]string, 0, len(perms))
	for _, p := range perms {
		scopes = append(scopes, p.Scope())
	}
	return strings.Join(scopes, " ")
}

// TestShadowScanPublishThroughTheRouteStack sends a publish through the real
// route stack (the permission check, then the scan token check): a scan token
// (this service's client, the publish scope alone, an ouId and no ouHandle)
// reaches the handler and finishes the scan; an Admin's user token, whose
// role holds the publish permission, and tokens with other scopes do not.
func TestShadowScanPublishThroughTheRouteStack(t *testing.T) {
	ctx := context.Background()
	cfg := config.GetConfig()
	origClient := cfg.IDP.ClientID
	origEnabled := cfg.ShadowRunner.Enabled
	cfg.IDP.ClientID = shadowScanTokenClient
	cfg.ShadowRunner.Enabled = true // source code scans are off by default
	t.Cleanup(func() {
		cfg.IDP.ClientID = origClient
		cfg.ShadowRunner.Enabled = origEnabled
	})

	ouID := "ou-" + uuid.NewString()
	repo := repositories.NewShadowAgentRepository(db.GetDB())
	conn := &models.ShadowConnection{
		ID: uuid.New(), OUID: ouID, Name: "repos", Provider: models.ShadowProviderGitHub,
		Config: models.ShadowConnectionConfig{
			Endpoints: map[string]string{"api": "https://api.github.com"}, Organization: "acme", AllRepositories: true,
		},
		SecretHints: map[string]string{},
	}
	require.NoError(t, repo.CreateConnection(ctx, conn))
	t.Cleanup(func() { _, _ = repo.DeleteConnection(context.Background(), ouID, conn.ID) })
	scan := &models.ShadowScan{
		ID: uuid.New(), OUID: ouID, Status: models.ShadowScanPending, ConnectionIDs: []string{conn.ID.String()},
	}
	require.NoError(t, repo.CreateScan(ctx, scan))
	t.Cleanup(func() { db.GetDB().Exec("DELETE FROM shadow_scans WHERE id = ?", scan.ID) })

	var current *jwtassertion.TokenClaims
	app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: apitestutils.CreateMockOpenChoreoClient()},
		claimsMiddleware(&current))
	exp := jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}
	body := `{"complete":true,"scannedRepositories":["acme/agent"],"failedRepositories":[],
		"agents":[{"externalId":"acme/agent:agent.py","name":"agent","repository":"agent",
		"attributes":{"reviewedBy":"rules"},
		"evidence":[{"code":"github.d2_agent_import","source":"agent.py","observed":{}}]}]}`
	publish := func(claims *jwtassertion.TokenClaims) (int, string) {
		current = claims
		req := httptest.NewRequest(http.MethodPost, "/api/v1/shadow-agent-publisher/scans/"+scan.ID.String()+"/results",
			strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		return rr.Code, rr.Body.String()
	}

	refused := map[string]*jwtassertion.TokenClaims{
		"an Admin's user token": {
			Sub: "admin-user", OuId: ouID, OuHandle: "acme-org", ClientID: "amp-console", Scope: adminScopes(),
			RegisteredClaims: exp,
		},
		"a user token with only the publish scope": {
			Sub: "admin-user", OuId: ouID, OuHandle: "acme-org", ClientID: "amp-console",
			Scope: rbac.ShadowAgentPublish.Scope(), RegisteredClaims: exp,
		},
		"the scan client with another scope too": {
			OuId: ouID, ClientID: shadowScanTokenClient,
			Scope: rbac.ShadowAgentPublish.Scope() + " " + rbac.ShadowAgentManage.Scope(), RegisteredClaims: exp,
		},
		"a build token": {OuId: ouID, ClientID: shadowScanTokenClient, Scope: rbac.AgentBuild.Scope(), RegisteredClaims: exp},
	}
	for name, claims := range refused {
		code, resp := publish(claims)
		assert.Equal(t, http.StatusForbidden, code, "%s: %s", name, resp)
	}
	got, err := repo.GetScan(ctx, ouID, scan.ID)
	require.NoError(t, err)
	require.Equal(t, models.ShadowScanPending, got.Status, "a refused token changes nothing")

	code, resp := publish(&jwtassertion.TokenClaims{
		OuId: ouID, ClientID: shadowScanTokenClient, Scope: rbac.ShadowAgentPublish.Scope(), RegisteredClaims: exp,
	})
	require.Equal(t, http.StatusOK, code, resp)
	assert.Contains(t, resp, `"status":"succeeded"`)
	assert.Contains(t, resp, `"findingsCount":1`)

	got, err = repo.GetScan(ctx, ouID, scan.ID)
	require.NoError(t, err)
	assert.Equal(t, models.ShadowScanSucceeded, got.Status)
	require.Len(t, got.Results, 1)
	assert.Equal(t, []string{"agent"}, got.Results[0].ScannedRepositories, "acme/agent and agent are one repository")
	rows, _, err := repo.ListFindings(ctx, ouID, repositories.ShadowFindingFilter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "agent", rows[0].Scope)

	// The same token cannot publish twice.
	code, resp = publish(current)
	assert.Equal(t, http.StatusConflict, code, resp)
	assert.Contains(t, resp, "already published")
}

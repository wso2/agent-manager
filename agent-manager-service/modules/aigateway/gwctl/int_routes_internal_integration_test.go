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
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/api"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// intAPI is routeAPI plus the internal server, where gateways call in with
// their api-key, and one registered gateway holding a known token.
type intAPI struct {
	*routeAPI
	internal http.Handler
	gateway  *gwmodels.Gateway
	token    string
}

// newINTAPI serves the public and internal servers for a fresh organization
// that has one gateway, mapped to the default environment, with a token.
func newINTAPI(t *testing.T) *intAPI {
	t.Helper()
	gdb := db.GetDB()
	org := dbtest.Org(t, gdb)
	var handle string
	require.NoError(t, gdb.Raw(`SELECT handle FROM organizations WHERE id = ?`, org).Scan(&handle).Error)
	clients := wiring.TestClients{
		PlatformRepository: apitestutils.CreateMockPlatformRepository(),
		SecretMgmtClient:   apitestutils.CreateMockSecretManagementClient(),
	}
	h, host := apitestutils.MakeAppClientWithHost(t, clients, tokenFor(org))
	params, err := wiring.InitializeTestAppParamsWithClientMocks(config.GetConfig(), gdb, tokenFor(org), clients)
	require.NoError(t, err)
	a := &intAPI{
		routeAPI: &routeAPI{t: t, h: h, db: gdb, org: org, handle: handle},
		internal: api.MakeInternalHTTPHandlerWithModules(params, host),
		gateway:  gwtest.Gateway(t, gdb, org, gwmodels.GatewayRoleBoth),
	}
	a.token, _ = a.intNewToken(a.gateway.UUID)
	return a
}

// intNewToken issues a token for gateway through the public route, and returns it and its id.
func (a *intAPI) intNewToken(gateway uuid.UUID) (string, string) {
	a.t.Helper()
	body := expect(a.t, a.do(http.MethodPost, "/orgs/{org}/gateways/"+gateway.String()+"/tokens", nil), http.StatusOK)
	token, _ := body["token"].(string)
	id, _ := body["tokenId"].(string)
	require.NotEmpty(a.t, token, "token response: %v", body)
	return token, id
}

// call sends method path (below /api/internal/v1) to the internal server with
// the gateway's api-key; opts run after, so they can replace or drop it.
func (a *intAPI) call(method, path string, body any, opts ...reqOpt) *httptest.ResponseRecorder {
	a.t.Helper()
	var raw []byte
	switch b := body.(type) {
	case nil:
	case string:
		raw = []byte(b)
	default:
		var err error
		raw, err = json.Marshal(b)
		require.NoError(a.t, err)
	}
	req := httptest.NewRequestWithContext(context.Background(), method, "/api/internal/v1"+path, bytes.NewReader(raw))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("api-key", a.token)
	for _, o := range opts {
		o(req)
	}
	rec := httptest.NewRecorder()
	a.internal.ServeHTTP(rec, req)
	return rec
}

// intNoKey drops the api-key header.
func intNoKey(r *http.Request) { r.Header.Del("api-key") }

// intKey sends key as the api-key.
func intKey(key string) reqOpt { return withHeader("api-key", key) }

// intArtifact writes an artifact of kind in the test's organization and returns its id.
func (a *intAPI) intArtifact(kind, handle string) uuid.UUID {
	a.t.Helper()
	return a.intArtifactIn(a.org, kind, handle)
}

// intArtifactIn writes an artifact of kind in org and returns its id.
func (a *intAPI) intArtifactIn(org, kind, handle string) uuid.UUID {
	a.t.Helper()
	id := uuid.Must(uuid.NewV7())
	require.NoError(a.t, gwrepo.NewArtifactRepo(a.db, gwtest.NewSecrets()).Create(a.db, &gwmodels.Artifact{
		UUID: id, Handle: handle, Name: handle, Version: "v1.0", Kind: kind, OUID: org,
	}))
	return id
}

// intDeploy starts deploying content of the artifact to gateway and returns the deployment's id.
func (a *intAPI) intDeploy(gateway, artifact uuid.UUID, kind, handle, content string) uuid.UUID {
	a.t.Helper()
	placement := gwmodels.Placement{
		ArtifactRef: gwmodels.ArtifactRef{OUID: a.org, ArtifactUUID: artifact, Kind: kind, Handle: handle},
		GatewayUUID: gateway,
	}
	deployment := &gwmodels.Deployment{Name: unique("d"), Content: []byte(content)}
	require.NoError(a.t, a.db.Transaction(func(tx *gorm.DB) error {
		return gwrepo.NewDeploymentRepo(a.db).StartDeploy(context.Background(), tx, placement, deployment)
	}))
	return deployment.DeploymentID
}

// intZipEntry asserts rec is a ZIP of one file, named name, and returns its content.
func intZipEntry(t *testing.T, rec *httptest.ResponseRecorder, name string) string {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.Equal(t, "application/zip", rec.Header().Get("Content-Type"))
	body := rec.Body.Bytes()
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	require.NoError(t, err)
	require.Len(t, archive.File, 1)
	require.Equal(t, name, archive.File[0].Name)
	f, err := archive.File[0].Open()
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()
	content, err := io.ReadAll(f)
	require.NoError(t, err)
	return string(content)
}

// intRoute is one internal route, called with a body it accepts.
type intRoute struct {
	method, path string
	body         any
}

// intAllRoutes are the internal routes a gateway calls with its api-key.
func intAllRoutes(gateway uuid.UUID) []intRoute {
	id := uuid.NewString()
	return []intRoute{
		{http.MethodGet, "/llm-providers/api-keys", nil},
		{http.MethodGet, "/llm-proxies/api-keys", nil},
		{http.MethodGet, "/apis/api-keys", nil},
		{http.MethodGet, "/apis/" + id, nil},
		{http.MethodGet, "/subscription-plans", nil},
		{http.MethodGet, "/deployments", nil},
		{http.MethodPost, "/deployments/fetch-batch", map[string]any{"deploymentIds": []string{id}}},
		{http.MethodPost, "/artifacts/exists", map[string]any{"artifactIds": []string{id}}},
		{http.MethodGet, "/applications", nil},
		{http.MethodPost, "/gateways/" + gateway.String() + "/manifest", map[string]any{}},
		{http.MethodGet, "/llm-providers/" + id, nil},
		{http.MethodGet, "/llm-proxies/" + id, nil},
		{http.MethodGet, "/mcp-proxies/" + id, nil},
		{http.MethodGet, "/agents/" + id, nil},
	}
}

// TestINTRoutesRefuseCallsWithoutAValidKey pins that every internal route
// answers 401 to a missing, unknown or revoked api-key.
func TestINTRoutesRefuseCallsWithoutAValidKey(t *testing.T) {
	a := newINTAPI(t)
	revoked, revokedID := a.intNewToken(a.gateway.UUID)
	expect(t, a.do(http.MethodDelete, "/orgs/{org}/gateways/"+a.gateway.UUID.String()+"/tokens/"+revokedID, nil), http.StatusNoContent)

	for _, r := range intAllRoutes(a.gateway.UUID) {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			rec := a.call(r.method, r.path, r.body, intNoKey)
			require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), "API key is required")

			for _, key := range []string{"not-a-token", uuid.NewString() + "-forged", revoked} {
				rec = a.call(r.method, r.path, r.body, intKey(key))
				require.Equal(t, http.StatusUnauthorized, rec.Code, "key %q: %s", key, rec.Body.String())
				require.Contains(t, rec.Body.String(), "Invalid or expired API key")
			}
		})
	}
}

// TestINTRoutesSubscriptionPlansAreEmpty pins that a gateway is served an empty plan list.
func TestINTRoutesSubscriptionPlansAreEmpty(t *testing.T) {
	a := newINTAPI(t)

	plans := expectInto[[]map[string]any](t, a.call(http.MethodGet, "/subscription-plans", nil), http.StatusOK)
	require.Empty(t, plans)
	require.NotNil(t, plans, "an empty JSON array, not null")
}

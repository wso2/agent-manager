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
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	client "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// TestIPFlow_EndToEnd walks the whole Integration Platform flow over HTTP, as IP runs it in
// production: a root-OU M2M token naming the tenant org in X-Impersonate-Org creates a project MCP
// proxy and binds it to an environment (waiting for the component to be deployed), the gateway's
// ack makes it Deployed, an AM agent of the same project binds it as a tool (as the tenant's own
// user), and the proxy can then only be deleted once the agent has let go of it.
func TestIPFlow_EndToEnd(t *testing.T) {
	suffix := uuid.NewString()[:8]
	tenant := uuid.NewString()
	tenantHandle := "tenant-" + suffix
	project := "ip-" + suffix
	projectUUID := uuid.NewString()
	devEnv := "dev-" + suffix
	envUUID := uuid.MustParse(apitestutils.DefaultEnvironmentUUID)
	handle := "orders-" + suffix
	component := "orders-mcp-" + suffix

	// The cloud resolver's shape: only the allow-listed client of the root OU may impersonate.
	middleware.SetOrgOverrideResolver(func(r *http.Request) (string, bool) {
		claims := jwtassertion.GetTokenClaims(r.Context())
		if claims == nil || claims.OuId != overrideRootOUID || claims.ClientID != overrideTrustedAgent {
			return "", false
		}
		return middleware.ImpersonatedOrgHeader(r)
	})
	t.Cleanup(func() { middleware.SetOrgOverrideResolver(nil) })
	lookup := func(_ context.Context, ouID string) (string, error) {
		if ouID == tenant {
			return tenantHandle, nil
		}
		return "", fmt.Errorf("lookup of %s: %w", ouID, middleware.ErrOverrideOrgNotFound)
	}

	// The upstream is unknown until the component is deployed ("" means the release binding has not
	// reconciled yet), which is when IP's ensure call has to retry.
	var mu sync.Mutex
	serviceURL := ""
	setServiceURL := func(u string) {
		mu.Lock()
		defer mu.Unlock()
		serviceURL = u
	}
	oc := apitestutils.CreateMockOpenChoreoClient()
	baseProject := oc.GetProjectFunc
	oc.GetProjectFunc = func(ctx context.Context, ns, name string) (*models.ProjectResponse, error) {
		p, err := baseProject(ctx, ns, name)
		if err != nil {
			return nil, err
		}
		p.Name, p.UUID = name, projectUUID
		return p, nil
	}
	oc.GetEnvironmentFunc = func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
		return &models.EnvironmentResponse{UUID: envUUID.String(), Name: name}, nil
	}
	oc.ListEnvironmentsFunc = func(context.Context, string) ([]*models.EnvironmentResponse, error) {
		return []*models.EnvironmentResponse{{UUID: envUUID.String(), Name: devEnv}}, nil
	}
	oc.GetReleaseBindingEndpointServiceURLFunc = func(context.Context, string, string, string, string, string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		return serviceURL, nil
	}
	oc.UpdateReleaseBindingEnvVarsFunc = func(context.Context, string, string, string, string, []client.EnvVar) error { return nil }
	oc.RemoveComponentEnvironmentVariablesFunc = func(context.Context, string, string, string, []string) error { return nil }
	oc.RemoveReleaseBindingEnvVarsFunc = func(context.Context, string, string, string, string, []string) error { return nil }
	app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{
		OpenChoreoClient: oc,
		SecretMgmtClient: apitestutils.CreateMockSecretManagementClient(),
		OUHandleLookup:   lookup,
	}, tokenFromHeaders(t))

	gdb := db.DB(context.Background())
	deploymentRepo := repositories.NewDeploymentRepo(gdb)
	gatewayID := uuid.New()
	t.Cleanup(func() {
		gdb.Exec(`DELETE FROM agent_configurations WHERE ou_id = ?`, tenant)
		gdb.Exec(`DELETE FROM mcp_proxy_mappings WHERE source_mcp_proxy_uuid IN (SELECT uuid FROM artifacts WHERE ou_id = ? AND handle LIKE ?)`, tenant, "%"+suffix+"%")
		gdb.Exec(`DELETE FROM artifacts WHERE ou_id = ? AND handle LIKE ?`, tenant, "%"+suffix+"%")
		gdb.Exec(`DELETE FROM gateway_environment_mappings WHERE gateway_uuid = ?`, gatewayID)
		gdb.Exec(`DELETE FROM gateways WHERE uuid = ?`, gatewayID)
	})
	// The environment's gateway exists before the binding, so the binding deploys straight away.
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, TRUE)`,
		gatewayID, "ip-gw-"+suffix, "ip-gw-"+suffix, "dev-gw.example.com", models.GatewayRoleBoth, tenant).Error)
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateway_environment_mappings (gateway_uuid, environment_uuid) VALUES (?, ?)`, gatewayID, envUUID).Error)

	type caller struct{ sub, client, ouID, ouHandle string }
	ip := caller{sub: overrideTrustedAgent, client: overrideTrustedAgent, ouID: overrideRootOUID, ouHandle: overrideRootHandle}
	user := caller{sub: "tenant-user", client: "AGENT_MANAGER_CONSOLE", ouID: tenant, ouHandle: tenantHandle}
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
			req.Header.Set(client.HeaderImpersonateOrg, impersonate)
		}
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		return rr
	}
	ipDo := func(method, suffixPath string, body any) *httptest.ResponseRecorder {
		t.Helper()
		return do(ip, method, fmt.Sprintf("/orgs/x/projects/%s/mcp-proxies%s", project, suffixPath), body, tenant)
	}
	deployments := func(artifact uuid.UUID) int64 {
		t.Helper()
		var n int64
		require.NoError(t, gdb.Raw(`SELECT count(*) FROM deployments WHERE artifact_uuid = ?`, artifact).Scan(&n).Error)
		return n
	}

	proxyBody := map[string]any{
		"name": "Orders MCP " + suffix, "context": "/orders",
		"source": map[string]string{"componentName": component, "endpointName": "mcp"},
	}
	envPath := "/" + handle + "/environments/" + devEnv
	bindBody := map[string]any{"security": map[string]any{"apiKey": map[string]any{"enabled": true, "key": "X-API-Key", "in": "header"}}}

	// 1. IP ensures the proxy.
	rr := ipDo(http.MethodPut, "/"+handle, proxyBody)
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var n int64
	require.NoError(t, gdb.Raw(`SELECT count(*) FROM artifacts WHERE handle = ? AND ou_id = ?`, handle, tenant).Scan(&n).Error)
	require.Equal(t, int64(1), n, "the proxy lands in the tenant org named by the header")
	rr = ipDo(http.MethodPut, "/"+handle, proxyBody)
	require.Equal(t, http.StatusOK, rr.Code, "an identical re-PUT of the proxy is a no-op: "+rr.Body.String())

	// 2. The component is not deployed yet: the binding is refused and nothing is stored.
	rr = ipDo(http.MethodPut, envPath, bindBody)
	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
	require.Equal(t, "UPSTREAM_NOT_READY", errorCode(t, rr))
	rr = ipDo(http.MethodGet, envPath, nil)
	require.Equal(t, http.StatusNotFound, rr.Code, "a refused binding stores nothing")

	// 3. The component's release binding reconciles; the binding is created and starts deploying.
	setServiceURL("http://orders.dp-ns.svc.cluster.local:8080")
	rr = ipDo(http.MethodPut, envPath, bindBody)
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String()) // a new binding is 201
	binding := decodeBinding(t, rr)
	assert.Equal(t, "Deploying", binding.DeploymentStatus)
	assert.Equal(t, gatewayID.String(), binding.GatewayID)
	assert.Equal(t, "/orders", binding.Context, "the context reads back as sent")
	assert.Equal(t, "https://dev-gw.example.com/"+project+"/orders/mcp", binding.PublicURL)

	var raw string
	require.NoError(t, gdb.Raw(`SELECT b.artifact_uuid::text FROM mcp_proxy_endpoint_environments b
		JOIN artifacts a ON a.uuid = b.mcp_proxy_uuid WHERE a.handle = ? AND a.ou_id = ? AND b.environment_uuid = ?`,
		handle, tenant, envUUID).Row().Scan(&raw))
	artifact, err := uuid.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, int64(1), deployments(artifact))

	// 4. The gateway acknowledges the deployment; IP polls the binding and sees Deployed.
	row, err := deploymentRepo.GetStatusRecord(context.Background(), models.Placement{
		ArtifactRef: models.ArtifactRef{OUID: tenant, ArtifactUUID: artifact, Kind: models.KindMCPMapping}, GatewayUUID: gatewayID,
	})
	require.NoError(t, err)
	require.NotNil(t, row)
	require.NotNil(t, row.DeploymentID)
	applied, err := deploymentRepo.ApplyAck(context.Background(), gatewayID.String(), models.DeploymentAck{
		DeploymentID: row.DeploymentID.String(), ArtifactID: artifact.String(), ResourceType: "mcpproxy",
		Action: "deploy", Status: "success", PerformedAt: row.PerformedAt,
	}, func(context.Context, *gorm.DB, models.Placement, uuid.UUID) error { return nil })
	require.NoError(t, err)
	require.True(t, applied)
	rr = ipDo(http.MethodGet, envPath, nil)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Equal(t, "Deployed", decodeBinding(t, rr).DeploymentStatus)

	// An identical re-PUT of the binding on a Deployed one is a no-op: 200, no new deployment.
	rr = ipDo(http.MethodPut, envPath, bindBody)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Equal(t, "Deployed", decodeBinding(t, rr).DeploymentStatus)
	assert.Equal(t, int64(1), deployments(artifact), "an identical PUT makes no new deployment")

	// 5. An agent of the same project, run by a tenant user (no impersonation), binds the proxy as a tool.
	// The URL injected into the agent carries the project prefix the gateway serves the proxy at.
	wantURL := "https://dev-gw.example.com/" + project + "/orders/mcp"
	mcpConfigs := fmt.Sprintf("/orgs/%s/projects/%s/agents/agent-one/mcp-configs", tenantHandle, project)
	rr = do(user, http.MethodPost, mcpConfigs, map[string]any{
		"name": "tools", "type": "mcp",
		"envMappings": map[string]any{devEnv: map[string]any{"providerName": handle}},
	}, "")
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	var cfg models.AgentModelConfigResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &cfg))
	var injected []string
	for _, call := range oc.UpdateReleaseBindingEnvVarsCalls() {
		for _, ev := range call.EnvVars {
			injected = append(injected, ev.Value)
		}
	}
	require.Contains(t, injected, wantURL, "the agent's MCP URL is <gateway>/<project><context>/mcp")

	// 6. IP cannot delete the proxy while the agent uses it.
	rr = ipDo(http.MethodDelete, "/"+handle, nil)
	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
	require.Equal(t, "MCP_PROXY_IN_USE", errorCode(t, rr))
	rr = ipDo(http.MethodGet, "/"+handle, nil)
	require.Equal(t, http.StatusOK, rr.Code, "nothing was deleted")

	// Nor unbind it from the environment the agent is mapped in: the agent's keys were sent against this
	// binding's artifact, and a re-bind would mint a new one they are never re-sent to.
	t.Run("unbinding an environment an agent is mapped in is 409 MCP_PROXY_IN_USE", func(t *testing.T) {
		rr := ipDo(http.MethodDelete, envPath, nil)
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		require.Equal(t, "MCP_PROXY_IN_USE", errorCode(t, rr))
		rr = ipDo(http.MethodGet, envPath, nil)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.Equal(t, "Deployed", decodeBinding(t, rr).DeploymentStatus, "the binding is untouched")
		assert.Equal(t, int64(1), deployments(artifact))
	})

	// 7. The agent lets go of it; the binding can then be dropped, and the proxy deleted.
	rr = do(user, http.MethodDelete, mcpConfigs+"/"+cfg.UUID, nil, "")
	require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
	t.Run("once the agent is unmapped the environment unbinds", func(t *testing.T) {
		rr := ipDo(http.MethodDelete, envPath, nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
		rr = ipDo(http.MethodGet, envPath, nil)
		require.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
	})
	rr = ipDo(http.MethodDelete, "/"+handle, nil)
	require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
	rr = ipDo(http.MethodGet, "/"+handle, nil)
	require.Equal(t, http.StatusNotFound, rr.Code)
	require.NoError(t, gdb.Raw(`SELECT count(*) FROM artifacts WHERE ou_id = ? AND handle LIKE ?`, tenant, "%"+suffix+"%").Scan(&n).Error)
	assert.Zero(t, n, "the proxy and its binding's gateway artifact are gone")
}

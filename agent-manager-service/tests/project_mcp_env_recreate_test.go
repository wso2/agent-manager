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
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// TestProjectMCPBinding_EnvironmentRecreatedWithSameName deletes an environment a project proxy is bound in,
// re-creates one with the same name (a new UUID), and binds the proxy to it again. The environment-removal sweep
// drops the binding's endpoint with its environment, so the re-bind does not collide with the endpoint handle.
// An org proxy's endpoint survives the sweep as before.
func TestProjectMCPBinding_EnvironmentRecreatedWithSameName(t *testing.T) {
	suffix := uuid.NewString()[:8]
	project := "rc-" + suffix
	projectUUID := uuid.NewString()
	devEnv, orgEnv := "dev-"+suffix, "orgenv-"+suffix
	handle := "recreate-" + suffix
	ouID := jwtassertion.MockOUID

	var mu sync.Mutex
	envs := map[string]uuid.UUID{devEnv: uuid.New(), orgEnv: uuid.New()}
	envUUID := func(name string) uuid.UUID {
		mu.Lock()
		defer mu.Unlock()
		return envs[name]
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
		mu.Lock()
		defer mu.Unlock()
		id, ok := envs[name]
		if !ok {
			return nil, utils.ErrNotFound
		}
		return &models.EnvironmentResponse{UUID: id.String(), Name: name}, nil
	}
	oc.DeleteEnvironmentFunc = func(context.Context, string, string) error { return nil }
	oc.GetReleaseBindingEndpointServiceURLFunc = func(context.Context, string, string, string, string, string, string) (string, error) {
		return "http://recreate.dp-ns.svc.cluster.local:8080", nil
	}
	app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: oc}, jwtassertion.NewMockMiddleware(t))
	h := &projectProxyHarness{t: t, app: app, org: testOrgName}
	gdb := db.DB(context.Background())
	deploymentRepo := repositories.NewDeploymentRepo(gdb)

	gatewayID := uuid.New()
	orgHandle := "recreate-org-" + suffix
	t.Cleanup(func() {
		gdb.Exec(`DELETE FROM artifacts WHERE ou_id = ? AND handle LIKE ?`, ouID, "%"+suffix+"%")
		gdb.Exec(`DELETE FROM gateway_environment_mappings WHERE gateway_uuid = ?`, gatewayID)
		gdb.Exec(`DELETE FROM gateways WHERE uuid = ?`, gatewayID)
	})
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, TRUE)`,
		gatewayID, "rc-gw-"+suffix, "rc-gw-"+suffix, "dev-gw.example.com", models.GatewayRoleBoth, ouID).Error)
	mapGateway := func(t *testing.T, env uuid.UUID) {
		t.Helper()
		require.NoError(t, gdb.Exec(
			`INSERT INTO gateway_environment_mappings (gateway_uuid, environment_uuid) VALUES (?, ?)`, gatewayID, env).Error)
	}
	deleteEnvironment := func(t *testing.T, name string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/orgs/%s/environments/%s", testOrgName, name), nil)
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
	}
	count := func(t *testing.T, query string, args ...any) int64 {
		t.Helper()
		var n int64
		require.NoError(t, gdb.Raw(query, args...).Scan(&n).Error)
		return n
	}
	endpointRows := func(t *testing.T, proxyHandle string) int64 {
		t.Helper()
		return count(t, `SELECT count(*) FROM mcp_proxy_endpoints e JOIN artifacts a ON a.uuid = e.mcp_proxy_uuid
			WHERE a.handle = ? AND a.ou_id = ?`, proxyHandle, ouID)
	}
	bindingRows := func(t *testing.T, proxyHandle string) int64 {
		t.Helper()
		return count(t, `SELECT count(*) FROM mcp_proxy_endpoint_environments b JOIN artifacts a ON a.uuid = b.mcp_proxy_uuid
			WHERE a.handle = ? AND a.ou_id = ?`, proxyHandle, ouID)
	}
	envPath := "/" + handle + "/environments/" + devEnv

	rr := h.do(http.MethodPut, project, "/"+handle, map[string]any{
		"name": "Recreate " + suffix, "context": "/recreate",
		"source": map[string]string{"componentName": "recreate", "endpointName": "mcp"},
	})
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	t.Run("bind, delete the environment, re-create it with the same name and bind again", func(t *testing.T) {
		mapGateway(t, envUUID(devEnv))
		rr := h.do(http.MethodPut, project, envPath, map[string]any{})
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		require.Equal(t, "Deploying", decodeBinding(t, rr).DeploymentStatus)
		require.EqualValues(t, 1, endpointRows(t, handle))

		deleteEnvironment(t, devEnv)
		assert.Zero(t, bindingRows(t, handle), "the sweep removes the binding")
		assert.Zero(t, endpointRows(t, handle), "and the binding's endpoint with it")

		// The environment comes back under the same name with a new id.
		mu.Lock()
		envs[devEnv] = uuid.New()
		mu.Unlock()
		mapGateway(t, envUUID(devEnv))
		rr = h.do(http.MethodPut, project, envPath, map[string]any{})
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		binding := decodeBinding(t, rr)
		assert.Equal(t, "Deploying", binding.DeploymentStatus)
		assert.Equal(t, envUUID(devEnv).String(), binding.EnvironmentUUID)

		var raw string
		require.NoError(t, gdb.Raw(`SELECT b.artifact_uuid::text FROM mcp_proxy_endpoint_environments b
			JOIN artifacts a ON a.uuid = b.mcp_proxy_uuid WHERE a.handle = ? AND a.ou_id = ? AND b.environment_uuid = ?`,
			handle, ouID, envUUID(devEnv)).Row().Scan(&raw))
		artifact := uuid.MustParse(raw)
		row, err := deploymentRepo.GetStatusRecord(context.Background(), models.Placement{
			ArtifactRef: models.ArtifactRef{OUID: ouID, ArtifactUUID: artifact, Kind: models.KindMCPMapping}, GatewayUUID: gatewayID,
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
		rr = h.do(http.MethodGet, project, envPath, nil)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.Equal(t, "Deployed", decodeBinding(t, rr).DeploymentStatus)

		rr = h.do(http.MethodDelete, project, envPath, nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
	})

	t.Run("an endpoint orphaned before the sweep dropped it is reused by the next binding", func(t *testing.T) {
		// The state a sweep before this fix left: the binding's endpoint, bound to no environment.
		require.NoError(t, gdb.Exec(`INSERT INTO mcp_proxy_endpoints (uuid, mcp_proxy_uuid, handle, name, configuration)
			SELECT ?, uuid, ?, ?, '{}'::jsonb FROM artifacts WHERE handle = ? AND ou_id = ?`,
			uuid.New(), devEnv, devEnv, handle, ouID).Error)
		rr := h.do(http.MethodPut, project, envPath, map[string]any{})
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		assert.Equal(t, "Deploying", decodeBinding(t, rr).DeploymentStatus)
		assert.EqualValues(t, 1, endpointRows(t, handle))
		assert.EqualValues(t, 1, bindingRows(t, handle))
	})

	t.Run("an org proxy keeps its endpoint when an environment is removed", func(t *testing.T) {
		proxy := &models.MCPProxy{Configuration: models.MCPProxyConfig{Name: "Org " + suffix, Version: "v1"}}
		require.NoError(t, repositories.NewMCPProxyRepo(gdb).Create(context.Background(), gdb, proxy,
			orgHandle, "Org "+suffix, "v1", ouID, nil, nil))
		var proxyUUID uuid.UUID
		require.NoError(t, gdb.Raw(`SELECT uuid FROM artifacts WHERE handle = ? AND ou_id = ?`, orgHandle, ouID).Row().Scan(&proxyUUID))
		endpoint := &models.MCPProxyEndpoint{UUID: uuid.New(), MCPProxyUUID: proxyUUID, Handle: "main", Name: "main", Status: models.StatusCreated}
		require.NoError(t, gdb.Create(endpoint).Error)
		artifact := &models.Artifact{
			UUID: uuid.New(), Handle: orgHandle + "-main-" + suffix, Name: orgHandle + "-main-" + suffix,
			Version: "v1", Kind: models.KindMCPMapping, OUID: ouID,
		}
		require.NoError(t, gdb.Create(artifact).Error)
		require.NoError(t, gdb.Create(&models.MCPProxyEndpointEnvironment{
			MCPProxyUUID: proxyUUID, EndpointUUID: endpoint.UUID, EnvironmentUUID: envUUID(orgEnv),
			ArtifactUUID: artifact.UUID, Status: models.StatusCreated,
		}).Error)

		deleteEnvironment(t, orgEnv)
		assert.Zero(t, bindingRows(t, orgHandle), "the environment's binding is removed")
		assert.EqualValues(t, 1, endpointRows(t, orgHandle), "the org proxy's endpoint is kept")
		assert.Zero(t, count(t, `SELECT count(*) FROM artifacts WHERE uuid = ?`, artifact.UUID), "its env artifact is torn down")
	})
}

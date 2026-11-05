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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

func decodeBinding(t *testing.T, rr *httptest.ResponseRecorder) models.MCPProxyEnvironmentResponse {
	t.Helper()
	var b models.MCPProxyEnvironmentResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &b), rr.Body.String())
	return b
}

// TestProjectMCPBindingLifecycle drives a project proxy's environment binding through the API and the reconcile
// model: PUT with the component not deployed yet, PUT with no gateway, placement when a gateway is mapped, the
// gateway's ack, idempotent re-PUTs, a context clash at PUT and at placement, a Failed binding re-queued by an
// identical PUT, and DELETE.
func TestProjectMCPBindingLifecycle(t *testing.T) {
	suffix := uuid.NewString()[:8]
	project := "pm-" + suffix
	projectUUID := uuid.NewString()
	envs := map[string]uuid.UUID{"dev-" + suffix: uuid.New(), "stg-" + suffix: uuid.New()}
	devEnv, stgEnv := "dev-"+suffix, "stg-"+suffix

	var mu sync.Mutex
	serviceURL := ""
	var serviceURLErr error
	setUpstream := func(url string, err error) {
		mu.Lock()
		defer mu.Unlock()
		serviceURL, serviceURLErr = url, err
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
		id, ok := envs[name]
		if !ok {
			return nil, utils.ErrNotFound
		}
		return &models.EnvironmentResponse{UUID: id.String(), Name: name}, nil
	}
	oc.GetReleaseBindingEndpointServiceURLFunc = func(_ context.Context, _, projectName, component, endpoint, _ string) (string, error) {
		assert.Equal(t, project, projectName)
		mu.Lock()
		defer mu.Unlock()
		return serviceURL, serviceURLErr
	}
	app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: oc}, jwtassertion.NewMockMiddleware(t))
	h := &projectProxyHarness{t: t, app: app, org: testOrgName}
	ouID := jwtassertion.MockOUID
	gdb := db.DB(context.Background())

	deploymentRepo := repositories.NewDeploymentRepo(gdb)
	hub := &lifecycleEventHub{}
	kind := services.NewMCPProxyKind(repositories.NewGatewayRepo(gdb), deploymentRepo)
	core := reconcile.NewCore(gdb, deploymentRepo, hub, reconcile.Kinds{kind},
		reconcile.Config{Lease: time.Minute, AttemptBudget: 3}, testLifecycleLogger()).WithGateways(repositories.NewGatewayRepo(gdb))
	worker := reconcile.NewWorker(core, testLifecycleLogger())

	gatewayID := uuid.New()
	t.Cleanup(func() {
		gdb.Exec(`DELETE FROM artifacts WHERE ou_id = ? AND handle LIKE ?`, ouID, "%"+suffix+"%")
		gdb.Exec(`DELETE FROM gateway_environment_mappings WHERE gateway_uuid = ?`, gatewayID)
		gdb.Exec(`DELETE FROM gateways WHERE uuid = ?`, gatewayID)
	})
	mapGateway := func(t *testing.T, env uuid.UUID) {
		t.Helper()
		require.NoError(t, gdb.Exec(
			`INSERT INTO gateway_environment_mappings (gateway_uuid, environment_uuid) VALUES (?, ?)`, gatewayID, env).Error)
	}

	weather := "weather-" + suffix
	tools := "tools-" + suffix
	for handle, ctxPath := range map[string]string{weather: "/weather", tools: "/weather/tools"} {
		rr := h.do(http.MethodPut, project, "/"+handle, map[string]any{
			"name": "Proxy " + handle, "context": ctxPath,
			"source": map[string]string{"componentName": handle, "endpointName": "mcp"},
		})
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	}
	envPath := func(handle, env string) string { return "/" + handle + "/environments/" + env }

	endpointRows := func(t *testing.T, handle string) int64 {
		t.Helper()
		var n int64
		require.NoError(t, gdb.Raw(`SELECT count(*) FROM mcp_proxy_endpoints e
			JOIN artifacts a ON a.uuid = e.mcp_proxy_uuid WHERE a.handle = ? AND a.ou_id = ?`, handle, ouID).Scan(&n).Error)
		return n
	}
	bindingRows := func(t *testing.T, handle string) int64 {
		t.Helper()
		var n int64
		require.NoError(t, gdb.Raw(`SELECT count(*) FROM mcp_proxy_endpoint_environments b
			JOIN artifacts a ON a.uuid = b.mcp_proxy_uuid WHERE a.handle = ? AND a.ou_id = ?`, handle, ouID).Scan(&n).Error)
		return n
	}
	// artifactOf is the weather proxy's gateway artifact in env.
	artifactOf := func(t *testing.T, handle string, env uuid.UUID) uuid.UUID {
		t.Helper()
		var raw string
		require.NoError(t, gdb.Raw(`SELECT b.artifact_uuid::text FROM mcp_proxy_endpoint_environments b
			JOIN artifacts a ON a.uuid = b.mcp_proxy_uuid WHERE a.handle = ? AND a.ou_id = ? AND b.environment_uuid = ?`,
			handle, ouID, env).Row().Scan(&raw))
		id, err := uuid.Parse(raw)
		require.NoError(t, err)
		return id
	}
	statusRow := func(t *testing.T, artifact uuid.UUID) *models.DeploymentStatusRecord {
		t.Helper()
		row, err := deploymentRepo.GetStatusRecord(context.Background(), models.Placement{
			ArtifactRef: models.ArtifactRef{OUID: ouID, ArtifactUUID: artifact, Kind: models.KindMCPMapping}, GatewayUUID: gatewayID,
		})
		require.NoError(t, err)
		return row
	}
	ack := func(t *testing.T, artifact uuid.UUID, status, code string) {
		t.Helper()
		row := statusRow(t, artifact)
		require.NotNil(t, row)
		require.NotNil(t, row.DeploymentID)
		applied, err := deploymentRepo.ApplyAck(context.Background(), gatewayID.String(), models.DeploymentAck{
			DeploymentID: row.DeploymentID.String(), ArtifactID: artifact.String(), ResourceType: "mcpproxy",
			Action: "deploy", Status: status, PerformedAt: row.PerformedAt, ErrorCode: code,
		}, func(context.Context, *gorm.DB, models.Placement, uuid.UUID) error { return nil })
		require.NoError(t, err)
		require.True(t, applied)
	}

	t.Run("component not deployed yet is 409 UPSTREAM_NOT_READY and stores nothing", func(t *testing.T) {
		setUpstream("", nil)
		rr := h.do(http.MethodPut, project, envPath(weather, devEnv), map[string]any{})
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeUpstreamNotReady, errorCode(t, rr))

		setUpstream("", occlient.ErrEndpointNotFound)
		rr = h.do(http.MethodPut, project, envPath(weather, devEnv), map[string]any{})
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeUpstreamNotReady, errorCode(t, rr))
		assert.Zero(t, bindingRows(t, weather))
		assert.Zero(t, endpointRows(t, weather), "an unbound project proxy has no endpoint")
	})

	setUpstream("http://weather.dp-ns.svc.cluster.local:8080", nil)

	t.Run("jwt is 400", func(t *testing.T) {
		rr := h.do(http.MethodPut, project, envPath(weather, devEnv), map[string]any{
			"security": map[string]any{"enabled": true, "jwt": map[string]any{"enabled": true}},
		})
		require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
		assert.Contains(t, rr.Body.String(), "jwt security is not available yet")
		assert.Zero(t, bindingRows(t, weather))
	})

	t.Run("unknown environment is 404", func(t *testing.T) {
		rr := h.do(http.MethodPut, project, envPath(weather, "nope"), map[string]any{})
		require.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeEnvironmentNotFound, errorCode(t, rr))
	})

	t.Run("no gateway: stored with reason NO_GATEWAY", func(t *testing.T) {
		rr := h.do(http.MethodPut, project, envPath(weather, devEnv), map[string]any{})
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		b := decodeBinding(t, rr)
		assert.Empty(t, b.DeploymentStatus)
		assert.Equal(t, models.MCPDeploymentStatusReasonNoGateway, b.DeploymentStatusReason)
		assert.Equal(t, "/weather", b.Context)
		assert.EqualValues(t, 1, bindingRows(t, weather))
		assert.EqualValues(t, 1, endpointRows(t, weather), "one endpoint per binding")

		rr = h.do(http.MethodPut, project, envPath(weather, devEnv), map[string]any{})
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	})

	var weatherDev uuid.UUID
	t.Run("placement deploys it once a gateway is mapped, and the ack makes it Deployed", func(t *testing.T) {
		require.NoError(t, gdb.Exec(
			`INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id, is_active)
			 VALUES (?, ?, ?, ?, ?, ?, TRUE)`,
			gatewayID, "pm-gw-"+suffix, "pm-gw-"+suffix, "dev-gw.example.com", models.GatewayRoleBoth, ouID).Error)
		mapGateway(t, envs[devEnv])
		require.NoError(t, worker.PlaceEnv(context.Background(), ouID, envs[devEnv]))

		weatherDev = artifactOf(t, weather, envs[devEnv])
		rr := h.do(http.MethodGet, project, envPath(weather, devEnv), nil)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		b := decodeBinding(t, rr)
		assert.Equal(t, models.MCPDeploymentStatusDeploying, b.DeploymentStatus)
		assert.Equal(t, gatewayID.String(), b.GatewayID)

		current, err := deploymentRepo.GetPullable(context.Background(), ouID, weatherDev, gatewayID.String())
		require.NoError(t, err)
		require.NotNil(t, current)
		var deployed services.MCPProxyDeploymentYAML
		require.NoError(t, yaml.Unmarshal(current.Content, &deployed))
		assert.Equal(t, "/"+project+"/weather", deployed.Spec.Context)
		assert.Equal(t, project+" Proxy "+weather, deployed.Spec.DisplayName)
		assert.Equal(t, "http://weather.dp-ns.svc.cluster.local:8080/mcp", deployed.Spec.Upstream.URL)

		ack(t, weatherDev, "success", "")
		rr = h.do(http.MethodGet, project, envPath(weather, devEnv), nil)
		b = decodeBinding(t, rr)
		assert.Equal(t, models.MCPDeploymentStatusDeployed, b.DeploymentStatus)
		assert.Equal(t, "/weather", b.Context, "context reads back unprefixed")
		assert.Equal(t, "https://dev-gw.example.com/"+project+"/weather/mcp", b.PublicURL)

		proxy := decodeProxy(t, h.do(http.MethodGet, project, "/"+weather, nil))
		require.Len(t, proxy.Environments, 1)
		assert.Equal(t, devEnv, proxy.Environments[0].EnvironmentName)
		assert.Equal(t, models.MCPDeploymentStatusDeployed, proxy.Environments[0].DeploymentStatus)
	})

	t.Run("identical PUT on Deployed is 200 with no new deployment", func(t *testing.T) {
		before := statusRow(t, weatherDev)
		rr := h.do(http.MethodPut, project, envPath(weather, devEnv), map[string]any{})
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		after := statusRow(t, weatherDev)
		assert.Equal(t, *before.DeploymentID, *after.DeploymentID)
		assert.Equal(t, models.DeploymentStatusDeployed, after.Status)
	})

	t.Run("an overlapping context on the gateway is 409 CONTEXT_IN_USE and stores nothing", func(t *testing.T) {
		rr := h.do(http.MethodPut, project, envPath(tools, devEnv), map[string]any{})
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeContextInUse, errorCode(t, rr))
		assert.Zero(t, bindingRows(t, tools))
	})

	t.Run("a clash found at placement marks the binding Failed", func(t *testing.T) {
		rr := h.do(http.MethodPut, project, envPath(tools, stgEnv), map[string]any{})
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		assert.Equal(t, models.MCPDeploymentStatusReasonNoGateway, decodeBinding(t, rr).DeploymentStatusReason)

		// The same gateway now also serves the staging environment, where it already holds /<project>/weather.
		mapGateway(t, envs[stgEnv])
		require.NoError(t, worker.PlaceEnv(context.Background(), ouID, envs[stgEnv]))

		b := decodeBinding(t, h.do(http.MethodGet, project, envPath(tools, stgEnv), nil))
		assert.Equal(t, models.MCPDeploymentStatusFailed, b.DeploymentStatus)
		assert.Equal(t, utils.ErrCodeContextInUse, b.DeploymentStatusReason)
		assert.Nil(t, statusRow(t, artifactOf(t, tools, envs[stgEnv])).DeploymentID, "nothing was sent")
	})

	t.Run("identical PUT on a Failed binding re-queues it", func(t *testing.T) {
		ack(t, weatherDev, "failed", "UPSTREAM_UNREACHABLE")
		b := decodeBinding(t, h.do(http.MethodGet, project, envPath(weather, devEnv), nil))
		require.Equal(t, models.MCPDeploymentStatusFailed, b.DeploymentStatus)
		before := statusRow(t, weatherDev)

		rr := h.do(http.MethodPut, project, envPath(weather, devEnv), map[string]any{})
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.Equal(t, models.MCPDeploymentStatusDeploying, decodeBinding(t, rr).DeploymentStatus)
		assert.NotEqual(t, *before.DeploymentID, *statusRow(t, weatherDev).DeploymentID, "a new deployment was sent")

		ack(t, weatherDev, "success", "")
		assert.Equal(t, models.MCPDeploymentStatusDeployed,
			decodeBinding(t, h.do(http.MethodGet, project, envPath(weather, devEnv), nil)).DeploymentStatus)
	})

	t.Run("changing the proxy's context redeploys its bindings", func(t *testing.T) {
		before := statusRow(t, weatherDev)
		rr := h.do(http.MethodPut, project, "/"+weather, map[string]any{
			"name": "Proxy " + weather, "context": "/forecast",
			"source": map[string]string{"componentName": weather, "endpointName": "mcp"},
		})
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		updated := decodeProxy(t, rr)
		require.Len(t, updated.Environments, 1)
		assert.Equal(t, "/forecast", updated.Environments[0].Context, "the upsert response reads the new context")
		assert.Equal(t, "https://dev-gw.example.com/"+project+"/forecast/mcp", updated.Environments[0].PublicURL)
		after := statusRow(t, weatherDev)
		require.NotEqual(t, *before.DeploymentID, *after.DeploymentID)
		current, err := deploymentRepo.GetPullable(context.Background(), ouID, weatherDev, gatewayID.String())
		require.NoError(t, err)
		var deployed services.MCPProxyDeploymentYAML
		require.NoError(t, yaml.Unmarshal(current.Content, &deployed))
		assert.Equal(t, "/"+project+"/forecast", deployed.Spec.Context)
		ack(t, weatherDev, "success", "")
		b := decodeBinding(t, h.do(http.MethodGet, project, envPath(weather, devEnv), nil))
		assert.Equal(t, "/forecast", b.Context)
		assert.Equal(t, "https://dev-gw.example.com/"+project+"/forecast/mcp", b.PublicURL)
	})

	t.Run("a binding with security disabled is accepted and renders no api-key policy", func(t *testing.T) {
		open := "open-" + suffix
		rr := h.do(http.MethodPut, project, "/"+open, map[string]any{
			"name": "Proxy " + open, "context": "/open",
			"source": map[string]string{"componentName": open, "endpointName": "mcp"},
		})
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		for i, env := range []string{devEnv, stgEnv} {
			ctxPath := map[string]any{"enabled": false}
			rr = h.do(http.MethodPut, project, envPath(open, env), map[string]any{"security": ctxPath})
			if i == 1 {
				// The same gateway serves both environments, so a second binding of the same context clashes.
				require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
				break
			}
			require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		}
		b := decodeBinding(t, h.do(http.MethodGet, project, envPath(open, devEnv), nil))
		require.NotNil(t, b.Security)
		require.NotNil(t, b.Security.Enabled)
		assert.False(t, *b.Security.Enabled)
		assert.Nil(t, b.Security.APIKey)
		assert.EqualValues(t, 1, endpointRows(t, open), "one endpoint per binding")

		current, err := deploymentRepo.GetPullable(context.Background(), ouID, artifactOf(t, open, envs[devEnv]), gatewayID.String())
		require.NoError(t, err)
		require.NotNil(t, current)
		var deployed services.MCPProxyDeploymentYAML
		require.NoError(t, yaml.Unmarshal(current.Content, &deployed))
		for _, policy := range deployed.Spec.Policies {
			assert.NotEqual(t, "api-key-auth", policy.Name)
		}
		rr = h.do(http.MethodDelete, project, "/"+open, nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
		assert.Zero(t, endpointRows(t, open))
	})

	t.Run("other projects can't reach the binding", func(t *testing.T) {
		projectUUIDOther := uuid.NewString()
		baseGet := oc.GetProjectFunc
		oc.GetProjectFunc = func(ctx context.Context, ns, name string) (*models.ProjectResponse, error) {
			p, err := baseGet(ctx, ns, name)
			if err == nil && name != project {
				p.UUID = projectUUIDOther
			}
			return p, err
		}
		t.Cleanup(func() { oc.GetProjectFunc = baseGet })
		rr := h.do(http.MethodGet, "other-"+suffix, envPath(weather, devEnv), nil)
		assert.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
		rr = h.do(http.MethodDelete, "other-"+suffix, envPath(weather, devEnv), nil)
		assert.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
	})

	t.Run("DELETE removes the binding and its deployment", func(t *testing.T) {
		rr := h.do(http.MethodDelete, project, envPath(weather, devEnv), nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
		rr = h.do(http.MethodGet, project, envPath(weather, devEnv), nil)
		assert.Equal(t, http.StatusNotFound, rr.Code)
		assert.Zero(t, bindingRows(t, weather))
		assert.Zero(t, endpointRows(t, weather))

		var n int64
		require.NoError(t, gdb.Raw(`SELECT count(*) FROM deployments WHERE artifact_uuid = ?`, weatherDev).Scan(&n).Error)
		assert.Zero(t, n)
		require.NoError(t, gdb.Raw(`SELECT count(*) FROM artifacts WHERE uuid = ?`, weatherDev).Scan(&n).Error)
		assert.Zero(t, n)
		require.NoError(t, gdb.Raw(`SELECT count(*) FROM mcp_proxy_endpoints e JOIN artifacts a ON a.uuid = e.mcp_proxy_uuid
			WHERE a.handle = ? AND e.name = ?`, weather, devEnv).Scan(&n).Error)
		assert.Zero(t, n, "the binding's endpoint is gone")

		rr = h.do(http.MethodDelete, project, envPath(weather, devEnv), nil)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("deleting the proxy tears down its remaining bindings", func(t *testing.T) {
		rr := h.do(http.MethodDelete, project, "/"+tools, nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
		assert.Zero(t, bindingRows(t, tools))
	})
}

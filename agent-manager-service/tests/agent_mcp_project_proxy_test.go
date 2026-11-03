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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	client "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// agentProjectProxyFixture is a project proxy "weather" (context /weather) of one project with a Deployed
// binding in the default environment, an org proxy that is in the catalog, and a mock OpenChoreo client that
// knows two projects.
type agentProjectProxyFixture struct {
	app          http.Handler
	oc           *clientmocks.OpenChoreoClientMock
	project      string // the proxy's project
	otherProject string
	devEnv       string
	weather      string // project proxy handle
	orgProxy     string // org proxy handle, in catalog
	cleanup      func()
}

func newAgentProjectProxyFixture(t *testing.T) *agentProjectProxyFixture {
	t.Helper()
	suffix := uuid.NewString()[:8]
	f := &agentProjectProxyFixture{
		project: "pa-" + suffix, otherProject: "pb-" + suffix, devEnv: "dev-" + suffix,
		weather: "weather-" + suffix, orgProxy: "orgmcp-" + suffix,
	}
	projectUUIDs := map[string]string{f.project: uuid.NewString(), f.otherProject: uuid.NewString()}
	envUUID := uuid.MustParse(apitestutils.DefaultEnvironmentUUID)

	oc := apitestutils.CreateMockOpenChoreoClient()
	baseProject := oc.GetProjectFunc
	oc.GetProjectFunc = func(ctx context.Context, ns, name string) (*models.ProjectResponse, error) {
		p, err := baseProject(ctx, ns, name)
		if err != nil {
			return nil, err
		}
		p.Name = name
		if id, ok := projectUUIDs[name]; ok {
			p.UUID = id
		}
		return p, nil
	}
	oc.ListEnvironmentsFunc = func(context.Context, string) ([]*models.EnvironmentResponse, error) {
		return []*models.EnvironmentResponse{{UUID: envUUID.String(), Name: f.devEnv}}, nil
	}
	oc.GetReleaseBindingEndpointServiceURLFunc = func(context.Context, string, string, string, string, string) (string, error) {
		return "http://weather.dp-ns.svc.cluster.local:8080", nil
	}
	oc.UpdateReleaseBindingEnvVarsFunc = func(context.Context, string, string, string, string, []client.EnvVar) error { return nil }
	f.oc = oc
	f.app = apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: oc}, jwtassertion.NewMockMiddleware(t))

	ouID := jwtassertion.MockOUID
	gdb := db.DB(context.Background())
	gatewayID := uuid.New()
	deploymentRepo := repositories.NewDeploymentRepo(gdb)
	core := reconcile.NewCore(gdb, deploymentRepo, &lifecycleEventHub{},
		reconcile.Kinds{services.NewMCPProxyKind(repositories.NewGatewayRepo(gdb), deploymentRepo)},
		reconcile.Config{Lease: time.Minute, AttemptBudget: 3}, testLifecycleLogger())
	worker := reconcile.NewWorker(core, testLifecycleLogger())

	f.cleanup = func() {
		gdb.Exec(`DELETE FROM agent_configurations WHERE ou_id = ? AND project_name IN (?, ?)`, ouID, f.project, f.otherProject)
		gdb.Exec(`DELETE FROM mcp_proxy_mappings WHERE source_mcp_proxy_uuid IN (SELECT uuid FROM artifacts WHERE ou_id = ? AND handle LIKE ?)`, ouID, "%"+suffix+"%")
		gdb.Exec(`DELETE FROM artifacts WHERE ou_id = ? AND handle LIKE ?`, ouID, "%"+suffix+"%")
		gdb.Exec(`DELETE FROM gateway_environment_mappings WHERE gateway_uuid = ?`, gatewayID)
		gdb.Exec(`DELETE FROM gateways WHERE uuid = ?`, gatewayID)
	}
	t.Cleanup(f.cleanup)

	h := &projectProxyHarness{t: t, app: f.app, org: testOrgName}
	rr := h.do(http.MethodPut, f.project, "/"+f.weather, map[string]any{
		"name": "Proxy " + f.weather, "context": "/weather",
		"source": map[string]string{"componentName": f.weather, "endpointName": "mcp"},
	})
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	rr = h.do(http.MethodPut, f.project, "/"+f.weather+"/environments/"+f.devEnv, map[string]any{
		"security": map[string]any{"enabled": false},
	})
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	require.NoError(t, gdb.Exec(
		`INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, TRUE)`,
		gatewayID, "pa-gw-"+suffix, "pa-gw-"+suffix, "dev-gw.example.com", models.GatewayRoleBoth, ouID).Error)
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateway_environment_mappings (gateway_uuid, environment_uuid) VALUES (?, ?)`, gatewayID, envUUID).Error)
	require.NoError(t, worker.PlaceEnv(context.Background(), ouID, envUUID))

	var artifact uuid.UUID
	var raw string
	require.NoError(t, gdb.Raw(`SELECT b.artifact_uuid::text FROM mcp_proxy_endpoint_environments b
		JOIN artifacts a ON a.uuid = b.mcp_proxy_uuid WHERE a.handle = ? AND a.ou_id = ? AND b.environment_uuid = ?`,
		f.weather, ouID, envUUID).Row().Scan(&raw))
	artifact, err := uuid.Parse(raw)
	require.NoError(t, err)
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
	return f
}

func (f *agentProjectProxyFixture) createMCPConfig(project, agent, proxy string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{
		"name": "tools", "type": "mcp",
		"envMappings": map[string]any{f.devEnv: map[string]any{"providerName": proxy}},
	})
	url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/mcp-configs", testOrgName, project, agent)
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	f.app.ServeHTTP(rr, req)
	return rr
}

// A same-project proxy is bindable although it is not in the catalog, and the URL the agent is handed
// carries the project prefix the gateway serves it at.
func TestAgentBindsSameProjectProxy_OK(t *testing.T) {
	f := newAgentProjectProxyFixture(t)
	rr := f.createMCPConfig(f.project, "agent-one", f.weather)
	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

	var injected []string
	for _, call := range f.oc.UpdateReleaseBindingEnvVarsCalls() {
		for _, ev := range call.EnvVars {
			injected = append(injected, ev.Value)
		}
	}
	require.Contains(t, injected, "https://dev-gw.example.com/"+f.project+"/weather/mcp",
		"the URL injected into the agent carries the project prefix")
}

// Another project's agent can't see the proxy: it is reported as not found, not as forbidden.
func TestAgentBindsOtherProjectProxy_NotFound(t *testing.T) {
	f := newAgentProjectProxyFixture(t)
	rr := f.createMCPConfig(f.otherProject, "agent-two", f.weather)
	require.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
}

// An org proxy is bindable only while it is in the catalog, as before.
func TestAgentBindsOrgProxy_Unchanged(t *testing.T) {
	f := newAgentProjectProxyFixture(t)
	gdb := db.DB(context.Background())
	require.NoError(t, repositories.NewMCPProxyRepo(gdb).Create(context.Background(), gdb,
		&models.MCPProxy{Configuration: models.MCPProxyConfig{Name: "Org " + f.orgProxy, Version: "v1"}},
		f.orgProxy, "Org "+f.orgProxy, "v1", jwtassertion.MockOUID, nil, nil))

	rr := f.createMCPConfig(f.project, "agent-three", f.orgProxy)
	require.Equal(t, http.StatusCreated, rr.Code, "an org proxy in the catalog binds: "+rr.Body.String())

	require.NoError(t, gdb.Exec(`UPDATE artifacts SET in_catalog = FALSE WHERE ou_id = ? AND handle = ?`,
		jwtassertion.MockOUID, f.orgProxy).Error)
	rr = f.createMCPConfig(f.project, "agent-four", f.orgProxy)
	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	require.Contains(t, rr.Body.String(), "must be in catalog")
}

func (f *agentProjectProxyFixture) createAgentWithMCP(project, proxy string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{
		"name": "mcp-agent", "displayName": "MCP Agent",
		"provisioning": map[string]any{
			"type": "internal",
			"repository": map[string]any{
				"url": "https://github.com/test/test-repo", "branch": "main", "appPath": "/agent-sample",
			},
		},
		"agentType": map[string]any{"type": "agent-api", "subType": "chat-api"},
		"build": map[string]any{
			"type": "buildpack",
			"buildpack": map[string]any{
				"language": "python", "languageVersion": "3.11", "runCommand": "uvicorn app:app --host 0.0.0.0 --port 8000",
			},
		},
		"inputInterface": map[string]any{"type": "HTTP"},
		"mcpConfig":      []map[string]any{{"proxyName": proxy}},
	})
	url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, project)
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	f.app.ServeHTTP(rr, req)
	return rr
}

// The agent-create preflight accepts the agent's own project's proxy and hides another project's, before
// anything is provisioned.
func TestAgentCreatePreflight_ProjectProxy(t *testing.T) {
	f := newAgentProjectProxyFixture(t)
	seedIngressGatewayForDefaultEnvironment(t, jwtassertion.MockOUID)

	rr := f.createAgentWithMCP(f.otherProject, f.weather)
	require.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
	require.Empty(t, f.oc.CreateComponentCalls(), "nothing is provisioned for a proxy the agent can't see")

	f.createAgentWithMCP(f.project, f.weather)
	require.NotEmpty(t, f.oc.CreateComponentCalls(), "the same project's proxy passes the preflight")
}

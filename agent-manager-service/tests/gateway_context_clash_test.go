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

// TestGatewayContextClashAcrossKinds checks the one context rule every kind's deploy runs, in both directions, on
// one gateway: an org MCP proxy deployed first blocks a project binding under it, a Deployed project binding blocks
// an org MCP proxy and an LLM provider at an overlapping context and stays Deployed, root and another vhost don't
// clash, and the binding's identical ensure-PUT is still 200.
func TestGatewayContextClashAcrossKinds(t *testing.T) {
	suffix := uuid.NewString()[:8]
	project := "cc-" + suffix
	projectUUID := uuid.NewString()
	env, envUUID := "dev-"+suffix, uuid.New()

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
		if name != env {
			return nil, utils.ErrNotFound
		}
		return &models.EnvironmentResponse{UUID: envUUID.String(), Name: name}, nil
	}
	oc.GetReleaseBindingEndpointServiceURLFunc = func(context.Context, string, string, string, string, string) (string, error) {
		return "http://mcp.dp-ns.svc.cluster.local:8080", nil
	}
	app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: oc}, jwtassertion.NewMockMiddleware(t))
	h := &projectProxyHarness{t: t, app: app, org: testOrgName}
	ouID := jwtassertion.MockOUID
	gdb := db.DB(context.Background())
	deploymentRepo := repositories.NewDeploymentRepo(gdb)

	orgDo := func(method, path string, body any) *httptest.ResponseRecorder {
		t.Helper()
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(method, fmt.Sprintf("/api/v1/orgs/%s%s", testOrgName, path), bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		return rr
	}

	// A self-hosted gateway takes an org proxy's upstream without the public-address check; it must be egress.
	gatewayID := uuid.New()
	t.Cleanup(func() {
		gdb.Exec(`DELETE FROM artifacts WHERE ou_id = ? AND handle LIKE ?`, ouID, "%"+suffix+"%")
		gdb.Exec(`DELETE FROM gateway_environment_mappings WHERE gateway_uuid = ?`, gatewayID)
		gdb.Exec(`DELETE FROM gateways WHERE uuid = ?`, gatewayID)
	})
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id, is_active, hosting)
		 VALUES (?, ?, ?, ?, ?, ?, TRUE, ?)`,
		gatewayID, "cc-gw-"+suffix, "cc-gw-"+suffix, "cc-gw.example.com", models.GatewayRoleEgress, ouID,
		models.GatewayHostingSelfHosted).Error)
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateway_environment_mappings (gateway_uuid, environment_uuid) VALUES (?, ?)`, gatewayID, envUUID).Error)

	orgProxy := func(handle, ctxPath string, vhost *string) *httptest.ResponseRecorder {
		t.Helper()
		body := map[string]any{
			"id": handle, "name": "Org " + handle, "version": "v1", "context": ctxPath,
			"endpoints": []map[string]any{{
				"id":           "main",
				"upstream":     map[string]any{"main": map[string]any{"url": "https://mcp.example.com/mcp"}},
				"environments": []map[string]any{{"environmentUuid": envUUID.String(), "gatewayId": gatewayID.String()}},
			}},
		}
		if vhost != nil {
			body["vhost"] = *vhost
		}
		return orgDo(http.MethodPost, "/mcp-proxies", body)
	}
	projectProxy := func(handle, ctxPath string) {
		t.Helper()
		rr := h.do(http.MethodPut, project, "/"+handle, map[string]any{
			"name": "Proxy " + handle, "context": ctxPath,
			"source": map[string]string{"componentName": handle, "endpointName": "mcp"},
		})
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	}
	// A self-hosted gateway is never chosen automatically, so every write names it.
	pinned := map[string]any{"gatewayId": gatewayID.String()}
	bindingPath := func(handle string) string { return "/" + handle + "/environments/" + env }
	bindingRows := func(handle string) int64 {
		t.Helper()
		var n int64
		require.NoError(t, gdb.Raw(`SELECT count(*) FROM mcp_proxy_endpoint_environments b
			JOIN artifacts a ON a.uuid = b.mcp_proxy_uuid WHERE a.handle = ? AND a.ou_id = ?`, handle, ouID).Scan(&n).Error)
		return n
	}
	ackBinding := func(handle string) {
		t.Helper()
		var raw string
		require.NoError(t, gdb.Raw(`SELECT b.artifact_uuid::text FROM mcp_proxy_endpoint_environments b
			JOIN artifacts a ON a.uuid = b.mcp_proxy_uuid WHERE a.handle = ? AND a.ou_id = ?`, handle, ouID).Row().Scan(&raw))
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
	}

	t.Run("an org MCP proxy deployed first blocks a project binding under it", func(t *testing.T) {
		rr := orgProxy("org-weather-"+suffix, "/"+project+"/weather", nil)
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())

		weather := "weather-" + suffix
		projectProxy(weather, "/weather")
		rr = h.do(http.MethodPut, project, bindingPath(weather), pinned)
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeContextInUse, errorCode(t, rr))
		assert.Zero(t, bindingRows(weather), "nothing is stored")
	})

	tools := "tools-" + suffix
	t.Run("a Deployed project binding blocks later artifacts of every kind and stays Deployed", func(t *testing.T) {
		projectProxy(tools, "/tools")
		rr := h.do(http.MethodPut, project, bindingPath(tools), pinned)
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		ackBinding(tools)

		orgHandle := "org-tools-" + suffix
		rr = orgProxy(orgHandle, "/"+project+"/tools/v2", nil)
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeContextInUse, errorCode(t, rr))
		assert.Equal(t, http.StatusNotFound, orgDo(http.MethodGet, "/mcp-proxies/"+orgHandle, nil).Code, "nothing is stored")

		template := "tpl-" + suffix
		rr = orgDo(http.MethodPost, "/llm-provider-templates", map[string]any{"id": template, "name": "Template " + suffix})
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		t.Cleanup(func() { gdb.Exec(`DELETE FROM llm_provider_templates WHERE handle = ?`, template) })
		provider := "llm-tools-" + suffix
		rr = orgDo(http.MethodPost, "/llm-providers", map[string]any{
			"id": provider, "name": "LLM " + suffix, "version": "v1.0", "context": "/" + project,
			"template": template, "upstream": map[string]any{"main": map[string]any{"url": "https://api.openai.com/v1"}},
		})
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		rr = orgDo(http.MethodPost, "/llm-providers/"+provider+"/deployments", map[string]any{"gatewayId": gatewayID.String()})
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeContextInUse, errorCode(t, rr))

		b := decodeBinding(t, h.do(http.MethodGet, project, bindingPath(tools), nil))
		assert.Equal(t, models.MCPDeploymentStatusDeployed, b.DeploymentStatus, "the incumbent is untouched")
	})

	t.Run("root and another vhost don't clash", func(t *testing.T) {
		rr := orgProxy("org-root-"+suffix, "/", nil)
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		other := "other-" + suffix + ".example.com"
		rr = orgProxy("org-vhost-"+suffix, "/"+project+"/tools", &other)
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
	})

	t.Run("an identical ensure-PUT on the Deployed binding is still 200", func(t *testing.T) {
		rr := h.do(http.MethodPut, project, bindingPath(tools), pinned)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.Equal(t, models.MCPDeploymentStatusDeployed, decodeBinding(t, rr).DeploymentStatus)
	})
}

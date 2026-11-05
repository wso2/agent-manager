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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

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

// TestProjectMCPProxyOfDeletedProject drives the proxies a project leaves when it is deleted in OpenChoreo:
// the project routes still read and remove them by the stored project name, PUT refuses, a re-created project of
// the same name adopts a proxy by re-PUTting its handle, and the org route removes one whose project is gone.
func TestProjectMCPProxyOfDeletedProject(t *testing.T) {
	suffix := uuid.NewString()[:8]
	project := "po-" + suffix
	devEnv, devEnvUUID := "dev-"+suffix, uuid.New()

	// projectUUID is the live project's UUID; empty while the project is deleted.
	var mu sync.Mutex
	projectUUID := uuid.NewString()
	setProject := func(id string) {
		mu.Lock()
		defer mu.Unlock()
		projectUUID = id
	}

	oc := apitestutils.CreateMockOpenChoreoClient()
	oc.GetProjectFunc = func(_ context.Context, _, name string) (*models.ProjectResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		if name != project || projectUUID == "" {
			return nil, fmt.Errorf("project %s: %w", name, utils.ErrProjectNotFound)
		}
		return &models.ProjectResponse{Name: name, UUID: projectUUID}, nil
	}
	oc.GetEnvironmentFunc = func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
		if name != devEnv {
			return nil, utils.ErrNotFound
		}
		return &models.EnvironmentResponse{UUID: devEnvUUID.String(), Name: name}, nil
	}
	oc.GetReleaseBindingEndpointServiceURLFunc = func(context.Context, string, string, string, string, string, string) (string, error) {
		return "http://orders.dp-ns.svc.cluster.local:8080", nil
	}
	app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: oc}, jwtassertion.NewMockMiddleware(t))
	h := &projectProxyHarness{t: t, app: app, org: testOrgName}
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
	ouID := jwtassertion.MockOUID
	gdb := db.DB(context.Background())

	deploymentRepo := repositories.NewDeploymentRepo(gdb)
	kind := services.NewMCPProxyKind(repositories.NewGatewayRepo(gdb), deploymentRepo)
	core := reconcile.NewCore(gdb, deploymentRepo, &lifecycleEventHub{}, reconcile.Kinds{kind},
		reconcile.Config{Lease: time.Minute, AttemptBudget: 3}, testLifecycleLogger()).WithGateways(repositories.NewGatewayRepo(gdb))
	worker := reconcile.NewWorker(core, testLifecycleLogger())

	gatewayID := uuid.New()
	t.Cleanup(func() {
		gdb.Exec(`DELETE FROM artifacts WHERE ou_id = ? AND handle LIKE ?`, ouID, "%"+suffix+"%")
		gdb.Exec(`DELETE FROM gateway_environment_mappings WHERE gateway_uuid = ?`, gatewayID)
		gdb.Exec(`DELETE FROM gateways WHERE uuid = ?`, gatewayID)
	})
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, TRUE)`,
		gatewayID, "po-gw-"+suffix, "po-gw-"+suffix, "dev-gw.example.com", models.GatewayRoleBoth, ouID).Error)
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateway_environment_mappings (gateway_uuid, environment_uuid) VALUES (?, ?)`, gatewayID, devEnvUUID).Error)

	envPath := func(handle string) string { return "/" + handle + "/environments/" + devEnv }
	body := func(handle string) map[string]any {
		return map[string]any{
			"name": "Proxy " + handle, "context": "/" + handle,
			"source": map[string]string{"componentName": handle, "endpointName": "mcp"},
		}
	}
	// publish creates the proxy in the live project and binds it to the dev environment's gateway, returning the
	// binding's gateway artifact.
	publish := func(t *testing.T, handle string) uuid.UUID {
		t.Helper()
		rr := h.do(http.MethodPut, project, "/"+handle, body(handle))
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		rr = h.do(http.MethodPut, project, envPath(handle), map[string]any{})
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		require.NoError(t, worker.PlaceEnv(context.Background(), ouID, devEnvUUID))
		var raw string
		require.NoError(t, gdb.Raw(`SELECT b.artifact_uuid::text FROM mcp_proxy_endpoint_environments b
			JOIN artifacts a ON a.uuid = b.mcp_proxy_uuid WHERE a.handle = ? AND a.ou_id = ?`, handle, ouID).Row().Scan(&raw))
		artifact := uuid.MustParse(raw)
		current, err := deploymentRepo.GetPullable(context.Background(), ouID, artifact, gatewayID.String())
		require.NoError(t, err)
		require.NotNil(t, current, "the binding is deployed to the gateway")
		return artifact
	}
	count := func(t *testing.T, query string, args ...any) int64 {
		t.Helper()
		var n int64
		require.NoError(t, gdb.Raw(query, args...).Scan(&n).Error)
		return n
	}
	// tornDown asserts the gateway artifact and its deployments are gone.
	tornDown := func(t *testing.T, artifact uuid.UUID) {
		t.Helper()
		assert.Zero(t, count(t, `SELECT count(*) FROM deployments WHERE artifact_uuid = ?`, artifact))
		assert.Zero(t, count(t, `SELECT count(*) FROM artifacts WHERE uuid = ?`, artifact))
	}
	proxyGone := func(t *testing.T, handle string) {
		t.Helper()
		assert.Zero(t, count(t, `SELECT count(*) FROM artifacts WHERE ou_id = ? AND handle = ?`, ouID, handle))
	}

	t.Run("the project routes still read and remove the proxies of a deleted project", func(t *testing.T) {
		setProject(uuid.NewString())
		orders := "orders-" + suffix
		artifact := publish(t, orders)
		setProject("")

		rr := h.do(http.MethodGet, project, "", nil)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		var list models.ProjectMCPProxyListResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &list))
		require.Len(t, list.List, 1)
		assert.Equal(t, orders, list.List[0].ID)

		rr = h.do(http.MethodGet, project, "/"+orders, nil)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.Equal(t, project, decodeProxy(t, rr).ProjectName)
		rr = h.do(http.MethodGet, project, envPath(orders), nil)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

		rr = h.do(http.MethodPut, project, "/"+orders, body(orders))
		require.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeProjectNotFound, errorCode(t, rr))
		rr = h.do(http.MethodPut, project, envPath(orders), map[string]any{})
		require.Equal(t, http.StatusNotFound, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeProjectNotFound, errorCode(t, rr))

		other := "px-" + suffix
		rr = h.do(http.MethodGet, other, "/"+orders, nil)
		require.Equal(t, http.StatusNotFound, rr.Code, "another deleted project's name does not reach it")
		assert.Equal(t, utils.ErrCodeProjectNotFound, errorCode(t, rr))
		rr = h.do(http.MethodDelete, other, "/"+orders, nil)
		require.Equal(t, http.StatusNotFound, rr.Code)
		rr = h.do(http.MethodGet, other, "", nil)
		require.Equal(t, http.StatusNotFound, rr.Code)
		assert.Equal(t, utils.ErrCodeProjectNotFound, errorCode(t, rr))

		rr = h.do(http.MethodDelete, project, envPath(orders), nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
		tornDown(t, artifact)

		rr = h.do(http.MethodDelete, project, "/"+orders, nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
		proxyGone(t, orders)
		rr = h.do(http.MethodGet, project, "/"+orders, nil)
		require.Equal(t, http.StatusNotFound, rr.Code)
		assert.Equal(t, utils.ErrCodeProjectNotFound, errorCode(t, rr))
	})

	t.Run("deleting a deleted project's proxy tears down its bindings", func(t *testing.T) {
		setProject(uuid.NewString())
		billing := "billing-" + suffix
		artifact := publish(t, billing)
		setProject("")

		rr := h.do(http.MethodDelete, project, "/"+billing, nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
		tornDown(t, artifact)
		proxyGone(t, billing)
	})

	t.Run("a re-created project adopts the proxy by re-PUTting its handle", func(t *testing.T) {
		setProject(uuid.NewString())
		stock := "stock-" + suffix
		artifact := publish(t, stock)
		setProject("")
		recreated := uuid.NewString()
		setProject(recreated)

		rr := h.do(http.MethodGet, project, "", nil)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		var list models.ProjectMCPProxyListResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &list))
		assert.Empty(t, list.List, "the re-created project owns nothing yet")
		rr = orgDo(http.MethodDelete, "/mcp-proxies/"+stock, nil)
		require.Equal(t, http.StatusConflict, rr.Code, "a project of that name exists: the org route still refuses")
		assert.Equal(t, utils.ErrCodeProjectOwned, errorCode(t, rr))

		rr = h.do(http.MethodPut, project, "/"+stock, body(stock))
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		p := decodeProxy(t, rr)
		assert.Equal(t, project, p.ProjectName)
		require.Len(t, p.Environments, 1, "the binding stays")
		assert.Equal(t, devEnv, p.Environments[0].EnvironmentName)
		assert.EqualValues(t, 1, count(t, `SELECT count(*) FROM mcp_proxies m JOIN artifacts a ON a.uuid = m.uuid
			WHERE a.ou_id = ? AND a.handle = ? AND m.project_uuid = ? AND m.project_name = ?`, ouID, stock, recreated, project))

		rr = h.do(http.MethodGet, project, "", nil)
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &list))
		require.Len(t, list.List, 1)
		assert.Equal(t, stock, list.List[0].ID)
		rr = h.do(http.MethodPut, project, envPath(stock), map[string]any{})
		require.Equal(t, http.StatusOK, rr.Code, "re-PUTting the binding refreshes it in the re-created project: %s", rr.Body.String())

		rr = h.do(http.MethodDelete, project, "/"+stock, nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
		tornDown(t, artifact)
		proxyGone(t, stock)
	})

	t.Run("an org handle is never adopted", func(t *testing.T) {
		setProject(uuid.NewString())
		orgHandle := "orgowned-" + suffix
		require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
			return repositories.NewMCPProxyRepo(gdb).Create(context.Background(), tx, &models.MCPProxy{
				Configuration: models.MCPProxyConfig{Name: orgHandle, Version: "v1"},
			}, orgHandle, orgHandle, "v1", ouID, nil, nil)
		}))
		rr := h.do(http.MethodPut, project, "/"+orgHandle, body(orgHandle))
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeHandleInUse, errorCode(t, rr))
		assert.EqualValues(t, 1, count(t, `SELECT count(*) FROM mcp_proxies m JOIN artifacts a ON a.uuid = m.uuid
			WHERE a.ou_id = ? AND a.handle = ? AND m.project_uuid IS NULL`, ouID, orgHandle))
	})

	t.Run("the org route removes a proxy whose project is gone", func(t *testing.T) {
		setProject(uuid.NewString())
		ledger := "ledger-" + suffix
		artifact := publish(t, ledger)

		rr := orgDo(http.MethodDelete, "/mcp-proxies/"+ledger, nil)
		require.Equal(t, http.StatusConflict, rr.Code, "refused while the project exists")
		assert.Equal(t, utils.ErrCodeProjectOwned, errorCode(t, rr))

		setProject("")
		rr = orgDo(http.MethodPut, "/mcp-proxies/"+ledger, map[string]any{"id": ledger, "name": "x", "version": "v1"})
		require.Equal(t, http.StatusConflict, rr.Code, "an org update still refuses: %s", rr.Body.String())
		assert.Equal(t, utils.ErrCodeProjectOwned, errorCode(t, rr))

		rr = orgDo(http.MethodDelete, "/mcp-proxies/"+ledger, nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
		tornDown(t, artifact)
		proxyGone(t, ledger)
	})
}

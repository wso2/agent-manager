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

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
	"gorm.io/gorm"
)

type projectProxyHarness struct {
	t   *testing.T
	app http.Handler
	org string
}

func (h *projectProxyHarness) do(method, project, suffix string, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(h.t, err)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/mcp-proxies%s", h.org, project, suffix)
	req := httptest.NewRequest(method, url, reader)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.app.ServeHTTP(rr, req)
	return rr
}

func errorCode(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	return body.Code
}

func decodeProxy(t *testing.T, rr *httptest.ResponseRecorder) models.ProjectMCPProxyResponse {
	t.Helper()
	var p models.ProjectMCPProxyResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &p), rr.Body.String())
	return p
}

func TestProjectMCPProxyCRUD(t *testing.T) {
	projectUUIDs := map[string]string{}
	projA := "pa-" + uuid.NewString()[:8]
	projB := "pb-" + uuid.NewString()[:8]
	projectUUIDs[projA] = uuid.NewString()
	projectUUIDs[projB] = uuid.NewString()

	oc := apitestutils.CreateMockOpenChoreoClient()
	base := oc.GetProjectFunc
	oc.GetProjectFunc = func(ctx context.Context, ns, name string) (*models.ProjectResponse, error) {
		p, err := base(ctx, ns, name)
		if err != nil {
			return nil, err
		}
		p.UUID = projectUUIDs[name]
		return p, nil
	}
	app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: oc}, jwtassertion.NewMockMiddleware(t))
	h := &projectProxyHarness{t: t, app: app, org: testOrgName}
	ouID := jwtassertion.MockOUID
	gdb := db.DB(context.Background())

	suffix := uuid.NewString()[:8]
	handle := "orders-mcp-" + suffix
	req := map[string]any{
		"name":    "Orders MCP " + suffix,
		"context": "/orders-" + suffix,
		"source":  map[string]string{"componentName": "orders-mcp", "endpointName": "mcp"},
	}
	t.Cleanup(func() {
		gdb.Exec(`DELETE FROM artifacts WHERE ou_id = ? AND handle LIKE ?`, ouID, "%"+suffix)
	})

	var firstBody []byte
	t.Run("create returns 201 and one endpoint row", func(t *testing.T) {
		rr := h.do(http.MethodPut, projA, "/"+handle, req)
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		firstBody = rr.Body.Bytes()
		p := decodeProxy(t, rr)
		assert.Equal(t, handle, p.ID)
		assert.Equal(t, "/orders-"+suffix, p.Context)
		assert.Equal(t, "v1", p.Version)
		assert.Equal(t, projA, p.ProjectName)
		assert.Equal(t, "orders-mcp", p.Source.ComponentName)
		assert.Equal(t, "[]", string(mustField(t, rr.Body.Bytes(), "environments")))

		var n int64
		require.NoError(t, gdb.Raw(`SELECT count(*) FROM mcp_proxy_endpoints e JOIN artifacts a ON a.uuid = e.mcp_proxy_uuid
			WHERE a.handle = ? AND a.ou_id = ? AND e.handle = ?`, handle, ouID, handle).Scan(&n).Error)
		assert.EqualValues(t, 1, n)
	})

	t.Run("identical body is 200 with nothing changed", func(t *testing.T) {
		for range 3 {
			rr := h.do(http.MethodPut, projA, "/"+handle, req)
			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			assert.JSONEq(t, string(firstBody), rr.Body.String())
		}
		var n int64
		require.NoError(t, gdb.Raw(`SELECT count(*) FROM mcp_proxy_endpoints e JOIN artifacts a ON a.uuid = e.mcp_proxy_uuid
			WHERE a.handle = ? AND a.ou_id = ?`, handle, ouID).Scan(&n).Error)
		assert.EqualValues(t, 1, n, "still exactly one endpoint row")
	})

	t.Run("changed fields update, source is immutable", func(t *testing.T) {
		changed := map[string]any{}
		for k, v := range req {
			changed[k] = v
		}
		changed["description"] = "now described"
		rr := h.do(http.MethodPut, projA, "/"+handle, changed)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.Equal(t, "now described", decodeProxy(t, rr).Description)

		changed["source"] = map[string]string{"componentName": "orders-mcp", "endpointName": "other"}
		rr = h.do(http.MethodPut, projA, "/"+handle, changed)
		require.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Equal(t, utils.ErrCodeImmutableField, errorCode(t, rr))
	})

	t.Run("get and list are scoped to the project", func(t *testing.T) {
		rr := h.do(http.MethodGet, projA, "/"+handle, nil)
		require.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, handle, decodeProxy(t, rr).ID)

		rr = h.do(http.MethodGet, projB, "/"+handle, nil)
		assert.Equal(t, http.StatusNotFound, rr.Code, "another project's proxy is not found")

		rr = h.do(http.MethodGet, projA, "?component=orders-mcp&endpoint=mcp", nil)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		var list models.ProjectMCPProxyListResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &list))
		require.Len(t, list.List, 1)
		assert.Equal(t, handle, list.List[0].ID)
		assert.NotNil(t, list.List[0].Environments)

		rr = h.do(http.MethodGet, projA, "?component=other", nil)
		require.Equal(t, http.StatusOK, rr.Code)
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &list))
		assert.Empty(t, list.List)

		rr = h.do(http.MethodGet, projB, "", nil)
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &list))
		assert.Empty(t, list.List, "project B lists none of project A's proxies")
	})

	t.Run("handle owned by another project or an org proxy is 409 HANDLE_IN_USE", func(t *testing.T) {
		rr := h.do(http.MethodPut, projB, "/"+handle, req)
		require.Equal(t, http.StatusConflict, rr.Code)
		assert.Equal(t, utils.ErrCodeHandleInUse, errorCode(t, rr))

		orgHandle := "org-proxy-" + suffix
		require.NoError(t, repositories.NewMCPProxyRepo(gdb).Create(context.Background(), gdb,
			&models.MCPProxy{Configuration: models.MCPProxyConfig{Name: "Org " + suffix, Version: "v1"}},
			orgHandle, "Org "+suffix, "v1", ouID, nil, nil))
		rr = h.do(http.MethodPut, projA, "/"+orgHandle, req)
		require.Equal(t, http.StatusConflict, rr.Code)
		assert.Equal(t, utils.ErrCodeHandleInUse, errorCode(t, rr))
		rr = h.do(http.MethodGet, projA, "/"+orgHandle, nil)
		assert.Equal(t, http.StatusNotFound, rr.Code, "an org proxy is not a project proxy")
		rr = h.do(http.MethodDelete, projA, "/"+orgHandle, nil)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("two projects may use the same display name", func(t *testing.T) {
		other := map[string]any{"name": req["name"], "source": req["source"]}
		rr := h.do(http.MethodPut, projB, "/b-"+handle, other)
		require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
		rr = h.do(http.MethodDelete, projB, "/b-"+handle, nil)
		assert.Equal(t, http.StatusNoContent, rr.Code)
	})

	t.Run("unknown project is 404 PROJECT_NOT_FOUND", func(t *testing.T) {
		rr := h.do(http.MethodGet, "nonexistent-proj", "", nil)
		require.Equal(t, http.StatusNotFound, rr.Code)
		assert.Equal(t, utils.ErrCodeProjectNotFound, errorCode(t, rr))
	})

	t.Run("validation errors are 400", func(t *testing.T) {
		rr := h.do(http.MethodPut, projA, "/Bad_Handle", req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		rr = h.do(http.MethodPut, projA, "/x-"+suffix, map[string]any{"name": "n"})
		assert.Equal(t, http.StatusBadRequest, rr.Code, "source is required")
	})

	t.Run("delete is 409 MCP_PROXY_IN_USE while an agent uses the proxy", func(t *testing.T) {
		var proxyUUID string
		require.NoError(t, gdb.Raw(`SELECT uuid FROM artifacts WHERE handle = ? AND ou_id = ?`, handle, ouID).Scan(&proxyUUID).Error)
		configUUID := uuid.NewString()
		require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(`INSERT INTO agent_configurations (uuid, name, agent_id, type_id, ou_id, project_name)
				VALUES (?, ?, 'agent-x', 2, ?, ?)`, configUUID, "cfg-"+suffix, ouID, projA).Error; err != nil {
				return err
			}
			return tx.Exec(`INSERT INTO agent_mcp_config_proxy (config_uuid, type_id, mcp_proxy_uuid) VALUES (?, 2, ?)`,
				configUUID, proxyUUID).Error
		}))

		rr := h.do(http.MethodDelete, projA, "/"+handle, nil)
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeMCPProxyInUse, errorCode(t, rr))
		assert.Equal(t, http.StatusOK, h.do(http.MethodGet, projA, "/"+handle, nil).Code, "nothing was deleted")

		require.NoError(t, gdb.Exec(`DELETE FROM agent_configurations WHERE uuid = ?`, configUUID).Error)
	})

	t.Run("delete from another project is 404, then delete removes the proxy and its endpoint", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, h.do(http.MethodDelete, projB, "/"+handle, nil).Code)

		rr := h.do(http.MethodDelete, projA, "/"+handle, nil)
		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
		assert.Equal(t, http.StatusNotFound, h.do(http.MethodGet, projA, "/"+handle, nil).Code)

		var n int64
		require.NoError(t, gdb.Raw(`SELECT count(*) FROM mcp_proxy_endpoints WHERE handle = ?`, handle).Scan(&n).Error)
		assert.Zero(t, n)
		// Deleting again is a 404, not an error.
		assert.Equal(t, http.StatusNotFound, h.do(http.MethodDelete, projA, "/"+handle, nil).Code)
	})
}

// TestProjectMCPProxyCreateNeedsCreatePermission: a token that may update but not create can
// refresh an existing proxy and is refused (403) when the upsert would insert.
func TestProjectMCPProxyCreateNeedsCreatePermission(t *testing.T) {
	projectUUID := uuid.NewString()
	proj := "pc-" + uuid.NewString()[:8]
	oc := apitestutils.CreateMockOpenChoreoClient()
	base := oc.GetProjectFunc
	oc.GetProjectFunc = func(ctx context.Context, ns, name string) (*models.ProjectResponse, error) {
		p, err := base(ctx, ns, name)
		if err != nil {
			return nil, err
		}
		p.UUID = projectUUID
		return p, nil
	}
	full := jwtassertion.NewMockMiddleware(t)
	updateOnly := func(next http.Handler) http.Handler {
		return full(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := jwtassertion.ContextWithScopes(r.Context(),
				rbac.MCPServerUpdate.Scope()+" "+rbac.MCPServerRead.Scope())
			next.ServeHTTP(w, r.WithContext(ctx))
		}))
	}
	fullApp := &projectProxyHarness{
		t: t, org: testOrgName,
		app: apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: oc}, full),
	}
	limitedApp := &projectProxyHarness{
		t: t, org: testOrgName,
		app: apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: oc}, updateOnly),
	}

	suffix := uuid.NewString()[:8]
	handle := "perm-mcp-" + suffix
	t.Cleanup(func() {
		db.DB(context.Background()).Exec(`DELETE FROM artifacts WHERE ou_id = ? AND handle = ?`, jwtassertion.MockOUID, handle)
	})
	body := map[string]any{"name": "Perm " + suffix, "source": map[string]string{"componentName": "c", "endpointName": "e"}}

	rr := limitedApp.do(http.MethodPut, proj, "/"+handle, body)
	require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())

	require.Equal(t, http.StatusCreated, fullApp.do(http.MethodPut, proj, "/"+handle, body).Code)
	rr = limitedApp.do(http.MethodPut, proj, "/"+handle, body)
	assert.Equal(t, http.StatusOK, rr.Code, "an update needs only the update permission")
}

func mustField(t *testing.T, raw []byte, field string) json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &m))
	return m[field]
}

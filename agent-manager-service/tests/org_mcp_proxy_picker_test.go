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
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// TestOrgMCPProxyRoutesAndProjectProxies drives the org routes over HTTP against a real
// project proxy: the picker list, the read-only get, and the PROJECT_OWNED refusals.
func TestOrgMCPProxyRoutesAndProjectProxies(t *testing.T) {
	projName := "pp-" + uuid.NewString()[:8]
	projUUID := uuid.NewString()
	oc := apitestutils.CreateMockOpenChoreoClient()
	base := oc.GetProjectFunc
	oc.GetProjectFunc = func(ctx context.Context, ns, name string) (*models.ProjectResponse, error) {
		p, err := base(ctx, ns, name)
		if err != nil {
			return nil, err
		}
		p.UUID = projUUID
		return p, nil
	}
	app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: oc}, jwtassertion.NewMockMiddleware(t))
	h := &projectProxyHarness{t: t, app: app, org: testOrgName}
	ouID := jwtassertion.MockOUID
	gdb := db.DB(context.Background())

	orgDo := func(method, suffix string, body any) *httptest.ResponseRecorder {
		t.Helper()
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(method, fmt.Sprintf("/api/v1/orgs/%s/mcp-proxies%s", testOrgName, suffix), bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		return rr
	}

	suffix := uuid.NewString()[:8]
	projHandle := "picker-proj-" + suffix
	orgHandle := "picker-org-" + suffix
	t.Cleanup(func() {
		gdb.Exec(`DELETE FROM artifacts WHERE ou_id = ? AND handle LIKE ?`, ouID, "%"+suffix)
	})

	require.Equal(t, http.StatusCreated, h.do(http.MethodPut, projName, "/"+projHandle, map[string]any{
		"name":    "Picker " + suffix,
		"context": "/picker-" + suffix,
		"source":  map[string]string{"componentName": "c", "endpointName": "e"},
	}).Code)
	require.NoError(t, repositories.NewMCPProxyRepo(gdb).Create(context.Background(), gdb,
		&models.MCPProxy{Configuration: models.MCPProxyConfig{Name: "Org " + suffix, Version: "v1"}},
		orgHandle, "Org "+suffix, "v1", ouID, nil, nil))
	// Give the project proxy an upstream so the read-only view has something to strip.
	const secretUpstream = "https://internal-upstream.example.com/mcp"
	require.NoError(t, gdb.Exec(`UPDATE mcp_proxy_endpoints SET configuration = jsonb_build_object('upstream', jsonb_build_object('url', ?::text))
		WHERE mcp_proxy_uuid = (SELECT uuid FROM artifacts WHERE handle = ? AND ou_id = ?)`,
		secretUpstream, projHandle, ouID).Error)

	listIDs := func(rr *httptest.ResponseRecorder) map[string]string {
		t.Helper()
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		var resp models.MCPProxyListResponse
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
		out := map[string]string{}
		for _, it := range resp.List {
			pn := ""
			if it.ProjectName != nil {
				pn = *it.ProjectName
			}
			out[*it.ID] = pn
		}
		return out
	}

	t.Run("list without project shows org proxies only", func(t *testing.T) {
		got := listIDs(orgDo(http.MethodGet, "?limit=100", nil))
		assert.Contains(t, got, orgHandle)
		assert.NotContains(t, got, projHandle)
	})

	t.Run("list with project shows org proxies and that project's", func(t *testing.T) {
		got := listIDs(orgDo(http.MethodGet, "?limit=100&project="+projName, nil))
		assert.Contains(t, got, orgHandle)
		require.Contains(t, got, projHandle)
		assert.Equal(t, projName, got[projHandle])
		assert.Empty(t, got[orgHandle])
	})

	t.Run("list with an unknown project is 404", func(t *testing.T) {
		rr := orgDo(http.MethodGet, "?project=nonexistent-proj", nil)
		require.Equal(t, http.StatusNotFound, rr.Code)
		assert.Equal(t, utils.ErrCodeProjectNotFound, errorCode(t, rr))
	})

	t.Run("get of a project proxy is read-only and hides the upstream", func(t *testing.T) {
		rr := orgDo(http.MethodGet, "/"+projHandle, nil)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.NotContains(t, rr.Body.String(), "internal-upstream")
		var dto models.MCPProxyDTO
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &dto))
		require.NotNil(t, dto.ProjectName)
		assert.Equal(t, projName, *dto.ProjectName)
		for _, ep := range dto.Endpoints {
			assert.Nil(t, ep.Upstream.Main)
		}
	})

	t.Run("org put and delete of a project proxy are 409 PROJECT_OWNED and change nothing", func(t *testing.T) {
		put := orgDo(http.MethodPut, "/"+projHandle, map[string]any{"id": projHandle, "name": "Hijack", "version": "v1"})
		require.Equal(t, http.StatusConflict, put.Code, put.Body.String())
		assert.Equal(t, utils.ErrCodeProjectOwned, errorCode(t, put))

		del := orgDo(http.MethodDelete, "/"+projHandle, nil)
		require.Equal(t, http.StatusConflict, del.Code, del.Body.String())
		assert.Equal(t, utils.ErrCodeProjectOwned, errorCode(t, del))

		var n int64
		require.NoError(t, gdb.Raw(`SELECT count(*) FROM mcp_proxies m JOIN artifacts a ON a.uuid = m.uuid
			WHERE a.handle = ? AND a.ou_id = ? AND m.project_name = ?`, projHandle, ouID, projName).Scan(&n).Error)
		assert.EqualValues(t, 1, n, "the project proxy is untouched")
		assert.Equal(t, http.StatusOK, h.do(http.MethodGet, projName, "/"+projHandle, nil).Code)
	})

	t.Run("scope mutations on a project proxy are 409 PROJECT_OWNED", func(t *testing.T) {
		scopes := "/" + projHandle + "/scopes"
		rr := orgDo(http.MethodPost, scopes, map[string]any{"action": "read"})
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeProjectOwned, errorCode(t, rr))
		rr = orgDo(http.MethodPut, scopes+"/read", map[string]any{"description": "x"})
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeProjectOwned, errorCode(t, rr))
		rr = orgDo(http.MethodDelete, scopes+"/read", nil)
		require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
		assert.Equal(t, utils.ErrCodeProjectOwned, errorCode(t, rr))
	})

	t.Run("org proxies still delete through the org route", func(t *testing.T) {
		rr := orgDo(http.MethodDelete, "/"+orgHandle, nil)
		assert.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
	})
}

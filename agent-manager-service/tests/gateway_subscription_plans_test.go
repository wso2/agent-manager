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
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aihub"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// With the AI Hub on beside the real AI Gateway, an MCP proxy offers the AI Hub's plans and the AI Gateway
// counts the MCP proxies that offer a plan, which keeps it in use. With the AI Hub off, an MCP proxy can't
// offer plans.
func TestGatewayTakesPlansFromTheAIHub(t *testing.T) {
	ctx := context.Background()
	app, host := apitestutils.MakeAppClientWithModules(t, wiring.TestClients{
		PlatformRepository: apitestutils.CreateMockPlatformRepository(),
		SecretMgmtClient:   apitestutils.CreateMockSecretManagementClient(),
	}, jwtassertion.NewMockMiddleware(t), aihub.Name)
	call := func(method, path string, body any) (int, []byte) {
		var reader *bytes.Reader
		if body == nil {
			reader = bytes.NewReader(nil)
		} else {
			raw, err := json.Marshal(body)
			require.NoError(t, err)
			reader = bytes.NewReader(raw)
		}
		req := httptest.NewRequest(method, path, reader)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		return rr.Code, rr.Body.Bytes()
	}
	proxies := fmt.Sprintf("/m/ai-gateway/orgs/%s/mcp-proxies", testOrgName)
	ensureOrgRow(t, jwtassertion.MockOUID)
	env := uuid.MustParse(apitestutils.DefaultEnvironmentUUID)
	dbtest.EnvironmentNamed(t, db.GetDB(), jwtassertion.MockOUID, env, "default-"+env.String()[:8])
	gwtest.GatewayIn(t, db.GetDB(), jwtassertion.MockOUID, gwmodels.GatewayRoleEgress, env)
	proxy := func(id string, plans []string) map[string]any {
		return map[string]any{
			"id": id, "name": "Plans " + id, "version": "v1.0", "context": "/" + id,
			"endpoints": []map[string]any{{
				"id": "primary", "upstream": map[string]any{"main": map[string]any{"url": "https://1.1.1.1/mcp"}},
				"environments": []map[string]any{{"environmentUuid": env.String()}},
			}},
			"subscriptionPlans": plans,
		}
	}
	suffix := uuid.NewString()[:8]
	plan := "gw-plan-" + suffix

	t.Run("without the AI Hub, an MCP proxy can't offer plans", func(t *testing.T) {
		_ = host.Disable(ctx, aihub.Name) // fails only when it isn't active
		code, body := call(http.MethodPost, proxies, proxy("no-hub-"+suffix, []string{plan}))
		assert.Equal(t, http.StatusBadRequest, code, string(body))
		assert.Contains(t, string(body), "AI Hub module is not installed")
	})

	if _, err := host.Install(ctx, aihub.Name, false); err != nil {
		require.NoError(t, host.Enable(ctx, aihub.Name), "install: %v", err)
	}
	t.Cleanup(func() { _ = host.Disable(context.Background(), aihub.Name) })
	code, body := call(http.MethodPost, plansPath, map[string]any{"id": plan, "displayName": "Gateway plan"})
	require.Equal(t, http.StatusCreated, code, string(body))

	usage := func(t *testing.T) map[string]any {
		t.Helper()
		code, body := call(http.MethodGet, plansPath+"/"+plan, nil)
		require.Equal(t, http.StatusOK, code, string(body))
		var p struct {
			Usage map[string]any `json:"usage"`
		}
		require.NoError(t, json.Unmarshal(body, &p))
		return p.Usage
	}

	t.Run("an MCP proxy offers the plan, which is then in use", func(t *testing.T) {
		assert.Equal(t, false, usage(t)["inUse"])
		code, body := call(http.MethodPost, proxies, proxy("offers-"+suffix, []string{plan}))
		require.Equal(t, http.StatusCreated, code, string(body))
		var created struct {
			SubscriptionPlans []string `json:"subscriptionPlans"`
		}
		require.NoError(t, json.Unmarshal(body, &created))
		assert.Equal(t, []string{plan}, created.SubscriptionPlans)
		u := usage(t)
		assert.Equal(t, true, u["inUse"])
		assert.EqualValues(t, 1, u["mcpProxies"], "counted by the AI Gateway")
		code, body = call(http.MethodDelete, plansPath+"/"+plan, nil)
		assert.Equal(t, http.StatusConflict, code, string(body))
	})

	t.Run("once the MCP proxy is gone, the plan is free", func(t *testing.T) {
		code, body := call(http.MethodDelete, proxies+"/offers-"+suffix, nil)
		require.Equal(t, http.StatusNoContent, code, string(body))
		assert.Equal(t, false, usage(t)["inUse"])
		code, body = call(http.MethodDelete, plansPath+"/"+plan, nil)
		assert.Equal(t, http.StatusNoContent, code, string(body))
	})
}

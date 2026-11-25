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
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// These create and update agents through the core's agent routes with the
// gateway's extension block, which the gateway validates and stores in the
// agent's own transaction.

const svExtProject = "ext-proj"

// svExtAgent is an external agent create request carrying the gateway's
// extension block.
func svExtAgent(name string, ext map[string]any) map[string]any {
	return map[string]any{
		"name": name, "displayName": name,
		"provisioning": map[string]any{"type": "external"},
		"agentType":    map[string]any{"type": "external-agent-api"},
		"extensions":   map[string]any{"ai-gateway": ext},
	}
}

func svAgentsPath(a *routeAPI) string {
	return "/api/v1/orgs/" + a.handle + "/projects/" + svExtProject + "/agents"
}

// An agent created with model and MCP bindings and settings gets them: the
// configurations and the deploy settings read back through the gateway.
func TestSVAgentExtensionCreateStoresBindingsAndSettings(t *testing.T) {
	a := newRouteAPIWithRealCore(t)
	dbtest.EnsureProject(t, a.db, a.org, svExtProject)
	gw := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleBoth)
	provider := unique("svextp")
	agCreateProvider(t, a, provider, gw)
	proxy := unique("svextm")
	agCreateMCPProxy(t, a, proxy)

	name := unique("ext")
	expect(t, a.raw(http.MethodPost, svAgentsPath(a), svExtAgent(name, map[string]any{
		"modelConfig": []any{map[string]any{"providerName": provider}},
		"mcpConfig":   []any{map[string]any{"proxyName": proxy}},
		"settings":    map[string]any{"enableApiKeySecurity": true, "resilienceTimeoutSeconds": 30},
	})), http.StatusAccepted)

	agentID := dbtest.EnsureAgent(t, a.db, a.org, svExtProject, name)
	var bindings []gwsvc.AgentBinding
	require.NoError(t, a.db.Where("agent_id = ?", agentID).Order("kind").Find(&bindings).Error)
	got := map[string]string{}
	for _, b := range bindings {
		got[b.Kind] = b.Target
	}
	assert.Equal(t, map[string]string{"llm": provider, "mcp": proxy}, got, "the agent's bindings are stored with it")

	configs := a.do(http.MethodGet, "/orgs/{org}/projects/"+svExtProject+"/agents/"+name+"/configurations", nil)
	require.Equal(t, http.StatusOK, configs.Code, configs.Body.String())
	assert.Contains(t, configs.Body.String(), `"resilienceTimeoutSeconds":30`, "the settings are stored on create")

	rec := a.raw(http.MethodPut, svAgentsPath(a)+"/"+name, map[string]any{
		"displayName": name, "extensions": map[string]any{"ai-gateway": map[string]any{"settings": map[string]any{"enableApiKeySecurity": false}}},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, errorMessage(t, rec), "accepted on create only")
}

// The extension is validated before the agent is written: a refused block
// answers 400 naming the field, and no agent is created.
func TestSVAgentExtensionRefusesInvalidBlocks(t *testing.T) {
	a := newRouteAPIWithRealCore(t)
	dbtest.EnsureProject(t, a.db, a.org, svExtProject)

	tests := []struct {
		name string
		ext  map[string]any
		want string
	}{
		{"unknown provider", map[string]any{"modelConfig": []any{map[string]any{"providerName": "nope"}}}, "extensions.ai-gateway.modelConfig"},
		{"unknown MCP proxy", map[string]any{"mcpConfig": []any{map[string]any{"proxyName": "nope"}}}, "extensions.ai-gateway.mcpConfig"},
		{"both securities", map[string]any{"settings": map[string]any{"enableApiKeySecurity": true, "enableOAuthSecurity": true}}, "mutually exclusive"},
		{"bad timeout", map[string]any{"settings": map[string]any{"resilienceTimeoutSeconds": -5}}, "extensions.ai-gateway.settings"},
		{"unknown field", map[string]any{"bogus": true}, "bogus"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			name := unique("bad")
			rec := a.raw(http.MethodPost, svAgentsPath(a), svExtAgent(name, tc.ext))
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Contains(t, errorMessage(t, rec), tc.want)
			got := a.raw(http.MethodGet, svAgentsPath(a)+"/"+name, nil)
			assert.Equal(t, http.StatusNotFound, got.Code, "no agent is created")
		})
	}
}

// A name whose previous agent the gateway is still tearing down is refused
// with 409 until the teardown is done.
func TestSVAgentExtensionRefusesANameStillBeingTornDown(t *testing.T) {
	a := newRouteAPIWithRealCore(t)
	dbtest.EnsureProject(t, a.db, a.org, svExtProject)
	name := unique("gone")
	now := time.Now().UTC()
	require.NoError(t, a.db.Create(&gwsvc.AgentTeardown{
		AgentID: uuid.Must(uuid.NewV7()), OrgID: uuid.MustParse(a.org), ProjectName: svExtProject, AgentName: name,
		CreatedAt: now, UpdatedAt: now,
	}).Error)

	rec := a.raw(http.MethodPost, svAgentsPath(a), svExtAgent(name, map[string]any{}))
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, errorMessage(t, rec), "still revoking")
}

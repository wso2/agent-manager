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
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

const (
	gwProject = "gw-proj"
	gwAgent   = "gw-agent"
)

// gwAgentPath is path under the test agent's routes.
func gwAgentPath(path string) string {
	return "/orgs/{org}/projects/" + gwProject + "/agents/" + gwAgent + path
}

// gwCreateProvider creates a catalog LLM provider through the API, placed on gateways (ids), and returns its id.
func gwCreateProvider(t *testing.T, a *routeAPI, handle string, gateways ...string) string {
	t.Helper()
	return gwCreateProviderWith(t, a, handle, nil, gateways...)
}

// gwCreateProviderWith is gwCreateProvider with extra spec fields.
func gwCreateProviderWith(t *testing.T, a *routeAPI, handle string, extra map[string]any, gateways ...string) string {
	t.Helper()
	spec := map[string]any{
		"displayName": "Provider " + handle, "version": "v1.0", "context": "/" + handle, "template": "openai",
		"upstream": map[string]any{"main": map[string]any{
			"url":  "https://api.openai.com/v1",
			"auth": map[string]any{"type": "api-key", "header": "Authorization", "value": "Bearer sk-test"},
		}},
	}
	if gateways != nil {
		spec["gateways"] = gateways
	}
	for k, v := range extra {
		spec[k] = v
	}
	doc := expectInto[gwspec.LlmProviderDocument](t, a.do(http.MethodPost, "/llm-providers", map[string]any{
		"apiVersion": "ai-control-plane.choreolabs.com/v1", "kind": "LlmProvider",
		"metadata": map[string]any{"name": handle}, "spec": spec,
	}), http.StatusCreated)
	require.NotNil(t, doc.Metadata.Uid)
	return *doc.Metadata.Uid
}

// gwMarkDeployed records that the gateway acknowledged the artifact's pending placement: a deployment
// is stored and the placement row reads DEPLOYED, as the gateway's ack would leave it.
func gwMarkDeployed(t *testing.T, a *routeAPI, artifact, gateway string) {
	t.Helper()
	dep := uuid.NewString()
	require.NoError(t, a.db.Exec(`INSERT INTO mod_ai_gateway_artifact_deployments
		(id, org_id, artifact_id, gateway_id, name, content, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		dep, a.org, artifact, gateway, "dep", []byte("{}"), time.Now().UTC()).Error)
	res := a.db.Exec(`UPDATE mod_ai_gateway_artifact_gateway_status
		SET deployment_id = ?, status = 'DEPLOYED', status_desired = 'DEPLOYED', next_attempt_at = NULL
		WHERE artifact_id = ? AND gateway_id = ?`, dep, artifact, gateway)
	require.NoError(t, res.Error)
	require.EqualValues(t, 1, res.RowsAffected, "the provider was placed on the gateway")
}

// gwCatalog is the LLM provider catalog listing.
type gwCatalog struct {
	Entries []gwCatalogEntry `json:"entries"`
	Total   int              `json:"total"`
	Limit   int              `json:"limit"`
	Offset  int              `json:"offset"`
}

// gwCatalogEntry is one catalog entry, with the LLM provider fields a kind=LlmProvider listing adds.
type gwCatalogEntry struct {
	UUID        string                            `json:"uuid"`
	Handle      string                            `json:"handle"`
	Name        string                            `json:"name"`
	Kind        string                            `json:"kind"`
	InCatalog   bool                              `json:"inCatalog"`
	Template    string                            `json:"template"`
	Context     *string                           `json:"context"`
	Deployments []gwspec.CatalogDeploymentSummary `json:"deployments"`

	ModelProviders []gwspec.LLMModelProvider   `json:"modelProviders"`
	Security       *gwspec.SecuritySummary     `json:"security"`
	RateLimiting   *gwspec.RateLimitingSummary `json:"rateLimiting"`
}

// TestGWRoutesCatalog lists catalog entries, filtered by kind, name and hosting, and pages them.
func TestGWRoutesCatalog(t *testing.T) {
	a := newRouteAPI(t)
	gw := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	deployed, idle := unique("cat"), unique("cat")
	window := map[string]any{"duration": 1, "unit": "minute"}
	gwMarkDeployed(t, a, gwCreateProviderWith(t, a, deployed, map[string]any{
		"modelProviders": []any{map[string]any{"id": "openai", "name": "OpenAI", "models": []any{
			map[string]any{"id": "gpt-4o", "name": "GPT-4o", "description": "omni"},
		}}},
		"security": map[string]any{"enabled": true, "apiKey": map[string]any{"enabled": true, "key": "x-api-key", "in": "header"}},
		"rateLimiting": map[string]any{
			"providerLevel": map[string]any{"global": map[string]any{
				"request": map[string]any{"enabled": true, "count": 100, "reset": window},
				"token":   map[string]any{"enabled": true, "count": 5000, "reset": window},
				"cost":    map[string]any{"enabled": true, "amount": 2.5, "reset": window},
			}},
			"consumerLevel": map[string]any{"global": map[string]any{
				"request": map[string]any{"enabled": true, "count": 10, "reset": window},
			}},
		},
	}, gw.UUID.String()), gw.UUID.String())
	gwCreateProvider(t, a, idle)

	all := gwCatalogQuery(t, a, "")
	require.Equal(t, 2, all.Total)
	require.Equal(t, 50, all.Limit)
	handles := []string{}
	for _, e := range all.Entries {
		require.Equal(t, gwmodels.CatalogKindLLMProvider, e.Kind)
		require.True(t, e.InCatalog)
		require.Equal(t, "Provider "+e.Handle, e.Name)
		handles = append(handles, e.Handle)
	}
	require.ElementsMatch(t, []string{deployed, idle}, handles)

	page := gwCatalogQuery(t, a, "?limit=1&offset=1")
	require.Equal(t, 2, page.Total)
	require.Equal(t, 1, page.Offset)
	require.Len(t, page.Entries, 1)

	require.Equal(t, 0, gwCatalogQuery(t, a, "?kind=agent").Total)

	llm := gwCatalogQuery(t, a, "?kind="+gwmodels.CatalogKindLLMProvider)
	require.Equal(t, 2, llm.Total)
	byHandle := map[string]gwCatalogEntry{}
	for _, e := range llm.Entries {
		byHandle[e.Handle] = e
	}
	require.Equal(t, "Provider "+deployed, byHandle[deployed].Name)
	require.Equal(t, "openai", byHandle[deployed].Template)
	require.NotNil(t, byHandle[deployed].Context)
	require.Equal(t, "/"+deployed, *byHandle[deployed].Context)
	require.Len(t, byHandle[deployed].Deployments, 1)
	require.Equal(t, gw.UUID.String(), byHandle[deployed].Deployments[0].GatewayId)
	require.Empty(t, byHandle[idle].Deployments)

	// The provider's models, security and rate limits are summarised.
	got := byHandle[deployed]
	require.Len(t, got.ModelProviders, 1)
	require.Equal(t, "openai", got.ModelProviders[0].Id)
	require.Equal(t, "gpt-4o", got.ModelProviders[0].Models[0].Id)
	require.Equal(t, "omni", *got.ModelProviders[0].Models[0].Description)
	require.NotNil(t, got.Security)
	require.True(t, *got.Security.ApiKeyEnabled)
	require.Equal(t, "header", *got.Security.ApiKeyIn)
	require.NotNil(t, got.RateLimiting)
	provider := got.RateLimiting.ProviderLevel
	require.True(t, *provider.GlobalEnabled)
	require.InDelta(t, 100, provider.Request.Limit, 0.001)
	require.EqualValues(t, 1, provider.Request.ResetDuration)
	require.Equal(t, "minute", provider.Request.ResetUnit)
	require.InDelta(t, 5000, provider.Token.Limit, 0.001)
	require.InDelta(t, 2.5, provider.Cost.Limit, 0.001)
	require.InDelta(t, 10, got.RateLimiting.ConsumerLevel.Request.Limit, 0.001)
	require.Nil(t, byHandle[idle].Security)

	named := gwCatalogQuery(t, a, "?kind="+gwmodels.CatalogKindLLMProvider+"&name="+deployed)
	require.Equal(t, 1, named.Total)
	require.Equal(t, deployed, named.Entries[0].Handle)

	// The environment filter keeps the providers live on its gateways; hosting narrows it further.
	inEnv := "?kind=" + gwmodels.CatalogKindLLMProvider + "&environmentId=" + a.defaultEnv().String()
	live := gwCatalogQuery(t, a, inEnv)
	require.Equal(t, 1, live.Total)
	require.Equal(t, deployed, live.Entries[0].Handle)
	require.Equal(t, gw.UUID.String(), live.Entries[0].Deployments[0].GatewayId)
	require.Equal(t, 1, gwCatalogQuery(t, a, inEnv+"&gatewayHosting=MANAGED").Total)
	require.Equal(t, 0, gwCatalogQuery(t, a, inEnv+"&gatewayHosting=SELF_HOSTED").Total)

	// Another organization sees none of them.
	other := newRouteAPI(t)
	empty := gwCatalogQuery(t, other, "")
	require.Equal(t, 0, empty.Total)
	require.Empty(t, empty.Entries)
}

// gwCatalogQuery lists the LLM provider catalog with query q.
func gwCatalogQuery(t *testing.T, a *routeAPI, q string) gwCatalog {
	t.Helper()
	return expectInto[gwCatalog](t, a.do(http.MethodGet, "/orgs/{org}/catalog"+q, nil), http.StatusOK)
}

// TestGWRoutesCatalogRejections refuses each invalid catalog query with its message.
func TestGWRoutesCatalogRejections(t *testing.T) {
	a := newRouteAPI(t)
	for _, tc := range []struct {
		query, message string
	}{
		{"?limit=0", "Invalid limit parameter. Must be between 1 and 100"},
		{"?limit=1000", "Invalid limit parameter. Must be between 1 and 100"},
		{"?offset=-1", "Invalid offset parameter. Must be non-negative"},
		{"?kind=widget", "Invalid kind parameter. Must be one of: LlmProvider, agent, mcp, McpProxy, McpProxyMapping"},
		{"?name=" + strings.Repeat("n", 256), "Name parameter exceeds maximum length of 255 characters"},
		{"?environmentId=not-a-uuid", "Invalid environment UUID format"},
		{"?environmentId=" + strings.Repeat("a", 101), "Invalid environment UUID format"},
		{"?gatewayHosting=cloud", "Invalid gatewayHosting parameter. Must be one of: MANAGED, SELF_HOSTED"},
	} {
		t.Run(tc.query[:min(len(tc.query), 24)], func(t *testing.T) {
			rec := a.do(http.MethodGet, "/orgs/{org}/catalog"+tc.query, nil)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Equal(t, tc.message, errorMessage(t, rec))
		})
	}
}

// TestGWRoutesDeleteGatewayWithDeployments refuses to delete a gateway that still holds a deployment.
func TestGWRoutesDeleteGatewayWithDeployments(t *testing.T) {
	a := newRouteAPI(t)
	gw := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	gwMarkDeployed(t, a, gwCreateProvider(t, a, unique("dep"), gw.UUID.String()), gw.UUID.String())

	rec := a.do(http.MethodDelete, "/orgs/{org}/gateways/"+gw.UUID.String(), nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.NotEmpty(t, errorMessage(t, rec))

	got := expectInto[gwspec.GatewayResponse](t, a.do(http.MethodGet, "/orgs/{org}/gateways/"+gw.UUID.String(), nil), http.StatusOK)
	require.Len(t, got.Environments, 1, "the refused delete kept the gateway and its mapping")
}

// TestGWRoutesEnvironmentGateways lists the gateways mapped to the environment.
func TestGWRoutesEnvironmentGateways(t *testing.T) {
	a := newRouteAPI(t)
	ingress := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleIngress)
	egress := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	gwtest.GatewayIn(t, a.db, a.org, gwmodels.GatewayRoleEgress) // unmapped

	type gateway struct {
		UUID        string `json:"uuid"`
		Name        string `json:"name"`
		GatewayType string `json:"gatewayType"`
		Hosting     string `json:"hosting"`
		Vhost       string `json:"vhost"`
	}
	list := expectInto[struct {
		Gateways []gateway `json:"gateways"`
	}](t, a.do(http.MethodGet, "/orgs/{org}/environments/default/gateways", nil), http.StatusOK)
	byID := map[string]gateway{}
	for _, g := range list.Gateways {
		byID[g.UUID] = g
	}
	require.Len(t, byID, 2, "the unmapped gateway is not listed")
	require.Equal(t, "INGRESS", byID[ingress.UUID.String()].GatewayType)
	require.Equal(t, "EGRESS", byID[egress.UUID.String()].GatewayType)
	require.Equal(t, ingress.Name, byID[ingress.UUID.String()].Name)
	require.Equal(t, "MANAGED", byID[ingress.UUID.String()].Hosting)
	require.Equal(t, ingress.Vhost, byID[ingress.UUID.String()].Vhost)

	// Either the environment or the gateway read permission admits the caller.
	expect(t, a.do(http.MethodGet, "/orgs/{org}/environments/default/gateways", nil, withScopes(scope(gwperm.Read))), http.StatusOK)
	rec := a.do(http.MethodGet, "/orgs/{org}/environments/default/gateways", nil, withScopes(scope(gwperm.LLMProviderRead)))
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}

// TestGWRoutesAgentDeploymentsAndConfigurations reads an external agent's deployments and gateway configuration.
func TestGWRoutesAgentDeploymentsAndConfigurations(t *testing.T) {
	a := newRouteAPI(t)
	a.agent(gwProject, gwAgent)

	deps := expect(t, a.do(http.MethodGet, gwAgentPath("/deployments"), nil), http.StatusOK)
	require.Contains(t, deps, gwmodels.DefaultEnvironmentName)
	def, ok := deps[gwmodels.DefaultEnvironmentName].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "active", def["status"])
	require.Equal(t, "Default", def["environmentDisplayName"])

	cfg := expectInto[gwspec.AgentConfigurationResponse](t, a.do(http.MethodGet, gwAgentPath("/configurations"), nil), http.StatusOK)
	require.Equal(t, gwProject, cfg.ProjectName)
	require.Equal(t, gwAgent, cfg.AgentName)
	require.Equal(t, gwmodels.DefaultEnvironmentName, cfg.Environment)
	require.Empty(t, cfg.Bindings)
	require.Nil(t, cfg.EnableApiKeySecurity, "nothing is saved yet")

	rec := a.do(http.MethodGet, "/orgs/{org}/projects/no-such-project/agents/"+gwAgent+"/deployments", nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())

	for _, p := range []string{"/deployments", "/configurations"} {
		rec = a.do(http.MethodGet, "/orgs/{org}/projects/"+gwProject+"/agents/no-such-agent"+p, nil)
		require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
		require.Equal(t, "Agent not found", errorMessage(t, rec))
		rec = a.do(http.MethodGet, gwAgentPath(p), nil, withScopes(scope(gwperm.Read)))
		require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	}
}

// TestGWRoutesAgentDeploySettings saves an external agent's deploy settings and reads them back.
func TestGWRoutesAgentDeploySettings(t *testing.T) {
	a := newRouteAPI(t)
	a.agent(gwProject, gwAgent)
	gw := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleBoth)
	path := gwAgentPath("/deploy-settings")

	rec := a.do(http.MethodPut, path, "{bad")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "Invalid request body", errorMessage(t, rec))
	rec = a.do(http.MethodPut, path, map[string]any{"enableApiKeySecurity": true})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Equal(t, "environmentName is required", errorMessage(t, rec))
	rec = a.do(http.MethodPut, "/orgs/{org}/projects/"+gwProject+"/agents/no-such-agent/deploy-settings",
		map[string]any{"environmentName": "default"})
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Equal(t, "Agent not found", errorMessage(t, rec))
	rec = a.do(http.MethodPut, path, map[string]any{"environmentName": "default"}, withScopes(scope(gwperm.AgentRead)))
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())

	// A platform-hosted agent takes its settings elsewhere.
	hosted := a.agent(gwProject, "gw-hosted")
	require.NoError(t, a.db.Exec(`UPDATE agents SET provisioning_type = 'internal' WHERE id = ?`, hosted).Error)
	rec = a.do(http.MethodPut, "/orgs/{org}/projects/"+gwProject+"/agents/gw-hosted/deploy-settings",
		map[string]any{"environmentName": "default"})
	require.Equal(t, http.StatusNotImplemented, rec.Code, rec.Body.String())

	// An agent the gateway does not route has no deploy settings to save.
	rec = a.do(http.MethodPut, path, map[string]any{"environmentName": "default", "enableApiKeySecurity": true})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Equal(t, "Invalid input provided", errorMessage(t, rec))

	// Route the agent through the gateway, then save.
	rec = a.do(http.MethodPut, gwAgentPath("/environments/default/agent-card/source"),
		map[string]any{"url": "https://1.1.1.1/.well-known/agent-card.json"})
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	rec = a.do(http.MethodPut, gwAgentPath("/environments/default/a2a-proxy"), map[string]any{})
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	rec = a.do(http.MethodPut, path, map[string]any{
		"environmentName": "default", "enableApiKeySecurity": true, "resilienceTimeoutSeconds": 45,
	})
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	cfg := expectInto[gwspec.AgentConfigurationResponse](t, a.do(http.MethodGet, gwAgentPath("/configurations"), nil), http.StatusOK)
	require.NotNil(t, cfg.EnableApiKeySecurity)
	require.True(t, *cfg.EnableApiKeySecurity)
	require.NotNil(t, cfg.ResilienceTimeoutSeconds)
	require.EqualValues(t, 45, *cfg.ResilienceTimeoutSeconds)

	// API key and OAuth security are mutually exclusive.
	rec = a.do(http.MethodPut, path, map[string]any{
		"environmentName": "default", "enableOAuthSecurity": true,
		"oauthConfig": map[string]any{"issuers": []string{"https://idp.example.com"}},
	})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	cfg = expectInto[gwspec.AgentConfigurationResponse](t, a.do(http.MethodGet, gwAgentPath("/configurations"), nil), http.StatusOK)
	require.True(t, *cfg.EnableApiKeySecurity, "the refused save changed nothing")

	// The gateway now serves the agent, so it cannot leave the environment.
	rec = a.do(http.MethodDelete, "/orgs/{org}/gateways/"+gw.UUID.String()+"/environments/default", nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Contains(t, errorMessage(t, rec), "The gateway still serves this environment's agents or proxies")
	got := expectInto[gwspec.GatewayResponse](t, a.do(http.MethodGet, "/orgs/{org}/gateways/"+gw.UUID.String(), nil), http.StatusOK)
	require.Len(t, got.Environments, 1)
}

// TestGWRoutesPlatformGatewayFailureSummary reports the fleet's failures to the platform admin organization only.
func TestGWRoutesPlatformGatewayFailureSummary(t *testing.T) {
	a := newRouteAPI(t)
	const path = "/platform/gateways/failure-summary"

	rec := a.do(http.MethodGet, path, nil)
	require.Equal(t, http.StatusForbidden, rec.Code, "an ordinary organization is refused: %s", rec.Body.String())

	cfg := config.GetConfig()
	prev := cfg.PlatformAdminOUID
	cfg.PlatformAdminOUID = a.org
	t.Cleanup(func() { cfg.PlatformAdminOUID = prev })

	// A managed gateway disconnected for an hour is failed.
	failed := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	require.NoError(t, a.db.Model(&gwmodels.Gateway{}).Where("id = ?", failed.UUID).
		Updates(map[string]any{"is_active": false, "updated_at": time.Now().Add(-time.Hour)}).Error)

	type summary struct {
		Total             int64   `json:"total"`
		Considered        int64   `json:"considered"`
		Failed            int64   `json:"failed"`
		Healthy           bool    `json:"healthy"`
		FailurePercentage float64 `json:"failurePercentage"`
		StalenessSeconds  int     `json:"stalenessThresholdSeconds"`
		MaxAgeSeconds     int     `json:"maxAgeSeconds"`
		FailedGateways    []struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			OrganizationID string `json:"organizationId"`
		} `json:"failedGateways"`
	}

	// With details the answer is always 200, and names the failed gateway.
	s := expectInto[summary](t, a.do(http.MethodGet, path+"?includeDetails=true", nil), http.StatusOK)
	require.GreaterOrEqual(t, s.Failed, int64(1))
	require.GreaterOrEqual(t, s.Total, s.Considered)
	require.Equal(t, 300, s.StalenessSeconds)
	require.Equal(t, 604800, s.MaxAgeSeconds)
	var named bool
	for _, g := range s.FailedGateways {
		if g.ID == failed.UUID.String() {
			named = true
			require.Equal(t, failed.Name, g.Name)
			require.Equal(t, a.org, g.OrganizationID)
		}
	}
	require.True(t, named, "the failed gateway is listed")

	// Without details an unhealthy fleet answers 503, a healthy one 200; the body is the same document.
	rec = a.do(http.MethodGet, path, nil)
	s = expectInto[summary](t, rec, rec.Code)
	if s.Healthy {
		require.Equal(t, http.StatusOK, rec.Code)
	} else {
		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	}
	require.Empty(t, s.FailedGateways)
}

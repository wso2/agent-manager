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

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// TestINTRoutesPullServesTheGatewaysDeployedArtifact pins the per-artifact
// pulls: a ZIP of the artifact's YAML for the calling gateway's deployment,
// 404 for an id it has no deployment of, 400 for an id that isn't a UUID.
func TestINTRoutesPullServesTheGatewaysDeployedArtifact(t *testing.T) {
	for _, c := range []struct {
		route, kind, prefix, notFound string
	}{
		{"/agents/", gwmodels.KindAgent, "agent-", "No active deployment found for this agent"},
		{"/mcp-proxies/", gwmodels.KindMCPProxy, "mcp-proxy-", "MCP proxy not found"},
		{"/apis/", gwmodels.KindRestAPI, "api-", "No active deployment found for this API"},
	} {
		t.Run(c.kind, func(t *testing.T) {
			a := newINTAPI(t)
			handle := unique("h")
			artifact := a.intArtifact(c.kind, handle)
			a.intDeploy(a.gateway.UUID, artifact, c.kind, handle, "kind: "+c.kind+"\nmetadata:\n  name: "+handle+"\n")

			content := intZipEntry(t, a.call(http.MethodGet, c.route+artifact.String(), nil), c.prefix+artifact.String()+".yaml")
			require.Contains(t, content, "name: "+handle)

			rec := a.call(http.MethodGet, c.route+uuid.NewString(), nil)
			require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), c.notFound)

			rec = a.call(http.MethodGet, c.route+"not-a-uuid", nil)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), "must be an artifact UUID")

			// Another gateway of the organization has no deployment of the artifact.
			other := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleBoth)
			otherToken, _ := a.intNewToken(other.UUID)
			rec = a.call(http.MethodGet, c.route+artifact.String(), nil, intKey(otherToken))
			require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
		})
	}
}

// TestINTRoutesPullServesADeployedLLMProviderAndProxy pins the LLM provider
// (by id) and proxy (by handle) pulls: the deployed YAML for this gateway, 404
// when it has none.
func TestINTRoutesPullServesADeployedLLMProviderAndProxy(t *testing.T) {
	a := newINTAPI(t)
	providerHandle := unique("provider")
	provider := a.intArtifact(gwmodels.KindLLMProvider, providerHandle)
	require.NoError(t, a.db.Create(&gwmodels.LLMProvider{
		UUID: provider, OrgID: a.org, TemplateHandle: "openai", Status: "CREATED",
	}).Error)
	owner := a.agent("intproj", unique("owner"))
	var projectID string
	require.NoError(t, a.db.Raw(`SELECT project_id FROM agents WHERE id = ?`, owner).Scan(&projectID).Error)
	proxyHandle := unique("proxy")
	proxy := a.intArtifact(gwmodels.KindLLMProxy, proxyHandle)
	require.NoError(t, a.db.Create(&gwmodels.LLMProxy{
		UUID: proxy, OrgID: a.org, ProjectUUID: uuid.MustParse(projectID), ProviderUUID: provider, Status: "deployed",
	}).Error)

	// Placed nowhere yet: the records exist but this gateway has nothing to pull.
	rec := a.call(http.MethodGet, "/llm-providers/"+provider.String(), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "No active deployment found for this LLM provider")
	rec = a.call(http.MethodGet, "/llm-proxies/"+proxyHandle, nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "No active deployment found for this LLM proxy")

	a.intDeploy(a.gateway.UUID, provider, gwmodels.KindLLMProvider, providerHandle,
		"kind: LlmProvider\nmetadata:\n  name: "+providerHandle+"\nspec:\n  upstream:\n    url: https://llm.example.com\n")
	a.intDeploy(a.gateway.UUID, proxy, gwmodels.KindLLMProxy, proxyHandle,
		"kind: LlmProxy\nmetadata:\n  name: "+proxyHandle+"\nspec:\n  provider:\n    id: "+providerHandle+"\n")

	content := intZipEntry(t, a.call(http.MethodGet, "/llm-providers/"+provider.String(), nil),
		"llm-provider-"+provider.String()+".yaml")
	require.Contains(t, content, "name: "+providerHandle)
	content = intZipEntry(t, a.call(http.MethodGet, "/llm-proxies/"+proxyHandle, nil),
		"llm-proxy-"+proxyHandle+".yaml")
	require.Contains(t, content, "name: "+proxyHandle)

	rec = a.call(http.MethodGet, "/llm-proxies/"+unique("unknown"), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "LLM proxy not found")
	for _, id := range []string{uuid.NewString(), unique("unknown")} {
		rec = a.call(http.MethodGet, "/llm-providers/"+id, nil)
		require.Equal(t, http.StatusNotFound, rec.Code, "%s: %s", id, rec.Body.String())
		require.Contains(t, rec.Body.String(), "LLM provider not found")
	}
}

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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// The routes serve one environment (the core rewrites every environment to
// the default one), so these call the wired service directly to cover
// configurations that span several environments.

// svEnvs is a test agent with a gateway serving the default environment and
// staging, and the service that manages its configurations.
type svEnvs struct {
	a       *routeAPI
	gateway *gwmodels.Gateway
	staging uuid.UUID
	svc     gwsvc.AgentConfigurationService
	ctx     context.Context
}

// svSetup maps the default environment's gateway to staging too, unless
// ownStagingGateway, when staging gets a gateway of its own.
func svSetup(t *testing.T, ownStagingGateway bool) *svEnvs {
	t.Helper()
	a, gw := agSetup(t)
	staging := uuid.Must(uuid.NewV7())
	dbtest.EnvironmentNamed(t, a.db, a.org, staging, "staging")
	if ownStagingGateway {
		gwtest.GatewayIn(t, a.db, a.org, gwmodels.GatewayRoleBoth, staging)
	} else {
		gwtest.MapGateway(t, a.db, a.org, gw.UUID, staging)
	}
	ctx := audit.WithRecorder(modkit.WithOrgID(context.Background(), a.org), audit.NewNoopRecorder())
	return &svEnvs{a: a, gateway: gw, staging: staging, svc: aigateway.Current().AgentConfigurationService, ctx: ctx}
}

func svLLM(provider string) gwmodels.EnvModelConfigRequest {
	return gwmodels.EnvModelConfigRequest{ProviderName: provider}
}

// providerOf is the provider each environment's proxy uses.
func svProviders(t *testing.T, resp *gwmodels.AgentModelConfigResponse) map[string]string {
	t.Helper()
	out := map[string]string{}
	for env, m := range resp.EnvModelConfig {
		require.NotNil(t, m.LLMProxy, "environment %s has a proxy", env)
		require.NotNil(t, m.LLMProxy.ProviderName)
		out[env] = *m.LLMProxy.ProviderName
	}
	return out
}

// An LLM configuration follows its environments: an update can change an
// environment's provider, add an environment and drop one, and the agent's
// deletion removes the whole configuration.
func TestSVLLMConfigAcrossEnvironments(t *testing.T) {
	s := svSetup(t, false)
	p1 := unique("svp1")
	p2 := unique("svp2")
	agCreateProvider(t, s.a, p1, s.gateway)
	agCreateProvider(t, s.a, p2, s.gateway)

	created, err := s.svc.Create(s.ctx, s.a.org, agProject, agAgent, gwmodels.CreateAgentModelConfigRequest{
		Name: "models", Type: gwmodels.AgentConfigTypeLLM,
		EnvMappings: map[string]gwmodels.EnvModelConfigRequest{"default": svLLM(p1), "staging": svLLM(p1)},
	}, "tester")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"default": p1, "staging": p1}, svProviders(t, created))
	id := uuid.MustParse(created.UUID)

	updated, err := s.svc.Update(s.ctx, id, s.a.org, agProject, agAgent, gwmodels.UpdateAgentModelConfigRequest{
		Description: "only default", EnvMappings: map[string]gwmodels.EnvModelConfigRequest{"default": svLLM(p2)},
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"default": p2}, svProviders(t, updated), "staging is dropped, default switches provider")

	readBack, err := s.svc.Get(s.ctx, id, s.a.org, agProject, agAgent)
	require.NoError(t, err)
	assert.Equal(t, "only default", readBack.Description)
	assert.Equal(t, map[string]string{"default": p2}, svProviders(t, readBack))

	readded, err := s.svc.Update(s.ctx, id, s.a.org, agProject, agAgent, gwmodels.UpdateAgentModelConfigRequest{
		EnvMappings: map[string]gwmodels.EnvModelConfigRequest{"default": svLLM(p2), "staging": svLLM(p1)},
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"default": p2, "staging": p1}, svProviders(t, readded))

	require.NoError(t, s.svc.DeleteForAgentDeletion(s.ctx, id, s.a.org, agProject, agAgent, true))
	_, err = s.svc.Get(s.ctx, id, s.a.org, agProject, agAgent)
	require.ErrorIs(t, err, gwerr.ErrAgentConfigNotFound)
}

// svEnvNames is the environments a configuration is mapped to.
func svEnvNames(resp *gwmodels.AgentModelConfigResponse) []string {
	out := make([]string, 0, len(resp.EnvModelConfig))
	for env := range resp.EnvModelConfig {
		out = append(out, env)
	}
	return out
}

// An MCP configuration follows its environments too, and removing an
// environment tears down the configuration's mapping there and strips the
// environment from the proxy.
func TestSVMCPConfigAcrossEnvironments(t *testing.T) {
	s := svSetup(t, true)
	proxy := unique("svmcp")
	body := agMCPProxyBody(proxy, s.a.defaultEnv().String())
	endpoint := body["spec"].(map[string]any)["endpoints"].([]any)[0].(map[string]any)
	endpoint["environments"] = []any{
		map[string]any{"environmentUuid": s.a.defaultEnv().String()},
		map[string]any{"environmentUuid": s.staging.String()},
	}
	expect(t, s.a.do(http.MethodPost, "/mcp-proxies", body), http.StatusCreated)

	created, err := s.svc.Create(s.ctx, s.a.org, agProject, agAgent, gwmodels.CreateAgentModelConfigRequest{
		Name: "tools", Type: gwmodels.AgentConfigTypeMCP,
		EnvMappings: map[string]gwmodels.EnvModelConfigRequest{"default": svLLM(proxy), "staging": svLLM(proxy)},
	}, "tester")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"default", "staging"}, svEnvNames(created))
	id := uuid.MustParse(created.UUID)

	narrowed, err := s.svc.UpdateMCP(s.ctx, id, s.a.org, agProject, agAgent, gwmodels.UpdateAgentModelConfigRequest{
		EnvMappings: map[string]gwmodels.EnvModelConfigRequest{"default": svLLM(proxy)},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"default"}, svEnvNames(narrowed))

	widened, err := s.svc.UpdateMCP(s.ctx, id, s.a.org, agProject, agAgent, gwmodels.UpdateAgentModelConfigRequest{
		EnvMappings: map[string]gwmodels.EnvModelConfigRequest{"default": svLLM(proxy), "staging": svLLM(proxy)},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"default", "staging"}, svEnvNames(widened))

	// Nothing calls this yet (no environment is ever deleted in this
	// distribution); it is reached through the concrete service.
	cleaner, ok := s.svc.(interface {
		CleanupEnvironmentMCPArtifacts(ctx context.Context, ouID string, envUUID uuid.UUID, envName string) error
	})
	require.True(t, ok)
	require.NoError(t, cleaner.CleanupEnvironmentMCPArtifacts(s.ctx, s.a.org, s.staging, "staging"))
	readBack, err := s.svc.GetMCP(s.ctx, id, s.a.org, agProject, agAgent)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"default"}, svEnvNames(readBack), "the removed environment's mapping is gone")
	proxyDoc := expect(t, s.a.do(http.MethodGet, "/mcp-proxies/"+proxy, nil), http.StatusOK)
	assert.NotContains(t, fmt.Sprint(proxyDoc), s.staging.String(), "the proxy no longer names the removed environment")
	assert.Contains(t, fmt.Sprint(proxyDoc), s.a.defaultEnv().String())

	require.NoError(t, s.svc.DeleteMCP(s.ctx, id, s.a.org, agProject, agAgent))
	_, err = s.svc.GetMCP(s.ctx, id, s.a.org, agProject, agAgent)
	require.ErrorIs(t, err, gwerr.ErrAgentConfigNotFound)
}

// svJSON is v encoded as JSON, for asserting that a response names something.
func svJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return string(raw)
}

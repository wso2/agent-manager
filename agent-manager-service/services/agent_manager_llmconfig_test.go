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

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// spyConfigService records the request passed to Create and stubs the LLM env var resolver.
// Only Create and BuildSystemManagedEnvVarsFromConfig are exercised; the embedded interface
// satisfies the rest (and panics if any other method is called).
type spyConfigService struct {
	AgentConfigurationService
	lastReq        models.CreateAgentModelConfigRequest
	systemEnvVars  []client.EnvVar
	systemEnvVarsE error
}

func (s *spyConfigService) Create(_ context.Context, _, _, _ string,
	req models.CreateAgentModelConfigRequest, _ string,
) (*models.AgentModelConfigResponse, error) {
	s.lastReq = req
	return &models.AgentModelConfigResponse{}, nil
}

func (s *spyConfigService) BuildSystemManagedEnvVarsFromConfig(_ context.Context, _, _, _, _ string) ([]client.EnvVar, error) {
	return s.systemEnvVars, s.systemEnvVarsE
}

// LLM providers are deliberately configured in the entry environment only: createLLMConfig
// rolls the whole configuration back when a provider's gateway cannot be resolved in an
// environment, so spanning the pipeline would make agent creation fail whenever a higher
// environment is not yet set up. See createAgentLLMConfigs.
func TestCreateAgentLLMConfigs_KeysUnderFirstEnv(t *testing.T) {
	spy := &spyConfigService{}
	s := &agentManagerService{agentConfigurationService: spy}

	req := &spec.CreateAgentRequest{
		Name:        "my-agent",
		ModelConfig: []spec.ModelConfigRequest{{ProviderName: "openai"}},
	}

	err := s.createAgentLLMConfigs(context.Background(), "org", "proj", "Development", req)
	require.NoError(t, err)

	require.Len(t, spy.lastReq.EnvMappings, 1, "exactly one env mapping")
	got, ok := spy.lastReq.EnvMappings["Development"]
	require.True(t, ok, "config must be keyed under firstEnv")
	require.Equal(t, "openai", got.ProviderName)
}

// An MCP connection is environment-agnostic — the same proxy backs every environment — so
// creation binds it across the whole pipeline for the same reason.
func TestCreateAgentMCPConfigs_KeysUnderEveryPipelineEnvironment(t *testing.T) {
	spy := &spyConfigService{}
	s := &agentManagerService{agentConfigurationService: spy}

	req := &spec.CreateAgentRequest{
		Name:      "my-agent",
		McpConfig: []spec.MCPConfigRequest{{ProxyName: "booking"}},
	}

	err := s.createAgentMCPConfigs(context.Background(), "org", "proj",
		[]string{"Development", "Staging"}, req)
	require.NoError(t, err)

	require.Len(t, spy.lastReq.EnvMappings, 2, "every pipeline environment must be mapped")
	for _, envName := range []string{"Development", "Staging"} {
		got, ok := spy.lastReq.EnvMappings[envName]
		require.Truef(t, ok, "MCP proxy must be bound in %s", envName)
		// createMCPConfig reads the MCP proxy handle from ProviderName.
		require.Equal(t, "booking", got.ProviderName)
	}
}

// TestMergeKindWorkloadSystemEnvVars_InjectsLLMEnvVars verifies that for a kind-sourced agent
// with an LLM configuration, the resolved system-managed LLM env vars are appended to the
// user-supplied env vars that get baked into the Workload CR. Regression test for the bug where
// LLM provider keys were written to the (unused) Component workflow params instead of the Workload.
func TestMergeKindWorkloadSystemEnvVars_InjectsLLMEnvVars(t *testing.T) {
	llmVars := []client.EnvVar{
		{Key: "OPENAI_BASE_URL", Value: "https://gw/openai"},
		{Key: "OPENAI_API_KEY", ValueFrom: &client.EnvVarValueFrom{
			SecretKeyRef: &client.SecretKeyRef{Name: "secret-ref", Key: "api-key"},
		}},
	}
	spy := &spyConfigService{systemEnvVars: llmVars}
	s := &agentManagerService{agentConfigurationService: spy}

	userVars := []client.EnvVar{{Key: "USER_VAR", Value: "v"}}
	got, err := s.mergeKindWorkloadSystemEnvVars(context.Background(), "my-agent", "org", "proj", "Development", userVars)
	require.NoError(t, err)
	require.Equal(t, append(append([]client.EnvVar{}, userVars...), llmVars...), got,
		"user env vars must be preserved and LLM env vars appended")
}

// TestMergeKindWorkloadSystemEnvVars_InjectsMCPEnvVars verifies that a kind-sourced agent
// configured with ONLY an MCP connection (no LLM provider) still gets its system-managed MCP env
// vars baked into the Workload CR. Regression test for the bug where the injection was gated on
// the presence of a model config, so an MCP-only agent deployed with no MCP URL or API key in its
// container and failed on every tool call.
func TestMergeKindWorkloadSystemEnvVars_InjectsMCPEnvVars(t *testing.T) {
	mcpVars := []client.EnvVar{
		{Key: "MY_AGENT_MCP_1_URL", Value: "https://gw/default/booking/mcp"},
		{Key: "MY_AGENT_MCP_1_API_KEY", ValueFrom: &client.EnvVarValueFrom{
			SecretKeyRef: &client.SecretKeyRef{Name: "secret-ref", Key: "api-key"},
		}},
	}
	spy := &spyConfigService{systemEnvVars: mcpVars}
	s := &agentManagerService{agentConfigurationService: spy}

	userVars := []client.EnvVar{{Key: "USER_VAR", Value: "v"}}
	got, err := s.mergeKindWorkloadSystemEnvVars(context.Background(), "my-agent", "org", "proj", "Development", userVars)
	require.NoError(t, err)
	require.Equal(t, append(append([]client.EnvVar{}, userVars...), mcpVars...), got,
		"user env vars must be preserved and MCP env vars appended")
}

// TestMergeKindWorkloadSystemEnvVars_NoSystemConfig verifies that an agent whose configs yield no
// system-managed vars gets its user env vars back unchanged. The resolver is still consulted — it
// is the authority on which configs exist — and reports the empty result itself.
func TestMergeKindWorkloadSystemEnvVars_NoSystemConfig(t *testing.T) {
	spy := &spyConfigService{systemEnvVars: nil}
	s := &agentManagerService{agentConfigurationService: spy}

	userVars := []client.EnvVar{{Key: "USER_VAR", Value: "v"}}
	got, err := s.mergeKindWorkloadSystemEnvVars(context.Background(), "my-agent", "org", "proj", "Development", userVars)
	require.NoError(t, err)
	require.Equal(t, userVars, got)
}

// TestMergeKindWorkloadSystemEnvVars_ResolverError verifies the resolver error is propagated so the
// caller can roll back the partially-created agent rather than deploying without system keys.
func TestMergeKindWorkloadSystemEnvVars_ResolverError(t *testing.T) {
	resolverErr := errors.New("boom")
	spy := &spyConfigService{systemEnvVarsE: resolverErr}
	s := &agentManagerService{agentConfigurationService: spy}

	_, err := s.mergeKindWorkloadSystemEnvVars(context.Background(), "my-agent", "org", "proj", "Development", nil)
	require.ErrorIs(t, err, resolverErr, "resolver error must stay unwrappable so callers can inspect it")
}

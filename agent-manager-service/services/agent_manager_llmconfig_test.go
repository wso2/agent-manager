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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// spyConfigService records the request passed to Create.
// Only Create is exercised; the embedded interface
// satisfies the rest (and panics if any other method is called).
type spyConfigService struct {
	AgentConfigurationService
	lastReq models.CreateAgentModelConfigRequest
}

func (s *spyConfigService) Create(_ context.Context, _, _, _ string,
	req models.CreateAgentModelConfigRequest, _ string,
) (*models.AgentModelConfigResponse, error) {
	s.lastReq = req
	return &models.AgentModelConfigResponse{}, nil
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
	// Agent creation is the only caller allowed to defer a broken higher environment; a
	// direct MCP create stays strict (see createMCPConfig).
	require.True(t, spy.lastReq.DeferUnbindableMCPEnvs, "agent creation must opt into deferred binding")
}

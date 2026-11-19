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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

func TestListAgentEnvironmentResources_ReadsProvidersGuardrailsAndTools(t *testing.T) {
	devUUID, goneEnvUUID := uuid.New(), uuid.New()
	llmConfigUUID := uuid.New()
	openaiUUID, deletedProviderUUID := uuid.New(), uuid.New()
	endpointArtifact := uuid.New()
	tools := []map[string]interface{}{{"name": "search_issues"}, {"name": "get_pull_request"}}
	proxyTools := []map[string]interface{}{{"name": "only-on-the-proxy"}}

	configRepo := &repomocks.AgentConfigurationRepositoryMock{
		ListByAgentAndTypeFunc: func(_ context.Context, _, _, _ string, typeID uint, _, _ int) ([]models.AgentConfiguration, error) {
			assert.Equal(t, models.AgentConfigTypeIDLLM, typeID)
			return []models.AgentConfiguration{{UUID: llmConfigUUID}}, nil
		},
		GetByUUIDFunc: func(_ context.Context, configUUID uuid.UUID, _ string) (*models.AgentConfiguration, error) {
			require.Equal(t, llmConfigUUID, configUUID)
			return &models.AgentConfiguration{EnvMappings: []models.EnvAgentModelMapping{
				{
					EnvironmentUUID:     devUUID,
					LLMProxy:            &models.LLMProxy{ProviderUUID: openaiUUID},
					PolicyConfiguration: models.LLMPolicies{{Name: "pii-masking"}, {Name: "word-count"}},
				},
				// A provider that was deleted is left out.
				{EnvironmentUUID: devUUID, LLMProxy: &models.LLMProxy{ProviderUUID: deletedProviderUUID}},
				// An environment that no longer exists is left out.
				{EnvironmentUUID: goneEnvUUID, LLMProxy: &models.LLMProxy{ProviderUUID: openaiUUID}},
			}}, nil
		},
		ListMCPConfigsByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.AgentConfiguration, error) {
			return []models.AgentConfiguration{{EnvMCPMappings: []models.EnvAgentMCPMapping{{
				EnvironmentUUID: devUUID,
				ArtifactUUID:    endpointArtifact,
				MCPProxy: &models.MCPProxy{
					Artifact:      &models.Artifact{Handle: "github-mcp", Name: "GitHub MCP"},
					Configuration: models.MCPProxyConfig{Capabilities: &models.MCPProxyCapabilities{Tools: &proxyTools}},
					Endpoints: []models.MCPProxyEndpoint{{
						Configuration: models.MCPEndpointConfig{Capabilities: &models.MCPProxyCapabilities{Tools: &tools}},
						Environments:  []models.MCPProxyEndpointEnvironment{{ArtifactUUID: endpointArtifact}},
					}},
				},
			}}}}, nil
		},
	}
	providerRepo := &repomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc: func(providerID string, _ string) (*models.LLMProvider, error) {
			if providerID == deletedProviderUUID.String() {
				return nil, gorm.ErrRecordNotFound
			}
			return &models.LLMProvider{
				Artifact:      &models.Artifact{Handle: "openai", Name: "OpenAI"},
				Configuration: models.LLMProviderConfig{Policies: []models.LLMPolicy{{Name: "pii-masking"}, {Name: "content-safety"}}},
			}, nil
		},
	}
	svc := &agentConfigurationService{
		agentConfigRepo: configRepo,
		llmProviderRepo: providerRepo,
		infraResourceManager: stubInfraManager{listOrgEnvs: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{UUID: devUUID.String(), Name: "development"}}, nil
		}},
	}

	resources, err := svc.ListAgentEnvironmentResources(context.Background(), "acme", "proj1", "notes")

	require.NoError(t, err)
	assert.Equal(t, map[string]*models.AgentEnvironmentResources{
		"development": {
			LLMProviders: []models.AgentRegistryResourceRef{{ID: "openai", DisplayName: "OpenAI"}},
			// The agent's own guardrails, then the provider's, each once.
			Guardrails: []string{"pii-masking", "word-count", "content-safety"},
			// The tools of the endpoint deployed as the agent's artifact, not the proxy's own.
			MCPServers: []models.AgentRegistryResourceRef{
				{ID: "github-mcp", DisplayName: "GitHub MCP", Tools: []string{"search_issues", "get_pull_request"}},
			},
		},
	}, resources)
}

func TestMCPToolNames_UsesTheEndpointInTheMappingsEnvironment(t *testing.T) {
	devUUID, prodUUID := uuid.New(), uuid.New()
	devTools := []map[string]interface{}{{"name": "greet_new"}}
	prodTools := []map[string]interface{}{{"name": "greet_old"}}
	proxy := &models.MCPProxy{Endpoints: []models.MCPProxyEndpoint{
		{
			Configuration: models.MCPEndpointConfig{Capabilities: &models.MCPProxyCapabilities{Tools: &devTools}},
			Environments:  []models.MCPProxyEndpointEnvironment{{EnvironmentUUID: devUUID, ArtifactUUID: uuid.New()}},
		},
		{
			Configuration: models.MCPEndpointConfig{Capabilities: &models.MCPProxyCapabilities{Tools: &prodTools}},
			Environments:  []models.MCPProxyEndpointEnvironment{{EnvironmentUUID: prodUUID, ArtifactUUID: uuid.New()}},
		},
	}}
	// The agent's mapping has its own artifact, which no endpoint is deployed as.
	mapping := models.EnvAgentMCPMapping{EnvironmentUUID: devUUID, ArtifactUUID: uuid.New(), MCPProxy: proxy}

	assert.Equal(t, []string{"greet_new"}, mcpToolNames(mapping))
}

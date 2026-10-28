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
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

func TestSystemManagedMCPURLReturnsProxyURLForMatchingMapping(t *testing.T) {
	configUUID := uuid.New()
	envUUID := uuid.New()
	mappingArtifactUUID := uuid.New()
	sharedArtifactUUID := uuid.New()
	gatewayUUID := uuid.New()
	contextPath := "/shared-mcp"
	svc := &agentConfigurationService{
		envMCPMappingRepo: &repomocks.EnvAgentMCPMappingRepositoryMock{
			ListByConfigFunc: func(_ context.Context, gotConfigUUID uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
				require.Equal(t, configUUID, gotConfigUUID)
				return []models.EnvAgentMCPMapping{
					{
						ConfigUUID:      configUUID,
						EnvironmentUUID: envUUID,
						MCPProxyUUID:    uuid.New(),
						ArtifactUUID:    mappingArtifactUUID,
						MCPProxy: &models.MCPProxy{
							Configuration: models.MCPProxyConfig{
								Context: &contextPath,
							},
							Endpoints: []models.MCPProxyEndpoint{
								{
									Environments: []models.MCPProxyEndpointEnvironment{
										{EnvironmentUUID: envUUID, ArtifactUUID: sharedArtifactUUID},
									},
								},
							},
						},
					},
				}, nil
			},
		},
		mcpProxyService: &MCPProxyService{
			deploymentRepo: &repomocks.DeploymentRepositoryMock{
				GetDeployedGatewaysByProviderFunc: func(gotArtifactUUID uuid.UUID, gotOrg string) ([]string, error) {
					require.Equal(t, sharedArtifactUUID, gotArtifactUUID)
					require.Equal(t, "org", gotOrg)
					return []string{gatewayUUID.String()}, nil
				},
			},
		},
		gatewayRepo: &repomocks.GatewayRepositoryMock{
			EnvironmentMappingExistsFunc: func(gotGatewayID string, gotEnvID string) (bool, error) {
				require.Equal(t, gatewayUUID.String(), gotGatewayID)
				require.Equal(t, envUUID.String(), gotEnvID)
				return true, nil
			},
			GetByUUIDFunc: func(gotGatewayID string) (*models.Gateway, error) {
				require.Equal(t, gatewayUUID.String(), gotGatewayID)
				// RuntimeURL seeded so this asserts MCP stays on the public vhost rather
				// than merely lacking an internal address to switch to.
				return &models.Gateway{
					UUID:       gatewayUUID,
					Vhost:      "https://gateway.example.com",
					RuntimeURL: "http://api-platform-acme-dev-gw-gateway-gateway-runtime.acme-dev:22893",
				}, nil
			},
		},
	}

	url, err := svc.systemManagedMCPURL(context.Background(), &models.AgentConfiguration{
		UUID:        configUUID,
		Name:        "tools",
		ProjectName: "project",
		AgentID:     "agent",
	}, "org", "dev", envUUID)

	require.NoError(t, err)
	require.Equal(t, "https://gateway.example.com/shared-mcp/mcp", url)
}

func TestSystemManagedMCPURLMissingSharedArtifactReturnsError(t *testing.T) {
	configUUID := uuid.New()
	envUUID := uuid.New()
	svc := &agentConfigurationService{
		envMCPMappingRepo: &repomocks.EnvAgentMCPMappingRepositoryMock{
			ListByConfigFunc: func(_ context.Context, gotConfigUUID uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
				require.Equal(t, configUUID, gotConfigUUID)
				return []models.EnvAgentMCPMapping{
					{
						ConfigUUID:      configUUID,
						EnvironmentUUID: envUUID,
						MCPProxyUUID:    uuid.New(),
						ArtifactUUID:    uuid.New(),
						MCPProxy: &models.MCPProxy{
							Endpoints: []models.MCPProxyEndpoint{
								{
									Environments: []models.MCPProxyEndpointEnvironment{
										{EnvironmentUUID: envUUID, ArtifactUUID: uuid.Nil},
									},
								},
							},
						},
					},
				}, nil
			},
		},
	}

	url, err := svc.systemManagedMCPURL(context.Background(), &models.AgentConfiguration{
		UUID: configUUID,
	}, "org", "dev", envUUID)

	require.ErrorContains(t, err, "MCP proxy shared artifact not found")
	require.Empty(t, url)
}

func TestSystemManagedMCPURLMissingEnvMappingReturnsEmptyURL(t *testing.T) {
	configUUID := uuid.New()
	targetEnvUUID := uuid.New()
	otherEnvUUID := uuid.New()
	svc := &agentConfigurationService{
		envMCPMappingRepo: &repomocks.EnvAgentMCPMappingRepositoryMock{
			ListByConfigFunc: func(_ context.Context, gotConfigUUID uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
				require.Equal(t, configUUID, gotConfigUUID)
				return []models.EnvAgentMCPMapping{
					{
						ConfigUUID:      configUUID,
						EnvironmentUUID: otherEnvUUID,
						MCPProxyUUID:    uuid.New(),
						ArtifactUUID:    uuid.New(),
					},
				}, nil
			},
		},
	}

	url, err := svc.systemManagedMCPURL(context.Background(), &models.AgentConfiguration{
		UUID: configUUID,
	}, "org", "dev", targetEnvUUID)

	require.NoError(t, err)
	require.Empty(t, url)
}

// TestSystemManagedLLMURLUsesGatewayRuntimeURL pins the address a platform-hosted agent's
// pod is handed for its LLM proxy. The pod's NetworkPolicy only permits egress to gateway
// namespaces on the runtime port, so the public vhost here is a dead address.
func TestSystemManagedLLMURLUsesGatewayRuntimeURL(t *testing.T) {
	gateway := newGateway(t, models.GatewayRoleEgress, true)
	gateway.Vhost = "https://gateway.example.com"
	gateway.RuntimeURL = "http://api-platform-acme-dev-gw-gateway-gateway-runtime.acme-dev:22893"

	url, err := callSystemManagedLLMURL(t, gateway, "/llm/proxy")

	require.NoError(t, err)
	require.Equal(t, "http://api-platform-acme-dev-gw-gateway-gateway-runtime.acme-dev:22893/llm/proxy", url)
}

// TestSystemManagedLLMURLFailsClosedWithoutRuntimeURL guards the fallback: a gateway with no
// in-cluster address must abort the injection rather than hand the pod a vhost it cannot reach.
func TestSystemManagedLLMURLFailsClosedWithoutRuntimeURL(t *testing.T) {
	gateway := newGateway(t, models.GatewayRoleEgress, true)
	gateway.Vhost = "https://gateway.example.com"
	gateway.RuntimeURL = ""

	_, err := callSystemManagedLLMURL(t, gateway, "/llm/proxy")

	require.ErrorIs(t, err, errGatewayRuntimeURLUnregistered)
}

// TestSystemManagedLLMURLFallsBackToVhostWhenEnabled covers the cloud gateway: nothing
// registers a runtimeUrl there, and the pod reaches the gateway over the public vhost, so
// with the fallback enabled the injection must resolve rather than abort.
func TestSystemManagedLLMURLFallsBackToVhostWhenEnabled(t *testing.T) {
	enableGatewayPublicVhostFallback(t)
	gateway := newGateway(t, models.GatewayRoleEgress, true)
	gateway.Vhost = "https://gateway.example.com"
	gateway.RuntimeURL = ""

	url, err := callSystemManagedLLMURL(t, gateway, "/llm/proxy")

	require.NoError(t, err)
	require.Equal(t, "https://gateway.example.com/llm/proxy", url)
}

// callSystemManagedLLMURL resolves the LLM URL for a config whose single mapping is deployed
// to gateway, reusing gatewayFixtureRepo so gateway lookup behaves as the real SQL does.
func callSystemManagedLLMURL(t *testing.T, gateway *models.Gateway, contextPath string) (string, error) {
	t.Helper()
	configUUID, envUUID, proxyUUID := uuid.New(), uuid.New(), uuid.New()
	svc := &agentConfigurationService{
		envMappingRepo: &repomocks.EnvAgentModelMappingRepositoryMock{
			GetByConfigAndEnvFunc: func(_ context.Context, gotConfigUUID, gotEnvUUID uuid.UUID) (*models.EnvAgentModelMapping, error) {
				require.Equal(t, configUUID, gotConfigUUID)
				require.Equal(t, envUUID, gotEnvUUID)
				return &models.EnvAgentModelMapping{
					ConfigUUID:      configUUID,
					EnvironmentUUID: envUUID,
					LLMProxyUUID:    proxyUUID,
					LLMProxy: &models.LLMProxy{
						UUID:          proxyUUID,
						Handle:        "acme-openai-proxy",
						Configuration: models.LLMProxyConfig{Context: &contextPath},
					},
				}, nil
			},
		},
		llmProxyDeploymentService: &LLMProxyDeploymentService{
			proxyRepo: &repomocks.LLMProxyRepositoryMock{
				GetByIDFunc: func(gotProxyID, gotOuID string) (*models.LLMProxy, error) {
					require.Equal(t, "acme-openai-proxy", gotProxyID)
					require.Equal(t, "org", gotOuID)
					return &models.LLMProxy{UUID: proxyUUID}, nil
				},
			},
			deploymentRepo: &repomocks.DeploymentRepositoryMock{
				GetDeploymentsWithStateFunc: func(artifactUUID, orgUUID string, _, _ *string, _ int) ([]*models.Deployment, error) {
					require.Equal(t, proxyUUID.String(), artifactUUID)
					require.Equal(t, "org", orgUUID)
					return []*models.Deployment{{GatewayUUID: gateway.UUID}}, nil
				},
			},
		},
		gatewayRepo: gatewayFixtureRepo(t, envUUID.String(), []*models.Gateway{gateway}),
	}

	return svc.systemManagedLLMURL(context.Background(), &models.AgentConfiguration{
		UUID:        configUUID,
		Name:        "model",
		ProjectName: "project",
		AgentID:     "agent",
	}, "org", "dev", envUUID)
}

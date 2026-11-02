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
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// mcpURLTestFixture bundles the UUIDs and mocks shared by the three regression tests below:
// a mapping whose per-agent key-holder artifact (ArtifactUUID) never gets a deployment_status
// row, and whose MCP proxy carries the shared per-environment artifact that the gateway
// actually deployed against.
type mcpURLTestFixture struct {
	envUUID            uuid.UUID
	sharedArtifactUUID uuid.UUID
	keyHolderUUID      uuid.UUID
	gatewayUUID        uuid.UUID
	ouID               string
	mapping            models.EnvAgentMCPMapping
	gatewayRepo        *repomocks.GatewayRepositoryMock
	deploymentRepo     *repomocks.DeploymentRepositoryMock
}

func newMCPURLTestFixture() *mcpURLTestFixture {
	envUUID := uuid.New()
	sharedArtifactUUID := uuid.New()
	keyHolderUUID := uuid.New()
	gatewayUUID := uuid.New()
	ouID := "org1"

	proxyContext := "/proxy-ctx"
	proxy := &models.MCPProxy{
		UUID:          uuid.New(),
		Configuration: models.MCPProxyConfig{Name: "proxy1", Context: &proxyContext},
		Endpoints: []models.MCPProxyEndpoint{
			{
				UUID: uuid.New(),
				Environments: []models.MCPProxyEndpointEnvironment{
					{EnvironmentUUID: envUUID, ArtifactUUID: sharedArtifactUUID},
				},
			},
		},
	}

	mapping := models.EnvAgentMCPMapping{
		EnvironmentUUID: envUUID,
		MCPProxyUUID:    proxy.UUID,
		ArtifactUUID:    keyHolderUUID, // the per-agent key-holder artifact; never has a deployment row
		MCPProxy:        proxy,
	}

	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(_ context.Context, orgUUID string, artifactUUID uuid.UUID) ([]string, error) {
			// Only the shared artifact has a deployment row.
			if artifactUUID == sharedArtifactUUID {
				return []string{gatewayUUID.String()}, nil
			}
			return nil, nil
		},
	}

	gatewayRepo := &repomocks.GatewayRepositoryMock{
		EnvironmentMappingExistsFunc: func(gatewayID string, environmentID string) (bool, error) {
			return gatewayID == gatewayUUID.String() && environmentID == envUUID.String(), nil
		},
		GetByUUIDFunc: func(gatewayId string) (*models.Gateway, error) {
			if gatewayId == gatewayUUID.String() {
				return &models.Gateway{UUID: gatewayUUID, Vhost: "https://gw.example.com"}, nil
			}
			return nil, gorm.ErrRecordNotFound
		},
		// No fallback gateway for the environment: forces resolveEgressGatewayForEnvironment
		// to fail when the deployment-row lookup above didn't already resolve a gateway.
		ListWithFiltersFunc: func(filters repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return nil, nil
		},
	}

	return &mcpURLTestFixture{
		envUUID:            envUUID,
		sharedArtifactUUID: sharedArtifactUUID,
		keyHolderUUID:      keyHolderUUID,
		gatewayUUID:        gatewayUUID,
		ouID:               ouID,
		mapping:            mapping,
		gatewayRepo:        gatewayRepo,
		deploymentRepo:     deploymentRepo,
	}
}

func (f *mcpURLTestFixture) newService() *agentConfigurationService {
	return &agentConfigurationService{
		gatewayRepo:     f.gatewayRepo,
		mcpProxyService: &MCPProxyService{deploymentRepo: f.deploymentRepo},
		logger:          discardLogger(),
	}
}

// buildConfigResponse must populate the MCP proxy URL from the proxy's shared
// per-environment artifact, not the per-agent key-holder artifact — the latter has
// no deployment_status row, so the gateway lookup silently returns no URL.
func TestBuildConfigResponse_MCPURLUsesSharedArtifact(t *testing.T) {
	f := newMCPURLTestFixture()
	svc := f.newService()
	svc.infraResourceManager = stubInfraManager{
		listOrgEnvs: func(ctx context.Context, ouID string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{UUID: f.envUUID.String(), Name: "dev"}}, nil
		},
	}

	config := &models.AgentConfiguration{
		UUID:           uuid.New(),
		OUID:           f.ouID,
		ProjectName:    "proj1",
		AgentID:        "agent1",
		Name:           "cfg1",
		EnvMCPMappings: []models.EnvAgentMCPMapping{f.mapping},
	}

	resp, err := svc.buildConfigResponse(context.Background(), config, true)

	require.NoError(t, err)
	got := resp.EnvModelConfig["dev"].LLMProxy
	require.NotNil(t, got)
	require.NotNil(t, got.URL, "MCP URL must be populated; passing the key-holder artifact leaves it nil")
}

// buildExternalAgentConfigResponse must resolve the gateway from the shared artifact for
// external agents too, when no one-time credential URL was already provided.
func TestExternalAgentConfig_MCPURLUsesSharedArtifact(t *testing.T) {
	f := newMCPURLTestFixture()
	svc := f.newService()
	svc.infraResourceManager = stubInfraManager{
		listOrgEnvs: func(ctx context.Context, ouID string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{UUID: f.envUUID.String(), Name: "dev"}}, nil
		},
	}

	config := &models.AgentConfiguration{
		UUID:        uuid.New(),
		OUID:        f.ouID,
		ProjectName: "proj1",
		AgentID:     "agent1",
		Name:        "cfg1",
	}
	reloadedConfig := &models.AgentConfiguration{
		UUID:           config.UUID,
		OUID:           config.OUID,
		ProjectName:    config.ProjectName,
		AgentID:        config.AgentID,
		Name:           config.Name,
		EnvMCPMappings: []models.EnvAgentMCPMapping{f.mapping},
	}
	svc.agentConfigRepo = &repomocks.AgentConfigurationRepositoryMock{
		GetByUUIDFunc: func(ctx context.Context, configUUID uuid.UUID, ouID string) (*models.AgentConfiguration, error) {
			return reloadedConfig, nil
		},
	}

	resp, err := svc.buildExternalAgentConfigResponse(context.Background(), config, map[string]envCredentialData{})

	require.NoError(t, err)
	got := resp.EnvModelConfig["dev"].LLMProxy
	require.NotNil(t, got)
	require.NotNil(t, got.URL, "MCP URL must be populated; passing the key-holder artifact leaves it nil")
}

// TestPhase1bReinjection_MCPURLUsesSharedArtifact drives the resolution the Phase 1b MCP
// re-injection branch performs at agent_configuration_service.go:3086. That branch lives
// inside Update(), which returns early to updateMCPConfig for TypeID == MCP before ever
// reaching it — so it cannot be driven end-to-end through any public entry point. This test
// exercises the same resolveMCPMappingAPIID + resolveGatewayForMCPArtifact composition used
// at that line directly, at the closest available seam.
func TestPhase1bReinjection_MCPURLUsesSharedArtifact(t *testing.T) {
	f := newMCPURLTestFixture()
	svc := f.newService()

	sharedArtifactUUID := svc.resolveMCPMappingAPIID(context.Background(), &f.mapping, f.ouID)
	require.Equal(t, f.sharedArtifactUUID, sharedArtifactUUID, "resolveMCPMappingAPIID must resolve the proxy's shared artifact, not the key-holder artifact")

	gateway, err := svc.resolveGatewayForMCPArtifact(context.Background(), sharedArtifactUUID, f.ouID, f.envUUID)
	require.NoError(t, err)
	require.NotNil(t, gateway)
	require.Equal(t, f.gatewayUUID, gateway.UUID)
}

// selfHostedMCPConfigService wires the services the MCP create/update binding checks read, with the
// fixture's proxy placed on a self-hosted gateway.
func selfHostedMCPConfigService(t *testing.T) (*agentConfigurationService, *mcpURLTestFixture) {
	t.Helper()
	f := newMCPURLTestFixture()
	proxy := f.mapping.MCPProxy
	f.gatewayRepo.GetByUUIDFunc = func(string) (*models.Gateway, error) {
		return &models.Gateway{UUID: f.gatewayUUID, Hosting: models.GatewayHostingSelfHosted}, nil
	}
	svc := f.newService()
	svc.mcpProxyRepo = &repomocks.MCPProxyRepositoryMock{
		GetByHandleFunc: func(context.Context, string, string) (*models.MCPProxy, error) { return proxy, nil },
	}
	svc.envMCPMappingRepo = &repomocks.EnvAgentMCPMappingRepositoryMock{
		ListByConfigFunc: func(context.Context, uuid.UUID) ([]models.EnvAgentMCPMapping, error) { return nil, nil },
	}
	svc.agentConfigRepo = &repomocks.AgentConfigurationRepositoryMock{
		CreateFunc:         func(context.Context, *gorm.DB, *models.AgentConfiguration) error { return nil },
		SetMCPProxyRefFunc: func(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) error { return nil },
		SetMCPEnvScopeFunc: func(context.Context, *gorm.DB, uuid.UUID, []uuid.UUID, []uuid.UUID) error { return nil },
		DeleteFunc:         func(context.Context, *gorm.DB, uuid.UUID, string) error { return nil },
	}
	database, err := gorm.Open(postgres.New(postgres.Config{Conn: &noopTxPool{}}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	svc.db = database
	svc.envVariableRepo = &repomocks.AgentEnvConfigVariableRepositoryMock{
		ListByConfigFunc: func(context.Context, uuid.UUID) ([]models.AgentEnvConfigVariable, error) { return nil, nil },
	}
	svc.infraResourceManager = stubInfraManager{
		listOrgEnvs: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{UUID: f.envUUID.String(), Name: "dev"}}, nil
		},
	}
	svc.ocClient = &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(context.Context, string, string) (*models.DeploymentPipelineResponse, error) {
			return nil, utils.ErrDeploymentPipelineNotFound
		},
		GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Provisioning: models.Provisioning{Type: "external"}}, nil
		},
	}
	return svc, f
}

func TestMCPConfig_RefusesSelfHostedProxy(t *testing.T) {
	mappings := map[string]models.EnvModelConfigRequest{"dev": {ProviderName: "proxy1"}}

	t.Run("create", func(t *testing.T) {
		svc, f := selfHostedMCPConfigService(t)
		_, err := svc.createMCPConfig(context.Background(), f.ouID, "proj1", "agent1",
			models.CreateAgentModelConfigRequest{Name: "cfg1", Type: "mcp", EnvMappings: mappings}, "user", true)
		require.ErrorIs(t, err, utils.ErrInvalidInput)
		require.ErrorContains(t, err, "self-hosted gateway in environment dev")
	})

	t.Run("create with unbindable environments deferred still refuses", func(t *testing.T) {
		svc, f := selfHostedMCPConfigService(t)
		_, err := svc.createMCPConfig(context.Background(), f.ouID, "proj1", "agent1",
			models.CreateAgentModelConfigRequest{Name: "cfg1", Type: "mcp", EnvMappings: mappings, DeferUnbindableMCPEnvs: true}, "user", true)
		require.ErrorIs(t, err, utils.ErrInvalidInput)
	})

	t.Run("update", func(t *testing.T) {
		svc, f := selfHostedMCPConfigService(t)
		existing := &models.AgentConfiguration{UUID: uuid.New(), OUID: f.ouID, Name: "cfg1"}
		_, err := svc.updateMCPConfig(context.Background(), existing, f.ouID, "proj1", "agent1",
			models.UpdateAgentModelConfigRequest{EnvMappings: mappings})
		require.ErrorIs(t, err, utils.ErrInvalidInput)
		require.ErrorContains(t, err, "self-hosted gateway in environment dev")
	})
}

// A deferred binding becomes deployable later, when the proxy is updated onto a self-hosted gateway in the
// environment. The reconcile must not bind the agent there.
func TestDeployableMCPEnvs_SkipsSelfHostedGateway(t *testing.T) {
	f := newMCPURLTestFixture()
	proxy := f.mapping.MCPProxy

	t.Run("self-hosted gateway is not bindable", func(t *testing.T) {
		f.gatewayRepo.GetByUUIDFunc = func(string) (*models.Gateway, error) {
			return &models.Gateway{UUID: f.gatewayUUID, Hosting: models.GatewayHostingSelfHosted}, nil
		}
		assert.Empty(t, f.newService().deployableMCPEnvs(context.Background(), proxy, f.ouID, []uuid.UUID{f.envUUID}))
	})

	t.Run("managed gateway is bindable", func(t *testing.T) {
		f.gatewayRepo.GetByUUIDFunc = func(string) (*models.Gateway, error) {
			return &models.Gateway{UUID: f.gatewayUUID}, nil
		}
		assert.Equal(t, []uuid.UUID{f.envUUID}, f.newService().deployableMCPEnvs(context.Background(), proxy, f.ouID, []uuid.UUID{f.envUUID}))
	})
}

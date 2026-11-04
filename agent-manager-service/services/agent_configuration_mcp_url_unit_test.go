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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
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

	resp, err := svc.buildConfigResponse(context.Background(), config, externalAgentPolicy)

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
			return &models.AgentResponse{Provisioning: models.Provisioning{Type: string(utils.InternalAgent)}}, nil
		},
	}
	return svc, f
}

// A refusal for one self-hosted environment must leave the whole request unapplied: the per-environment steps
// commit one by one, so nothing may have run for the managed environment either.
func TestMCPConfig_SelfHostedRefusalAppliesNothing(t *testing.T) {
	svc, f := selfHostedMCPConfigService(t)
	managedEnv, managedArtifact, managedGateway := uuid.New(), uuid.New(), uuid.New()
	proxy := f.mapping.MCPProxy
	proxy.Endpoints[0].Environments = append(proxy.Endpoints[0].Environments,
		models.MCPProxyEndpointEnvironment{EnvironmentUUID: managedEnv, ArtifactUUID: managedArtifact})
	selfHostedGateway := f.gatewayUUID
	f.deploymentRepo.PlacedGatewaysFunc = func(_ context.Context, _ string, artifact uuid.UUID) ([]string, error) {
		if artifact == managedArtifact {
			return []string{managedGateway.String()}, nil
		}
		return []string{selfHostedGateway.String()}, nil
	}
	f.gatewayRepo.EnvironmentMappingExistsFunc = func(string, string) (bool, error) { return true, nil }
	f.gatewayRepo.GetByUUIDFunc = func(id string) (*models.Gateway, error) {
		if id == managedGateway.String() {
			return &models.Gateway{UUID: managedGateway}, nil
		}
		return &models.Gateway{UUID: selfHostedGateway, Hosting: models.GatewayHostingSelfHosted}, nil
	}
	svc.infraResourceManager = stubInfraManager{
		listOrgEnvs: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{
				{UUID: managedEnv.String(), Name: "managed"}, {UUID: f.envUUID.String(), Name: "selfhosted"},
			}, nil
		},
	}
	scopeRecorded := false
	repo := svc.agentConfigRepo.(*repomocks.AgentConfigurationRepositoryMock)
	repo.SetMCPEnvScopeFunc = func(context.Context, *gorm.DB, uuid.UUID, []uuid.UUID, []uuid.UUID) error {
		scopeRecorded = true
		return nil
	}
	// Any mapping write or OpenChoreo env-var update panics: those mocks have no funcs set.
	existing := &models.AgentConfiguration{
		UUID: uuid.New(), OUID: f.ouID, Name: "cfg1",
		EnvMCPMappings: []models.EnvAgentMCPMapping{{EnvironmentUUID: managedEnv, MCPProxyUUID: proxy.UUID, MCPProxy: proxy}},
	}

	_, err := svc.updateMCPConfig(context.Background(), existing, f.ouID, "proj1", "agent1", models.UpdateAgentModelConfigRequest{
		EnvMappings: map[string]models.EnvModelConfigRequest{"managed": {ProviderName: "proxy1"}, "selfhosted": {ProviderName: "proxy1"}},
	})
	require.ErrorIs(t, err, utils.ErrInvalidInput)
	require.ErrorContains(t, err, "self-hosted gateway in environment selfhosted")
	assert.False(t, scopeRecorded, "the connection scope must not be recorded for a refused update")
}

func TestMCPConfig_RefusesSelfHostedProxy(t *testing.T) {
	mappings := map[string]models.EnvModelConfigRequest{"dev": {ProviderName: "proxy1"}}

	t.Run("create", func(t *testing.T) {
		svc, f := selfHostedMCPConfigService(t)
		_, err := svc.createMCPConfig(context.Background(), f.ouID, "proj1", "agent1",
			models.CreateAgentModelConfigRequest{Name: "cfg1", Type: "mcp", EnvMappings: mappings}, "user", platformAgentPolicy)
		require.ErrorIs(t, err, utils.ErrInvalidInput)
		require.ErrorContains(t, err, "self-hosted gateway in environment dev")
	})

	t.Run("create with unbindable environments deferred still refuses", func(t *testing.T) {
		svc, f := selfHostedMCPConfigService(t)
		_, err := svc.createMCPConfig(context.Background(), f.ouID, "proj1", "agent1",
			models.CreateAgentModelConfigRequest{Name: "cfg1", Type: "mcp", EnvMappings: mappings, DeferUnbindableMCPEnvs: true}, "user", platformAgentPolicy)
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
		bindable, selfHosted := f.newService().deployableMCPEnvs(context.Background(), proxy, f.ouID, []uuid.UUID{f.envUUID})
		assert.Empty(t, bindable)
		assert.Equal(t, []uuid.UUID{f.envUUID}, selfHosted)
	})

	t.Run("managed gateway is bindable", func(t *testing.T) {
		f.gatewayRepo.GetByUUIDFunc = func(string) (*models.Gateway, error) {
			return &models.Gateway{UUID: f.gatewayUUID}, nil
		}
		bindable, selfHosted := f.newService().deployableMCPEnvs(context.Background(), proxy, f.ouID, []uuid.UUID{f.envUUID})
		assert.Equal(t, []uuid.UUID{f.envUUID}, bindable)
		assert.Empty(t, selfHosted)
	})

	t.Run("the reconcile does not bind an internal agent there", func(t *testing.T) {
		svc, proxy, configUUID, activated := selfHostedBackfillService(t, utils.InternalAgent)
		require.NoError(t, svc.reconcileConfigMCPBindings(context.Background(), svc.newMCPReconcileScope(context.Background(), "org1"), proxy, configUUID, nil))
		assert.Empty(t, *activated)
	})
}

// selfHostedBackfillService wires reconcileConfigMCPBindings for one MCP connection whose proxy has become
// deployable on a self-hosted gateway in an environment the connection is configured for. It records each
// environment an activation starts in, and stops that activation at its first read.
func selfHostedBackfillService(
	t *testing.T, provisioning utils.AgentProvisioningType,
) (svc *agentConfigurationService, proxy *models.MCPProxy, configUUID uuid.UUID, activated *[]uuid.UUID) {
	t.Helper()
	f := newMCPURLTestFixture()
	f.gatewayRepo.GetByUUIDFunc = func(string) (*models.Gateway, error) {
		return &models.Gateway{UUID: f.gatewayUUID, Hosting: models.GatewayHostingSelfHosted}, nil
	}
	proxy = f.mapping.MCPProxy
	configUUID = uuid.New()
	config := &models.AgentConfiguration{
		UUID: configUUID, OUID: f.ouID, ProjectName: "proj1", AgentID: "agent1", Name: "cfg1",
		TypeID:       models.AgentConfigTypeIDMCP,
		MCPProxyRef:  mcpProxyRef(configUUID, proxy.UUID),
		EnvVariables: mcpVarRows(configUUID, f.envUUID),
	}
	svc = f.newService()
	svc.agentConfigRepo = &repomocks.AgentConfigurationRepositoryMock{
		GetByUUIDFunc: func(context.Context, uuid.UUID, string) (*models.AgentConfiguration, error) { return config, nil },
	}
	svc.ocClient = &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(context.Context, string, string) (*models.DeploymentPipelineResponse, error) {
			return nil, utils.ErrDeploymentPipelineNotFound
		},
		GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Provisioning: models.Provisioning{Type: string(provisioning)}}, nil
		},
	}
	svc.infraResourceManager = stubInfraManager{
		listOrgEnvs: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{UUID: f.envUUID.String(), Name: "dev"}}, nil
		},
	}
	activated = &[]uuid.UUID{}
	svc.envVariableRepo = &repomocks.AgentEnvConfigVariableRepositoryMock{
		ListByConfigAndEnvFunc: func(_ context.Context, _ uuid.UUID, envUUID uuid.UUID) ([]models.AgentEnvConfigVariable, error) {
			*activated = append(*activated, envUUID)
			return nil, errors.New("activation stopped by the test")
		},
	}
	return svc, proxy, configUUID, activated
}

// An external agent runs outside the platform, so a proxy that became deployable on a self-hosted gateway is
// one it can use: the reconcile binds it there.
func TestDeployableMCPEnvs_IncludesSelfHostedForExternalAgent(t *testing.T) {
	svc, proxy, configUUID, activated := selfHostedBackfillService(t, utils.ExternalAgent)
	err := svc.reconcileConfigMCPBindings(context.Background(), svc.newMCPReconcileScope(context.Background(), "org1"), proxy, configUUID, nil)
	require.ErrorContains(t, err, "activation stopped by the test")
	require.Len(t, *activated, 1)
	assert.Equal(t, proxy.Endpoints[0].Environments[0].EnvironmentUUID, (*activated)[0])
}

// A self-hosted gateway cannot validate Agent Identity tokens, so the reconcile leaves an external agent
// unbound where its proxy uses identity on a self-hosted gateway, as create and update refuse it there.
func TestReconcileMCPBindings_SkipsIdentityProxyOnSelfHostedForExternalAgent(t *testing.T) {
	svc, proxy, configUUID, activated := selfHostedBackfillService(t, utils.ExternalAgent)
	on := true
	proxy.Endpoints[0].Configuration.Security = &models.SecurityConfig{Enabled: &on, Identity: &models.IdentitySecurity{Enabled: &on}}

	err := svc.reconcileConfigMCPBindings(context.Background(), svc.newMCPReconcileScope(context.Background(), "org1"), proxy, configUUID, nil)

	require.NoError(t, err)
	assert.Empty(t, *activated)
}

// An external agent may bind to an MCP proxy on a self-hosted gateway: create and update both resolve the
// requested environments through resolveMCPEnvGateways, which accepts the gateway for it.
func TestMCPConfig_ExternalAgentBindsToSelfHostedProxy(t *testing.T) {
	svc, f := selfHostedMCPConfigService(t)
	envMap := map[string]*models.EnvironmentResponse{"dev": {UUID: f.envUUID.String(), Name: "dev"}}
	proxiesByEnv := map[string]*models.MCPProxy{"dev": f.mapping.MCPProxy}

	resolved, err := svc.resolveMCPEnvGateways(context.Background(), f.ouID, proxiesByEnv, envMap, externalAgentPolicy)

	require.NoError(t, err)
	require.NoError(t, resolved["dev"].err)
	require.NotNil(t, resolved["dev"].gateway)
	assert.Equal(t, f.gatewayUUID, resolved["dev"].gateway.UUID)
	assert.False(t, resolved["dev"].gateway.IsManaged())
}

// A self-hosted gateway has no ThunderKeyManager, so an Agent Identity proxy there would reject every token
// the external agent sends. The binding is refused rather than made unusable.
func TestMCPConfig_ExternalAgentRefusedForIdentityProxyOnSelfHosted(t *testing.T) {
	svc, f := selfHostedMCPConfigService(t)
	on := true
	proxy := f.mapping.MCPProxy
	proxy.Endpoints[0].Configuration.Security = &models.SecurityConfig{Enabled: &on, Identity: &models.IdentitySecurity{Enabled: &on}}
	envMap := map[string]*models.EnvironmentResponse{"dev": {UUID: f.envUUID.String(), Name: "dev"}}

	_, err := svc.resolveMCPEnvGateways(context.Background(), f.ouID, map[string]*models.MCPProxy{"dev": proxy}, envMap, externalAgentPolicy)

	require.ErrorIs(t, err, errMCPIdentityOnSelfHostedGateway)
	require.ErrorIs(t, err, utils.ErrInvalidInput)
	require.ErrorContains(t, err, "environment dev")
}

// An external agent reaches its MCP proxy from outside the platform, so its URL points at the self-hosted
// gateway the proxy is placed on.
func TestExternalAgentMCPURL_UsesSelfHostedGateway(t *testing.T) {
	f := newMCPURLTestFixture()
	f.gatewayRepo.GetByUUIDFunc = func(string) (*models.Gateway, error) {
		return &models.Gateway{UUID: f.gatewayUUID, Vhost: "https://customer.example.com", Hosting: models.GatewayHostingSelfHosted}, nil
	}
	ctx := context.Background()
	config := &models.AgentConfiguration{
		UUID: uuid.New(), OUID: f.ouID, ProjectName: "proj1", AgentID: "agent1", Name: "cfg1",
		EnvMCPMappings: []models.EnvAgentMCPMapping{f.mapping},
	}
	newService := func() *agentConfigurationService {
		svc := f.newService()
		svc.infraResourceManager = stubInfraManager{
			listOrgEnvs: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
				return []*models.EnvironmentResponse{{UUID: f.envUUID.String(), Name: "dev"}}, nil
			},
		}
		svc.agentConfigRepo = &repomocks.AgentConfigurationRepositoryMock{
			GetByUUIDFunc: func(context.Context, uuid.UUID, string) (*models.AgentConfiguration, error) { return config, nil },
		}
		return svc
	}
	const wantURL = "https://customer.example.com/proxy-ctx/mcp"

	t.Run("config response", func(t *testing.T) {
		resp, err := newService().buildConfigResponse(ctx, config, externalAgentPolicy)
		require.NoError(t, err)
		require.NotNil(t, resp.EnvModelConfig["dev"].LLMProxy.URL)
		assert.Equal(t, wantURL, *resp.EnvModelConfig["dev"].LLMProxy.URL)
	})

	t.Run("external agent config response", func(t *testing.T) {
		resp, err := newService().buildExternalAgentConfigResponse(ctx, config, map[string]envCredentialData{})
		require.NoError(t, err)
		require.NotNil(t, resp.EnvModelConfig["dev"].LLMProxy.URL)
		assert.Equal(t, wantURL, *resp.EnvModelConfig["dev"].LLMProxy.URL)
	})
}

// An agent bound to a proxy whose artifact ended up on a self-hosted gateway anyway (the proxy-side
// checks refuse the ways there) gets no URL for it: an OpenChoreo agent must never be pointed at the
// customer's gateway, neither as its MCP URL nor as its AgentID token audience.
func TestAgentMCPURLs_EmptyOnSelfHostedGateway(t *testing.T) {
	f := newMCPURLTestFixture()
	f.gatewayRepo.GetByUUIDFunc = func(string) (*models.Gateway, error) {
		return &models.Gateway{UUID: f.gatewayUUID, Vhost: "https://customer.example.com", Hosting: models.GatewayHostingSelfHosted}, nil
	}
	ctx := context.Background()
	config := &models.AgentConfiguration{
		UUID: uuid.New(), OUID: f.ouID, ProjectName: "proj1", AgentID: "agent1", Name: "cfg1",
		EnvMCPMappings: []models.EnvAgentMCPMapping{f.mapping},
	}

	t.Run("config response", func(t *testing.T) {
		svc := f.newService()
		svc.infraResourceManager = stubInfraManager{
			listOrgEnvs: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
				return []*models.EnvironmentResponse{{UUID: f.envUUID.String(), Name: "dev"}}, nil
			},
		}
		resp, err := svc.buildConfigResponse(ctx, config, platformAgentPolicy)
		require.NoError(t, err)
		require.NotNil(t, resp.EnvModelConfig["dev"].LLMProxy)
		assert.Nil(t, resp.EnvModelConfig["dev"].LLMProxy.URL)
	})

	t.Run("AgentID audience", func(t *testing.T) {
		svc := f.newService()
		svc.envMCPMappingRepo = &repomocks.EnvAgentMCPMappingRepositoryMock{
			ListByConfigFunc: func(context.Context, uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
				return []models.EnvAgentMCPMapping{f.mapping}, nil
			},
		}
		url, err := svc.systemManagedMCPURL(ctx, config, f.ouID, "dev", f.envUUID)
		require.NoError(t, err)
		assert.Empty(t, url)
	})

	t.Run("renamed URL variable", func(t *testing.T) {
		url, err := f.newService().renamedMCPProxyURL(ctx, config, &f.mapping, "dev", f.ouID)
		require.NoError(t, err)
		assert.Empty(t, url)
	})

	t.Run("injected variables", func(t *testing.T) {
		svc := f.newService()
		var injected []client.EnvVar
		svc.ocClient = &clientmocks.OpenChoreoClientMock{
			UpdateReleaseBindingEnvVarsFunc: func(_ context.Context, _, _, _, _ string, envVars []client.EnvVar) error {
				injected = envVars
				return nil
			},
		}
		templates := []EnvConfigTemplate{{Key: "url", Name: "MCP_URL"}, {Key: "apikey", Name: "MCP_API_KEY"}}
		err := svc.injectMCPMappingEnvVars(ctx, config, &f.mapping, f.mapping.MCPProxy, "dev", f.ouID, templates, "")
		require.NoError(t, err)
		assert.Equal(t, []client.EnvVar{{Key: "MCP_URL"}, {Key: "MCP_API_KEY"}}, injected)
	})
}

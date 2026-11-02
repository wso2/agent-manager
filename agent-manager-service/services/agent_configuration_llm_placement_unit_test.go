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

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// llmPlacementFixture is an org whose providers are placed on one managed or one self-hosted gateway; every
// gateway is mapped to every environment. Mocks the code under test must not reach have no funcs and panic.
type llmPlacementFixture struct {
	svc         *agentConfigurationService
	envs        map[string]*models.EnvironmentResponse
	providers   map[string]*models.LLMProvider
	placedOn    map[uuid.UUID]*models.Gateway
	gatewayRepo *repomocks.GatewayRepositoryMock
}

const llmPlacementOU = "org1"

func newLLMPlacementFixture(t *testing.T, envNames ...string) *llmPlacementFixture {
	t.Helper()
	f := &llmPlacementFixture{
		envs:      map[string]*models.EnvironmentResponse{},
		providers: map[string]*models.LLMProvider{},
		placedOn:  map[uuid.UUID]*models.Gateway{},
	}
	for _, name := range envNames {
		f.envs[name] = &models.EnvironmentResponse{UUID: uuid.NewString(), Name: name}
	}
	f.gatewayRepo = &repomocks.GatewayRepositoryMock{
		EnvironmentMappingExistsFunc: func(string, string) (bool, error) { return true, nil },
		GetByUUIDFunc: func(id string) (*models.Gateway, error) {
			for _, gw := range f.placedOn {
				if gw.UUID.String() == id {
					return gw, nil
				}
			}
			return nil, gorm.ErrRecordNotFound
		},
	}
	deployments := &repomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(_ context.Context, _ string, artifact uuid.UUID) ([]string, error) {
			if gw, ok := f.placedOn[artifact]; ok {
				return []string{gw.UUID.String()}, nil
			}
			return nil, nil
		},
	}
	providerRepo := &repomocks.LLMProviderRepositoryMock{
		GetByHandleFunc: func(handle, _ string) (*models.LLMProvider, error) {
			if p, ok := f.providers[handle]; ok {
				return p, nil
			}
			return nil, gorm.ErrRecordNotFound
		},
	}
	f.svc = &agentConfigurationService{
		gatewayRepo:               f.gatewayRepo,
		llmProviderRepo:           providerRepo,
		llmProxyDeploymentService: &LLMProxyDeploymentService{deploymentRepo: deployments},
		infraResourceManager: stubInfraManager{
			listOrgEnvs: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
				out := make([]*models.EnvironmentResponse, 0, len(f.envs))
				for _, e := range f.envs {
					out = append(out, e)
				}
				return out, nil
			},
		},
	}
	return f
}

// provider adds a catalog provider placed on a managed or self-hosted gateway, or on none.
func (f *llmPlacementFixture) provider(handle, hosting string) *models.LLMProvider {
	p := &models.LLMProvider{UUID: uuid.New(), InCatalog: true}
	f.providers[handle] = p
	if hosting != "" {
		f.placedOn[p.UUID] = &models.Gateway{UUID: uuid.New(), Hosting: hosting}
	}
	return p
}

func (f *llmPlacementFixture) env(name string) uuid.UUID { return uuid.MustParse(f.envs[name].UUID) }

func TestLLMConfig_AcceptsManagedProvider(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev")
	p := f.provider("managed-p", models.GatewayHostingManaged)

	require.NoError(t, f.svc.requireManagedProviderPlacement(context.Background(), llmPlacementOU, p.UUID, f.env("dev")))
}

func TestLLMConfig_RefusesSelfHostedOnlyProvider(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev")
	selfHosted := f.provider("self-p", models.GatewayHostingSelfHosted)
	unplaced := f.provider("unplaced-p", "")

	for name, p := range map[string]*models.LLMProvider{"self-hosted only": selfHosted, "not deployed": unplaced} {
		t.Run(name, func(t *testing.T) {
			err := f.svc.requireManagedProviderPlacement(context.Background(), llmPlacementOU, p.UUID, f.env("dev"))
			require.ErrorIs(t, err, utils.ErrInvalidInput)
			assert.ErrorContains(t, err, "has no deployment on a managed gateway in environment "+f.envs["dev"].UUID)
		})
	}

	t.Run("create", func(t *testing.T) {
		_, err := f.svc.createLLMConfig(context.Background(), llmPlacementOU, "proj", "agent",
			models.CreateAgentModelConfigRequest{Name: "cfg", Type: "llm", EnvMappings: map[string]models.EnvModelConfigRequest{
				"dev": {ProviderName: "self-p"},
			}}, "user", true)
		require.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.ErrorContains(t, err, "managed gateway")
	})
}

// The per-environment steps commit one by one, so one refused environment must leave every other environment
// unapplied. The unstubbed repositories and OpenChoreo client panic if the OK environment is touched.
func TestLLMConfig_RefusalAppliesNothing(t *testing.T) {
	newFixture := func(t *testing.T) *llmPlacementFixture {
		f := newLLMPlacementFixture(t, "ok", "bad")
		f.provider("managed-p", models.GatewayHostingManaged)
		f.provider("self-p", models.GatewayHostingSelfHosted)
		return f
	}

	t.Run("create", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.svc.createLLMConfig(context.Background(), llmPlacementOU, "proj", "agent",
			models.CreateAgentModelConfigRequest{Name: "cfg", Type: "llm", EnvMappings: map[string]models.EnvModelConfigRequest{
				"ok": {ProviderName: "managed-p"}, "bad": {ProviderName: "self-p"},
			}}, "user", true)
		require.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.ErrorContains(t, err, f.envs["bad"].UUID)
	})

	t.Run("update changing providers and adding an environment", func(t *testing.T) {
		f := newFixture(t)
		oldProvider := f.provider("old-p", models.GatewayHostingManaged)
		f.svc.agentConfigRepo = &repomocks.AgentConfigurationRepositoryMock{
			GetByUUIDFunc: func(_ context.Context, id uuid.UUID, ouID string) (*models.AgentConfiguration, error) {
				return &models.AgentConfiguration{
					UUID: id, OUID: ouID, ProjectName: "proj", AgentID: "agent", Name: "cfg",
					EnvMappings: []models.EnvAgentModelMapping{{
						EnvironmentUUID: f.env("ok"),
						LLMProxy:        &models.LLMProxy{Configuration: models.LLMProxyConfig{Provider: oldProvider.UUID.String()}},
					}},
				}, nil
			},
		}
		f.svc.ocClient = &clientmocks.OpenChoreoClientMock{
			GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
				return &models.AgentResponse{KindName: "agent-api"}, nil
			},
		}

		_, err := f.svc.Update(context.Background(), uuid.New(), llmPlacementOU, "proj", "agent",
			models.UpdateAgentModelConfigRequest{EnvMappings: map[string]models.EnvModelConfigRequest{
				"ok": {ProviderName: "managed-p"}, "bad": {ProviderName: "self-p"},
			}})
		require.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.ErrorContains(t, err, f.envs["bad"].UUID)
	})

	t.Run("update changing the provider of an existing environment", func(t *testing.T) {
		f := newFixture(t)
		oldProvider := f.provider("old-p", models.GatewayHostingManaged)
		f.svc.agentConfigRepo = &repomocks.AgentConfigurationRepositoryMock{
			GetByUUIDFunc: func(_ context.Context, id uuid.UUID, ouID string) (*models.AgentConfiguration, error) {
				return &models.AgentConfiguration{
					UUID: id, OUID: ouID, ProjectName: "proj", AgentID: "agent", Name: "cfg",
					EnvMappings: []models.EnvAgentModelMapping{{
						EnvironmentUUID: f.env("bad"),
						LLMProxy:        &models.LLMProxy{Configuration: models.LLMProxyConfig{Provider: oldProvider.UUID.String()}},
					}},
				}, nil
			},
		}
		f.svc.ocClient = &clientmocks.OpenChoreoClientMock{
			GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
				return &models.AgentResponse{KindName: "agent-api"}, nil
			},
		}

		_, err := f.svc.Update(context.Background(), uuid.New(), llmPlacementOU, "proj", "agent",
			models.UpdateAgentModelConfigRequest{EnvMappings: map[string]models.EnvModelConfigRequest{
				"bad": {ProviderName: "self-p"},
			}})
		require.ErrorIs(t, err, utils.ErrInvalidInput)
	})
}

// An environment keeping its provider is not re-checked: editing its policies must not start failing because
// the provider was later placed on a self-hosted gateway.
func TestRequireManagedProviderPlacements_SkipsUnchangedProvider(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev")
	p := f.provider("self-p", models.GatewayHostingSelfHosted)
	existing := map[string]*models.EnvAgentModelMapping{
		"dev": {LLMProxy: &models.LLMProxy{Configuration: models.LLMProxyConfig{Provider: p.UUID.String()}}},
	}
	requested := map[string]models.EnvModelConfigRequest{"dev": {ProviderName: "self-p"}}

	require.NoError(t, f.svc.requireManagedProviderPlacements(context.Background(), llmPlacementOU, requested, f.envs, existing))
	require.Error(t, f.svc.requireManagedProviderPlacements(context.Background(), llmPlacementOU, requested, f.envs, nil))
}

func TestResolveGatewayForProvider_PrefersManagedAnchor(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev")
	selfHosted := &models.Gateway{UUID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), Hosting: models.GatewayHostingSelfHosted}
	managed := &models.Gateway{UUID: uuid.MustParse("ffffffff-0000-0000-0000-000000000002")}
	providerUUID := uuid.New()
	f.svc.llmProxyDeploymentService = &LLMProxyDeploymentService{deploymentRepo: &repomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
			return []string{selfHosted.UUID.String(), managed.UUID.String()}, nil
		},
	}}
	f.gatewayRepo.GetByUUIDFunc = func(id string) (*models.Gateway, error) {
		if id == managed.UUID.String() {
			return managed, nil
		}
		return selfHosted, nil
	}

	got, err := f.svc.resolveGatewayForProvider(context.Background(), providerUUID.String(), llmPlacementOU, f.env("dev"))

	require.NoError(t, err)
	assert.Equal(t, managed.UUID, got.UUID)
}

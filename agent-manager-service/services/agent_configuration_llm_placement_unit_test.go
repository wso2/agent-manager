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
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
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

	require.NoError(t, platformAgentPolicy.requireProvider(context.Background(), f.svc.llmProxyDeploymentService, f.svc.gatewayRepo, llmPlacementOU, p.UUID, f.env("dev")))
}

func TestLLMConfig_RefusesSelfHostedOnlyProvider(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev")
	selfHosted := f.provider("self-p", models.GatewayHostingSelfHosted)
	unplaced := f.provider("unplaced-p", "")

	for name, p := range map[string]*models.LLMProvider{"self-hosted only": selfHosted, "not deployed": unplaced} {
		t.Run(name, func(t *testing.T) {
			err := platformAgentPolicy.requireProvider(context.Background(), f.svc.llmProxyDeploymentService, f.svc.gatewayRepo, llmPlacementOU, p.UUID, f.env("dev"))
			require.ErrorIs(t, err, utils.ErrInvalidInput)
			assert.ErrorContains(t, err, "not deployed to a managed gateway in this environment (platform-hosted agents can only use")
			assert.ErrorContains(t, err, deployProviderHint)
		})
	}

	t.Run("create", func(t *testing.T) {
		_, err := f.svc.createLLMConfig(context.Background(), llmPlacementOU, "proj", "agent",
			models.CreateAgentModelConfigRequest{Name: "cfg", Type: "llm", EnvMappings: map[string]models.EnvModelConfigRequest{
				"dev": {ProviderName: "self-p"},
			}}, "user", platformAgentPolicy)
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
			}}, "user", platformAgentPolicy)
		require.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.ErrorContains(t, err, `LLM provider "self-p" in environment bad: `)
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
			models.UpdateAgentModelConfigRequest{Name: "renamed", EnvMappings: map[string]models.EnvModelConfigRequest{
				"ok": {ProviderName: "managed-p"}, "bad": {ProviderName: "self-p"},
			}})
		require.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.ErrorContains(t, err, `LLM provider "self-p" in environment bad: `)
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
			models.UpdateAgentModelConfigRequest{Name: "renamed", EnvMappings: map[string]models.EnvModelConfigRequest{
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

	require.NoError(t, f.svc.requireProviderPlacements(context.Background(), llmPlacementOU, requested, f.envs, existing, platformAgentPolicy))
	require.Error(t, f.svc.requireProviderPlacements(context.Background(), llmPlacementOU, requested, f.envs, nil, platformAgentPolicy))
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

	got, err := f.svc.resolveGatewayForProvider(context.Background(), providerUUID.String(), llmPlacementOU, f.env("dev"), platformAgentPolicy)

	require.NoError(t, err)
	assert.Equal(t, managed.UUID, got.UUID)
}

// selfHostedOnlyProviderAnchor places the provider on a self-hosted gateway only, in an environment that also
// has a managed egress gateway, and returns that managed gateway.
func selfHostedOnlyProviderAnchor(f *llmPlacementFixture) (*LLMProxyDeploymentService, *models.Gateway) {
	selfHosted := &models.Gateway{
		UUID: uuid.New(), GatewayFunctionalityType: models.GatewayRoleBoth, Hosting: models.GatewayHostingSelfHosted,
	}
	managed := &models.Gateway{UUID: uuid.New(), GatewayFunctionalityType: models.GatewayRoleBoth}
	f.gatewayRepo.GetByUUIDFunc = func(id string) (*models.Gateway, error) {
		if id == managed.UUID.String() {
			return managed, nil
		}
		return selfHosted, nil
	}
	f.gatewayRepo.ListWithFiltersFunc = func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
		return []*models.Gateway{selfHosted, managed}, nil
	}
	return &LLMProxyDeploymentService{deploymentRepo: &repomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
			return []string{selfHosted.UUID.String()}, nil
		},
	}}, managed
}

// Inference is managed-only, so an agent's gateway never anchors on a self-hosted placement of its provider.
func TestResolveGatewayForProvider_IgnoresSelfHostedOnlyAnchor(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev")
	deployments, managed := selfHostedOnlyProviderAnchor(f)
	f.svc.llmProxyDeploymentService = deployments

	got, err := f.svc.resolveGatewayForProvider(context.Background(), uuid.NewString(), llmPlacementOU, f.env("dev"), platformAgentPolicy)

	require.NoError(t, err)
	assert.Equal(t, managed.UUID, got.UUID)
}

// externalAgentComponent is an OpenChoreo client whose agent is externally provisioned.
func externalAgentComponent() *clientmocks.OpenChoreoClientMock {
	return &clientmocks.OpenChoreoClientMock{
		GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				KindName:     "agent-api",
				Provisioning: models.Provisioning{Type: string(utils.ExternalAgent)},
			}, nil
		},
	}
}

// An external agent runs outside OpenChoreo, so a provider placed only on a self-hosted gateway serves it, even
// in an environment that also has a managed egress gateway.
func TestLLMConfig_ExternalAgentBindsToSelfHostedOnlyProvider(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev")
	p := f.provider("self-p", models.GatewayHostingSelfHosted)
	requested := map[string]models.EnvModelConfigRequest{"dev": {ProviderName: "self-p"}}

	require.NoError(t, f.svc.requireProviderPlacements(context.Background(), llmPlacementOU, requested, f.envs, nil, externalAgentPolicy))

	deployments, _ := selfHostedOnlyProviderAnchor(f)
	f.svc.llmProxyDeploymentService = deployments
	got, err := f.svc.resolveGatewayForProvider(context.Background(), p.UUID.String(), llmPlacementOU, f.env("dev"), externalAgentPolicy)

	require.NoError(t, err)
	assert.False(t, got.IsManaged(), "an external agent's proxy goes where its provider is: the self-hosted gateway")
}

// A provider on both a managed and a self-hosted gateway in one environment serves an external agent from the
// managed one, whatever order the placements come in.
func TestLLMConfig_ExternalAgentPrefersManagedInMixedEnv(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev")
	selfHosted := &models.Gateway{UUID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), Hosting: models.GatewayHostingSelfHosted}
	managed := &models.Gateway{UUID: uuid.MustParse("ffffffff-0000-0000-0000-000000000002")}
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

	got, err := f.svc.resolveGatewayForProvider(context.Background(), uuid.NewString(), llmPlacementOU, f.env("dev"), externalAgentPolicy)

	require.NoError(t, err)
	assert.Equal(t, managed.UUID, got.UUID)
}

func TestLLMConfig_ExternalAgentRefusedWhenProviderNotInEnv(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev")
	unplaced := f.provider("unplaced-p", "")
	elsewhere := f.provider("elsewhere-p", models.GatewayHostingSelfHosted)
	f.gatewayRepo.EnvironmentMappingExistsFunc = func(gatewayID, _ string) (bool, error) {
		return gatewayID != f.placedOn[elsewhere.UUID].UUID.String(), nil
	}
	notInEnv := "not deployed to a gateway in this environment; " + deployProviderHint

	for name, p := range map[string]*models.LLMProvider{"not deployed": unplaced, "deployed to another environment": elsewhere} {
		t.Run(name, func(t *testing.T) {
			err := externalAgentPolicy.requireProvider(context.Background(), f.svc.llmProxyDeploymentService, f.svc.gatewayRepo, llmPlacementOU, p.UUID, f.env("dev"))
			require.ErrorIs(t, err, utils.ErrInvalidInput)
			assert.ErrorContains(t, err, notInEnv)
			assert.NotContains(t, err.Error(), "managed")
		})
	}

	t.Run("create", func(t *testing.T) {
		_, err := f.svc.createLLMConfig(context.Background(), llmPlacementOU, "proj", "agent",
			models.CreateAgentModelConfigRequest{Name: "cfg", Type: "llm", EnvMappings: map[string]models.EnvModelConfigRequest{
				"dev": {ProviderName: "unplaced-p"},
			}}, "user", externalAgentPolicy)
		require.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.ErrorContains(t, err, notInEnv)
	})

	t.Run("update", func(t *testing.T) {
		managed := f.provider("managed-p", models.GatewayHostingManaged)
		f.svc.agentConfigRepo = &repomocks.AgentConfigurationRepositoryMock{
			GetByUUIDFunc: func(_ context.Context, id uuid.UUID, ouID string) (*models.AgentConfiguration, error) {
				return &models.AgentConfiguration{
					UUID: id, OUID: ouID, ProjectName: "proj", AgentID: "agent", Name: "cfg",
					EnvMappings: []models.EnvAgentModelMapping{{
						EnvironmentUUID: f.env("dev"),
						LLMProxy:        &models.LLMProxy{Configuration: models.LLMProxyConfig{Provider: managed.UUID.String()}},
					}},
				}, nil
			},
		}
		f.svc.ocClient = externalAgentComponent()

		_, err := f.svc.Update(context.Background(), uuid.New(), llmPlacementOU, "proj", "agent",
			models.UpdateAgentModelConfigRequest{Name: "renamed", EnvMappings: map[string]models.EnvModelConfigRequest{
				"dev": {ProviderName: "unplaced-p"},
			}})
		require.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.ErrorContains(t, err, `LLM provider "unplaced-p" in environment dev: `)
		assert.ErrorContains(t, err, notInEnv)
	})
}

// errReachedProxyWrite stands in for the proxy store: a call that reaches it got past gateway selection.
var errReachedProxyWrite = errors.New("reached the proxy write")

// A new environment on an external agent's config places its proxy on the provider's self-hosted gateway; an
// internal agent's still finds no managed gateway to place it on.
func TestLLMConfig_ExternalAgentNewEnvOnSelfHostedProvider(t *testing.T) {
	newFixture := func(t *testing.T) *llmPlacementFixture {
		f := newLLMPlacementFixture(t, "dev")
		p := f.provider("self-p", models.GatewayHostingSelfHosted)
		selfHosted := f.placedOn[p.UUID]
		selfHosted.GatewayFunctionalityType = models.GatewayRoleBoth
		f.gatewayRepo.ListWithFiltersFunc = func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{selfHosted}, nil
		}
		f.svc.logger = slog.Default()
		f.svc.ocClient = &clientmocks.OpenChoreoClientMock{
			GetProjectFunc: func(context.Context, string, string) (*models.ProjectResponse, error) {
				return &models.ProjectResponse{UUID: uuid.NewString()}, nil
			},
		}
		f.svc.llmProxyService = &LLMProxyService{providerRepo: &repomocks.LLMProviderRepositoryMock{
			GetByUUIDFunc: func(string, string) (*models.LLMProvider, error) { return nil, errReachedProxyWrite },
		}}
		return f
	}
	processNewEnv := func(f *llmPlacementFixture, policy placementPolicy) error {
		config := &models.AgentConfiguration{UUID: uuid.New(), OUID: llmPlacementOU, ProjectName: "proj", AgentID: "agent", Name: "cfg"}
		_, _, err := f.svc.processNewEnv(context.Background(), config.UUID, config, f.envs["dev"], f.env("dev"), "dev",
			models.EnvModelConfigRequest{ProviderName: "self-p"}, llmPlacementOU, nil, policy, "")
		return err
	}

	t.Run("external", func(t *testing.T) {
		require.ErrorIs(t, processNewEnv(newFixture(t), externalAgentPolicy), errReachedProxyWrite)
	})
	t.Run("internal", func(t *testing.T) {
		require.ErrorIs(t, processNewEnv(newFixture(t), platformAgentPolicy), errNoEgressGatewayForEnvironment)
	})
}

// Redeploying an external agent's proxy after a config change keeps it on its self-hosted gateway; an internal
// agent's proxy there is refused as before.
func TestLLMConfig_ExternalAgentProxyUpdateOnSelfHostedPlacement(t *testing.T) {
	newFixture := func(t *testing.T) (*llmPlacementFixture, *models.EnvAgentModelMapping) {
		f := newLLMPlacementFixture(t, "dev")
		f.provider("self-p", models.GatewayHostingSelfHosted)
		proxy := &models.LLMProxy{UUID: uuid.New(), Handle: "cfg-proxy"}
		f.placedOn[proxy.UUID] = &models.Gateway{UUID: uuid.New(), Hosting: models.GatewayHostingSelfHosted}
		f.svc.llmProxyDeploymentService.proxyRepo = &repomocks.LLMProxyRepositoryMock{
			GetByIDFunc: func(string, string) (*models.LLMProxy, error) { return proxy, nil },
		}
		f.svc.logger = slog.Default()
		f.svc.llmProxyService = &LLMProxyService{proxyRepo: &repomocks.LLMProxyRepositoryMock{
			GetByIDFunc: func(string, string) (*models.LLMProxy, error) { return nil, errReachedProxyWrite },
		}}
		return f, &models.EnvAgentModelMapping{EnvironmentUUID: f.env("dev"), LLMProxy: proxy}
	}
	processEnvProxyUpdate := func(f *llmPlacementFixture, mapping *models.EnvAgentModelMapping, policy placementPolicy) error {
		config := &models.AgentConfiguration{UUID: uuid.New(), OUID: llmPlacementOU, ProjectName: "proj", AgentID: "agent", Name: "cfg"}
		_, err := f.svc.processEnvProxyUpdate(context.Background(), config, f.envs["dev"], f.env("dev"), "dev",
			models.EnvModelConfigRequest{ProviderName: "self-p"}, mapping, llmPlacementOU, policy)
		return err
	}

	t.Run("external", func(t *testing.T) {
		f, mapping := newFixture(t)
		require.ErrorIs(t, processEnvProxyUpdate(f, mapping, externalAgentPolicy), errReachedProxyWrite)
	})
	t.Run("internal", func(t *testing.T) {
		f, mapping := newFixture(t)
		require.ErrorIs(t, processEnvProxyUpdate(f, mapping, platformAgentPolicy), errSelfHostedAnchor)
	})
}

// An external agent's LLM proxy URL on GET is the public one create returns: the gateway's vhost gets a
// scheme when it was registered without one.
func TestExternalAgentLLMURL_AddsSchemeToBareVhost(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev")
	contextPath := "/llm/proxy"
	proxy := &models.LLMProxy{UUID: uuid.New(), Handle: "cfg-proxy", Configuration: models.LLMProxyConfig{Context: &contextPath}}
	f.placedOn[proxy.UUID] = &models.Gateway{UUID: uuid.New(), Vhost: "gw.example.com", Hosting: models.GatewayHostingSelfHosted}
	f.svc.llmProxyDeploymentService.proxyRepo = &repomocks.LLMProxyRepositoryMock{
		GetByIDFunc: func(string, string) (*models.LLMProxy, error) { return proxy, nil },
	}
	f.svc.llmProviderRepo = &repomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc: func(string, string) (*models.LLMProvider, error) { return nil, gorm.ErrRecordNotFound },
	}
	f.svc.logger = slog.Default()
	config := &models.AgentConfiguration{
		UUID: uuid.New(), OUID: llmPlacementOU, ProjectName: "proj", AgentID: "agent", Name: "cfg",
		EnvMappings: []models.EnvAgentModelMapping{{EnvironmentUUID: f.env("dev"), LLMProxy: proxy}},
	}

	resp, err := f.svc.buildConfigResponse(context.Background(), config, externalAgentPolicy)

	require.NoError(t, err)
	require.NotNil(t, resp.EnvModelConfig["dev"].LLMProxy.URL)
	assert.Equal(t, "https://gw.example.com/llm/proxy", *resp.EnvModelConfig["dev"].LLMProxy.URL)
}

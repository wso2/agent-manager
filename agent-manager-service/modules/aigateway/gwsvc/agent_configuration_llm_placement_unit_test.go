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

package gwsvc

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// llmPlacementFixture is an org whose providers are placed on one managed or one self-hosted gateway; every
// gateway is mapped to every environment. Mocks the code under test must not reach have no funcs and panic.
type llmPlacementFixture struct {
	svc         *agentConfigurationService
	envs        map[string]*models.EnvironmentResponse
	providers   map[string]*gwmodels.LLMProvider
	placedOn    map[uuid.UUID]*gwmodels.Gateway
	gatewayRepo *gwrepomocks.GatewayRepositoryMock
}

const llmPlacementOU = "org1"

func newLLMPlacementFixture(t *testing.T, envNames ...string) *llmPlacementFixture {
	t.Helper()
	f := &llmPlacementFixture{
		envs:      map[string]*models.EnvironmentResponse{},
		providers: map[string]*gwmodels.LLMProvider{},
		placedOn:  map[uuid.UUID]*gwmodels.Gateway{},
	}
	for _, name := range envNames {
		f.envs[name] = &models.EnvironmentResponse{UUID: uuid.NewString(), Name: name}
	}
	f.gatewayRepo = &gwrepomocks.GatewayRepositoryMock{
		EnvironmentMappingExistsFunc: func(string, string) (bool, error) { return true, nil },
		GetByUUIDFunc: func(id string) (*gwmodels.Gateway, error) {
			for _, gw := range f.placedOn {
				if gw.UUID.String() == id {
					return gw, nil
				}
			}
			return nil, gorm.ErrRecordNotFound
		},
	}
	deployments := &gwrepomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(_ context.Context, _ string, artifact uuid.UUID) ([]string, error) {
			if gw, ok := f.placedOn[artifact]; ok {
				return []string{gw.UUID.String()}, nil
			}
			return nil, nil
		},
	}
	providerRepo := &gwrepomocks.LLMProviderRepositoryMock{
		GetByHandleFunc: func(handle, _ string) (*gwmodels.LLMProvider, error) {
			if p, ok := f.providers[handle]; ok {
				return p, nil
			}
			return nil, gorm.ErrRecordNotFound
		},
	}
	f.svc = &agentConfigurationService{
		logger:                    discardLogger(),
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
func (f *llmPlacementFixture) provider(handle, hosting string) *gwmodels.LLMProvider {
	p := &gwmodels.LLMProvider{UUID: uuid.New(), InCatalog: true}
	f.providers[handle] = p
	if hosting != "" {
		f.placedOn[p.UUID] = &gwmodels.Gateway{UUID: uuid.New(), Hosting: hosting}
	}
	return p
}

func (f *llmPlacementFixture) env(name string) uuid.UUID { return uuid.MustParse(f.envs[name].UUID) }

func TestLLMConfig_AcceptsManagedProvider(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev")
	p := f.provider("managed-p", gwmodels.GatewayHostingManaged)

	require.NoError(t, platformAgentPolicy.requireProvider(context.Background(), f.svc.llmProxyDeploymentService, f.svc.gatewayRepo, llmPlacementOU, p.UUID, f.env("dev")))
}

func TestLLMConfig_RefusesSelfHostedOnlyProvider(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev")
	selfHosted := f.provider("self-p", gwmodels.GatewayHostingSelfHosted)
	unplaced := f.provider("unplaced-p", "")

	for name, p := range map[string]*gwmodels.LLMProvider{"self-hosted only": selfHosted, "not deployed": unplaced} {
		t.Run(name, func(t *testing.T) {
			err := platformAgentPolicy.requireProvider(context.Background(), f.svc.llmProxyDeploymentService, f.svc.gatewayRepo, llmPlacementOU, p.UUID, f.env("dev"))
			require.ErrorIs(t, err, httpx.ErrInvalidInput)
			assert.ErrorContains(t, err, "not deployed to a managed gateway in this environment (platform-hosted agents can only use")
			assert.ErrorContains(t, err, deployProviderHint)
		})
	}

	t.Run("create", func(t *testing.T) {
		_, err := f.svc.createLLMConfig(context.Background(), llmPlacementOU, "proj", "agent",
			gwmodels.CreateAgentModelConfigRequest{Name: "cfg", Type: "llm", EnvMappings: map[string]gwmodels.EnvModelConfigRequest{
				"dev": {ProviderName: "self-p"},
			}}, "user", platformAgentPolicy)
		require.ErrorIs(t, err, httpx.ErrInvalidInput)
		assert.ErrorContains(t, err, "managed gateway")
	})
}

// The per-environment steps commit one by one, so one refused environment must leave every other environment
// unapplied. The unstubbed repositories panic if the OK environment is touched.
func TestLLMConfig_RefusalAppliesNothing(t *testing.T) {
	newFixture := func(t *testing.T) *llmPlacementFixture {
		f := newLLMPlacementFixture(t, "ok", "bad")
		f.provider("managed-p", gwmodels.GatewayHostingManaged)
		f.provider("self-p", gwmodels.GatewayHostingSelfHosted)
		return f
	}

	t.Run("create", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.svc.createLLMConfig(context.Background(), llmPlacementOU, "proj", "agent",
			gwmodels.CreateAgentModelConfigRequest{Name: "cfg", Type: "llm", EnvMappings: map[string]gwmodels.EnvModelConfigRequest{
				"ok": {ProviderName: "managed-p"}, "bad": {ProviderName: "self-p"},
			}}, "user", platformAgentPolicy)
		require.ErrorIs(t, err, httpx.ErrInvalidInput)
		assert.ErrorContains(t, err, `LLM provider "self-p" in environment bad: `)
	})
}

// An environment keeping its provider is not re-checked: editing its policies must not start failing because
// the provider was later placed on a self-hosted gateway.
func TestRequireManagedProviderPlacements_SkipsUnchangedProvider(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev")
	p := f.provider("self-p", gwmodels.GatewayHostingSelfHosted)
	existing := map[string]*gwmodels.EnvAgentModelMapping{
		"dev": {LLMProxy: &gwmodels.LLMProxy{Configuration: gwmodels.LLMProxyConfig{Provider: p.UUID.String()}}},
	}
	requested := map[string]gwmodels.EnvModelConfigRequest{"dev": {ProviderName: "self-p"}}

	require.NoError(t, f.svc.requireProviderPlacements(context.Background(), llmPlacementOU, requested, f.envs, existing, platformAgentPolicy))
	require.Error(t, f.svc.requireProviderPlacements(context.Background(), llmPlacementOU, requested, f.envs, nil, platformAgentPolicy))
}

func TestResolveGatewayForProvider_PrefersManagedAnchor(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev")
	selfHosted := &gwmodels.Gateway{UUID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), Hosting: gwmodels.GatewayHostingSelfHosted}
	managed := &gwmodels.Gateway{UUID: uuid.MustParse("ffffffff-0000-0000-0000-000000000002")}
	providerUUID := uuid.New()
	f.svc.llmProxyDeploymentService = &LLMProxyDeploymentService{deploymentRepo: &gwrepomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
			return []string{selfHosted.UUID.String(), managed.UUID.String()}, nil
		},
	}}
	f.gatewayRepo.GetByUUIDFunc = func(id string) (*gwmodels.Gateway, error) {
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
func selfHostedOnlyProviderAnchor(f *llmPlacementFixture) (*LLMProxyDeploymentService, *gwmodels.Gateway) {
	selfHosted := &gwmodels.Gateway{
		UUID: uuid.New(), GatewayFunctionalityType: gwmodels.GatewayRoleBoth, Hosting: gwmodels.GatewayHostingSelfHosted,
	}
	managed := &gwmodels.Gateway{UUID: uuid.New(), GatewayFunctionalityType: gwmodels.GatewayRoleBoth}
	f.gatewayRepo.GetByUUIDFunc = func(id string) (*gwmodels.Gateway, error) {
		if id == managed.UUID.String() {
			return managed, nil
		}
		return selfHosted, nil
	}
	f.gatewayRepo.ListWithFiltersFunc = func(gwrepo.GatewayFilterOptions) ([]*gwmodels.Gateway, error) {
		return []*gwmodels.Gateway{selfHosted, managed}, nil
	}
	return &LLMProxyDeploymentService{deploymentRepo: &gwrepomocks.DeploymentRepositoryMock{
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
func externalAgentComponent() *repomocks.PlatformRepositoryMock {
	return &repomocks.PlatformRepositoryMock{
		GetAgentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				Provisioning: models.Provisioning{Type: string(gwutil.ExternalAgent)},
			}, nil
		},
	}
}

// An external agent runs outside OpenChoreo, so a provider placed only on a self-hosted gateway serves it, even
// in an environment that also has a managed egress gateway.
func TestLLMConfig_ExternalAgentBindsToSelfHostedOnlyProvider(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev")
	p := f.provider("self-p", gwmodels.GatewayHostingSelfHosted)
	requested := map[string]gwmodels.EnvModelConfigRequest{"dev": {ProviderName: "self-p"}}

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
	selfHosted := &gwmodels.Gateway{UUID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), Hosting: gwmodels.GatewayHostingSelfHosted}
	managed := &gwmodels.Gateway{UUID: uuid.MustParse("ffffffff-0000-0000-0000-000000000002")}
	f.svc.llmProxyDeploymentService = &LLMProxyDeploymentService{deploymentRepo: &gwrepomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(context.Context, string, uuid.UUID) ([]string, error) {
			return []string{selfHosted.UUID.String(), managed.UUID.String()}, nil
		},
	}}
	f.gatewayRepo.GetByUUIDFunc = func(id string) (*gwmodels.Gateway, error) {
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
	elsewhere := f.provider("elsewhere-p", gwmodels.GatewayHostingSelfHosted)
	f.gatewayRepo.EnvironmentMappingExistsFunc = func(gatewayID, _ string) (bool, error) {
		return gatewayID != f.placedOn[elsewhere.UUID].UUID.String(), nil
	}
	notInEnv := "not deployed to a gateway in this environment; " + deployProviderHint

	for name, p := range map[string]*gwmodels.LLMProvider{"not deployed": unplaced, "deployed to another environment": elsewhere} {
		t.Run(name, func(t *testing.T) {
			err := externalAgentPolicy.requireProvider(context.Background(), f.svc.llmProxyDeploymentService, f.svc.gatewayRepo, llmPlacementOU, p.UUID, f.env("dev"))
			require.ErrorIs(t, err, httpx.ErrInvalidInput)
			assert.ErrorContains(t, err, notInEnv)
			assert.NotContains(t, err.Error(), "managed")
		})
	}

	t.Run("create", func(t *testing.T) {
		_, err := f.svc.createLLMConfig(context.Background(), llmPlacementOU, "proj", "agent",
			gwmodels.CreateAgentModelConfigRequest{Name: "cfg", Type: "llm", EnvMappings: map[string]gwmodels.EnvModelConfigRequest{
				"dev": {ProviderName: "unplaced-p"},
			}}, "user", externalAgentPolicy)
		require.ErrorIs(t, err, httpx.ErrInvalidInput)
		assert.ErrorContains(t, err, notInEnv)
	})

	t.Run("update", func(t *testing.T) {
		managed := f.provider("managed-p", gwmodels.GatewayHostingManaged)
		f.svc.agentConfigRepo = &gwrepomocks.AgentConfigurationRepositoryMock{
			GetByUUIDFunc: func(_ context.Context, id uuid.UUID, ouID string) (*gwmodels.AgentConfiguration, error) {
				return &gwmodels.AgentConfiguration{
					UUID: id, OUID: ouID, ProjectName: "proj", AgentID: "agent", Name: "cfg",
					EnvMappings: []gwmodels.EnvAgentModelMapping{{
						EnvironmentUUID: f.env("dev"),
						LLMProxy:        &gwmodels.LLMProxy{Configuration: gwmodels.LLMProxyConfig{Provider: managed.UUID.String()}},
					}},
				}, nil
			},
		}
		f.svc.ocClient = externalAgentComponent()

		_, err := f.svc.Update(context.Background(), uuid.New(), llmPlacementOU, "proj", "agent",
			gwmodels.UpdateAgentModelConfigRequest{Name: "renamed", EnvMappings: map[string]gwmodels.EnvModelConfigRequest{
				"dev": {ProviderName: "unplaced-p"},
			}})
		require.ErrorIs(t, err, httpx.ErrInvalidInput)
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
		p := f.provider("self-p", gwmodels.GatewayHostingSelfHosted)
		selfHosted := f.placedOn[p.UUID]
		selfHosted.GatewayFunctionalityType = gwmodels.GatewayRoleBoth
		f.gatewayRepo.ListWithFiltersFunc = func(gwrepo.GatewayFilterOptions) ([]*gwmodels.Gateway, error) {
			return []*gwmodels.Gateway{selfHosted}, nil
		}
		f.svc.logger = slog.Default()
		f.svc.ocClient = &repomocks.PlatformRepositoryMock{
			GetProjectFunc: func(context.Context, string, string) (*models.ProjectResponse, error) {
				return &models.ProjectResponse{UUID: uuid.NewString()}, nil
			},
		}
		f.svc.llmProxyService = &LLMProxyService{providerRepo: &gwrepomocks.LLMProviderRepositoryMock{
			GetByUUIDFunc: func(string, string) (*gwmodels.LLMProvider, error) { return nil, errReachedProxyWrite },
		}}
		return f
	}
	processNewEnv := func(f *llmPlacementFixture, policy placementPolicy) error {
		config := &gwmodels.AgentConfiguration{UUID: uuid.New(), OUID: llmPlacementOU, ProjectName: "proj", AgentID: "agent", Name: "cfg"}
		_, _, err := f.svc.processNewEnv(context.Background(), config.UUID, config, f.envs["dev"], f.env("dev"), "dev",
			gwmodels.EnvModelConfigRequest{ProviderName: "self-p"}, llmPlacementOU, nil, policy, "")
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
	newFixture := func(t *testing.T) (*llmPlacementFixture, *gwmodels.EnvAgentModelMapping) {
		f := newLLMPlacementFixture(t, "dev")
		f.provider("self-p", gwmodels.GatewayHostingSelfHosted)
		proxy := &gwmodels.LLMProxy{UUID: uuid.New(), Handle: "cfg-proxy"}
		f.placedOn[proxy.UUID] = &gwmodels.Gateway{UUID: uuid.New(), Hosting: gwmodels.GatewayHostingSelfHosted}
		f.svc.llmProxyDeploymentService.proxyRepo = &gwrepomocks.LLMProxyRepositoryMock{
			GetByIDFunc: func(string, string) (*gwmodels.LLMProxy, error) { return proxy, nil },
		}
		f.svc.logger = slog.Default()
		f.svc.llmProxyService = &LLMProxyService{proxyRepo: &gwrepomocks.LLMProxyRepositoryMock{
			GetByIDFunc: func(string, string) (*gwmodels.LLMProxy, error) { return nil, errReachedProxyWrite },
		}}
		return f, &gwmodels.EnvAgentModelMapping{EnvironmentUUID: f.env("dev"), LLMProxy: proxy}
	}
	processEnvProxyUpdate := func(f *llmPlacementFixture, mapping *gwmodels.EnvAgentModelMapping, policy placementPolicy) error {
		config := &gwmodels.AgentConfiguration{UUID: uuid.New(), OUID: llmPlacementOU, ProjectName: "proj", AgentID: "agent", Name: "cfg"}
		_, err := f.svc.processEnvProxyUpdate(context.Background(), config, f.envs["dev"], f.env("dev"), "dev",
			gwmodels.EnvModelConfigRequest{ProviderName: "self-p"}, mapping, llmPlacementOU, policy)
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
	proxy := &gwmodels.LLMProxy{UUID: uuid.New(), Handle: "cfg-proxy", Configuration: gwmodels.LLMProxyConfig{Context: &contextPath}}
	f.placedOn[proxy.UUID] = &gwmodels.Gateway{UUID: uuid.New(), Vhost: "gw.example.com", Hosting: gwmodels.GatewayHostingSelfHosted}
	f.svc.llmProxyDeploymentService.proxyRepo = &gwrepomocks.LLMProxyRepositoryMock{
		GetByIDFunc: func(string, string) (*gwmodels.LLMProxy, error) { return proxy, nil },
	}
	f.svc.llmProviderRepo = &gwrepomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc: func(string, string) (*gwmodels.LLMProvider, error) { return nil, gorm.ErrRecordNotFound },
	}
	f.svc.logger = slog.Default()
	config := &gwmodels.AgentConfiguration{
		UUID: uuid.New(), OUID: llmPlacementOU, ProjectName: "proj", AgentID: "agent", Name: "cfg",
		EnvMappings: []gwmodels.EnvAgentModelMapping{{EnvironmentUUID: f.env("dev"), LLMProxy: proxy}},
	}

	resp, err := f.svc.buildConfigResponse(context.Background(), config, externalAgentPolicy)

	require.NoError(t, err)
	require.NotNil(t, resp.EnvModelConfig["dev"].LLMProxy.URL)
	assert.Equal(t, "https://gw.example.com/llm/proxy", *resp.EnvModelConfig["dev"].LLMProxy.URL)
}

// agent sets the agent ProviderEnvironments reads its placement policy from.
func (f *llmPlacementFixture) agent(provisioning string) {
	f.svc.ocClient = &repomocks.PlatformRepositoryMock{
		GetAgentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Provisioning: models.Provisioning{Type: provisioning}}, nil
		},
	}
}

// ProviderEnvironments is the default the MCP attach tool maps a provider into, so it must be exactly the
// environments Create's placement check accepts: a provider deployed only to dev is offered in dev alone.
func TestProviderEnvironments_OnlyEnvironmentsTheProviderIsPlacedIn(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev", "prod")
	f.agent(string(gwutil.InternalAgent))
	f.provider("dev-only", gwmodels.GatewayHostingManaged)
	devUUID := f.envs["dev"].UUID
	f.gatewayRepo.EnvironmentMappingExistsFunc = func(_ string, envUUID string) (bool, error) { return envUUID == devUUID, nil }

	got, err := f.svc.ProviderEnvironments(context.Background(), llmPlacementOU, "proj", "agent", "dev-only")

	require.NoError(t, err)
	assert.Equal(t, []string{"dev"}, got)
	requested := map[string]gwmodels.EnvModelConfigRequest{"dev": {ProviderName: "dev-only"}}
	require.NoError(t, f.svc.requireProviderPlacements(context.Background(), llmPlacementOU, requested, f.envs, nil, platformAgentPolicy))
}

// The agent's kind decides which placements count, as it does in Create.
func TestProviderEnvironments_FollowsTheAgentsPlacementPolicy(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev", "prod")
	f.provider("self-p", gwmodels.GatewayHostingSelfHosted)

	f.agent(string(gwutil.InternalAgent))
	got, err := f.svc.ProviderEnvironments(context.Background(), llmPlacementOU, "proj", "agent", "self-p")
	require.NoError(t, err)
	assert.Empty(t, got, "a platform-hosted agent cannot use a provider placed only on a self-hosted gateway")

	f.agent(string(gwutil.ExternalAgent))
	got, err = f.svc.ProviderEnvironments(context.Background(), llmPlacementOU, "proj", "agent", "self-p")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"dev", "prod"}, got)
}

func TestProviderEnvironments_UnknownProvider(t *testing.T) {
	f := newLLMPlacementFixture(t, "dev")
	f.agent(string(gwutil.InternalAgent))

	_, err := f.svc.ProviderEnvironments(context.Background(), llmPlacementOU, "proj", "agent", "missing")

	require.ErrorIs(t, err, gwerr.ErrLLMProviderNotFound)
}

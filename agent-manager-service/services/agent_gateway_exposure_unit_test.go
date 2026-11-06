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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// gatewayExposureHarness exposes agents through a reconcile core over publisherMocks, whose plain and
// transaction-bound mocks stay apart: an exposure or artifact write outside the exposure transaction panics or fails.
type gatewayExposureHarness struct {
	*publisherMocks
	gateway *models.Gateway
	// pending are the PENDING rows the core inserted, all in the exposure transaction.
	pending   []models.Placement
	exposure  *AgentExposurePublisher
	a2a       *A2APublisher
	restKind  *RestAgentKind
	txConfigs *repomocks.AgentConfigRepositoryMock
}

func newGatewayExposureHarness(t *testing.T) *gatewayExposureHarness {
	t.Helper()
	m := newPublisherMocks(t)
	h := &gatewayExposureHarness{
		publisherMocks: m,
		gateway: &models.Gateway{
			UUID: uuid.New(), Vhost: "https://agents.example.com", GatewayFunctionalityType: models.GatewayRoleIngress,
		},
		txConfigs: &repomocks.AgentConfigRepositoryMock{UpsertFunc: func(context.Context, *models.AgentConfig) error { return nil }},
	}
	m.gateways.ListWithFiltersFunc = func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
		if h.gateway == nil {
			return nil, nil
		}
		return []*models.Gateway{h.gateway}, nil
	}
	m.artifacts.WithTxFunc = func(tx *gorm.DB) repositories.ArtifactRepository {
		assert.Same(t, txSentinel, tx, "the artifact is ensured in the exposure transaction")
		return m.artifacts
	}
	m.deployments.LockEnvironmentMappingFunc = mappedEverywhere
	m.deployments.InsertPendingFunc = func(_ context.Context, tx *gorm.DB, p models.Placement, _ string) (bool, error) {
		assert.Same(t, txSentinel, tx, "the PENDING row is written in the exposure transaction")
		h.pending = append(h.pending, p)
		return true, nil
	}
	m.txExposures.UpsertFunc = func(context.Context, *models.PlatformAgentExposure) error { return nil }
	m.exposures.GetFunc = exposedAgent
	exposures := NewPlatformAgentExposures(m.exposures, m.gateways, nil)
	a2aKind := &A2AAgentKind{exposures: exposures, proxyRepo: m.proxies, cardRepo: m.envCards, deploymentRepo: m.deployments, logger: testLogger()}
	h.restKind = &RestAgentKind{exposures: exposures, logger: testLogger()}
	core := reconcile.NewCore(nil, m.deployments, m.hub, reconcile.Kinds{a2aKind, h.restKind}, reconcile.Config{}, testLogger())
	h.exposure = NewAgentExposurePublisher(m.tx(), m.exposures, m.artifacts, m.deployments, m.gateways, core, testLogger())
	h.a2a = NewA2APublisher(m.tx(), m.exposures, m.proxies, m.envCards, m.artifacts, m.deployments, testLogger(), h.exposure, a2aKind)
	return h
}

// wire gives s the harness's publishers, artifacts and gateways, and saves its config in the exposure transaction.
func (h *gatewayExposureHarness) wire(t *testing.T, s *agentManagerService) {
	t.Helper()
	s.artifactRepo = h.artifacts
	s.gatewayRepo = h.gateways
	s.deploymentRepo = h.deployments
	s.a2aPublisher = h.a2a
	s.exposure = h.exposure
	s.restAgentKind = h.restKind
	plain, _ := s.agentConfigRepo.(*repomocks.AgentConfigRepositoryMock)
	if plain == nil {
		plain = &repomocks.AgentConfigRepositoryMock{}
		s.agentConfigRepo = plain
	}
	plain.WithTxFunc = func(tx *gorm.DB) repositories.AgentConfigRepository {
		assert.Same(t, txSentinel, tx, "the config is saved in the exposure transaction")
		return h.txConfigs
	}
}

// existingArtifact makes the agent's per-environment artifact a row of kind.
func (h *gatewayExposureHarness) existingArtifact(kind string) *models.Artifact {
	artifact := &models.Artifact{UUID: uuid.New(), OUID: "acme", Handle: "proj1/my-agent/env", Kind: kind}
	h.artifacts.GetByHandleFunc = func(string, string) (*models.Artifact, error) { return artifact, nil }
	return artifact
}

func (h *gatewayExposureHarness) exposed(t *testing.T) models.PlatformAgentExposure {
	t.Helper()
	calls := h.txExposures.UpsertCalls()
	require.Len(t, calls, 1, "one exposure, written in the transaction")
	return *calls[0].Exposure
}

func internalAPIAgent(subType string) *models.AgentResponse {
	return &models.AgentResponse{
		UUID: uuid.NewString(), Name: "my-agent", Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
		Type:           models.AgentType{Type: string(utils.AgentTypeAPI), SubType: subType},
		InputInterface: &models.InputInterface{Port: 8000, BasePath: "/"},
	}
}

var apiAgentShapes = []struct {
	subType  string
	kind     string
	protocol models.AgentExposureProtocol
}{
	{string(utils.AgentSubTypeChatAPI), models.KindRestAPI, models.AgentExposureProtocolREST},
	{string(utils.AgentSubTypeCustomAPI), models.KindRestAPI, models.AgentExposureProtocolREST},
	{string(utils.AgentSubTypeA2A), models.KindAgent, models.AgentExposureProtocolA2A},
}

func TestDeployAgentDetachesRouteTraitsAndExposes(t *testing.T) {
	for _, tc := range apiAgentShapes {
		t.Run(tc.subType, func(t *testing.T) {
			s, deployConfig := deployAPIAgentMocks(nil)
			h := newGatewayExposureHarness(t)
			h.wire(t, s)
			artifact := h.existingArtifact(tc.kind)
			oc := s.ocClient.(*clientmocks.OpenChoreoClientMock)
			agent := internalAPIAgent(tc.subType)
			oc.GetComponentFunc = func(context.Context, string, string, string) (*models.AgentResponse, error) { return agent, nil }
			var traitEnvConfigs map[string]interface{}
			oc.EnsureReleaseAndBindingFunc = func(_ context.Context, _, _, _, _ string, _ []client.EnvVar, _ []client.FileVar,
				traits map[string]interface{}, _ map[string]interface{},
			) error {
				traitEnvConfigs = traits
				return nil
			}

			_, err := s.DeployAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.DeployAgentRequest{ImageId: "registry.example.com/my-agent:v1"})

			require.NoError(t, err)
			assert.ElementsMatch(t, []client.TraitType{client.TraitAPIManagement, client.TraitA2AGatewayRoute}, deployConfig.TraitsToDetach)
			assert.Nil(t, findTraitRequest(deployConfig.TraitsToAttach, client.TraitAPIManagement))
			assert.Nil(t, findTraitRequest(deployConfig.TraitsToAttach, client.TraitA2AGatewayRoute))
			assert.NotContains(t, traitEnvConfigs, "my-agent-"+string(client.TraitAPIManagement))
			exposure := h.exposed(t)
			assert.Equal(t, tc.protocol, exposure.Protocol)
			assert.Equal(t, artifact.UUID, exposure.ArtifactUUID)
			require.Len(t, h.pending, 1)
			assert.Equal(t, tc.kind, h.pending[0].Kind)
			assert.Equal(t, artifact.UUID, h.pending[0].ArtifactUUID)
			assert.Equal(t, h.gateway.UUID, h.pending[0].GatewayUUID)
			assert.Len(t, h.txConfigs.UpsertCalls(), 1)
			assert.Equal(t, 1, h.transactions)
		})
	}
}

func TestCreateAttachesNoRouteTraits(t *testing.T) {
	for _, tc := range apiAgentShapes {
		t.Run(tc.subType, func(t *testing.T) {
			s := &agentManagerService{logger: testLogger()}
			subType := tc.subType
			req := &spec.CreateAgentRequest{
				Name:      "my-agent",
				AgentType: &spec.AgentType{Type: string(utils.AgentTypeAPI), SubType: &subType},
			}

			traits, _, err := s.buildCreateTraitRequests(context.Background(), "acme", "proj1", "dev", req)

			require.NoError(t, err)
			assert.Nil(t, findTraitRequest(traits, client.TraitAPIManagement))
			assert.Nil(t, findTraitRequest(traits, client.TraitA2AGatewayRoute))
		})
	}
}

// promoteRestAgentHarness is PromoteAgent of a chat-api agent from dev to staging, whose dev release carries
// sourceTraits and whose staging artifact is a row of kind; promoted holds the trait env configs PromoteComponent got.
func promoteRestAgentHarness(t *testing.T, kind string, sourceTraits ...client.TraitType) (
	s *agentManagerService, h *gatewayExposureHarness, artifact *models.Artifact, promoted *map[string]interface{},
) {
	t.Helper()
	s, called := promoteAgentTestFixture(t, []client.EnvVar{{Key: client.EnvVarAgentIDClientID, Value: "staging-client-id"}}, nil)
	h = newGatewayExposureHarness(t)
	s.agentConfigRepo = &repomocks.AgentConfigRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentConfig, error) {
			return &models.AgentConfig{}, nil
		},
	}
	h.wire(t, s)
	artifact = h.existingArtifact(kind)
	stagingUUID := uuid.New()
	oc := s.ocClient.(*clientmocks.OpenChoreoClientMock)
	oc.GetComponentFunc = func(context.Context, string, string, string) (*models.AgentResponse, error) {
		return internalAPIAgent(string(utils.AgentSubTypeChatAPI)), nil
	}
	oc.GetEnvironmentFunc = func(_ context.Context, _, envName string) (*models.EnvironmentResponse, error) {
		return &models.EnvironmentResponse{Name: envName, UUID: stagingUUID.String()}, nil
	}
	oc.GetReleaseBindingTraitsFunc = func(_ context.Context, _, _, environment string) ([]client.TraitType, bool, error) {
		assert.Equal(t, "dev", environment, "a promote follows the source environment's release")
		return sourceTraits, true, nil
	}
	promoted = new(map[string]interface{})
	oc.PromoteComponentFunc = func(_ context.Context, _, _, _, _, _ string, _ []client.EnvVar, _ []client.FileVar,
		traits, _ map[string]interface{},
	) error {
		*called = true
		*promoted = traits
		return nil
	}
	return s, h, artifact, promoted
}

func promoteToStaging(t *testing.T, s *agentManagerService) error {
	t.Helper()
	return s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev", TargetEnvironment: "staging",
	})
}

// apiConfigurationEnvConfig is the api-configuration trait's env config in traitEnvConfigs, or nil.
func apiConfigurationEnvConfig(traitEnvConfigs map[string]interface{}) map[string]interface{} {
	cfg, _ := traitEnvConfigs["my-agent-"+string(client.TraitAPIManagement)].(map[string]interface{})
	return cfg
}

func TestPromoteGatewayPushedExposes(t *testing.T) {
	s, h, artifact, promoted := promoteRestAgentHarness(t, models.KindRestAPI, client.TraitOTELInstrumentation)

	require.NoError(t, promoteToStaging(t, s))

	assert.Nil(t, apiConfigurationEnvConfig(*promoted))
	exposure := h.exposed(t)
	assert.Equal(t, models.AgentExposureProtocolREST, exposure.Protocol)
	assert.Equal(t, artifact.UUID, exposure.ArtifactUUID)
	require.Len(t, h.pending, 1)
	assert.Equal(t, models.KindRestAPI, h.pending[0].Kind)
	assert.Len(t, h.txConfigs.UpsertCalls(), 1)
}

// noPlacements makes the target hold no gateway-pushed release: the legacy promote's teardown finds nothing.
func (h *gatewayExposureHarness) noPlacements() {
	h.txDeployments.PlacedGatewaysFunc = func(context.Context, string, uuid.UUID) ([]string, error) { return nil, nil }
	h.txExposures.DeleteForAgentEnvFunc = func(context.Context, string, string, string, uuid.UUID) error { return nil }
}

func TestPromoteLegacyRESTStaysLegacy(t *testing.T) {
	s, h, artifact, promoted := promoteRestAgentHarness(t, models.KindAgent, client.TraitAPIManagement, client.TraitOTELInstrumentation)
	h.noPlacements()

	require.NoError(t, promoteToStaging(t, s))

	apiConfig := apiConfigurationEnvConfig(*promoted)
	require.NotNil(t, apiConfig, "PromoteComponent gets the api-configuration env config")
	assert.Equal(t, artifact.UUID.String(), apiConfig["artifactId"])
	assert.Empty(t, h.txExposures.UpsertCalls(), "no exposure is written")
	assert.Empty(t, h.pending)
	assert.Len(t, h.txConfigs.UpsertCalls(), 1)
}

func TestPromoteLegacyOverGatewayPushedTearsDown(t *testing.T) {
	s, h, artifact, promoted := promoteRestAgentHarness(t, models.KindRestAPI, client.TraitAPIManagement)
	h.txDeployments.PlacedGatewaysFunc = func(context.Context, string, uuid.UUID) ([]string, error) {
		return []string{h.gateway.UUID.String()}, nil
	}
	h.txDeployments.WithTxFunc = func(*gorm.DB) repositories.DeploymentRepository { return h.txDeployments }
	h.txDeployments.GetStatusRecordFunc = func(context.Context, models.Placement) (*models.DeploymentStatusRecord, error) {
		return &models.DeploymentStatusRecord{PerformedAt: models.NewPerformedAt(time.Now())}, nil
	}
	h.deployments.BeginUndeployFunc = func(_ context.Context, tx *gorm.DB, p models.Placement) (bool, error) {
		assert.Same(t, txSentinel, tx)
		assert.Equal(t, artifact.UUID, p.ArtifactUUID)
		return true, nil
	}
	h.txExposures.DeleteForAgentEnvFunc = func(context.Context, string, string, string, uuid.UUID) error { return nil }

	require.NoError(t, promoteToStaging(t, s))

	assert.Equal(t, artifact.UUID.String(), apiConfigurationEnvConfig(*promoted)["artifactId"])
	require.Len(t, h.hub.published, 1)
	assert.Equal(t, "api.undeployed", string(h.hub.published[0].EventType))
	assert.Same(t, txSentinel, h.hub.txs[0])
	assert.Len(t, h.txExposures.DeleteForAgentEnvCalls(), 1, "the exposure is deleted")
	assert.Empty(t, h.txExposures.UpsertCalls())
	assert.Len(t, h.txConfigs.UpsertCalls(), 1)
	assert.Equal(t, 1, h.transactions, "the config save, undeploy and exposure delete share one transaction")
}

// deploySettingsHarness is UpdateAgentDeploySettings for a chat-api agent whose RestApi is placed on the gateway,
// with a gateway-pushed release bound in dev.
func deploySettingsHarness(t *testing.T) (*agentManagerService, *gatewayExposureHarness, *models.Artifact) {
	t.Helper()
	oc := &clientmocks.OpenChoreoClientMock{
		GetReleaseBindingTraitsFunc: func(context.Context, string, string, string) ([]client.TraitType, bool, error) {
			return []client.TraitType{client.TraitOTELInstrumentation}, true, nil
		},
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return internalAPIAgent(string(utils.AgentSubTypeChatAPI)), nil
		},
		GetEnvironmentFunc: envWithUUID(),
		UpdateReleaseBindingTraitConfigsFunc: func(context.Context, string, string, string, map[string]interface{}, map[string]interface{}) error {
			return nil
		},
		GetReleaseBindingRolloutFunc: func(context.Context, string, string, string) (client.ReleaseBindingRollout, error) {
			return client.ReleaseBindingRollout{Deployed: true}, nil
		},
	}
	s := &agentManagerService{
		ocClient: oc,
		agentConfigRepo: &repomocks.AgentConfigRepositoryMock{
			GetFunc: func(context.Context, string, string, string, string) (*models.AgentConfig, error) {
				return nil, repositories.ErrAgentConfigNotFound
			},
		},
		logger: testLogger(),
	}
	h := newGatewayExposureHarness(t)
	h.wire(t, s)
	return s, h, h.existingArtifact(models.KindRestAPI)
}

// placedRow makes the agent's artifact already placed on the gateway, so a deploy re-queues its row.
func (h *gatewayExposureHarness) placedRow(t *testing.T) {
	t.Helper()
	h.deployments.InsertPendingFunc = func(_ context.Context, tx *gorm.DB, _ models.Placement, _ string) (bool, error) {
		assert.Same(t, txSentinel, tx)
		return false, nil
	}
}

func TestDeploySettingsRestAgentRequeues(t *testing.T) {
	timeout := int32(90)
	req := &spec.UpdateAgentDeploySettingsRequest{EnvironmentName: "dev", ResilienceTimeoutSeconds: &timeout}

	t.Run("re-renders the RestApi from the saved settings", func(t *testing.T) {
		s, h, artifact := deploySettingsHarness(t)
		h.placedRow(t)
		var traitEnvConfigs map[string]interface{}
		s.ocClient.(*clientmocks.OpenChoreoClientMock).UpdateReleaseBindingTraitConfigsFunc = func(
			_ context.Context, _, _, _ string, traits map[string]interface{}, _ map[string]interface{},
		) error {
			traitEnvConfigs = traits
			return nil
		}

		require.NoError(t, s.UpdateAgentDeploySettings(tierGrantedCtx(t), "acme", "proj1", "my-agent", req))

		assert.NotContains(t, traitEnvConfigs, "my-agent-"+string(client.TraitAPIManagement))
		saved := h.txConfigs.UpsertCalls()
		require.Len(t, saved, 1)
		assert.Equal(t, timeout, *saved[0].Config.ResilienceTimeoutSeconds)
		assert.Equal(t, models.AgentExposureProtocolREST, h.exposed(t).Protocol)
		queued := h.deployments.QueueRenderCalls()
		require.Len(t, queued, 1, "the placed row is queued")
		assert.Equal(t, artifact.UUID, queued[0].ArtifactUUID)
		assert.Same(t, txSentinel, queued[0].Tx, "in the save's transaction")
	})
	t.Run("a queue failure fails the request", func(t *testing.T) {
		s, h, _ := deploySettingsHarness(t)
		h.placedRow(t)
		h.deployments.QueueRenderFunc = func(context.Context, *gorm.DB, string, uuid.UUID, []uuid.UUID, string, repositories.QueueScope) (int64, error) {
			return 0, assert.AnError
		}

		err := s.UpdateAgentDeploySettings(tierGrantedCtx(t), "acme", "proj1", "my-agent", req)

		assert.ErrorIs(t, err, assert.AnError)
	})
}

// A settings save in an environment the agent is undeployed from saves the config and writes the release binding,
// but exposes nothing: re-activating the environment exposes the agent with the saved settings.
func TestDeploySettingsUndeployedOnlySaves(t *testing.T) {
	for _, subType := range []string{string(utils.AgentSubTypeChatAPI), string(utils.AgentSubTypeA2A)} {
		t.Run(subType, func(t *testing.T) {
			s, h, _ := deploySettingsHarness(t)
			oc := s.ocClient.(*clientmocks.OpenChoreoClientMock)
			oc.GetComponentFunc = func(context.Context, string, string, string) (*models.AgentResponse, error) {
				return internalAPIAgent(subType), nil
			}
			oc.GetReleaseBindingRolloutFunc = func(context.Context, string, string, string) (client.ReleaseBindingRollout, error) {
				return client.ReleaseBindingRollout{Deployed: false}, nil
			}
			plain := s.agentConfigRepo.(*repomocks.AgentConfigRepositoryMock)
			plain.UpsertFunc = func(context.Context, *models.AgentConfig) error { return nil }

			require.NoError(t, s.UpdateAgentDeploySettings(tierGrantedCtx(t), "acme", "proj1", "my-agent",
				&spec.UpdateAgentDeploySettingsRequest{EnvironmentName: "dev"}))

			assert.Len(t, plain.UpsertCalls(), 1, "the settings are saved")
			assert.Len(t, oc.UpdateReleaseBindingTraitConfigsCalls(), 1, "the release binding takes them")
			assert.Empty(t, h.txExposures.UpsertCalls(), "nothing is exposed")
			assert.Empty(t, h.pending)
			assert.Empty(t, h.deployments.QueueRenderCalls())
			assert.Empty(t, h.hub.published)
		})
	}
}

func TestDeploySettingsUnboundUsesGatewayPushed(t *testing.T) {
	s, h, artifact := deploySettingsHarness(t)
	s.ocClient.(*clientmocks.OpenChoreoClientMock).GetReleaseBindingTraitsFunc = func(
		context.Context, string, string, string,
	) ([]client.TraitType, bool, error) {
		return nil, false, nil
	}

	require.NoError(t, s.UpdateAgentDeploySettings(tierGrantedCtx(t), "acme", "proj1", "my-agent",
		&spec.UpdateAgentDeploySettingsRequest{EnvironmentName: "dev"}))

	assert.Equal(t, models.AgentExposureProtocolREST, h.exposed(t).Protocol)
	require.Len(t, h.pending, 1)
	assert.Equal(t, artifact.UUID, h.pending[0].ArtifactUUID)
}

func TestDeploySettingsLegacyWritesTraitConfig(t *testing.T) {
	s, h, _ := deploySettingsHarness(t)
	artifact := h.existingArtifact(models.KindAgent)
	oc := s.ocClient.(*clientmocks.OpenChoreoClientMock)
	oc.GetReleaseBindingTraitsFunc = func(_ context.Context, _, _, environment string) ([]client.TraitType, bool, error) {
		assert.Equal(t, "dev", environment)
		return []client.TraitType{client.TraitAPIManagement}, true, nil
	}
	var traitEnvConfigs map[string]interface{}
	oc.UpdateReleaseBindingTraitConfigsFunc = func(
		_ context.Context, _, _, _ string, traits map[string]interface{}, _ map[string]interface{},
	) error {
		traitEnvConfigs = traits
		return nil
	}
	plain := s.agentConfigRepo.(*repomocks.AgentConfigRepositoryMock)
	plain.UpsertFunc = func(context.Context, *models.AgentConfig) error { return nil }
	timeout := int32(90)

	require.NoError(t, s.UpdateAgentDeploySettings(tierGrantedCtx(t), "acme", "proj1", "my-agent",
		&spec.UpdateAgentDeploySettingsRequest{EnvironmentName: "dev", ResilienceTimeoutSeconds: &timeout}))

	apiConfig := apiConfigurationEnvConfig(traitEnvConfigs)
	require.NotNil(t, apiConfig, "the settings reach the release's api-configuration trait")
	assert.Equal(t, artifact.UUID.String(), apiConfig["artifactId"])
	assert.Equal(t, client.FormatResilienceTimeout(timeout), apiConfig["resilienceTimeout"])
	require.Len(t, plain.UpsertCalls(), 1)
	assert.Equal(t, timeout, *plain.UpsertCalls()[0].Config.ResilienceTimeoutSeconds)
	assert.Empty(t, h.txExposures.UpsertCalls(), "no exposure is written")
	assert.Empty(t, h.deployments.QueueRenderCalls())
	assert.Empty(t, h.artifacts.UpdateCalls(), "a legacy release never switches the artifact")
}

// endpointsHarness serves GetAgentEndpoints for a chat-api agent whose OpenChoreo endpoint is openChoreoURL.
func endpointsHarness(t *testing.T) (*agentManagerService, *gatewayExposureHarness) {
	t.Helper()
	envUUID := uuid.New()
	oc := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetProjectFunc: func(_ context.Context, _, name string) (*models.ProjectResponse, error) {
			return &models.ProjectResponse{Name: name, DeploymentPipeline: "default"}, nil
		},
		GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return internalAPIAgent(string(utils.AgentSubTypeChatAPI)), nil
		},
		GetEnvironmentFunc: func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{Name: name, UUID: envUUID.String()}, nil
		},
		GetComponentEndpointsFunc: func(context.Context, string, string, string, string) (map[string]models.EndpointsResponse, error) {
			return map[string]models.EndpointsResponse{
				"public":   {Endpoint: models.Endpoint{URL: openChoreoURL, Name: "public", Visibility: "Public"}, Schema: models.EndpointSchema{Content: "openapi"}},
				"internal": {Endpoint: models.Endpoint{URL: "http://my-agent.svc:8000", Name: "internal", Visibility: "Project"}},
			}, nil
		},
	}
	s := &agentManagerService{ocClient: oc, logger: testLogger()}
	h := newGatewayExposureHarness(t)
	h.wire(t, s)
	return s, h
}

const openChoreoURL = "http://default-default.am-gateway.localhost:19080/my-agent"

// placedRows makes the artifact's rows the given ones.
func (h *gatewayExposureHarness) placedRows(rows ...models.DeploymentStatusRecord) {
	h.deployments.StatusRecordsFunc = func(context.Context, string, uuid.UUID) ([]models.DeploymentStatusRecord, error) {
		return rows, nil
	}
}

func (h *gatewayExposureHarness) deployedRow() models.DeploymentStatusRecord {
	return models.DeploymentStatusRecord{
		GatewayUUID: h.gateway.UUID, Status: models.DeploymentStatusDeployed, StatusDesired: models.DeploymentStatusDeployed,
	}
}

func TestAgentEndpointsUseGatewayURL(t *testing.T) {
	t.Run("placed: every endpoint is the gateway URL", func(t *testing.T) {
		s, h := endpointsHarness(t)
		h.existingArtifact(models.KindRestAPI)
		h.placedRows(h.deployedRow())

		endpoints, err := s.GetAgentEndpoints(context.Background(), "acme", "proj1", "my-agent", "dev")

		require.NoError(t, err)
		require.Len(t, endpoints, 2)
		for name, endpoint := range endpoints {
			assert.Equal(t, "https://agents.example.com/my-agent", endpoint.URL, name)
		}
		assert.Equal(t, "openapi", endpoints["public"].Schema.Content, "schema kept")
		assert.Equal(t, "Project", endpoints["internal"].Visibility, "visibility kept")
	})
	t.Run("exposed, not placed yet: no URL", func(t *testing.T) {
		s, h := endpointsHarness(t)
		h.existingArtifact(models.KindRestAPI)
		h.placedRows()

		endpoints, err := s.GetAgentEndpoints(context.Background(), "acme", "proj1", "my-agent", "dev")

		require.NoError(t, err)
		for name, endpoint := range endpoints {
			assert.Empty(t, endpoint.URL, name)
		}
	})
	t.Run("gateway deleted: no URL", func(t *testing.T) {
		s, h := endpointsHarness(t)
		h.existingArtifact(models.KindRestAPI)
		h.placedRows()
		h.gateway = nil

		endpoints, err := s.GetAgentEndpoints(context.Background(), "acme", "proj1", "my-agent", "dev")

		require.NoError(t, err)
		for name, endpoint := range endpoints {
			assert.Empty(t, endpoint.URL, name)
		}
		key := agentExposureKey{OUID: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "dev"}
		status, err := h.exposure.ShownStatus(context.Background(), key, false)
		require.NoError(t, err)
		assert.Equal(t, noGatewayReason, status.Reason)
	})
}

func TestDeploymentResponseCarriesStatusReason(t *testing.T) {
	envUUID := uuid.New()
	soon := time.Now().Add(time.Minute)
	oc := &clientmocks.OpenChoreoClientMock{
		GetProjectFunc: func(_ context.Context, _, name string) (*models.ProjectResponse, error) {
			return &models.ProjectResponse{Name: name, DeploymentPipeline: "default"}, nil
		},
		GetDeploymentsFunc: func(context.Context, string, string, string, string) ([]*models.DeploymentResponse, error) {
			return []*models.DeploymentResponse{
				{
					Environment: "dev", EnvironmentUUID: envUUID.String(), Status: "Active",
					Endpoints: []models.Endpoint{{URL: openChoreoURL, Name: "public", Visibility: "Public"}},
				},
				{Environment: "prod", EnvironmentUUID: uuid.NewString(), Status: client.DeploymentStatusNotDeployed, Endpoints: []models.Endpoint{}},
			}, nil
		},
		GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return internalAPIAgent(string(utils.AgentSubTypeChatAPI)), nil
		},
		ListEnvironmentsFunc: func(context.Context, string) ([]*models.EnvironmentResponse, error) { return nil, nil },
	}
	s := &agentManagerService{ocClient: oc, logger: testLogger()}
	h := newGatewayExposureHarness(t)
	h.wire(t, s)
	h.existingArtifact(models.KindRestAPI)
	h.placedRows(models.DeploymentStatusRecord{
		GatewayUUID: h.gateway.UUID, Status: models.DeploymentStatusPending, StatusDesired: models.DeploymentStatusUndeployed,
		NextAttemptAt: &soon, RenderReason: waitingForEndpoint,
	})

	deployments, err := s.GetAgentDeployments(context.Background(), "acme", "proj1", "my-agent")
	require.NoError(t, err)
	response := utils.ConvertToDeploymentDetailsResponse(deployments)

	dev := response["dev"]
	require.NotNil(t, dev.GatewayStatus)
	assert.Equal(t, string(models.DeploymentStatusDeploying), *dev.GatewayStatus)
	require.NotNil(t, dev.GatewayStatusReason)
	assert.Equal(t, waitingForEndpoint, *dev.GatewayStatusReason)
	assert.Equal(t, "https://agents.example.com/my-agent", dev.Endpoints[0].Url)
	assert.Nil(t, response["prod"].GatewayStatus, "an environment with no binding has no gateway status")
}

// An environment whose gateway deployment can't be read keeps its OpenChoreo URL and shows no gateway status; the
// others are still resolved.
func TestDeploymentResponseFallsBackWhenGatewayUnreadable(t *testing.T) {
	devUUID, stagingUUID := uuid.New(), uuid.New()
	oc := &clientmocks.OpenChoreoClientMock{
		GetProjectFunc: func(_ context.Context, _, name string) (*models.ProjectResponse, error) {
			return &models.ProjectResponse{Name: name, DeploymentPipeline: "default"}, nil
		},
		GetDeploymentsFunc: func(context.Context, string, string, string, string) ([]*models.DeploymentResponse, error) {
			return []*models.DeploymentResponse{
				{
					Environment: "dev", EnvironmentUUID: devUUID.String(), Status: "Active",
					Endpoints: []models.Endpoint{{URL: openChoreoURL, Name: "public", Visibility: "Public"}},
				},
				{
					Environment: "staging", EnvironmentUUID: stagingUUID.String(), Status: "Active",
					Endpoints: []models.Endpoint{{URL: openChoreoURL, Name: "public", Visibility: "Public"}},
				},
			}, nil
		},
		GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return internalAPIAgent(string(utils.AgentSubTypeChatAPI)), nil
		},
		ListEnvironmentsFunc: func(context.Context, string) ([]*models.EnvironmentResponse, error) { return nil, nil },
	}
	s := &agentManagerService{ocClient: oc, logger: testLogger()}
	h := newGatewayExposureHarness(t)
	h.wire(t, s)
	h.existingArtifact(models.KindRestAPI)
	h.placedRows(h.deployedRow())
	h.gateways.ListWithFiltersFunc = func(opts repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
		if opts.EnvironmentID != nil && *opts.EnvironmentID == stagingUUID.String() {
			return nil, assert.AnError
		}
		return []*models.Gateway{h.gateway}, nil
	}

	deployments, err := s.GetAgentDeployments(context.Background(), "acme", "proj1", "my-agent")
	require.NoError(t, err)
	response := utils.ConvertToDeploymentDetailsResponse(deployments)

	staging := response["staging"]
	assert.Nil(t, staging.GatewayStatus)
	assert.Nil(t, staging.GatewayStatusReason)
	assert.Equal(t, openChoreoURL, staging.Endpoints[0].Url)
	dev := response["dev"]
	require.NotNil(t, dev.GatewayStatus)
	assert.Equal(t, string(models.DeploymentStatusDeployed), *dev.GatewayStatus)
	assert.Equal(t, "https://agents.example.com/my-agent", dev.Endpoints[0].Url)
}

// A REST agent still on its trait has no exposure: it is reached at its OpenChoreo URL and its deployment shows no
// gateway status, though its environment has an ingress gateway.
func TestDeploymentResponseOmitsGatewayStatusWithoutExposure(t *testing.T) {
	oc := &clientmocks.OpenChoreoClientMock{
		GetProjectFunc: func(_ context.Context, _, name string) (*models.ProjectResponse, error) {
			return &models.ProjectResponse{Name: name, DeploymentPipeline: "default"}, nil
		},
		GetDeploymentsFunc: func(context.Context, string, string, string, string) ([]*models.DeploymentResponse, error) {
			return []*models.DeploymentResponse{{
				Environment: "dev", EnvironmentUUID: uuid.NewString(), Status: "Active",
				Endpoints: []models.Endpoint{{URL: openChoreoURL, Name: "public", Visibility: "Public"}},
			}}, nil
		},
		GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return internalAPIAgent(string(utils.AgentSubTypeChatAPI)), nil
		},
		ListEnvironmentsFunc: func(context.Context, string) ([]*models.EnvironmentResponse, error) { return nil, nil },
	}
	s := &agentManagerService{ocClient: oc, logger: testLogger()}
	h := newGatewayExposureHarness(t)
	h.wire(t, s)
	h.existingArtifact(models.KindAgent)
	h.placedRows()
	h.exposures.GetFunc = func(context.Context, string, string, string, uuid.UUID) (*models.PlatformAgentExposure, error) {
		return nil, repositories.ErrPlatformAgentExposureNotFound
	}

	deployments, err := s.GetAgentDeployments(context.Background(), "acme", "proj1", "my-agent")
	require.NoError(t, err)
	dev := utils.ConvertToDeploymentDetailsResponse(deployments)["dev"]

	assert.Equal(t, openChoreoURL, dev.Endpoints[0].Url)
	assert.Nil(t, dev.GatewayStatus)
	assert.Nil(t, dev.GatewayStatusReason)
}

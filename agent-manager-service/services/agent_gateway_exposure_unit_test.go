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

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
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

func internalAPIAgent(subType string) *models.AgentResponse {
	return &models.AgentResponse{
		UUID: uuid.NewString(), Name: "my-agent", Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
		Type:           models.AgentType{Type: string(utils.AgentTypeAPI), SubType: subType},
		InputInterface: &models.InputInterface{Port: 8000, BasePath: "/"},
	}
}

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

// defaultEnvPlatform reports an internal REST agent in the org's default environment.
func defaultEnvPlatform(envUUID uuid.UUID) *repomocks.PlatformRepositoryMock {
	return &repomocks.PlatformRepositoryMock{
		GetAgentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return internalAPIAgent(string(utils.AgentSubTypeChatAPI)), nil
		},
		GetEnvironmentFunc: func(context.Context, string, string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: envUUID.String(), Name: models.DefaultEnvironmentName}, nil
		},
	}
}

func TestDeploymentResponseCarriesStatusReason(t *testing.T) {
	soon := time.Now().Add(time.Minute)
	s := &agentManagerService{platform: defaultEnvPlatform(uuid.New()), logger: testLogger()}
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

	deployment := response[models.DefaultEnvironmentName]
	require.NotNil(t, deployment.GatewayStatus)
	assert.Equal(t, string(models.DeploymentStatusDeploying), *deployment.GatewayStatus)
	require.NotNil(t, deployment.GatewayStatusReason)
	assert.Equal(t, waitingForEndpoint, *deployment.GatewayStatusReason)
}

// An environment whose gateway deployment can't be read shows no gateway status.
func TestDeploymentResponseFallsBackWhenGatewayUnreadable(t *testing.T) {
	s := &agentManagerService{platform: defaultEnvPlatform(uuid.New()), logger: testLogger()}
	h := newGatewayExposureHarness(t)
	h.wire(t, s)
	h.existingArtifact(models.KindRestAPI)
	h.placedRows(h.deployedRow())
	h.gateways.ListWithFiltersFunc = func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
		return nil, assert.AnError
	}

	deployments, err := s.GetAgentDeployments(context.Background(), "acme", "proj1", "my-agent")
	require.NoError(t, err)
	deployment := utils.ConvertToDeploymentDetailsResponse(deployments)[models.DefaultEnvironmentName]

	assert.Nil(t, deployment.GatewayStatus)
	assert.Nil(t, deployment.GatewayStatusReason)
}

// An agent with no exposure shows no gateway status, though its environment has an ingress gateway.
func TestDeploymentResponseOmitsGatewayStatusWithoutExposure(t *testing.T) {
	s := &agentManagerService{platform: defaultEnvPlatform(uuid.New()), logger: testLogger()}
	h := newGatewayExposureHarness(t)
	h.wire(t, s)
	h.existingArtifact(models.KindAgent)
	h.placedRows()
	h.exposures.GetFunc = func(context.Context, string, string, string, uuid.UUID) (*models.PlatformAgentExposure, error) {
		return nil, repositories.ErrPlatformAgentExposureNotFound
	}

	deployments, err := s.GetAgentDeployments(context.Background(), "acme", "proj1", "my-agent")
	require.NoError(t, err)
	deployment := utils.ConvertToDeploymentDetailsResponse(deployments)[models.DefaultEnvironmentName]

	assert.Nil(t, deployment.GatewayStatus)
	assert.Nil(t, deployment.GatewayStatusReason)
}

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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconv"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/reconcile"
)

// gatewayExposureHarness exposes agents through a reconcile core over publisherMocks, whose plain and
// transaction-bound mocks stay apart: an exposure or artifact write outside the exposure transaction panics or fails.
type gatewayExposureHarness struct {
	*publisherMocks
	gateway *gwmodels.Gateway
	// pending are the PENDING rows the core inserted, all in the exposure transaction.
	pending   []gwmodels.Placement
	exposure  *AgentExposurePublisher
	a2a       *A2APublisher
	txConfigs *gwrepomocks.AgentConfigRepositoryMock
}

func newGatewayExposureHarness(t *testing.T) *gatewayExposureHarness {
	t.Helper()
	m := newPublisherMocks(t)
	h := &gatewayExposureHarness{
		publisherMocks: m,
		gateway: &gwmodels.Gateway{
			UUID: uuid.New(), Vhost: "https://agents.example.com", GatewayFunctionalityType: gwmodels.GatewayRoleIngress,
		},
		txConfigs: &gwrepomocks.AgentConfigRepositoryMock{UpsertFunc: func(context.Context, *gwmodels.AgentConfig) error { return nil }},
	}
	m.gateways.ListWithFiltersFunc = func(gwrepo.GatewayFilterOptions) ([]*gwmodels.Gateway, error) {
		if h.gateway == nil {
			return nil, nil
		}
		return []*gwmodels.Gateway{h.gateway}, nil
	}
	m.artifacts.WithTxFunc = func(tx *gorm.DB) gwrepo.ArtifactRepository {
		assert.Same(t, txSentinel, tx, "the artifact is ensured in the exposure transaction")
		return m.artifacts
	}
	m.deployments.LockEnvironmentMappingFunc = mappedEverywhere
	m.deployments.InsertPendingFunc = func(_ context.Context, tx *gorm.DB, p gwmodels.Placement, _ string) (bool, error) {
		assert.Same(t, txSentinel, tx, "the PENDING row is written in the exposure transaction")
		h.pending = append(h.pending, p)
		return true, nil
	}
	m.txExposures.UpsertFunc = func(context.Context, *gwmodels.PlatformAgentExposure) error { return nil }
	m.exposures.GetFunc = exposedAgent
	exposures := NewPlatformAgentExposures(m.exposures, m.gateways, nil)
	a2aKind := &A2AAgentKind{exposures: exposures, proxyRepo: m.proxies, cardRepo: m.envCards, deploymentRepo: m.deployments, logger: testLogger()}
	core := reconcile.NewCore(nil, m.deployments, m.hub, reconcile.Kinds{a2aKind}, reconcile.Config{}, testLogger())
	h.exposure = NewAgentExposurePublisher(m.tx(), m.exposures, m.artifacts, m.deployments, m.gateways, core, testLogger())
	h.a2a = NewA2APublisher(m.tx(), m.exposures, m.proxies, m.envCards, m.artifacts, m.deployments, testLogger(), h.exposure, a2aKind)
	return h
}

// wire gives s the harness's publishers, artifacts and gateways, and saves its config in the exposure transaction.
func (h *gatewayExposureHarness) wire(t *testing.T, s *AgentGatewayService) {
	t.Helper()
	s.artifactRepo = h.artifacts
	s.gatewayRepo = h.gateways
	s.a2aPublisher = h.a2a
	s.exposure = h.exposure
	plain, _ := s.agentConfigRepo.(*gwrepomocks.AgentConfigRepositoryMock)
	if plain == nil {
		plain = &gwrepomocks.AgentConfigRepositoryMock{}
		s.agentConfigRepo = plain
	}
	plain.WithTxFunc = func(tx *gorm.DB) gwrepo.AgentConfigRepository {
		assert.Same(t, txSentinel, tx, "the config is saved in the exposure transaction")
		return h.txConfigs
	}
}

// existingArtifact makes the agent's per-environment artifact a row of kind.
func (h *gatewayExposureHarness) existingArtifact(kind string) *gwmodels.Artifact {
	artifact := &gwmodels.Artifact{UUID: uuid.New(), OUID: "acme", Handle: "proj1/my-agent/env", Kind: kind}
	h.artifacts.GetByHandleFunc = func(string, string) (*gwmodels.Artifact, error) { return artifact, nil }
	return artifact
}

// placedRows makes the artifact's rows the given ones.
func (h *gatewayExposureHarness) placedRows(rows ...gwmodels.DeploymentStatusRecord) {
	h.deployments.StatusRecordsFunc = func(context.Context, string, uuid.UUID) ([]gwmodels.DeploymentStatusRecord, error) {
		return rows, nil
	}
}

func (h *gatewayExposureHarness) deployedRow() gwmodels.DeploymentStatusRecord {
	return gwmodels.DeploymentStatusRecord{
		GatewayUUID: h.gateway.UUID, Status: gwmodels.DeploymentStatusDeployed, StatusDesired: gwmodels.DeploymentStatusDeployed,
	}
}

// defaultEnvPlatform reports an external agent in the org's default environment.
func defaultEnvPlatform(envUUID uuid.UUID) *gwtest.Core {
	return &gwtest.Core{
		GetAgentFunc: func(context.Context, string, string, string) (*modkit.Agent, error) {
			agent := externalAgent()
			agent.ProjectName = "" // the project asked for
			return agent, nil
		},
		GetEnvironmentFunc: func(context.Context, string, string) (*modkit.Environment, error) {
			return &modkit.Environment{ID: envUUID.String(), Name: gwmodels.DefaultEnvironmentName}, nil
		},
	}
}

func TestDeploymentResponseCarriesStatusReason(t *testing.T) {
	soon := time.Now().Add(time.Minute)
	s := &AgentGatewayService{platform: defaultEnvPlatform(uuid.New()), logger: testLogger()}
	h := newGatewayExposureHarness(t)
	h.wire(t, s)
	h.existingArtifact(gwmodels.KindAgent)
	h.placedRows(gwmodels.DeploymentStatusRecord{
		GatewayUUID: h.gateway.UUID, Status: gwmodels.DeploymentStatusPending, StatusDesired: gwmodels.DeploymentStatusUndeployed,
		NextAttemptAt: &soon, RenderReason: waitingForEndpoint,
	})

	deployments, err := s.GetAgentDeployments(context.Background(), "acme", "proj1", "my-agent")
	require.NoError(t, err)
	response := gwconv.ConvertToDeploymentDetailsResponse(deployments)

	deployment := response[gwmodels.DefaultEnvironmentName]
	require.NotNil(t, deployment.GatewayStatus)
	assert.Equal(t, string(gwmodels.DeploymentStatusDeploying), *deployment.GatewayStatus)
	require.NotNil(t, deployment.GatewayStatusReason)
	assert.Equal(t, waitingForEndpoint, *deployment.GatewayStatusReason)
}

// An environment whose gateway deployment can't be read shows no gateway status.
func TestDeploymentResponseFallsBackWhenGatewayUnreadable(t *testing.T) {
	s := &AgentGatewayService{platform: defaultEnvPlatform(uuid.New()), logger: testLogger()}
	h := newGatewayExposureHarness(t)
	h.wire(t, s)
	h.existingArtifact(gwmodels.KindAgent)
	h.placedRows(h.deployedRow())
	h.gateways.ListWithFiltersFunc = func(gwrepo.GatewayFilterOptions) ([]*gwmodels.Gateway, error) {
		return nil, assert.AnError
	}

	deployments, err := s.GetAgentDeployments(context.Background(), "acme", "proj1", "my-agent")
	require.NoError(t, err)
	deployment := gwconv.ConvertToDeploymentDetailsResponse(deployments)[gwmodels.DefaultEnvironmentName]

	assert.Nil(t, deployment.GatewayStatus)
	assert.Nil(t, deployment.GatewayStatusReason)
}

// An agent with no exposure shows no gateway status, though its environment has an ingress gateway.
func TestDeploymentResponseOmitsGatewayStatusWithoutExposure(t *testing.T) {
	s := &AgentGatewayService{platform: defaultEnvPlatform(uuid.New()), logger: testLogger()}
	h := newGatewayExposureHarness(t)
	h.wire(t, s)
	h.existingArtifact(gwmodels.KindAgent)
	h.placedRows()
	h.exposures.GetFunc = func(context.Context, string, string, string, uuid.UUID) (*gwmodels.PlatformAgentExposure, error) {
		return nil, gwrepo.ErrPlatformAgentExposureNotFound
	}

	deployments, err := s.GetAgentDeployments(context.Background(), "acme", "proj1", "my-agent")
	require.NoError(t, err)
	deployment := gwconv.ConvertToDeploymentDetailsResponse(deployments)[gwmodels.DefaultEnvironmentName]

	assert.Nil(t, deployment.GatewayStatus)
	assert.Nil(t, deployment.GatewayStatusReason)
}

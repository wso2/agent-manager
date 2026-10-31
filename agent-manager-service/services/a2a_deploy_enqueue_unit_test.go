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
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// a2aPublishingAgentService is an agentManagerService whose only A2A collaborator is the publisher.
func a2aPublishingAgentService(pubs *repomocks.PlatformAgentExposureRepositoryMock, proxies *repomocks.A2AProxyRepositoryMock) *agentManagerService {
	pubs.WithTxFunc = func(*gorm.DB) repositories.PlatformAgentExposureRepository { return pubs }
	publisher := withA2ACore(NewA2APublisher(sentinelTx, pubs, proxies, nil, nil, a2aRenderQueue(),
		testLogger(), NewAgentExposurePublisher(sentinelTx, pubs, nil, a2aRenderQueue(), a2aIngressGateways(), nil, testLogger()), nil), nil)
	return NewAgentManagerService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		publisher, nil, nil, nil, testLogger()).(*agentManagerService)
}

// a2aRenderQueue accepts every PENDING row a deploy owes, on a gateway mapped to every environment.
func a2aRenderQueue() *repomocks.DeploymentRepositoryMock {
	return &repomocks.DeploymentRepositoryMock{
		LockEnvironmentMappingFunc: mappedEverywhere,
		InsertPendingFunc:          func(context.Context, *gorm.DB, models.Placement, string) (bool, error) { return true, nil },
	}
}

// a2aIngressGateways maps one ingress gateway to every environment.
func a2aIngressGateways() *repomocks.GatewayRepositoryMock {
	gateway := &models.Gateway{UUID: uuid.New(), GatewayFunctionalityType: models.GatewayRoleIngress}
	gateways := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{gateway}, nil
		},
	}
	gateways.WithTxFunc = func(*gorm.DB) repositories.GatewayRepository { return gateways }
	return gateways
}

// recordingConfigs is an agent config repository that accepts upserts in the sentinel transaction only.
func recordingConfigs(t *testing.T) (configs, txConfigs *repomocks.AgentConfigRepositoryMock) {
	txConfigs = &repomocks.AgentConfigRepositoryMock{UpsertFunc: func(context.Context, *models.AgentConfig) error { return nil }}
	configs = &repomocks.AgentConfigRepositoryMock{WithTxFunc: func(tx *gorm.DB) repositories.AgentConfigRepository {
		assert.Same(t, txSentinel, tx)
		return txConfigs
	}}
	return configs, txConfigs
}

// A deploy saves the environment's artifact, config and exposure in one transaction, the exposure against the
// artifact.
func TestRecordAgentEnvDeployExposesAPlatformA2AAgentInTheSaveTransaction(t *testing.T) {
	repo := &repomocks.PlatformAgentExposureRepositoryMock{
		UpsertFunc: func(context.Context, *models.PlatformAgentExposure) error { return nil },
	}
	svc := a2aPublishingAgentService(repo, &repomocks.A2AProxyRepositoryMock{})
	configs, txConfigs := recordingConfigs(t)
	svc.agentConfigRepo = configs
	agent := &models.AgentResponse{UUID: uuid.NewString(), Type: models.AgentType{Type: "agent-api", SubType: "a2a-agent"}}
	envUUID, artifact := uuid.New(), &models.Artifact{UUID: uuid.New()}
	cfg := &models.AgentConfig{OUID: "org-1", ProjectName: "checkout", AgentName: "trip-planner", EnvironmentName: "dev"}

	err := svc.recordAgentEnvDeploy(context.Background(), agent, cfg, envUUID.String(), func(tx *gorm.DB) (*models.Artifact, error) {
		assert.Same(t, txSentinel, tx, "the artifact is ensured in the transaction")
		return artifact, nil
	})

	require.NoError(t, err)
	require.Len(t, txConfigs.UpsertCalls(), 1)
	require.Len(t, repo.UpsertCalls(), 1)
	exposure := repo.UpsertCalls()[0].Exposure
	assert.Equal(t, "dev", exposure.EnvironmentName)
	assert.Equal(t, envUUID, exposure.EnvironmentUUID)
	assert.Equal(t, artifact.UUID, exposure.ArtifactUUID)
	assert.Equal(t, agent.UUID, exposure.AgentUUID.String())
}

// The exposure is part of the deploy: when it can't be written, the request fails and a retry reconciles.
func TestRecordAgentEnvDeployFailsWhenTheExposureCannotBeWritten(t *testing.T) {
	repo := &repomocks.PlatformAgentExposureRepositoryMock{
		UpsertFunc: func(context.Context, *models.PlatformAgentExposure) error { return assert.AnError },
	}
	svc := a2aPublishingAgentService(repo, &repomocks.A2AProxyRepositoryMock{})
	svc.agentConfigRepo, _ = recordingConfigs(t)
	agent := &models.AgentResponse{Type: models.AgentType{Type: "agent-api", SubType: "a2a-agent"}}
	cfg := &models.AgentConfig{OUID: "org-1", ProjectName: "checkout", AgentName: "trip-planner", EnvironmentName: "dev"}

	err := svc.recordAgentEnvDeploy(context.Background(), agent, cfg, uuid.NewString(), func(*gorm.DB) (*models.Artifact, error) {
		return &models.Artifact{UUID: uuid.New()}, nil
	})

	assert.ErrorIs(t, err, assert.AnError)
}

// A REST agent's policies ride on its release binding, which the caller has
// already updated; exposing it would publish an Agent resource for a non-agent.
func TestRecordAgentEnvDeployExposesNoOtherAgent(t *testing.T) {
	repo := &repomocks.PlatformAgentExposureRepositoryMock{}
	svc := a2aPublishingAgentService(repo, &repomocks.A2AProxyRepositoryMock{})
	svc.agentConfigRepo = &repomocks.AgentConfigRepositoryMock{UpsertFunc: func(context.Context, *models.AgentConfig) error { return nil }}
	agent := &models.AgentResponse{Type: models.AgentType{Type: "agent-api", SubType: "chat-api"}}
	cfg := &models.AgentConfig{OUID: "org-1", ProjectName: "checkout", AgentName: "greeter", EnvironmentName: "dev"}

	err := svc.recordAgentEnvDeploy(context.Background(), agent, cfg, uuid.NewString(), func(*gorm.DB) (*models.Artifact, error) {
		return &models.Artifact{UUID: uuid.New()}, nil
	})

	require.NoError(t, err)
	assert.Empty(t, repo.UpsertCalls())
	assert.Len(t, svc.agentConfigRepo.(*repomocks.AgentConfigRepositoryMock).UpsertCalls(), 1, "the config is still saved")
}

func TestRecordAgentEnvDeployRejectsAnUnparseableEnvironmentUUID(t *testing.T) {
	svc := a2aPublishingAgentService(&repomocks.PlatformAgentExposureRepositoryMock{}, &repomocks.A2AProxyRepositoryMock{})
	agent := &models.AgentResponse{Type: models.AgentType{Type: "agent-api", SubType: "a2a-agent"}}
	cfg := &models.AgentConfig{OUID: "org-1", ProjectName: "checkout", AgentName: "trip-planner", EnvironmentName: "dev"}

	err := svc.recordAgentEnvDeploy(context.Background(), agent, cfg, "not-a-uuid", func(*gorm.DB) (*models.Artifact, error) {
		return &models.Artifact{UUID: uuid.New()}, nil
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-a-uuid")
}

// A new A2A agent's first binding comes from create, not a deploy, so create
// must expose it in the first environment itself.
func TestEnqueueCreatedA2AAgentExposesTheFirstEnvironment(t *testing.T) {
	var enqueued []models.PlatformAgentExposure
	repo := &repomocks.PlatformAgentExposureRepositoryMock{
		UpsertFunc: func(ctx context.Context, pub *models.PlatformAgentExposure) error {
			enqueued = append(enqueued, *pub)
			return nil
		},
	}
	agentUUID := uuid.New()
	svc := a2aPublishingAgentService(repo, &repomocks.A2AProxyRepositoryMock{})
	svc.ocClient = &clientmocks.OpenChoreoClientMock{
		GetComponentFunc: func(_ context.Context, _, _, name string) (*models.AgentResponse, error) {
			return &models.AgentResponse{UUID: agentUUID.String(), Name: name}, nil
		},
	}
	envUUID := uuid.New()
	artifact := &models.Artifact{UUID: uuid.New()}

	svc.enqueueCreatedA2AAgent(context.Background(), "a2a-agent", "org-1", "checkout", "trip-planner", "dev", envUUID.String(), artifact)

	require.Len(t, enqueued, 1)
	assert.Equal(t, "dev", enqueued[0].EnvironmentName)
	assert.Equal(t, envUUID, enqueued[0].EnvironmentUUID)
	assert.Equal(t, artifact.UUID, enqueued[0].ArtifactUUID)
	assert.Equal(t, &agentUUID, enqueued[0].AgentUUID)
}

// The UUID only sharpens the card cleanup, so a failed lookup still records the row.
func TestEnqueueCreatedA2AAgentExposesEvenWhenTheAgentLookupFails(t *testing.T) {
	var enqueued []models.PlatformAgentExposure
	repo := &repomocks.PlatformAgentExposureRepositoryMock{
		UpsertFunc: func(ctx context.Context, pub *models.PlatformAgentExposure) error {
			enqueued = append(enqueued, *pub)
			return nil
		},
	}
	svc := a2aPublishingAgentService(repo, &repomocks.A2AProxyRepositoryMock{})
	svc.ocClient = &clientmocks.OpenChoreoClientMock{
		GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return nil, assert.AnError
		},
	}

	svc.enqueueCreatedA2AAgent(context.Background(), "a2a-agent", "org-1", "checkout", "trip-planner", "dev", uuid.New().String(), &models.Artifact{UUID: uuid.New()})

	require.Len(t, enqueued, 1)
	assert.Nil(t, enqueued[0].AgentUUID)
}

// Non-A2A agents and agents without an env artifact have nothing to publish; a
// nil UpsertFunc panics if any of these paths reaches the queue.
func TestEnqueueCreatedA2AAgentSkipsAgentsWithNothingToPublish(t *testing.T) {
	svc := a2aPublishingAgentService(&repomocks.PlatformAgentExposureRepositoryMock{}, &repomocks.A2AProxyRepositoryMock{})
	envUUID := uuid.New().String()

	assert.NotPanics(t, func() {
		svc.enqueueCreatedA2AAgent(context.Background(), "chat-api", "org-1", "checkout", "chat", "dev", envUUID, &models.Artifact{UUID: uuid.New()})
		svc.enqueueCreatedA2AAgent(context.Background(), "a2a-agent", "org-1", "checkout", "trip-planner", "dev", envUUID, nil)
		svc.enqueueCreatedA2AAgent(context.Background(), "a2a-agent", "org-1", "checkout", "trip-planner", "dev", "not-a-uuid", &models.Artifact{UUID: uuid.New()})
	})
}

// A service without an A2A publisher has no exposure queue to write to, so create
// must return rather than read the agent or publish.
func TestEnqueueCreatedA2AAgentWithoutPublisher(t *testing.T) {
	svc := &agentManagerService{logger: testLogger()}

	assert.NotPanics(t, func() {
		svc.enqueueCreatedA2AAgent(context.Background(), "a2a-agent", "org-1", "checkout", "trip-planner", "dev", uuid.NewString(), &models.Artifact{UUID: uuid.New()})
	})
}

// An external agent's record is its proxy row, proxied or not, so it gets no exposure.
func TestRecordAgentEnvDeployRecordsNoExposureForAnExternalAgent(t *testing.T) {
	pubs := &repomocks.PlatformAgentExposureRepositoryMock{} // UpsertFunc is nil: an exposure would panic
	svc := a2aPublishingAgentService(pubs, &repomocks.A2AProxyRepositoryMock{})
	svc.agentConfigRepo = &repomocks.AgentConfigRepositoryMock{UpsertFunc: func(context.Context, *models.AgentConfig) error { return nil }}
	cfg := &models.AgentConfig{OUID: "org-1", ProjectName: "checkout", AgentName: "weather", EnvironmentName: "dev"}

	err := svc.recordAgentEnvDeploy(context.Background(), externalAgent(), cfg, uuid.NewString(), func(*gorm.DB) (*models.Artifact, error) {
		return &models.Artifact{UUID: uuid.New()}, nil
	})

	require.NoError(t, err)
	assert.Empty(t, pubs.UpsertCalls())
}

// A same-named agent created later must not inherit a proxy, or its secret.
func TestClearInheritedA2AStateDropsPublicationsAndProxies(t *testing.T) {
	pubs := &repomocks.PlatformAgentExposureRepositoryMock{DeleteForAgentFunc: func(context.Context, string, string, string) error { return nil }}
	proxies := &repomocks.A2AProxyRepositoryMock{DeleteForAgentFunc: func(context.Context, string, string, string) error { return nil }}
	svc := a2aPublishingAgentService(pubs, proxies)

	require.NoError(t, svc.clearInheritedA2AState(context.Background(), "org-1", "checkout", "weather"))
	assert.Len(t, pubs.DeleteForAgentCalls(), 1)
	assert.Len(t, proxies.DeleteForAgentCalls(), 1)
}

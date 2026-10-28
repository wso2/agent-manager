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
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// deploySettingsService wires UpdateAgentDeploySettings for the external A2A agent in externalA2AAgent.
func deploySettingsService(configs *repomocks.AgentConfigRepositoryMock, publisher *A2APublisher) *agentManagerService {
	oc := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return externalA2AAgent(), nil
		},
		GetEnvironmentFunc: func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{Name: name, UUID: proxyTestEnvUUID}, nil
		},
		// UpdateReleaseBindingTraitConfigsFunc is nil: an external agent has no release binding.
	}
	artifacts := &repomocks.ArtifactRepositoryMock{
		GetByHandleFunc: func(handle, ouID string) (*models.Artifact, error) {
			return &models.Artifact{UUID: uuid.New(), Handle: handle, Kind: models.KindAgent, OUID: ouID}, nil
		},
	}
	return NewAgentManagerService(nil, oc, nil, nil, nil, configs, nil, nil, artifacts, nil, nil, nil, nil, nil, nil, nil,
		publisher, nil, nil, nil, testLogger()).(*agentManagerService)
}

// savedConfigs accepts the upsert inside the save transaction only; a plain Upsert reaches a nil func and panics.
func savedConfigs(t *testing.T) (configs, txConfigs *repomocks.AgentConfigRepositoryMock) {
	txConfigs = &repomocks.AgentConfigRepositoryMock{
		UpsertFunc: func(context.Context, *models.AgentConfig) error { return nil },
	}
	configs = &repomocks.AgentConfigRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentConfig, error) {
			return &models.AgentConfig{EnableApiKeySecurity: true, CORSEnabled: true}, nil
		},
		WithTxFunc: func(tx *gorm.DB) repositories.AgentConfigRepository {
			assert.Same(t, txSentinel, tx)
			return txConfigs
		},
	}
	return configs, txConfigs
}

// deploySettingsMocks is newPublisherMocks with the proxy repository bound to the transaction and its lock free.
func deploySettingsMocks(t *testing.T) *publisherMocks {
	m := newPublisherMocks(t)
	m.proxies.WithTxFunc = func(tx *gorm.DB) repositories.A2AProxyRepository {
		assert.Same(t, txSentinel, tx)
		return m.proxies
	}
	m.proxies.LockAgentEnvFunc = func(context.Context, string, string, string, uuid.UUID) error { return nil }
	return m
}

// The save and the republish are one transaction, decided under the environment's transition lock.
func TestUpdateAgentDeploySettingsRepublishesAProxiedExternalAgent(t *testing.T) {
	var steps []string
	m := deploySettingsMocks(t)
	m.txPubs.EnqueueFunc = func(_ context.Context, pub *models.A2APublication) error {
		steps = append(steps, "enqueue:"+string(pub.Source))
		return nil
	}
	m.proxies.LockAgentEnvFunc = func(_ context.Context, _, _, _ string, envUUID uuid.UUID) error {
		assert.Equal(t, uuid.MustParse(proxyTestEnvUUID), envUUID)
		steps = append(steps, "lock")
		return nil
	}
	m.proxies.GetByEnvUUIDFunc = func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
		steps = append(steps, "read-proxy")
		return &models.A2AProxy{}, nil
	}
	configs, txConfigs := savedConfigs(t)
	svc := deploySettingsService(configs, m.publisher())
	apiKeys := false

	err := svc.UpdateAgentDeploySettings(tierGrantedCtx(t), "org-1", "checkout", "weather",
		&spec.UpdateAgentDeploySettingsRequest{EnvironmentName: "dev", EnableApiKeySecurity: &apiKeys})

	require.NoError(t, err)
	require.Len(t, txConfigs.UpsertCalls(), 1)
	assert.False(t, txConfigs.UpsertCalls()[0].Config.EnableApiKeySecurity)
	assert.Equal(t, []string{"read-proxy", "lock", "read-proxy", "enqueue:external"}, steps,
		"the pre-check reads unlocked; the save re-decides under the lock and queues the republish in its transaction")
}

// A switch to direct that lands after the pre-check is seen under the lock: the settings are saved, nothing is queued.
func TestUpdateAgentDeploySettingsSeesASwitchToDirectUnderTheLock(t *testing.T) {
	reads := 0
	m := deploySettingsMocks(t)
	m.proxies.GetByEnvUUIDFunc = func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
		reads++
		if reads == 1 {
			return &models.A2AProxy{}, nil
		}
		return nil, repositories.ErrA2AProxyNotFound
	}
	configs, txConfigs := savedConfigs(t)
	svc := deploySettingsService(configs, m.publisher())

	err := svc.UpdateAgentDeploySettings(tierGrantedCtx(t), "org-1", "checkout", "weather",
		&spec.UpdateAgentDeploySettingsRequest{EnvironmentName: "dev"})

	require.NoError(t, err)
	assert.Len(t, txConfigs.UpsertCalls(), 1)
	assert.Empty(t, m.txPubs.EnqueueCalls(), "no external row is left behind for the reconciler to fail")
}

// Nothing is half-applied: a republish that cannot be queued rolls the settings back with it.
func TestUpdateAgentDeploySettingsFailsWhenTheRepublishCannotBeQueued(t *testing.T) {
	boom := errors.New("boom")
	m := deploySettingsMocks(t)
	m.txPubs.EnqueueFunc = func(context.Context, *models.A2APublication) error { return boom }
	m.proxies.GetByEnvUUIDFunc = func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
		return &models.A2AProxy{}, nil
	}
	configs, txConfigs := savedConfigs(t)
	svc := deploySettingsService(configs, m.publisher())
	apiKeys := false

	err := svc.UpdateAgentDeploySettings(tierGrantedCtx(t), "org-1", "checkout", "weather",
		&spec.UpdateAgentDeploySettingsRequest{EnvironmentName: "dev", EnableApiKeySecurity: &apiKeys})

	require.ErrorIs(t, err, boom)
	assert.Len(t, txConfigs.UpsertCalls(), 1, "the save ran inside the transaction the failed enqueue rolls back")
}

func TestUpdateAgentDeploySettingsRejectsADirectExternalEnvironment(t *testing.T) {
	m := deploySettingsMocks(t)
	m.proxies.GetByEnvUUIDFunc = func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
		return nil, repositories.ErrA2AProxyNotFound
	}
	configs, txConfigs := savedConfigs(t)
	svc := deploySettingsService(configs, m.publisher())

	err := svc.UpdateAgentDeploySettings(tierGrantedCtx(t), "org-1", "checkout", "weather",
		&spec.UpdateAgentDeploySettingsRequest{EnvironmentName: "dev"})

	require.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.Contains(t, err.Error(), "gateway")
	assert.Empty(t, txConfigs.UpsertCalls())
	assert.Zero(t, m.transactions, "refused before any transaction opens")
}

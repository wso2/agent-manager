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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// deploySettingsService wires UpdateAgentDeploySettings for the external agent in externalAgent.
func deploySettingsService(configs *gwrepomocks.AgentConfigRepositoryMock, publisher *A2APublisher) *AgentGatewayService {
	oc := &repomocks.PlatformRepositoryMock{
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetAgentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return externalAgent(), nil
		},
		GetEnvironmentFunc: func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{Name: name, UUID: proxyTestEnvUUID}, nil
		},
		// UpdateReleaseBindingTraitConfigsFunc is nil: an external agent has no release binding.
	}
	artifacts := &gwrepomocks.ArtifactRepositoryMock{
		GetByHandleFunc: func(handle, ouID string) (*gwmodels.Artifact, error) {
			return &gwmodels.Artifact{UUID: uuid.New(), Handle: handle, Kind: gwmodels.KindAgent, OUID: ouID}, nil
		},
	}
	artifacts.WithTxFunc = func(*gorm.DB) gwrepo.ArtifactRepository { return artifacts }
	return NewAgentGatewayService(nil, oc, configs, nil, artifacts, nil, nil, publisher, nil, publisher.exposure, testLogger())
}

// savedConfigs accepts the upsert inside the save transaction only; a plain Upsert reaches a nil func and panics.
func savedConfigs(t *testing.T) (configs, txConfigs *gwrepomocks.AgentConfigRepositoryMock) {
	txConfigs = &gwrepomocks.AgentConfigRepositoryMock{
		UpsertFunc: func(context.Context, *gwmodels.AgentConfig) error { return nil },
	}
	configs = &gwrepomocks.AgentConfigRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*gwmodels.AgentConfig, error) {
			return &gwmodels.AgentConfig{EnableApiKeySecurity: true, CORSEnabled: true}, nil
		},
		WithTxFunc: func(tx *gorm.DB) gwrepo.AgentConfigRepository {
			assert.Same(t, txSentinel, tx)
			return txConfigs
		},
	}
	return configs, txConfigs
}

// deploySettingsMocks is newPublisherMocks with the proxy repository bound to the transaction and its lock free.
func deploySettingsMocks(t *testing.T) *publisherMocks {
	m := newPublisherMocks(t)
	m.proxies.WithTxFunc = func(tx *gorm.DB) gwrepo.A2AProxyRepository {
		assert.Same(t, txSentinel, tx)
		return m.proxies
	}
	m.proxies.LockAgentEnvFunc = func(context.Context, string, string, string, uuid.UUID) error { return nil }
	m.deployments.QueueRenderFunc = func(context.Context, *gorm.DB, string, uuid.UUID, []uuid.UUID, string,
		gwrepo.QueueScope,
	) (int64, error) {
		return 0, nil
	}
	return m
}

// The save is decided under the environment's transition lock. A proxied external agent's record is its proxy row,
// so the save writes no exposure.
func TestUpdateAgentDeploySettingsDecidesAProxiedExternalAgentUnderTheLock(t *testing.T) {
	var steps []string
	m := deploySettingsMocks(t) // txExposures.UpsertFunc is nil: an exposure would panic
	m.proxies.LockAgentEnvFunc = func(_ context.Context, _, _, _ string, envUUID uuid.UUID) error {
		assert.Equal(t, uuid.MustParse(proxyTestEnvUUID), envUUID)
		steps = append(steps, "lock")
		return nil
	}
	m.proxies.GetByEnvUUIDFunc = func(context.Context, string, string, string, uuid.UUID) (*gwmodels.A2AProxy, error) {
		steps = append(steps, "read-proxy")
		return &gwmodels.A2AProxy{}, nil
	}
	configs, txConfigs := savedConfigs(t)
	svc := deploySettingsService(configs, m.publisher())
	apiKeys := false

	err := svc.UpdateAgentDeploySettings(tierGrantedCtx(t), "org-1", "checkout", "weather",
		&gwspec.UpdateAgentDeploySettingsRequest{EnvironmentName: "dev", EnableApiKeySecurity: &apiKeys})

	require.NoError(t, err)
	require.Len(t, txConfigs.UpsertCalls(), 1)
	assert.False(t, txConfigs.UpsertCalls()[0].Config.EnableApiKeySecurity)
	assert.Equal(t, []string{"read-proxy", "lock", "read-proxy"}, steps,
		"the pre-check reads unlocked; the save re-decides under the lock")
	require.Len(t, m.deployments.QueueRenderCalls(), 1, "the proxied agent's placed rows re-render after the save")
	assert.Equal(t, gwrepo.QueueScopePlaced, m.deployments.QueueRenderCalls()[0].Scope)
}

// The save has committed when a proxied agent's re-render is marked: a failure there is logged, not reported.
func TestUpdateAgentDeploySettingsSavesWhenTheReRenderCannotBeMarked(t *testing.T) {
	m := deploySettingsMocks(t)
	m.proxies.GetByEnvUUIDFunc = func(context.Context, string, string, string, uuid.UUID) (*gwmodels.A2AProxy, error) {
		return &gwmodels.A2AProxy{}, nil
	}
	m.deployments.QueueRenderFunc = func(context.Context, *gorm.DB, string, uuid.UUID, []uuid.UUID, string,
		gwrepo.QueueScope,
	) (int64, error) {
		return 0, errors.New("database is down")
	}
	configs, txConfigs := savedConfigs(t)
	svc := deploySettingsService(configs, m.publisher())

	err := svc.UpdateAgentDeploySettings(tierGrantedCtx(t), "org-1", "checkout", "weather",
		&gwspec.UpdateAgentDeploySettingsRequest{EnvironmentName: "dev"})

	require.NoError(t, err)
	assert.Len(t, txConfigs.UpsertCalls(), 1)
	assert.Len(t, m.deployments.QueueRenderCalls(), 1)
}

// A proxied external agent takes A2A CORS whatever subtype it was created with.
func TestUpdateAgentDeploySettingsAppliesA2ACORSToAProxiedExternalAgent(t *testing.T) {
	m := deploySettingsMocks(t)
	m.proxies.GetByEnvUUIDFunc = func(context.Context, string, string, string, uuid.UUID) (*gwmodels.A2AProxy, error) {
		return &gwmodels.A2AProxy{}, nil
	}
	configs, txConfigs := savedConfigs(t)
	svc := deploySettingsService(configs, m.publisher())
	enabled, inherit := true, false

	err := svc.UpdateAgentDeploySettings(tierGrantedCtx(t), "org-1", "checkout", "weather",
		&gwspec.UpdateAgentDeploySettingsRequest{
			EnvironmentName:     "dev",
			AgentCardCorsConfig: &gwspec.AgentCardCORSConfig{Inherit: &inherit, Enabled: &enabled, AllowOrigin: []string{"https://app.example.com"}},
		})

	require.NoError(t, err)
	require.Len(t, txConfigs.UpsertCalls(), 1)
	saved := txConfigs.UpsertCalls()[0].Config
	assert.Contains(t, saved.CORSAllowHeaders, a2aVersionHeader)
	assert.Equal(t, []string{"https://app.example.com"}, saved.CardCORSAllowOrigins)
}

// A switch to direct that lands after the pre-check is seen under the lock: the settings are saved, nothing is queued.
func TestUpdateAgentDeploySettingsSeesASwitchToDirectUnderTheLock(t *testing.T) {
	reads := 0
	m := deploySettingsMocks(t)
	m.proxies.GetByEnvUUIDFunc = func(context.Context, string, string, string, uuid.UUID) (*gwmodels.A2AProxy, error) {
		reads++
		if reads == 1 {
			return &gwmodels.A2AProxy{}, nil
		}
		return nil, gwrepo.ErrA2AProxyNotFound
	}
	configs, txConfigs := savedConfigs(t)
	svc := deploySettingsService(configs, m.publisher())

	err := svc.UpdateAgentDeploySettings(tierGrantedCtx(t), "org-1", "checkout", "weather",
		&gwspec.UpdateAgentDeploySettingsRequest{EnvironmentName: "dev"})

	require.NoError(t, err)
	assert.Len(t, txConfigs.UpsertCalls(), 1)
	assert.Empty(t, m.txExposures.UpsertCalls())
}

func TestUpdateAgentDeploySettingsRejectsADirectExternalEnvironment(t *testing.T) {
	m := deploySettingsMocks(t)
	m.proxies.GetByEnvUUIDFunc = func(context.Context, string, string, string, uuid.UUID) (*gwmodels.A2AProxy, error) {
		return nil, gwrepo.ErrA2AProxyNotFound
	}
	configs, txConfigs := savedConfigs(t)
	svc := deploySettingsService(configs, m.publisher())

	err := svc.UpdateAgentDeploySettings(tierGrantedCtx(t), "org-1", "checkout", "weather",
		&gwspec.UpdateAgentDeploySettingsRequest{EnvironmentName: "dev"})

	require.ErrorIs(t, err, httpx.ErrInvalidInput)
	assert.Contains(t, err.Error(), "gateway")
	assert.Empty(t, txConfigs.UpsertCalls())
	assert.Zero(t, m.transactions, "refused before any transaction opens")
}

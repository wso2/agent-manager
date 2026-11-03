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
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// TestPromoteAgent_RejectsCardCORSForNonAPIAgent pins the fix for a gap found in
// review: agentCardCorsConfig is resolved and validated inside PromoteAgent's
// `if isAPIAgent { ... }` block, so a non-API agent (which never enters that
// block) used to skip validateCardCORS entirely and promote successfully with
// the card CORS request silently ignored. PromoteAgent must instead reject it
// with utils.ErrInvalidInput, matching the global rule that agentCardCorsConfig
// only applies to A2A agents. Only ocClient.GetOrganization/GetComponent are
// wired: the guard must fire before the deployment-pipeline lookup (the next
// call PromoteAgent makes), i.e. before any side effect — a nil GetProjectDeploymentPipelineFunc
// would panic if reached, so passing without panicking is itself part of the proof.
func TestPromoteAgent_RejectsCardCORSForNonAPIAgent(t *testing.T) {
	svc := &agentManagerService{
		ocClient: &clientmocks.OpenChoreoClientMock{
			GetOrganizationFunc: func(_ context.Context, ouID string) (*models.OrganizationResponse, error) {
				return &models.OrganizationResponse{Name: ouID}, nil
			},
			GetComponentFunc: func(_ context.Context, _, _, name string) (*models.AgentResponse, error) {
				return &models.AgentResponse{
					UUID:         "agent-uuid",
					Name:         name,
					Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
					Type:         models.AgentType{Type: string(utils.AgentTypeExternalAPI)},
				}, nil
			},
		},
		logger: discardLogger(),
	}

	err := svc.PromoteAgent(context.Background(), "ou-1", "proj", "agent-1", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
		AgentCardCorsConfig: &spec.AgentCardCORSConfig{
			Enabled:     spec.PtrBool(true),
			AllowOrigin: []string{"*"},
		},
	})
	require.ErrorIs(t, err, utils.ErrInvalidInput)
}

// A promote that leaves agentCardCorsConfig out carries the source
// environment's card override to the target, and — because the reconciler
// builds the gateway resource from the stored target row — queues the target
// for publication only after that row is written.
func TestPromoteAgent_A2AInheritsSourceCardCORSAndRepublishes(t *testing.T) {
	s, promoted := promoteAgentTestFixture(t, []client.EnvVar{{Key: client.EnvVarAgentIDClientID, Value: "staging-client-id"}}, nil)

	stagingUUID := uuid.New()
	artifactUUID := uuid.New()
	ocMock, ok := s.ocClient.(*clientmocks.OpenChoreoClientMock)
	require.True(t, ok)
	ocMock.GetComponentFunc = func(_ context.Context, _, _, name string) (*models.AgentResponse, error) {
		return &models.AgentResponse{
			UUID:         "agent-uuid",
			Name:         name,
			Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
			Type:         models.AgentType{Type: string(utils.AgentTypeAPI), SubType: string(utils.AgentSubTypeA2A)},
		}, nil
	}
	ocMock.GetEnvironmentFunc = func(_ context.Context, _, envName string) (*models.EnvironmentResponse, error) {
		return &models.EnvironmentResponse{Name: envName, UUID: stagingUUID.String()}, nil
	}

	sourceOverride := &models.AgentConfig{
		CORSEnabled:              true,
		CORSAllowOrigins:         []string{"https://client.example"},
		CardCORSEnabled:          spec.PtrBool(true),
		CardCORSAllowOrigins:     []string{"https://cards.example"},
		CardCORSAllowHeaders:     []string{"Content-Type"},
		CardCORSAllowCredentials: spec.PtrBool(false),
	}
	var upserted *models.AgentConfig
	var events []string
	txConfigs := &repomocks.AgentConfigRepositoryMock{
		UpsertFunc: func(_ context.Context, cfg *models.AgentConfig) error {
			assert.True(t, *promoted, "the config is saved once the promote has succeeded")
			upserted = cfg
			events = append(events, "upsert")
			return nil
		},
	}
	s.agentConfigRepo = &repomocks.AgentConfigRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, envName string) (*models.AgentConfig, error) {
			require.Equal(t, "dev", envName, "promote resolves settings from the source environment")
			return sourceOverride, nil
		},
		WithTxFunc: func(tx *gorm.DB) repositories.AgentConfigRepository {
			assert.Same(t, txSentinel, tx, "the config is saved in the exposure's transaction")
			return txConfigs
		},
	}
	artifacts := &repomocks.ArtifactRepositoryMock{
		GetByHandleFunc: func(_, _ string) (*models.Artifact, error) {
			return &models.Artifact{UUID: artifactUUID, Kind: models.KindAgent}, nil
		},
	}
	artifacts.WithTxFunc = func(*gorm.DB) repositories.ArtifactRepository { return artifacts }
	s.artifactRepo = artifacts
	pubRepo := &repomocks.PlatformAgentExposureRepositoryMock{
		UpsertFunc: func(_ context.Context, _ *models.PlatformAgentExposure) error {
			events = append(events, "expose")
			return nil
		},
	}
	pubRepo.WithTxFunc = func(tx *gorm.DB) repositories.PlatformAgentExposureRepository {
		assert.Same(t, txSentinel, tx)
		return pubRepo
	}
	s.a2aPublisher = withA2ACore(NewA2APublisher(sentinelTx, pubRepo, &repomocks.A2AProxyRepositoryMock{}, nil, nil,
		a2aRenderQueue(), testLogger(),
		NewAgentExposurePublisher(sentinelTx, pubRepo, nil, a2aRenderQueue(), a2aIngressGateways(), nil, testLogger()), nil), nil)
	s.exposure = s.a2aPublisher.exposure

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})
	require.NoError(t, err)
	require.True(t, *promoted)

	require.NotNil(t, upserted, "the target environment's config is persisted")
	assert.Equal(t, "staging", upserted.EnvironmentName)
	assert.Equal(t, sourceOverride.CardCORSEnabled, upserted.CardCORSEnabled)
	assert.Equal(t, []string{"https://cards.example"}, upserted.CardCORSAllowOrigins)
	assert.Equal(t, []string{"Content-Type"}, upserted.CardCORSAllowHeaders)
	assert.Equal(t, sourceOverride.CardCORSAllowCredentials, upserted.CardCORSAllowCredentials)

	enqueued := pubRepo.UpsertCalls()
	require.Len(t, enqueued, 1, "the agent is exposed in the target")
	assert.Equal(t, "staging", enqueued[0].Exposure.EnvironmentName)
	assert.Equal(t, stagingUUID, enqueued[0].Exposure.EnvironmentUUID)
	assert.Equal(t, artifactUUID, enqueued[0].Exposure.ArtifactUUID)
	assert.Equal(t, []string{"upsert", "expose"}, events, "exposed only after the config it is built from is saved")
}

// The target's exposure is part of the promote: when it can't be written, the promote reports it and a retry
// reconciles.
func TestPromoteAgent_A2AFailsWhenTheExposureCannotBeWritten(t *testing.T) {
	s, _ := promoteAgentTestFixture(t, []client.EnvVar{{Key: client.EnvVarAgentIDClientID, Value: "staging-client-id"}}, nil)
	ocMock, ok := s.ocClient.(*clientmocks.OpenChoreoClientMock)
	require.True(t, ok)
	ocMock.GetComponentFunc = func(_ context.Context, _, _, name string) (*models.AgentResponse, error) {
		return &models.AgentResponse{
			UUID: "agent-uuid", Name: name, Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
			Type: models.AgentType{Type: string(utils.AgentTypeAPI), SubType: string(utils.AgentSubTypeA2A)},
		}, nil
	}
	ocMock.GetEnvironmentFunc = func(_ context.Context, _, envName string) (*models.EnvironmentResponse, error) {
		return &models.EnvironmentResponse{Name: envName, UUID: uuid.NewString()}, nil
	}
	txConfigs := &repomocks.AgentConfigRepositoryMock{UpsertFunc: func(context.Context, *models.AgentConfig) error { return nil }}
	s.agentConfigRepo = &repomocks.AgentConfigRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentConfig, error) {
			return &models.AgentConfig{}, nil
		},
		WithTxFunc: func(*gorm.DB) repositories.AgentConfigRepository { return txConfigs },
	}
	artifacts := &repomocks.ArtifactRepositoryMock{
		GetByHandleFunc: func(_, _ string) (*models.Artifact, error) {
			return &models.Artifact{UUID: uuid.New(), Kind: models.KindAgent}, nil
		},
	}
	artifacts.WithTxFunc = func(*gorm.DB) repositories.ArtifactRepository { return artifacts }
	s.artifactRepo = artifacts
	pubRepo := &repomocks.PlatformAgentExposureRepositoryMock{
		UpsertFunc: func(context.Context, *models.PlatformAgentExposure) error { return assert.AnError },
	}
	pubRepo.WithTxFunc = func(*gorm.DB) repositories.PlatformAgentExposureRepository { return pubRepo }
	s.a2aPublisher = withA2ACore(NewA2APublisher(sentinelTx, pubRepo, &repomocks.A2AProxyRepositoryMock{}, nil, nil,
		a2aRenderQueue(), testLogger(),
		NewAgentExposurePublisher(sentinelTx, pubRepo, nil, a2aRenderQueue(), a2aIngressGateways(), nil, testLogger()), nil), nil)
	s.exposure = s.a2aPublisher.exposure

	err := s.PromoteAgent(tierGrantedCtx(t), "acme", "proj1", "my-agent", &spec.PromoteAgentRequest{
		SourceEnvironment: "dev",
		TargetEnvironment: "staging",
	})

	assert.ErrorIs(t, err, assert.AnError)
}

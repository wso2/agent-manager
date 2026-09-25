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

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
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
	s.agentConfigRepo = &repomocks.AgentConfigRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, envName string) (*models.AgentConfig, error) {
			require.Equal(t, "dev", envName, "promote resolves settings from the source environment")
			return sourceOverride, nil
		},
		UpsertFunc: func(_ context.Context, cfg *models.AgentConfig) error {
			upserted = cfg
			events = append(events, "upsert")
			return nil
		},
	}
	s.artifactRepo = &repomocks.ArtifactRepositoryMock{
		GetByHandleFunc: func(_, _ string) (*models.Artifact, error) {
			return &models.Artifact{UUID: artifactUUID, Kind: models.KindAgent}, nil
		},
	}
	pubRepo := &repomocks.A2APublicationRepositoryMock{
		EnqueueFunc: func(_ context.Context, _ *models.A2APublication) error {
			events = append(events, "enqueue")
			return nil
		},
	}
	s.a2aPublicationRepo = pubRepo

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

	enqueued := pubRepo.EnqueueCalls()
	require.Len(t, enqueued, 1, "the target is queued for gateway publication")
	assert.Equal(t, "staging", enqueued[0].Pub.EnvironmentName)
	assert.Equal(t, stagingUUID, enqueued[0].Pub.EnvironmentUUID)
	assert.Equal(t, artifactUUID, enqueued[0].Pub.ArtifactUUID)
	assert.Equal(t, []string{"upsert", "enqueue"}, events, "queued only after the config it is built from is saved")
}

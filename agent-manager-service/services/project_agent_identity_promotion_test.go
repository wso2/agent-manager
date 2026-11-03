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
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

type projectAgentIdentityPromotionProviderStub struct {
	projectAgentIdentityProviderStub
	environments func(context.Context, string, string, string) ([]ProjectIdentityEnvironment, error)
	promote      func(context.Context, ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error)
}

func (s projectAgentIdentityPromotionProviderStub) Environments(
	ctx context.Context, organizationID, projectID, projectName string,
) ([]ProjectIdentityEnvironment, error) {
	return s.environments(ctx, organizationID, projectID, projectName)
}

func (s projectAgentIdentityPromotionProviderStub) Promote(
	ctx context.Context, request ProjectAgentIdentityPromotionRequest,
) (ProjectAgentIdentityResult, error) {
	return s.promote(ctx, request)
}

func TestPromotionStepsUsesIdentityPlatformTopology(t *testing.T) {
	steps, err := promotionSteps("development", []ProjectIdentityEnvironment{
		{Name: "production"},
		{Name: "development", PromotesTo: []string{"staging"}},
		{Name: "staging", PromotesTo: []string{"production"}},
	})
	require.NoError(t, err)
	require.Equal(t, []promotionStep{
		{Environment: "development"},
		{Environment: "staging", Source: "development"},
		{Environment: "production", Source: "staging"},
	}, steps)
}

func TestPromotionStepsSupportsBranchesDeterministically(t *testing.T) {
	steps, err := promotionSteps("development", []ProjectIdentityEnvironment{
		{Name: "development", PromotesTo: []string{"production", "staging"}},
		{Name: "staging"},
		{Name: "production"},
	})
	require.NoError(t, err)
	require.Equal(t, []promotionStep{
		{Environment: "development"},
		{Environment: "production", Source: "development"},
		{Environment: "staging", Source: "development"},
	}, steps)
}

func TestPromotionStepsRejectsIncompleteTopology(t *testing.T) {
	_, err := promotionSteps("development", []ProjectIdentityEnvironment{
		{Name: "development"},
		{Name: "production"},
	})
	require.ErrorContains(t, err, "unreachable")
}

func TestAttemptProvisionPromotesOnlyTheTargetAgentID(t *testing.T) {
	binding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a",
		EnvironmentName: "stage", PromotionSource: "development",
		ProvisioningType: models.AgentProvisioningTypeInternal,
	}
	source := &models.AgentThunderClient{
		OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a", EnvironmentName: "development",
		Status: models.AgentThunderStatusCompleted, ThunderAgentID: "agent-id", ThunderClientID: "source-client",
	}
	var update repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			return source, nil
		},
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, value repositories.AgentThunderAttemptUpdate) error {
			update = value
			return nil
		},
	}
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			return "target-secret-reference", nil
		},
	}
	promoted := false
	provider := projectAgentIdentityPromotionProviderStub{
		projectAgentIdentityProviderStub: projectAgentIdentityProviderStub{
			source: func(context.Context, string, string, string) (string, error) { return "development", nil },
			provision: func(context.Context, ProjectAgentIdentityRequest) (ProjectAgentIdentityResult, error) {
				t.Fatal("target environments must use promotion")
				return ProjectAgentIdentityResult{}, nil
			},
			activate: func(context.Context, ProjectAgentIdentityRequest, string) error {
				t.Fatal("a promoted target must not run source activation")
				return nil
			},
		},
		environments: func(context.Context, string, string, string) ([]ProjectIdentityEnvironment, error) {
			return nil, nil
		},
		promote: func(_ context.Context, request ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error) {
			promoted = true
			assert.Equal(t, "development", request.SourceEnvironment)
			assert.Equal(t, "stage", request.TargetEnvironment)
			assert.Equal(t, "agent-id", request.ExistingAgentID)
			assert.Equal(t, "source-client", request.ExistingClientID)
			assert.NotEmpty(t, request.TargetClientID)
			assert.NotEmpty(t, request.TargetClientSecret)
			return ProjectAgentIdentityResult{
				AgentID: request.ExistingAgentID, ClientID: request.TargetClientID, ClientSecret: request.TargetClientSecret,
			}, nil
		},
	}
	svc := newTestProvisioningService(repo, &clientmocks.EnvThunderResolverMock{}, store)
	require.NoError(t, svc.(ProjectAgentIdentityProviderSetter).SetProjectAgentIdentityProvider(
		thundersvc.IdentityManagementCPManaged, provider,
	))

	svc.AttemptProvision(context.Background(), binding)

	require.True(t, promoted)
	require.Equal(t, models.AgentThunderStatusCompleted, update.Status)
	require.NotNil(t, update.ThunderAgentID)
	require.Equal(t, "agent-id", *update.ThunderAgentID)
	require.NotNil(t, update.SecretRefPath)
	require.Equal(t, "target-secret-reference", *update.SecretRefPath)
}

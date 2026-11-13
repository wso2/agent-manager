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
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	clientmocks "github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	repomocks "github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type projectIdentityIDResolverFunc func(ctx context.Context, organizationID, projectName string) (string, error)

func (f projectIdentityIDResolverFunc) ProjectIdentityID(ctx context.Context, organizationID, projectName string) (string, error) {
	return f(ctx, organizationID, projectName)
}

// provisionWithResolver runs one source attempt in CP_MANAGED mode with the resolver and returns the
// requests the provider saw, whether the attempt was deferred and what it recorded.
func provisionWithResolver(t *testing.T, resolver ProjectIdentityIDResolver) (requests []ProjectAgentIdentityRequest, deferred bool, update *repositories.AgentThunderAttemptUpdate) {
	t.Helper()
	binding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "org-1", ProjectName: "project-1", AgentName: "agent-1",
		EnvironmentName: models.DefaultEnvironmentName, ProvisioningType: models.AgentProvisioningTypeInternal,
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc:     noExistingBindings,
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		ReleaseClaimFunc: func(context.Context, uuid.UUID, time.Time, string) error {
			deferred = true
			return nil
		},
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, value repositories.AgentThunderAttemptUpdate) error {
			update = &value
			return nil
		},
	}
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			return "agent-id-secret", nil
		},
	}
	provider := projectAgentIdentityProviderStub{
		provision: func(_ context.Context, request ProjectAgentIdentityRequest) (ProjectAgentIdentityResult, error) {
			requests = append(requests, request)
			return ProjectAgentIdentityResult{AgentID: "agent-id", ClientID: "client-id", ClientSecret: "client-secret"}, nil
		},
		activate: func(_ context.Context, request ProjectAgentIdentityRequest, _ string) error {
			requests = append(requests, request)
			return nil
		},
	}
	svc := newTestProvisioningService(repo, &clientmocks.EnvThunderResolverMock{}, store)
	require.NoError(t, svc.(ProjectAgentIdentityProviderSetter).SetProjectAgentIdentityProvider(thundersvc.IdentityManagementCPManaged, provider))
	svc.(ProjectIdentityIDResolverSetter).SetProjectIdentityIDResolver(resolver)

	svc.AttemptProvision(context.Background(), binding)
	return requests, deferred, update
}

func TestAttemptProvision_UsesTheProjectsIdentityID(t *testing.T) {
	const projectUID = "01a1b99b-649e-7ffa-af90-e7d2e80a8fcd"
	requests, deferred, update := provisionWithResolver(t, projectIdentityIDResolverFunc(
		func(_ context.Context, organizationID, projectName string) (string, error) {
			assert.Equal(t, "org-1", organizationID)
			assert.Equal(t, "project-1", projectName)
			return projectUID, nil
		},
	))

	require.False(t, deferred)
	require.Len(t, requests, 2, "provision and activate")
	for _, request := range requests {
		assert.Equal(t, projectUID, request.ProjectID, "the project the identity platform keeps for this one")
		assert.Equal(t, "project-1", request.ProjectName, "the name stays the name: naming and handles use it")
	}
	require.NotNil(t, update)
	assert.Equal(t, models.AgentThunderStatusCompleted, update.Status)
}

func TestAttemptProvision_KeepsTheNameWhenNoIDIsKnown(t *testing.T) {
	requests, deferred, _ := provisionWithResolver(t, projectIdentityIDResolverFunc(
		func(context.Context, string, string) (string, error) { return "", nil },
	))
	require.False(t, deferred)
	require.Len(t, requests, 2)
	for _, request := range requests {
		assert.Equal(t, "project-1", request.ProjectID)
	}
}

func TestAttemptProvision_WaitsWhenTheProjectIDCannotBeRead(t *testing.T) {
	requests, deferred, update := provisionWithResolver(t, projectIdentityIDResolverFunc(
		func(context.Context, string, string) (string, error) { return "", errors.New("openchoreo unavailable") },
	))

	assert.True(t, deferred, "released for the next tick")
	assert.Empty(t, requests, "no AgentID is created in a project the provider would invent")
	assert.Nil(t, update, "no attempt is used up")
}

func TestPlatformProjectIdentityIDResolver(t *testing.T) {
	tests := []struct {
		name    string
		project *models.ProjectResponse
		err     error
		want    string
		wantErr bool
	}{
		{name: "the project's UID, trimmed", project: &models.ProjectResponse{UUID: " 01a1b99b-649e-7ffa-af90-e7d2e80a8fcd "}, want: "01a1b99b-649e-7ffa-af90-e7d2e80a8fcd"},
		{name: "a project without a UID is not an error", project: &models.ProjectResponse{}, want: ""},
		{name: "no project is not an error", project: nil, want: ""},
		{name: "a failed read is an error", err: errors.New("boom"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oc := &repomocks.PlatformRepositoryMock{
				GetProjectFunc: func(_ context.Context, ouID, projectName string) (*models.ProjectResponse, error) {
					assert.Equal(t, "org-1", ouID)
					assert.Equal(t, "project-1", projectName)
					return tt.project, tt.err
				},
			}
			got, err := NewPlatformProjectIdentityIDResolver(oc).ProjectIdentityID(context.Background(), "org-1", "project-1")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// A project that no longer exists is not an outage: waiting for it would never end. The attempt
// fails, counts against the retries, and tells the user why.
func TestAttemptProvision_FailsWhenTheProjectNoLongerExists(t *testing.T) {
	requests, deferred, update := provisionWithResolver(t, projectIdentityIDResolverFunc(
		func(context.Context, string, string) (string, error) {
			return "", fmt.Errorf("get project: project project-1: %w", utils.ErrProjectNotFound)
		},
	))

	assert.False(t, deferred, "not released to wait for the next tick")
	assert.Empty(t, requests, "no AgentID is created for a project that is gone")
	require.NotNil(t, update, "the attempt is recorded")
	assert.Equal(t, "The project no longer exists.", update.LastError)
	assert.Equal(t, models.AgentThunderStatusPending, update.Status, "retried like any other attempt until they run out")
}

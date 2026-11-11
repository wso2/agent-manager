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
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/constants"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// stubAgentThunderProvisioning implements AgentThunderProvisioningService by
// embedding the (nil) interface and overriding only RegenerateFunc — any
// other method call panics on the nil embed, which is fine since tests using
// this stub never call them.
type stubAgentThunderProvisioning struct {
	AgentThunderProvisioningService
	RegenerateFunc                       func(ctx context.Context, orgName, projectName, agentName, envName string) (models.AgentProvisioningType, string, string, error)
	RevokeFunc                           func(ctx context.Context, orgName, projectName, agentName, envName string) (string, error)
	GetBindingStateFunc                  func(ctx context.Context, orgName, projectName, agentName, envName string) (*AgentThunderBindingState, error)
	GetAgentRolesFunc                    func(ctx context.Context, orgName, projectName, agentName, envName string) ([]thundersvc.ThunderRole, error)
	GetAgentGroupsFunc                   func(ctx context.Context, orgName, projectName, agentName, envName string) ([]thundersvc.ThunderGroup, error)
	GetIdentityViewsFunc                 func(ctx context.Context, ouID, projectName, agentName string) ([]models.AgentIdentityEnvironmentView, error)
	ProvisionForEnvironmentIfMissingFunc func(ctx context.Context, ouID, projectName, agentName, envName string, ownership models.AgentProvisioningType, requestedBy string) (bool, error)
}

func (s *stubAgentThunderProvisioning) GetBindingState(ctx context.Context, orgName, projectName, agentName, envName string) (*AgentThunderBindingState, error) {
	return s.GetBindingStateFunc(ctx, orgName, projectName, agentName, envName)
}

func (s *stubAgentThunderProvisioning) RegenerateSecret(ctx context.Context, orgName, projectName, agentName, envName string) (models.AgentProvisioningType, string, string, error) {
	return s.RegenerateFunc(ctx, orgName, projectName, agentName, envName)
}

func (s *stubAgentThunderProvisioning) RevokeSecret(ctx context.Context, orgName, projectName, agentName, envName string) (string, error) {
	return s.RevokeFunc(ctx, orgName, projectName, agentName, envName)
}

func (s *stubAgentThunderProvisioning) GetAgentRoles(ctx context.Context, orgName, projectName, agentName, envName string) ([]thundersvc.ThunderRole, error) {
	return s.GetAgentRolesFunc(ctx, orgName, projectName, agentName, envName)
}

func (s *stubAgentThunderProvisioning) GetAgentGroups(ctx context.Context, orgName, projectName, agentName, envName string) ([]thundersvc.ThunderGroup, error) {
	return s.GetAgentGroupsFunc(ctx, orgName, projectName, agentName, envName)
}

func (s *stubAgentThunderProvisioning) GetIdentityViews(ctx context.Context, ouID, projectName, agentName string) ([]models.AgentIdentityEnvironmentView, error) {
	return s.GetIdentityViewsFunc(ctx, ouID, projectName, agentName)
}

func (s *stubAgentThunderProvisioning) ProvisionForEnvironmentIfMissing(ctx context.Context, ouID, projectName, agentName, envName string, ownership models.AgentProvisioningType, requestedBy string) (bool, error) {
	return s.ProvisionForEnvironmentIfMissingFunc(ctx, ouID, projectName, agentName, envName, ownership, requestedBy)
}

func TestProvisionAgentIdentity_UsesAgentProvisioningType(t *testing.T) {
	for _, tc := range []struct {
		name      string
		agentType utils.AgentProvisioningType
		want      models.AgentProvisioningType
	}{
		{name: "internal", agentType: utils.InternalAgent, want: models.AgentProvisioningTypeInternal},
		{name: "external", agentType: utils.ExternalAgent, want: models.AgentProvisioningTypeExternal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got models.AgentProvisioningType
			provisioning := &stubAgentThunderProvisioning{
				ProvisionForEnvironmentIfMissingFunc: func(_ context.Context, _, _, _, _ string, ownership models.AgentProvisioningType, _ string) (bool, error) {
					got = ownership
					return false, nil
				},
				GetIdentityViewsFunc: func(context.Context, string, string, string) ([]models.AgentIdentityEnvironmentView, error) {
					return []models.AgentIdentityEnvironmentView{{EnvironmentName: "development", ProvisioningType: tc.want, Status: models.AgentThunderStatusPending}}, nil
				},
			}
			svc := &agentManagerService{
				platform: &repomocks.PlatformRepositoryMock{
					GetAgentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
						return &models.AgentResponse{Provisioning: models.Provisioning{Type: string(tc.agentType)}}, nil
					},
					GetEnvironmentFunc: func(context.Context, string, string) (*models.EnvironmentResponse, error) {
						return &models.EnvironmentResponse{}, nil
					},
				},
				agentThunderProvisioning: provisioning,
				logger:                   discardLogger(),
			}

			view, existed, err := svc.ProvisionAgentIdentity(context.Background(), "ou-1", "default", "agent-1", "development")
			require.NoError(t, err)
			assert.False(t, existed)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, "development", view.EnvironmentName)
		})
	}
}

func TestRegenerateAgentIdentitySecret_ExternalAgent_ReturnsSecret(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		RegenerateFunc: func(_ context.Context, _, _, _, _ string) (models.AgentProvisioningType, string, string, error) {
			return models.AgentProvisioningTypeExternal, "client-abc", "fresh-secret-xyz", nil
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub}

	resp, err := s.RegenerateAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "dev")

	require.NoError(t, err)
	assert.Equal(t, "dev", resp.EnvironmentName)
	assert.Equal(t, models.AgentProvisioningTypeExternal, resp.ProvisioningType)
	assert.Equal(t, "client-abc", resp.ClientID)
	assert.Equal(t, "fresh-secret-xyz", resp.ClientSecret,
		"an External agent must get its freshly regenerated secret back")
	assert.Equal(t, models.AgentRegenerateSecretStatus, resp.Status)
	assert.Empty(t, resp.WorkloadRefreshWarning, "external agents have no workload to refresh")
}

func TestRegenerateAgentIdentitySecret_InternalAgent_AlsoReturnsSecret(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		RegenerateFunc: func(_ context.Context, _, _, _, _ string) (models.AgentProvisioningType, string, string, error) {
			return models.AgentProvisioningTypeInternal, "client-def", "fresh-secret-internal", nil
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub, logger: discardLogger()}

	resp, err := s.RegenerateAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "dev")

	require.NoError(t, err)
	assert.Equal(t, models.AgentProvisioningTypeInternal, resp.ProvisioningType)
	assert.Equal(t, "fresh-secret-internal", resp.ClientSecret,
		"an Internal agent must ALSO get its freshly regenerated secret back — regenerate is not the "+
			"one-time-claim endpoint, withholding it here would just force a second call to see it")
	assert.Equal(t, models.AgentRegenerateSecretStatus, resp.Status)
}

func TestRevokeAgentIdentitySecret_ReturnsClientID(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		RevokeFunc: func(_ context.Context, _, _, _, envName string) (string, error) {
			assert.Equal(t, "dev", envName)
			return "client-abc", nil
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub, logger: discardLogger()}

	resp, err := s.RevokeAgentIdentitySecret(context.Background(), "acme", "proj1", "my-agent", "dev")

	require.NoError(t, err)
	assert.Equal(t, "client-abc", resp.ClientID)
	assert.Equal(t, "dev", resp.EnvironmentName)
	assert.Empty(t, resp.WorkloadRefreshWarning, "there is no workload to clean up")
}

func TestGetAgentRoles_DefaultEnvironment_DelegatesToProvisioning(t *testing.T) {
	wantRoles := []thundersvc.ThunderRole{{ID: "role-1", Name: "reader"}}
	stub := &stubAgentThunderProvisioning{
		GetAgentRolesFunc: func(_ context.Context, _, _, _, _ string) ([]thundersvc.ThunderRole, error) {
			return wantRoles, nil
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub, logger: slog.Default()}

	roles, err := s.GetAgentRoles(context.Background(), "acme", "proj1", "my-agent", models.DefaultEnvironmentName)

	require.NoError(t, err)
	assert.Equal(t, wantRoles, roles)
}

// TestGetAgentRoles_OtherEnvironment_Errors guards the same visibility
// rule GetAgentIdentity applies: a project only ever sees bindings for its
// own environments, and the only one is the org's default environment.
func TestGetAgentRoles_OtherEnvironment_Errors(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		GetAgentRolesFunc: func(_ context.Context, _, _, _, _ string) ([]thundersvc.ThunderRole, error) {
			t.Fatal("must not reach the provisioning layer for an environment the project cannot see")
			return nil, nil
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub, logger: slog.Default()}

	_, err := s.GetAgentRoles(context.Background(), "acme", "proj1", "my-agent", "prod")

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
}

func TestGetAgentGroups_DefaultEnvironment_DelegatesToProvisioning(t *testing.T) {
	wantGroups := []thundersvc.ThunderGroup{{ID: "group-1", Name: "operators"}}
	stub := &stubAgentThunderProvisioning{
		GetAgentGroupsFunc: func(_ context.Context, _, _, _, _ string) ([]thundersvc.ThunderGroup, error) {
			return wantGroups, nil
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub, logger: slog.Default()}

	groups, err := s.GetAgentGroups(context.Background(), "acme", "proj1", "my-agent", models.DefaultEnvironmentName)

	require.NoError(t, err)
	assert.Equal(t, wantGroups, groups)
}

// TestGetAgentGroups_OtherEnvironment_Errors is the groups counterpart
// to TestGetAgentRoles_OtherEnvironment_Errors.
func TestGetAgentGroups_OtherEnvironment_Errors(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		GetAgentGroupsFunc: func(_ context.Context, _, _, _, _ string) ([]thundersvc.ThunderGroup, error) {
			t.Fatal("must not reach the provisioning layer for an environment the project cannot see")
			return nil, nil
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub, logger: slog.Default()}

	_, err := s.GetAgentGroups(context.Background(), "acme", "proj1", "my-agent", "prod")

	require.Error(t, err)
	assert.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
}

// These tests cover the service-layer wiring that threads labels into and out
// of the platform repository.

func TestUpdateAgentBasicInfo_PassesLabelsThroughToClient(t *testing.T) {
	newLabels := map[string]string{"team": "ml"}
	emptyLabels := map[string]string{}

	testCases := []struct {
		name       string
		reqLabels  *map[string]string
		wantLabels *map[string]string
	}{
		{name: "nil means unchanged", reqLabels: nil, wantLabels: nil},
		{name: "empty map clears user labels", reqLabels: &emptyLabels, wantLabels: &emptyLabels},
		{name: "populated map replaces user labels", reqLabels: &newLabels, wantLabels: &newLabels},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var captured *map[string]string
			platform := &repomocks.PlatformRepositoryMock{
				GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
					return &models.OrganizationResponse{}, nil
				},
				GetProjectFunc: func(_ context.Context, _, _ string) (*models.ProjectResponse, error) {
					return &models.ProjectResponse{}, nil
				},
				GetAgentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
					return &models.AgentResponse{Name: "agent-1"}, nil
				},
				UpdateAgentBasicInfoFunc: func(_ context.Context, _, _, _, _, _ string, labels *map[string]string) error {
					captured = labels
					return nil
				},
			}
			s := &agentManagerService{platform: platform, logger: discardLogger()}

			_, err := s.UpdateAgentBasicInfo(context.Background(), "acme", "proj1", "agent-1", &spec.UpdateAgentBasicInfoRequest{
				DisplayName: "Agent 1",
				Description: "desc",
				Labels:      tc.reqLabels,
			})

			require.NoError(t, err)
			assert.Equal(t, tc.wantLabels, captured)
		})
	}
}

func TestGetAgent_LabelsPassThroughUnmodified(t *testing.T) {
	wantLabels := map[string]string{"team": "ml"}
	platform := &repomocks.PlatformRepositoryMock{
		GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{}, nil
		},
		GetAgentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Name: "agent-1", Labels: wantLabels}, nil
		},
	}
	configs := &repomocks.AgentConfigRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentConfig, error) {
			return nil, repositories.ErrAgentConfigNotFound
		},
	}
	s := &agentManagerService{platform: platform, agentConfigRepo: configs, logger: discardLogger()}

	agent, err := s.GetAgent(context.Background(), "acme", "proj1", "agent-1")

	require.NoError(t, err)
	assert.Equal(t, wantLabels, agent.Labels)
}

func TestListAgents_LabelsPassThroughUnmodified(t *testing.T) {
	agents := []*models.AgentResponse{
		{Name: "agent-1", Labels: map[string]string{"env": "prod"}},
		{Name: "agent-2", Labels: map[string]string{"env": "dev"}},
	}
	platform := &repomocks.PlatformRepositoryMock{
		GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{}, nil
		},
		ListAgentsFunc: func(_ context.Context, _, _ string) ([]*models.AgentResponse, error) {
			return agents, nil
		},
	}
	s := &agentManagerService{platform: platform, logger: discardLogger()}

	t.Run("unfiltered returns all agents with their labels intact", func(t *testing.T) {
		result, total, err := s.ListAgents(context.Background(), "acme", "proj1", nil, 10, 0)

		require.NoError(t, err)
		assert.Equal(t, int32(2), total)
		require.Len(t, result, 2)
		assert.Equal(t, map[string]string{"env": "prod"}, result[0].Labels)
	})

	t.Run("filtered narrows to the matching agent", func(t *testing.T) {
		result, total, err := s.ListAgents(context.Background(), "acme", "proj1", map[string]string{"env": "prod"}, 10, 0)

		require.NoError(t, err)
		assert.Equal(t, int32(1), total)
		require.Len(t, result, 1)
		assert.Equal(t, "agent-1", result[0].Name)
	})
}

func int32Ptr(v int32) *int32 {
	return &v
}

func TestResolveResilienceTimeoutSeconds(t *testing.T) {
	tests := []struct {
		name      string
		existing  *models.AgentConfig
		requested *int32
		withDef   bool
		want      int32
		wantErr   bool
	}{
		{"nothing set, withDefaults=true falls back to default", nil, nil, true, constants.DefaultResilienceTimeoutSeconds, false},
		{"nothing set, withDefaults=false yields zero (no override)", nil, nil, false, 0, false},
		{"existing DB value is used", &models.AgentConfig{ResilienceTimeoutSeconds: int32Ptr(60)}, nil, false, 60, false},
		{"request overrides existing DB value", &models.AgentConfig{ResilienceTimeoutSeconds: int32Ptr(60)}, int32Ptr(90), false, 90, false},
		{"out-of-bounds request is rejected", &models.AgentConfig{ResilienceTimeoutSeconds: int32Ptr(60)}, int32Ptr(10000), false, 0, true},
		{"minimum bound is accepted", nil, int32Ptr(constants.MinResilienceTimeoutSeconds), false, constants.MinResilienceTimeoutSeconds, false},
		{"maximum bound is accepted", nil, int32Ptr(constants.MaxResilienceTimeoutSeconds), false, constants.MaxResilienceTimeoutSeconds, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveResilienceTimeoutSeconds(tc.existing, tc.requested, tc.withDef)
			if tc.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, utils.ErrInvalidInput)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPopulateCreatedBy_NoProvisioning_LeavesUnset(t *testing.T) {
	s := &agentManagerService{logger: discardLogger()}
	agent := &models.AgentResponse{}

	s.populateCreatedBy(context.Background(), "acme", "proj1", "my-agent", agent)

	assert.Nil(t, agent.CreatedBy)
}

func TestPopulateCreatedBy_NoIdentityClient_LeavesUnset(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		GetIdentityViewsFunc: func(_ context.Context, _, _, _ string) ([]models.AgentIdentityEnvironmentView, error) {
			t.Fatal("must not fetch identity views when there is no identity client to resolve a username with")
			return nil, nil
		},
	}
	s := &agentManagerService{agentThunderProvisioning: stub, logger: discardLogger()}
	agent := &models.AgentResponse{}

	s.populateCreatedBy(context.Background(), "acme", "proj1", "my-agent", agent)

	assert.Nil(t, agent.CreatedBy)
}

func TestPopulateCreatedBy_IdentityViewsError_LeavesUnset(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		GetIdentityViewsFunc: func(_ context.Context, _, _, _ string) ([]models.AgentIdentityEnvironmentView, error) {
			return nil, errors.New("db unavailable")
		},
	}
	s := &agentManagerService{
		agentThunderProvisioning: stub,
		identityClient:           &clientmocks.IdentityClientMock{},
		logger:                   discardLogger(),
	}
	agent := &models.AgentResponse{}

	s.populateCreatedBy(context.Background(), "acme", "proj1", "my-agent", agent)

	assert.Nil(t, agent.CreatedBy)
}

func TestPopulateCreatedBy_NoRequestedBy_LeavesUnset(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		GetIdentityViewsFunc: func(_ context.Context, _, _, _ string) ([]models.AgentIdentityEnvironmentView, error) {
			return []models.AgentIdentityEnvironmentView{{EnvironmentName: "dev"}}, nil
		},
	}
	s := &agentManagerService{
		agentThunderProvisioning: stub,
		identityClient: &clientmocks.IdentityClientMock{
			GetUserFunc: func(_ context.Context, _ string) (*thundersvc.ThunderUser, error) {
				t.Fatal("must not resolve a user when no binding recorded a requester")
				return &thundersvc.ThunderUser{}, nil
			},
		},
		logger: discardLogger(),
	}
	agent := &models.AgentResponse{}

	s.populateCreatedBy(context.Background(), "acme", "proj1", "my-agent", agent)

	assert.Nil(t, agent.CreatedBy)
}

func TestPopulateCreatedBy_ResolvesUsernameFromAttributes(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		GetIdentityViewsFunc: func(_ context.Context, _, _, _ string) ([]models.AgentIdentityEnvironmentView, error) {
			return []models.AgentIdentityEnvironmentView{
				{EnvironmentName: "dev", RequestedBy: ""},
				{EnvironmentName: "staging", RequestedBy: "user-123"},
			}, nil
		},
	}
	s := &agentManagerService{
		agentThunderProvisioning: stub,
		identityClient: &clientmocks.IdentityClientMock{
			GetUserFunc: func(_ context.Context, userID string) (*thundersvc.ThunderUser, error) {
				assert.Equal(t, "user-123", userID)
				return &thundersvc.ThunderUser{
					ID:         userID,
					Display:    "John Doe",
					Attributes: map[string]any{"username": "john.doe"},
				}, nil
			},
		},
		logger: discardLogger(),
	}
	agent := &models.AgentResponse{}

	s.populateCreatedBy(context.Background(), "acme", "proj1", "my-agent", agent)

	require.NotNil(t, agent.CreatedBy)
	assert.Equal(t, "user-123", agent.CreatedBy.ID)
	assert.Equal(t, "john.doe", agent.CreatedBy.Display)
}

func TestPopulateCreatedBy_FallsBackToDisplayWhenNoUsernameAttribute(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		GetIdentityViewsFunc: func(_ context.Context, _, _, _ string) ([]models.AgentIdentityEnvironmentView, error) {
			return []models.AgentIdentityEnvironmentView{{RequestedBy: "user-123"}}, nil
		},
	}
	s := &agentManagerService{
		agentThunderProvisioning: stub,
		identityClient: &clientmocks.IdentityClientMock{
			GetUserFunc: func(_ context.Context, userID string) (*thundersvc.ThunderUser, error) {
				return &thundersvc.ThunderUser{ID: userID, Display: "John Doe"}, nil
			},
		},
		logger: discardLogger(),
	}
	agent := &models.AgentResponse{}

	s.populateCreatedBy(context.Background(), "acme", "proj1", "my-agent", agent)

	require.NotNil(t, agent.CreatedBy)
	assert.Equal(t, "John Doe", agent.CreatedBy.Display)
}

func TestPopulateCreatedBy_UserDeleted_KeepsIDOnly(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		GetIdentityViewsFunc: func(_ context.Context, _, _, _ string) ([]models.AgentIdentityEnvironmentView, error) {
			return []models.AgentIdentityEnvironmentView{{RequestedBy: "user-123"}}, nil
		},
	}
	s := &agentManagerService{
		agentThunderProvisioning: stub,
		identityClient: &clientmocks.IdentityClientMock{
			GetUserFunc: func(_ context.Context, _ string) (*thundersvc.ThunderUser, error) {
				return nil, &thundersvc.NotFoundError{Message: "user not found"}
			},
		},
		logger: discardLogger(),
	}
	agent := &models.AgentResponse{}

	s.populateCreatedBy(context.Background(), "acme", "proj1", "my-agent", agent)

	require.NotNil(t, agent.CreatedBy)
	assert.Equal(t, "user-123", agent.CreatedBy.ID)
	assert.Empty(t, agent.CreatedBy.Display)
}

func TestPopulateCreatedBy_UserLookupError_KeepsIDOnly(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		GetIdentityViewsFunc: func(_ context.Context, _, _, _ string) ([]models.AgentIdentityEnvironmentView, error) {
			return []models.AgentIdentityEnvironmentView{{RequestedBy: "user-123"}}, nil
		},
	}
	s := &agentManagerService{
		agentThunderProvisioning: stub,
		identityClient: &clientmocks.IdentityClientMock{
			GetUserFunc: func(_ context.Context, _ string) (*thundersvc.ThunderUser, error) {
				return nil, errors.New("thunder unreachable")
			},
		},
		logger: discardLogger(),
	}
	agent := &models.AgentResponse{}

	s.populateCreatedBy(context.Background(), "acme", "proj1", "my-agent", agent)

	require.NotNil(t, agent.CreatedBy)
	assert.Equal(t, "user-123", agent.CreatedBy.ID)
	assert.Empty(t, agent.CreatedBy.Display)
}

// A (nil, nil) return isn't something the real Thunder client does, but this is
// best-effort decoration on the GetAgent path: it must degrade to id-only
// rather than panic if any IdentityClient implementation ever behaves that way.
func TestPopulateCreatedBy_NilUserWithoutError_KeepsIDOnly(t *testing.T) {
	stub := &stubAgentThunderProvisioning{
		GetIdentityViewsFunc: func(_ context.Context, _, _, _ string) ([]models.AgentIdentityEnvironmentView, error) {
			return []models.AgentIdentityEnvironmentView{{RequestedBy: "user-123"}}, nil
		},
	}
	s := &agentManagerService{
		agentThunderProvisioning: stub,
		identityClient: &clientmocks.IdentityClientMock{
			GetUserFunc: func(_ context.Context, _ string) (*thundersvc.ThunderUser, error) {
				//nolint:nilnil // deliberately exercising a misbehaving implementation
				return nil, nil
			},
		},
		logger: discardLogger(),
	}
	agent := &models.AgentResponse{}

	s.populateCreatedBy(context.Background(), "acme", "proj1", "my-agent", agent)

	require.NotNil(t, agent.CreatedBy)
	assert.Equal(t, "user-123", agent.CreatedBy.ID)
	assert.Empty(t, agent.CreatedBy.Display)
}

func TestListOrgAgents_AggregatesAcrossProjects(t *testing.T) {
	platform := &repomocks.PlatformRepositoryMock{
		GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{}, nil
		},
		ListProjectsFunc: func(_ context.Context, _ string) ([]*models.ProjectResponse, error) {
			return []*models.ProjectResponse{
				{Name: "proj1", DisplayName: "Project One"},
				{Name: "proj2", DisplayName: "Project Two"},
			}, nil
		},
		ListAgentsFunc: func(_ context.Context, _ string, projectName string) ([]*models.AgentResponse, error) {
			switch projectName {
			case "proj1":
				return []*models.AgentResponse{{Name: "agent-a", DisplayName: "Agent A", ProjectName: "proj1"}}, nil
			case "proj2":
				return []*models.AgentResponse{{Name: "agent-b", DisplayName: "Agent B", ProjectName: "proj2"}}, nil
			default:
				return nil, fmt.Errorf("unexpected project name %q", projectName)
			}
		},
	}
	s := &agentManagerService{platform: platform, logger: discardLogger()}

	agents, err := s.ListOrgAgents(context.Background(), "acme")

	require.NoError(t, err)
	byName := make(map[string]*models.AgentSummary, len(agents))
	for _, a := range agents {
		byName[a.Name] = a
	}
	require.Contains(t, byName, "agent-a")
	require.Contains(t, byName, "agent-b")
	assert.Equal(t, "Project One", byName["agent-a"].ProjectDisplayName)
	assert.Equal(t, "Project Two", byName["agent-b"].ProjectDisplayName)
}

func TestListOrgAgents_OrganizationNotFound(t *testing.T) {
	platform := &repomocks.PlatformRepositoryMock{
		GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
			return nil, utils.ErrOrganizationNotFound
		},
	}
	s := &agentManagerService{platform: platform, logger: discardLogger()}

	_, err := s.ListOrgAgents(context.Background(), "acme")

	assert.ErrorIs(t, err, utils.ErrOrganizationNotFound)
}

func TestListOrgAgents_ProjectListFailurePropagates(t *testing.T) {
	platform := &repomocks.PlatformRepositoryMock{
		GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{}, nil
		},
		ListProjectsFunc: func(_ context.Context, _ string) ([]*models.ProjectResponse, error) {
			return nil, errors.New("openchoreo unavailable")
		},
	}
	s := &agentManagerService{platform: platform, logger: discardLogger()}

	_, err := s.ListOrgAgents(context.Background(), "acme")

	require.Error(t, err)
	assert.NotErrorIs(t, err, utils.ErrOrganizationNotFound, "an unrelated openchoreo failure must not be masked as not-found")
}

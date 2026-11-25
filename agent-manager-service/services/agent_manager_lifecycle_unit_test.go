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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// lifecycleProvisioning adds the agent lifecycle calls to stubAgentThunderProvisioning.
type lifecycleProvisioning struct {
	*stubAgentThunderProvisioning
	provisionForAgent func(ouID, project, agent string, ownership models.AgentProvisioningType, envs []string, requestedBy, awaitEnv string)
	deleteAll         func(ouID, project, agent string)
	retry             func(ouID, project, agent, env string) (models.AgentIdentityEnvironmentView, error)
}

func (p *lifecycleProvisioning) ProvisionForAgent(_ context.Context, ouID, project, agent string, ownership models.AgentProvisioningType, envs []string, requestedBy, awaitEnv string) {
	p.provisionForAgent(ouID, project, agent, ownership, envs, requestedBy, awaitEnv)
}

func (p *lifecycleProvisioning) DeleteAllBindings(_ context.Context, ouID, project, agent string) {
	p.deleteAll(ouID, project, agent)
}

func (p *lifecycleProvisioning) RetryProvisioning(_ context.Context, ouID, project, agent, env string) (models.AgentIdentityEnvironmentView, error) {
	return p.retry(ouID, project, agent, env)
}

// outcomes lists the recorded events as action:outcome.
func outcomes(r *recordingAuditor) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []string{}
	for _, e := range r.events {
		out = append(out, string(e.Action)+":"+string(e.Outcome))
	}
	return out
}

func withCaller(ctx context.Context, sub string) context.Context {
	return jwtassertion.ContextWithTokenClaims(ctx, &jwtassertion.TokenClaims{Sub: sub, OuId: "ou-1"})
}

func externalAgentRequest(name string) *spec.CreateAgentRequest {
	return &spec.CreateAgentRequest{Name: name, DisplayName: "Agent", Provisioning: spec.Provisioning{Type: string(utils.ExternalAgent)}}
}

func TestCreateAgent_RejectsHostedAgents(t *testing.T) {
	svc := &agentManagerService{platform: &repomocks.PlatformRepositoryMock{}, logger: discardLogger()}
	req := externalAgentRequest("a")
	req.Provisioning.Type = string(utils.InternalAgent)
	err := svc.CreateAgent(context.Background(), "ou-1", "p", req)
	require.ErrorIs(t, err, httpx.ErrNotImplemented)
}

func TestCreateAgent_StoresAndStartsIdentityProvisioning(t *testing.T) {
	var stored repositories.CreateAgentInput
	var provisioned []string
	var requestedBy string
	platform := &repomocks.PlatformRepositoryMock{
		CreateAgentFunc: func(_ context.Context, ouID, project string, in repositories.CreateAgentInput) (*models.AgentResponse, error) {
			stored = in
			return &models.AgentResponse{Name: in.Name}, nil
		},
		ListEnvironmentsFunc: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{Name: models.DefaultEnvironmentName}}, nil
		},
	}
	prov := &lifecycleProvisioning{
		stubAgentThunderProvisioning: &stubAgentThunderProvisioning{},
		provisionForAgent: func(ouID, project, agent string, ownership models.AgentProvisioningType, envs []string, by, awaitEnv string) {
			assert.Equal(t, models.AgentProvisioningTypeExternal, ownership)
			assert.Empty(t, awaitEnv, "creation never waits for an identity")
			provisioned, requestedBy = envs, by
		},
	}
	svc := &agentManagerService{platform: platform, agentThunderProvisioning: prov, logger: discardLogger()}

	req := externalAgentRequest("bot")
	req.Labels = &map[string]string{"team": "core"}
	req.Description = strPtr("does things")
	ext := map[string]map[string]any{"guardrails": {"enabled": true}}
	req.Extensions = &ext
	require.NoError(t, svc.CreateAgent(withCaller(context.Background(), "user-7"), "ou-1", "p", req))

	assert.Equal(t, "bot", stored.Name)
	assert.Equal(t, "does things", stored.Description)
	assert.Equal(t, map[string]string{"team": "core"}, stored.Labels)
	assert.Equal(t, "user-7", stored.CreatedBy)
	assert.JSONEq(t, `{"enabled":true}`, string(stored.Extensions["guardrails"]))
	assert.Equal(t, []string{models.DefaultEnvironmentName}, provisioned)
	assert.Equal(t, "user-7", requestedBy)
}

func TestCreateAgent_StoreFailureSkipsProvisioning(t *testing.T) {
	platform := &repomocks.PlatformRepositoryMock{
		CreateAgentFunc: func(context.Context, string, string, repositories.CreateAgentInput) (*models.AgentResponse, error) {
			return nil, utils.ErrAgentAlreadyExists
		},
	}
	// provisionForAgent is nil: calling it would panic.
	prov := &lifecycleProvisioning{stubAgentThunderProvisioning: &stubAgentThunderProvisioning{}}
	svc := &agentManagerService{platform: platform, agentThunderProvisioning: prov, logger: discardLogger()}
	err := svc.CreateAgent(context.Background(), "ou-1", "p", externalAgentRequest("dup"))
	require.ErrorIs(t, err, utils.ErrAgentAlreadyExists)
}

// An environment listing failure must not fail the create: the reconciler provisions later.
func TestCreateAgent_EnvironmentListFailureStillCreates(t *testing.T) {
	platform := &repomocks.PlatformRepositoryMock{
		CreateAgentFunc: func(context.Context, string, string, repositories.CreateAgentInput) (*models.AgentResponse, error) {
			return &models.AgentResponse{}, nil
		},
		ListEnvironmentsFunc: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
			return nil, errors.New("db down")
		},
	}
	prov := &lifecycleProvisioning{stubAgentThunderProvisioning: &stubAgentThunderProvisioning{}}
	svc := &agentManagerService{platform: platform, agentThunderProvisioning: prov, logger: discardLogger()}
	require.NoError(t, svc.CreateAgent(context.Background(), "ou-1", "p", externalAgentRequest("a")))

	platform.ListEnvironmentsFunc = func(context.Context, string) ([]*models.EnvironmentResponse, error) {
		return []*models.EnvironmentResponse{}, nil
	}
	require.NoError(t, svc.CreateAgent(context.Background(), "ou-1", "p", externalAgentRequest("b")))

	// Without a provisioning implementation nothing is listed at all.
	bare := &agentManagerService{platform: &repomocks.PlatformRepositoryMock{CreateAgentFunc: platform.CreateAgentFunc}, logger: discardLogger()}
	require.NoError(t, bare.CreateAgent(context.Background(), "ou-1", "p", externalAgentRequest("c")))
}

func TestCreateAgent_UnencodableExtensionIsRejected(t *testing.T) {
	svc := &agentManagerService{platform: &repomocks.PlatformRepositoryMock{}, logger: discardLogger()}
	req := externalAgentRequest("a")
	ext := map[string]map[string]any{"bad": {"f": func() {}}}
	req.Extensions = &ext
	err := svc.CreateAgent(context.Background(), "ou-1", "p", req)
	var mErr *modkit.Error
	require.ErrorAs(t, err, &mErr)

	_, err = svc.UpdateAgentBasicInfo(context.Background(), "ou-1", "p", "a", &spec.UpdateAgentBasicInfoRequest{Extensions: &ext})
	require.ErrorAs(t, err, &mErr)
}

func deletePlatform(deleteErr error) *repomocks.PlatformRepositoryMock {
	return &repomocks.PlatformRepositoryMock{
		GetProjectFunc: func(context.Context, string, string) (*models.ProjectResponse, error) {
			return &models.ProjectResponse{Name: "p"}, nil
		},
		CheckDeleteAgentFunc: func(context.Context, string, string, string) error { return nil },
		DeleteAgentFunc:      func(context.Context, string, string, string) error { return deleteErr },
	}
}

func TestDeleteAgent_RevokesIdentitiesAndAudits(t *testing.T) {
	rec := &recordingAuditor{}
	ctx := audit.WithRecorder(context.Background(), rec)
	var order []string
	platform := deletePlatform(nil)
	platform.DeleteAgentFunc = func(context.Context, string, string, string) error {
		order = append(order, "delete")
		return nil
	}
	prov := &lifecycleProvisioning{
		stubAgentThunderProvisioning: &stubAgentThunderProvisioning{},
		deleteAll: func(ouID, project, agent string) {
			assert.Equal(t, []string{"ou-1", "p", "bot"}, []string{ouID, project, agent})
			order = append(order, "revoke")
		},
	}
	svc := &agentManagerService{platform: platform, agentThunderProvisioning: prov, logger: discardLogger()}

	require.NoError(t, svc.DeleteAgent(ctx, "ou-1", "p", "bot"))
	assert.Equal(t, []string{"revoke", "delete"}, order, "identities are revoked while their rows still exist")
	assert.Contains(t, outcomes(rec), string(audit.ActionAgentDelete)+":"+string(audit.OutcomeSuccess))
}

func TestDeleteAgent_RefusalsLeaveTheAgent(t *testing.T) {
	errRefused := modkit.Reject("a module still uses this agent")
	for name, tc := range map[string]struct {
		platform func() *repomocks.PlatformRepositoryMock
		ctx      func() context.Context
		want     error
	}{
		"project missing": {
			platform: func() *repomocks.PlatformRepositoryMock {
				p := deletePlatform(nil)
				p.GetProjectFunc = func(context.Context, string, string) (*models.ProjectResponse, error) {
					return nil, utils.ErrProjectNotFound
				}
				return p
			},
			ctx:  func() context.Context { return audit.WithRecorder(context.Background(), &recordingAuditor{}) },
			want: utils.ErrProjectNotFound,
		},
		"module refuses": {
			platform: func() *repomocks.PlatformRepositoryMock {
				p := deletePlatform(nil)
				p.CheckDeleteAgentFunc = func(context.Context, string, string, string) error { return errRefused }
				return p
			},
			ctx:  func() context.Context { return audit.WithRecorder(context.Background(), &recordingAuditor{}) },
			want: errRefused,
		},
		"audit unavailable": {
			platform: func() *repomocks.PlatformRepositoryMock { return deletePlatform(nil) },
			ctx:      context.Background,
			want:     audit.ErrRecorderUnavailable,
		},
	} {
		t.Run(name, func(t *testing.T) {
			// deleteAll is nil, DeleteAgent of the platform must not run either: both would show in a panic or error.
			prov := &lifecycleProvisioning{stubAgentThunderProvisioning: &stubAgentThunderProvisioning{}}
			p := tc.platform()
			p.DeleteAgentFunc = nil
			svc := &agentManagerService{platform: p, agentThunderProvisioning: prov, logger: discardLogger()}
			require.ErrorIs(t, svc.DeleteAgent(tc.ctx(), "ou-1", "p", "bot"), tc.want)
		})
	}
}

func TestDeleteAgent_MissingAgentIsNotAnError(t *testing.T) {
	platform := deletePlatform(utils.ErrAgentNotFound)
	platform.CheckDeleteAgentFunc = func(context.Context, string, string, string) error { return utils.ErrAgentNotFound }
	svc := &agentManagerService{platform: platform, logger: discardLogger()}
	require.NoError(t, svc.DeleteAgent(auditableCtx(t), "ou-1", "p", "gone"))

	errDB := errors.New("db down")
	rec := &recordingAuditor{}
	svc.platform = deletePlatform(errDB)
	require.ErrorIs(t, svc.DeleteAgent(audit.WithRecorder(context.Background(), rec), "ou-1", "p", "bot"), errDB)
	assert.Contains(t, outcomes(rec), string(audit.ActionAgentDelete)+":"+string(audit.OutcomeFailure), "a failed delete is recorded as failed")
}

func TestGenerateName(t *testing.T) {
	ctx := context.Background()
	org := func(context.Context, string) (*models.OrganizationResponse, error) {
		return &models.OrganizationResponse{Name: "acme"}, nil
	}
	project := strPtr("p")

	t.Run("agent name free", func(t *testing.T) {
		svc := &agentManagerService{platform: &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: org,
			GetProjectFunc: func(context.Context, string, string) (*models.ProjectResponse, error) {
				return &models.ProjectResponse{Name: "p"}, nil
			},
			AgentExistsFunc: func(context.Context, string, string, string) (bool, error) { return false, nil },
		}, logger: discardLogger()}
		name, err := svc.GenerateName(ctx, "ou", spec.ResourceNameRequest{DisplayName: "My Agent", ResourceType: string(utils.ResourceTypeAgent), ProjectName: project})
		require.NoError(t, err)
		assert.Equal(t, "my-agent", name)
	})

	t.Run("agent name taken gets a suffix", func(t *testing.T) {
		svc := &agentManagerService{platform: &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: org,
			GetProjectFunc: func(context.Context, string, string) (*models.ProjectResponse, error) {
				return &models.ProjectResponse{Name: "p"}, nil
			},
			AgentExistsFunc: func(_ context.Context, _, _, name string) (bool, error) { return name == "my-agent", nil },
		}, logger: discardLogger()}
		name, err := svc.GenerateName(ctx, "ou", spec.ResourceNameRequest{DisplayName: "My Agent", ResourceType: string(utils.ResourceTypeAgent), ProjectName: project})
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(name, "my-agent-"), name)
	})

	t.Run("agent existence check fails", func(t *testing.T) {
		errDB := errors.New("db down")
		svc := &agentManagerService{platform: &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: org,
			GetProjectFunc: func(context.Context, string, string) (*models.ProjectResponse, error) {
				return &models.ProjectResponse{Name: "p"}, nil
			},
			AgentExistsFunc: func(context.Context, string, string, string) (bool, error) { return false, errDB },
		}, logger: discardLogger()}
		_, err := svc.GenerateName(ctx, "ou", spec.ResourceNameRequest{DisplayName: "x", ResourceType: string(utils.ResourceTypeAgent), ProjectName: project})
		require.ErrorIs(t, err, errDB)
	})

	t.Run("agent in a missing project", func(t *testing.T) {
		svc := &agentManagerService{platform: &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: org,
			GetProjectFunc: func(context.Context, string, string) (*models.ProjectResponse, error) {
				return nil, utils.ErrProjectNotFound
			},
		}, logger: discardLogger()}
		_, err := svc.GenerateName(ctx, "ou", spec.ResourceNameRequest{DisplayName: "x", ResourceType: string(utils.ResourceTypeAgent), ProjectName: project})
		require.ErrorIs(t, err, utils.ErrProjectNotFound)
	})

	t.Run("project names", func(t *testing.T) {
		taken := map[string]bool{"billing": true}
		svc := &agentManagerService{platform: &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: org,
			GetProjectFunc: func(_ context.Context, _, name string) (*models.ProjectResponse, error) {
				if taken[name] {
					return &models.ProjectResponse{Name: name}, nil
				}
				return nil, utils.ErrProjectNotFound
			},
		}, logger: discardLogger()}
		name, err := svc.GenerateName(ctx, "ou", spec.ResourceNameRequest{DisplayName: "Search", ResourceType: string(utils.ResourceTypeProject)})
		require.NoError(t, err)
		assert.Equal(t, "search", name)
		name, err = svc.GenerateName(ctx, "ou", spec.ResourceNameRequest{DisplayName: "Billing", ResourceType: string(utils.ResourceTypeProject)})
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(name, "billing-"), name)
	})

	t.Run("project lookup error is not read as a free name", func(t *testing.T) {
		errDB := errors.New("db down")
		svc := &agentManagerService{platform: &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: org,
			GetProjectFunc: func(context.Context, string, string) (*models.ProjectResponse, error) {
				return nil, errDB
			},
		}, logger: discardLogger()}
		_, err := svc.GenerateName(ctx, "ou", spec.ResourceNameRequest{DisplayName: "x", ResourceType: string(utils.ResourceTypeProject)})
		require.ErrorIs(t, err, errDB)
	})

	t.Run("other resource types", func(t *testing.T) {
		svc := &agentManagerService{platform: &repomocks.PlatformRepositoryMock{GetOrganizationFunc: org}, logger: discardLogger()}
		_, err := svc.GenerateName(ctx, "ou", spec.ResourceNameRequest{DisplayName: "x", ResourceType: string(utils.ResourceTypeEnvironment)})
		require.ErrorIs(t, err, httpx.ErrNotImplemented)
		_, err = svc.GenerateName(ctx, "ou", spec.ResourceNameRequest{DisplayName: "x", ResourceType: "gizmo"})
		require.ErrorContains(t, err, "invalid resource type")
	})

	t.Run("organization missing", func(t *testing.T) {
		svc := &agentManagerService{platform: &repomocks.PlatformRepositoryMock{
			GetOrganizationFunc: func(context.Context, string) (*models.OrganizationResponse, error) {
				return nil, utils.ErrOrganizationNotFound
			},
		}, logger: discardLogger()}
		_, err := svc.GenerateName(ctx, "ou", spec.ResourceNameRequest{DisplayName: "x", ResourceType: string(utils.ResourceTypeProject)})
		require.ErrorIs(t, err, utils.ErrOrganizationNotFound)
	})
}

func TestGetAgentIdentity_ShowsOnlyThePipelinesEnvironments(t *testing.T) {
	prov := &stubAgentThunderProvisioning{
		GetIdentityViewsFunc: func(context.Context, string, string, string) ([]models.AgentIdentityEnvironmentView, error) {
			return []models.AgentIdentityEnvironmentView{
				{EnvironmentName: models.DefaultEnvironmentName}, {EnvironmentName: "other-env"},
			}, nil
		},
	}
	svc := &agentManagerService{agentThunderProvisioning: prov, logger: discardLogger()}
	views, err := svc.GetAgentIdentity(context.Background(), "ou", "p", "a")
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.Equal(t, models.DefaultEnvironmentName, views[0].EnvironmentName)

	errDB := errors.New("db down")
	prov.GetIdentityViewsFunc = func(context.Context, string, string, string) ([]models.AgentIdentityEnvironmentView, error) {
		return nil, errDB
	}
	_, err = svc.GetAgentIdentity(context.Background(), "ou", "p", "a")
	require.ErrorIs(t, err, errDB)

	none := &agentManagerService{logger: discardLogger()}
	views, err = none.GetAgentIdentity(context.Background(), "ou", "p", "a")
	require.NoError(t, err)
	assert.Empty(t, views)
}

// Without a provisioning implementation every identity operation reports the identity as not provisioned.
func TestIdentityOperationsWithoutProvisioning(t *testing.T) {
	ctx := context.Background()
	svc := &agentManagerService{logger: discardLogger()}
	_, err := svc.RegenerateAgentIdentitySecret(ctx, "ou", "p", "a", "default")
	require.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
	_, err = svc.RevokeAgentIdentitySecret(ctx, "ou", "p", "a", "default")
	require.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
	_, _, err = svc.ProvisionAgentIdentity(ctx, "ou", "p", "a", "default")
	require.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
	_, err = svc.RetryAgentIdentityProvisioning(ctx, "ou", "p", "a", "default")
	require.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
	_, err = svc.GetAgentRoles(ctx, "ou", "p", "a", "default")
	require.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
	_, err = svc.GetAgentGroups(ctx, "ou", "p", "a", "default")
	require.ErrorIs(t, err, utils.ErrAgentIdentityNotProvisioned)
}

func TestProvisionAgentIdentity_Failures(t *testing.T) {
	ctx := context.Background()
	agentOf := func(kind string) func(context.Context, string, string, string) (*models.AgentResponse, error) {
		return func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Provisioning: models.Provisioning{Type: kind}}, nil
		}
	}
	envOK := func(context.Context, string, string) (*models.EnvironmentResponse, error) {
		return &models.EnvironmentResponse{}, nil
	}

	t.Run("agent missing", func(t *testing.T) {
		svc := &agentManagerService{platform: &repomocks.PlatformRepositoryMock{
			GetAgentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
				return nil, utils.ErrAgentNotFound
			},
		}, agentThunderProvisioning: &stubAgentThunderProvisioning{}, logger: discardLogger()}
		_, _, err := svc.ProvisionAgentIdentity(ctx, "ou", "p", "a", "default")
		require.ErrorIs(t, err, utils.ErrAgentNotFound)
	})

	t.Run("unknown provisioning type", func(t *testing.T) {
		svc := &agentManagerService{
			platform:                 &repomocks.PlatformRepositoryMock{GetAgentFunc: agentOf("hosted")},
			agentThunderProvisioning: &stubAgentThunderProvisioning{}, logger: discardLogger(),
		}
		_, _, err := svc.ProvisionAgentIdentity(ctx, "ou", "p", "a", "default")
		require.ErrorIs(t, err, httpx.ErrInvalidInput)
	})

	t.Run("environment missing", func(t *testing.T) {
		svc := &agentManagerService{platform: &repomocks.PlatformRepositoryMock{
			GetAgentFunc: agentOf(string(utils.ExternalAgent)),
			GetEnvironmentFunc: func(context.Context, string, string) (*models.EnvironmentResponse, error) {
				return nil, utils.ErrEnvironmentNotFound
			},
		}, agentThunderProvisioning: &stubAgentThunderProvisioning{}, logger: discardLogger()}
		_, _, err := svc.ProvisionAgentIdentity(ctx, "ou", "p", "a", "nope")
		require.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
	})

	t.Run("provisioning fails", func(t *testing.T) {
		errThunder := errors.New("thunder down")
		svc := &agentManagerService{platform: &repomocks.PlatformRepositoryMock{
			GetAgentFunc: agentOf(string(utils.ExternalAgent)), GetEnvironmentFunc: envOK,
		}, agentThunderProvisioning: &stubAgentThunderProvisioning{
			ProvisionForEnvironmentIfMissingFunc: func(context.Context, string, string, string, string, models.AgentProvisioningType, string) (bool, error) {
				return false, errThunder
			},
		}, logger: discardLogger()}
		_, _, err := svc.ProvisionAgentIdentity(ctx, "ou", "p", "a", "default")
		require.ErrorIs(t, err, errThunder)
	})

	t.Run("binding not visible after provisioning", func(t *testing.T) {
		var by string
		svc := &agentManagerService{platform: &repomocks.PlatformRepositoryMock{
			GetAgentFunc: agentOf(string(utils.InternalAgent)), GetEnvironmentFunc: envOK,
		}, agentThunderProvisioning: &stubAgentThunderProvisioning{
			ProvisionForEnvironmentIfMissingFunc: func(_ context.Context, _, _, _, _ string, _ models.AgentProvisioningType, requestedBy string) (bool, error) {
				by = requestedBy
				return true, nil
			},
			GetIdentityViewsFunc: func(context.Context, string, string, string) ([]models.AgentIdentityEnvironmentView, error) {
				return []models.AgentIdentityEnvironmentView{{EnvironmentName: "elsewhere"}}, nil
			},
		}, logger: discardLogger()}
		_, existed, err := svc.ProvisionAgentIdentity(withCaller(ctx, "user-1"), "ou", "p", "a", "default")
		require.ErrorContains(t, err, "vanished")
		assert.True(t, existed)
		assert.Equal(t, "user-1", by, "the caller is recorded as the requester")
	})
}

func TestRetryAgentIdentityProvisioning_Delegates(t *testing.T) {
	prov := &lifecycleProvisioning{
		stubAgentThunderProvisioning: &stubAgentThunderProvisioning{},
		retry: func(ouID, project, agent, env string) (models.AgentIdentityEnvironmentView, error) {
			return models.AgentIdentityEnvironmentView{EnvironmentName: env, Status: models.AgentThunderStatusPending}, nil
		},
	}
	svc := &agentManagerService{agentThunderProvisioning: prov, logger: discardLogger()}
	view, err := svc.RetryAgentIdentityProvisioning(context.Background(), "ou", "p", "a", "default")
	require.NoError(t, err)
	assert.Equal(t, models.AgentThunderStatusPending, view.Status)
}

func TestGetProjectAgent_AgentInAnotherProjectIsAMiss(t *testing.T) {
	platform := &repomocks.PlatformRepositoryMock{
		GetAgentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Name: "a", ProjectName: "other"}, nil
		},
	}
	_, err := GetProjectAgent(context.Background(), platform, "ou", "p", "a")
	require.ErrorIs(t, err, utils.ErrAgentNotFound)

	got, err := GetProjectAgent(context.Background(), platform, "ou", "other", "a")
	require.NoError(t, err)
	assert.Equal(t, "a", got.Name)

	assert.True(t, IsLegacySecretRefPath("org/proj/env/agent"))
	assert.False(t, IsLegacySecretRefPath("agent-secrets"))
}

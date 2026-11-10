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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	cleanupOrg   = "acme"
	cleanupProj  = "proj1"
	cleanupAgent = "my-agent"
)

// recordingSecretClient records every delete, and the context it ran on, so a test can check
// both what was deleted and that a cancelled request did not cancel the delete.
type recordingSecretClient struct {
	mu          sync.Mutex
	byLocation  []secretmanagersvc.SecretLocation
	byName      []string
	ctxErrs     []error
	hadDeadline []bool
}

func (r *recordingSecretClient) record(ctx context.Context) {
	_, hasDeadline := ctx.Deadline()
	r.ctxErrs = append(r.ctxErrs, ctx.Err())
	r.hadDeadline = append(r.hadDeadline, hasDeadline)
}

// mock returns a SecretManagementClientMock that records into r and answers each delete with
// errFor (nil when errFor is nil).
func (r *recordingSecretClient) mock(errFor func(secretRefName string) error) *clientmocks.SecretManagementClientMock {
	answer := func(name string) error {
		if errFor == nil {
			return nil
		}
		return errFor(name)
	}
	return &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, location secretmanagersvc.SecretLocation, _ map[string]string) (string, error) {
			return location.SecretRefName(), nil
		},
		DeleteSecretFunc: func(ctx context.Context, location secretmanagersvc.SecretLocation, secretRefName string) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.byLocation = append(r.byLocation, location)
			r.record(ctx)
			return answer(secretRefName)
		},
		DeleteSecretByNameFunc: func(ctx context.Context, orgName, secretRefName string) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.byName = append(r.byName, orgName+"/"+secretRefName)
			r.record(ctx)
			return answer(secretRefName)
		},
	}
}

func (r *recordingSecretClient) deleteCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byLocation) + len(r.byName)
}

// expectedAgentSecretLocations is the configuration secret and the agent API key secret of
// cleanupAgent in each of envs.
func expectedAgentSecretLocations(envs ...string) []secretmanagersvc.SecretLocation {
	var locations []secretmanagersvc.SecretLocation
	for _, env := range envs {
		locations = append(
			locations,
			secretmanagersvc.SecretLocation{
				OrgName: cleanupOrg, ProjectName: cleanupProj, EnvironmentName: env, EntityName: cleanupAgent,
			},
			secretmanagersvc.SecretLocation{
				OrgName: cleanupOrg, ProjectName: cleanupProj, EnvironmentName: env, AgentName: cleanupAgent,
				EntityName: cleanupAgent + "-agent-api-key",
			},
		)
	}
	return locations
}

func pipelineClient(pipelineErr error, envs []*models.EnvironmentResponse, envsErr error) *clientmocks.OpenChoreoClientMock {
	return &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(context.Context, string, string) (*models.DeploymentPipelineResponse, error) {
			if pipelineErr != nil {
				return nil, pipelineErr
			}
			return threeEnvPipeline(), nil
		},
		ListEnvironmentsFunc: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
			return envs, envsErr
		},
	}
}

// TestDeleteAgentSecrets covers the leak where an agent's API key secrets, and the
// configuration secrets of an agent that never deployed, survived its deletion: both are
// deleted at their known location in every pipeline environment, through the secret API.
func TestDeleteAgentSecrets(t *testing.T) {
	t.Run("deletes the configuration and API key secret in every environment, then the other references", func(t *testing.T) {
		rec := &recordingSecretClient{}
		svc := &agentManagerService{
			ocClient:         pipelineClient(nil, nil, nil),
			secretMgmtClient: rec.mock(nil),
			logger:           discardLogger(),
		}
		devConfig := expectedAgentSecretLocations("dev")[0].SecretRefName()
		devAPIKey := expectedAgentSecretLocations("dev")[1].SecretRefName()

		svc.deleteAgentSecrets(context.Background(), cleanupOrg, cleanupProj, cleanupAgent,
			[]string{devConfig, devAPIKey, "llm-dev-my-agent-proxy-secrets", "", "llm-dev-my-agent-proxy-secrets"})

		assert.ElementsMatch(t, expectedAgentSecretLocations("dev", "staging", "prod"), rec.byLocation)
		assert.Equal(t, []string{cleanupOrg + "/llm-dev-my-agent-proxy-secrets"}, rec.byName,
			"a reference already deleted by location, an empty one and a duplicate are each skipped")
	})

	t.Run("a failed or not-found delete does not stop the rest", func(t *testing.T) {
		rec := &recordingSecretClient{}
		calls := 0
		svc := &agentManagerService{
			ocClient: pipelineClient(nil, nil, nil),
			secretMgmtClient: rec.mock(func(string) error {
				calls++
				switch calls % 3 {
				case 0:
					return secretmanagersvc.ErrSecretNotFound
				case 1:
					return errors.New("secret manager unavailable")
				default:
					return utils.ErrNotFound
				}
			}),
			logger: discardLogger(),
		}

		svc.deleteAgentSecrets(context.Background(), cleanupOrg, cleanupProj, cleanupAgent, []string{"extra-secrets"})

		assert.Len(t, rec.byLocation, 6)
		assert.Equal(t, []string{cleanupOrg + "/extra-secrets"}, rec.byName)
	})

	t.Run("falls back to the organization's environments when the pipeline cannot be read", func(t *testing.T) {
		rec := &recordingSecretClient{}
		svc := &agentManagerService{
			ocClient: pipelineClient(errors.New("pipeline unavailable"),
				[]*models.EnvironmentResponse{{Name: "dev"}, nil, {Name: ""}, {Name: "prod"}}, nil),
			secretMgmtClient: rec.mock(nil),
			logger:           discardLogger(),
		}

		svc.deleteAgentSecrets(context.Background(), cleanupOrg, cleanupProj, cleanupAgent, nil)

		assert.ElementsMatch(t, expectedAgentSecretLocations("dev", "prod"), rec.byLocation)
	})

	t.Run("still deletes the references when no environment can be listed", func(t *testing.T) {
		rec := &recordingSecretClient{}
		svc := &agentManagerService{
			ocClient:         pipelineClient(errors.New("pipeline unavailable"), nil, errors.New("environments unavailable")),
			secretMgmtClient: rec.mock(nil),
			logger:           discardLogger(),
		}

		svc.deleteAgentSecrets(context.Background(), cleanupOrg, cleanupProj, cleanupAgent, []string{"my-agent-dev-secrets"})

		assert.Empty(t, rec.byLocation)
		assert.Equal(t, []string{cleanupOrg + "/my-agent-dev-secrets"}, rec.byName)
	})

	t.Run("does nothing without a secret management client", func(t *testing.T) {
		// A nil ocClient would panic if the environments were looked up.
		svc := &agentManagerService{logger: discardLogger()}

		assert.NotPanics(t, func() {
			svc.deleteAgentSecrets(context.Background(), cleanupOrg, cleanupProj, cleanupAgent, []string{"x-secrets"})
		})
	})
}

// TestDeleteAgentSecretsDetached_OutlivesTheRequest: DeleteAgent hands the cleanup its request
// context, which the edge may cancel at any moment. The deletes must still run, on a context
// that is live but bounded, and must be done when the call returns: a client that recreates
// the agent after the delete returns would otherwise race them for the same secret names.
func TestDeleteAgentSecretsDetached_OutlivesTheRequest(t *testing.T) {
	rec := &recordingSecretClient{}
	svc := &agentManagerService{
		ocClient:         pipelineClient(nil, nil, nil),
		secretMgmtClient: rec.mock(nil),
		logger:           discardLogger(),
	}
	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()

	svc.deleteAgentSecretsDetached(requestCtx, cleanupOrg, cleanupProj, cleanupAgent, []string{"extra-secrets"})

	require.Equal(t, 7, rec.deleteCount(), "every delete is done before the call returns")
	for i, ctxErr := range rec.ctxErrs {
		require.NoError(t, ctxErr, "delete %d ran on a cancelled context", i)
		assert.True(t, rec.hadDeadline[i], "delete %d ran on an unbounded context", i)
	}
}

type ctxKey struct{}

// TestCreateRollbackContext pins the three properties the rollback relies on: it survives the
// request's cancellation, it keeps the request's values, and it is bounded.
func TestCreateRollbackContext(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.WithValue(context.Background(), ctxKey{}, "caller"))
	cancelParent()

	ctx, cancel := createRollbackContext(parent)
	defer cancel()

	require.NoError(t, ctx.Err())
	assert.Equal(t, "caller", ctx.Value(ctxKey{}))
	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	assert.WithinDuration(t, time.Now().Add(createRollbackTimeout), deadline, 5*time.Second)
}

// disconnectingCreateFixture is a create that gets as far as the component, with the
// caller's request context cancelled as soon as the component is created: the cloud edge
// cuts a request at ~15s, which is about when a create reaches this point.
func disconnectingCreateFixture(t *testing.T) (*agentManagerService, *recordingSecretClient, *[]error, context.Context) {
	t.Helper()
	svc, _ := entitlementFixture(nil)
	oc, ok := svc.ocClient.(*clientmocks.OpenChoreoClientMock)
	require.True(t, ok)

	requestCtx, cancel := context.WithCancel(tierGrantedCtx(t))
	t.Cleanup(cancel)
	oc.CreateComponentFunc = func(context.Context, string, string, client.CreateComponentRequest) error {
		cancel()
		return nil
	}
	var deleteCtxErrs []error
	oc.DeleteComponentFunc = func(ctx context.Context, _, _, _ string) error {
		deleteCtxErrs = append(deleteCtxErrs, ctx.Err())
		return nil
	}
	rec := &recordingSecretClient{}
	svc.secretMgmtClient = rec.mock(nil)
	return svc, rec, &deleteCtxErrs, requestCtx
}

// TestCreateAgentRollback_SurvivesCallerDisconnect: a create that fails after the caller has
// gone must still delete the component and the configuration secret it stored. On the
// request context both deletes were cancelled, leaving a half-created agent and its secrets.
func TestCreateAgentRollback_SurvivesCallerDisconnect(t *testing.T) {
	svc, rec, deleteCtxErrs, requestCtx := disconnectingCreateFixture(t)
	svc.a2aCardRepo = &repomocks.A2AAgentCardRepositoryMock{
		DeleteForAgentFunc: func(context.Context, string, string, string) error {
			return errors.New("database unavailable")
		},
	}
	secret := spec.EnvironmentVariable{Key: "OPENAI_API_KEY"}
	secret.SetValue("sk-test")
	secret.SetIsSensitive(true)
	req := createAgentRequest(utils.InternalAgent)
	req.Configurations = &spec.Configurations{Env: []spec.EnvironmentVariable{secret}}

	err := svc.createComponentAgent(requestCtx, cleanupOrg, cleanupProj, req, "")

	require.ErrorContains(t, err, "database unavailable", "the create fails at the A2A state cleanup, after the component")
	require.ErrorIs(t, requestCtx.Err(), context.Canceled, "the fixture cancels the request mid-create")
	assert.Equal(t, []error{nil}, *deleteCtxErrs, "the component is deleted on a live context")
	assert.Equal(t, []secretmanagersvc.SecretLocation{expectedAgentSecretLocations("dev")[0]}, rec.byLocation)
	assert.Equal(t, []error{nil}, rec.ctxErrs, "the configuration secret is deleted on a live context")
	assert.Equal(t, []bool{true}, rec.hadDeadline)
}

// TestCreateAgentRollback_DeletesAgentAPIKeySecret: the agent API key is stored while the
// traits are built, so a create that fails after that point must delete it too. Nothing
// referenced it once the component was gone, so it leaked on every such rollback.
func TestCreateAgentRollback_DeletesAgentAPIKeySecret(t *testing.T) {
	svc, rec, deleteCtxErrs, requestCtx := disconnectingCreateFixture(t)
	requestCtx = jwtassertion.ContextWithTokenClaimsAndScope(requestCtx, &jwtassertion.TokenClaims{OuId: cleanupOrg})
	svc.tokenManagerService = &stubTokenManager{generateFunc: func(context.Context, GenerateTokenRequest) (*spec.TokenResponse, error) {
		return &spec.TokenResponse{Token: "agent-jwt"}, nil
	}}
	svc.agentConfigurationService = &stubAgentConfigurationService{
		ListFunc: func(context.Context, string, string, string, int, int) (*models.AgentModelConfigListResponse, error) {
			return &models.AgentModelConfigListResponse{}, nil
		},
	}
	svc.aiApplicationService = newAIAppServiceForCleanup()
	oc, ok := svc.ocClient.(*clientmocks.OpenChoreoClientMock)
	require.True(t, ok)
	oc.AttachTraitsFunc = func(context.Context, string, string, string, []client.TraitRequest) error {
		return errors.New("trait attachment failed")
	}
	req := createAgentRequest(utils.InternalAgent)
	req.Build = &spec.Build{DockerBuild: &spec.DockerBuild{Type: "docker"}}

	err := svc.createComponentAgent(requestCtx, cleanupOrg, cleanupProj, req, "")

	require.ErrorContains(t, err, "trait attachment failed", "the create fails after the API key was stored")
	assert.Equal(t, []secretmanagersvc.SecretLocation{expectedAgentSecretLocations("dev")[1]}, rec.byLocation)
	assert.Equal(t, []error{nil}, rec.ctxErrs, "the API key secret is deleted on a live context")
	assert.Equal(t, []error{nil}, *deleteCtxErrs)
}

// interruptedCreateFixture is a create whose CreateComponent call fails with createErr, with
// existing (nil for none) as the component a later read finds under the agent's name.
func interruptedCreateFixture(t *testing.T, createErr error, existing *models.AgentResponse) (*agentManagerService, *[]string) {
	t.Helper()
	svc, _ := entitlementFixture(nil)
	oc, ok := svc.ocClient.(*clientmocks.OpenChoreoClientMock)
	require.True(t, ok)
	oc.CreateComponentFunc = func(context.Context, string, string, client.CreateComponentRequest) error {
		return createErr
	}
	oc.GetComponentFunc = func(ctx context.Context, _, _, _ string) (*models.AgentResponse, error) {
		require.NoError(t, ctx.Err(), "the component is looked up on a live context")
		if existing == nil {
			return nil, utils.ErrAgentNotFound
		}
		return existing, nil
	}
	var deleted []string
	oc.DeleteComponentFunc = func(ctx context.Context, _, _, name string) error {
		require.NoError(t, ctx.Err(), "the component is deleted on a live context")
		deleted = append(deleted, name)
		return nil
	}
	return svc, &deleted
}

// TestCreateAgent_InterruptedComponentCreate: when the request context ends during
// CreateComponent, the server may have created the component anyway, and nothing else would
// delete it. The context error also hides a conflict, so only a component created by this
// create is deleted; an existing agent's is left alone.
func TestCreateAgent_InterruptedComponentCreate(t *testing.T) {
	interrupted := fmt.Errorf("failed to create component: %w", context.DeadlineExceeded)

	t.Run("deletes the component the interrupted create made", func(t *testing.T) {
		svc, deleted := interruptedCreateFixture(t, interrupted, &models.AgentResponse{Name: cleanupAgent, CreatedAt: time.Now()})

		err := svc.createComponentAgent(tierGrantedCtx(t), cleanupOrg, cleanupProj, createAgentRequest(utils.InternalAgent), "")

		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, []string{"my-agent"}, *deleted)
	})

	t.Run("allows for a control plane clock running behind", func(t *testing.T) {
		svc, deleted := interruptedCreateFixture(t, interrupted,
			&models.AgentResponse{Name: cleanupAgent, CreatedAt: time.Now().Add(-componentCreateClockSkew / 2)})

		err := svc.createComponentAgent(tierGrantedCtx(t), cleanupOrg, cleanupProj, createAgentRequest(utils.InternalAgent), "")

		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, []string{"my-agent"}, *deleted)
	})

	t.Run("leaves an existing agent's component alone", func(t *testing.T) {
		svc, deleted := interruptedCreateFixture(t, fmt.Errorf("failed to create component: %w", context.Canceled),
			&models.AgentResponse{Name: cleanupAgent, CreatedAt: time.Now().Add(-time.Hour)})

		err := svc.createComponentAgent(tierGrantedCtx(t), cleanupOrg, cleanupProj, createAgentRequest(utils.InternalAgent), "")

		require.ErrorIs(t, err, context.Canceled)
		assert.Empty(t, *deleted)
	})

	t.Run("does nothing when no component was created", func(t *testing.T) {
		svc, deleted := interruptedCreateFixture(t, interrupted, nil)

		err := svc.createComponentAgent(tierGrantedCtx(t), cleanupOrg, cleanupProj, createAgentRequest(utils.InternalAgent), "")

		require.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Empty(t, *deleted)
	})

	t.Run("never deletes on a conflict", func(t *testing.T) {
		svc, deleted := interruptedCreateFixture(t, fmt.Errorf("%w: component exists", utils.ErrConflict),
			&models.AgentResponse{Name: cleanupAgent, CreatedAt: time.Now()})

		err := svc.createComponentAgent(tierGrantedCtx(t), cleanupOrg, cleanupProj, createAgentRequest(utils.InternalAgent), "")

		require.ErrorIs(t, err, utils.ErrConflict)
		assert.Empty(t, *deleted)
	})
}

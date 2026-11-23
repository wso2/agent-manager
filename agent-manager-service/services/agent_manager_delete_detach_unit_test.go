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
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// shrinkIdentityDeleteRetryDelay makes the identity cleanup retries in a test back off for a
// microsecond instead of seconds.
func shrinkIdentityDeleteRetryDelay(t *testing.T) {
	t.Helper()
	original := agentIdentityDeleteRetryDelay
	agentIdentityDeleteRetryDelay = time.Microsecond
	t.Cleanup(func() { agentIdentityDeleteRetryDelay = original })
}

type deletionAgentConfigRepoStub struct {
	repositories.AgentConfigRepository
}

func (deletionAgentConfigRepoStub) DeleteAllByAgent(context.Context, string, string, string) error {
	return nil
}

type deletionMonitorManagerStub struct{ MonitorManagerService }

func (deletionMonitorManagerStub) DeleteMonitorsByAgent(context.Context, string, string, string) error {
	return nil
}

// deletableAgentOC is an OpenChoreo client for an agent that exists until DeleteComponent runs.
func deletableAgentOC(deleteComponent func(context.Context) error) *clientmocks.OpenChoreoClientMock {
	return &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(context.Context, string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{}, nil
		},
		GetProjectFunc: func(context.Context, string, string) (*models.ProjectResponse, error) {
			return &models.ProjectResponse{}, nil
		},
		GetWorkloadSecretRefNamesFunc: func(context.Context, string, string, string) ([]string, error) { return []string{}, nil },
		GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return &models.AgentResponse{}, nil
		},
		DeleteComponentFunc: func(ctx context.Context, _, _, _ string) error { return deleteComponent(ctx) },
	}
}

// deletionManager wires an agentManagerService whose post-delete cleanup finds nothing to do.
func deletionManager(oc *clientmocks.OpenChoreoClientMock, provisioning AgentThunderProvisioningService) *agentManagerService {
	return &agentManagerService{
		ocClient:                 oc,
		agentKindService:         deletionAgentKindStub{},
		logger:                   discardLogger(),
		agentThunderProvisioning: provisioning,
		agentConfigRepo:          deletionAgentConfigRepoStub{},
		agentConfigurationService: &stubAgentConfigurationService{
			ListFunc: func(context.Context, string, string, string, int, int) (*models.AgentModelConfigListResponse, error) {
				return &models.AgentModelConfigListResponse{}, nil
			},
		},
		aiApplicationService: newAIAppServiceForCleanup(),
		artifactRepo: &repomocks.ArtifactRepositoryMock{
			ListByHandlePrefixFunc: func(context.Context, []string, string, string) ([]*models.Artifact, error) {
				return []*models.Artifact{}, nil
			},
		},
		monitorManagerService: deletionMonitorManagerStub{},
	}
}

// A caller that stops waiting (the synthetic monitors' 30s client, the gateway's upstream cut)
// used to cancel the AgentID cleanup while it queued behind a provisioning attempt on the
// identity lock, leaving the agent with nothing left to retry its deletion.
func TestDeleteAgentFinishesAfterTheCallerStopsWaiting(t *testing.T) {
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeExternal)
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider()).(*agentThunderProvisioningService)
	oc := deletableAgentOC(func(ctx context.Context) error {
		require.NoError(t, ctx.Err(), "the component delete must not inherit the caller's cancellation")
		f.events = append(f.events, "component-delete")
		return nil
	})
	manager := deletionManager(oc, svc)

	// A provisioning attempt holds the identity lock, as one does for every newly created agent.
	releaseAttempt, err := svc.lockAgentIdentity(context.Background(), "org-a", "project-a", "agent-a")
	require.NoError(t, err)

	// The caller is already gone by the time the delete reaches the lock.
	callerCtx, cancelCaller := context.WithCancel(auditableCtx(t))
	cancelCaller()

	done := make(chan error, 1)
	go func() {
		_, deleteErr := manager.DeleteAgent(callerCtx, "org-a", "project-a", "agent-a")
		done <- deleteErr
	}()

	select {
	case deleteErr := <-done:
		t.Fatalf("delete returned while the identity lock was still held: %v", deleteErr)
	case <-time.After(100 * time.Millisecond):
	}
	releaseAttempt()

	select {
	case deleteErr := <-done:
		require.NoError(t, deleteErr)
	case <-time.After(5 * time.Second):
		t.Fatal("delete did not finish after the identity lock was released")
	}
	require.ElementsMatch(t, []string{"development", "stage", "production"}, f.providerEnvs())
	require.Len(t, oc.DeleteComponentCalls(), 1)
	require.Contains(t, f.events, "db-delete", "the AgentID bindings must be removed once the cleanup succeeds")
}

// goneCaller is the request context of a caller that already stopped waiting.
func goneCaller(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// A caller still waiting gets the failure at once and can retry the deletion itself, as before.
func TestDeleteAgentDoesNotRetryWhileTheCallerIsWaiting(t *testing.T) {
	shrinkIdentityDeleteRetryDelay(t)
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeExternal)
	f.deleteErr["stage"] = errors.New("stage unavailable")
	oc := deletableAgentOC(func(context.Context) error {
		t.Fatal("component must remain when identity cleanup fails")
		return nil
	})
	manager := deletionManager(oc, f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider()))

	handled, _, err := manager.deleteAgentWithIdentityCleanup(auditableCtx(t), context.Background(), "org-a", "project-a", "agent-a")

	require.Error(t, err)
	require.True(t, handled)
	require.Equal(t, []string{"development", "stage"}, f.providerEnvs(), "only one attempt while the caller waits")
}

func TestDeleteAgentRetriesATransientIdentityCleanupFailure(t *testing.T) {
	shrinkIdentityDeleteRetryDelay(t)
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeExternal)
	stageFailures := 1
	provider := projectAgentIdentityDeletionProviderStub{
		deleteIdentity: func(_ context.Context, request ProjectAgentIdentityDeleteRequest) error {
			f.requests = append(f.requests, request)
			if request.Environment == "stage" && stageFailures > 0 {
				stageFailures--
				return errors.New("identity platform did not answer in time")
			}
			return nil
		},
	}
	oc := deletableAgentOC(func(context.Context) error { return nil })
	manager := deletionManager(oc, f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, provider))

	handled, outside, err := manager.deleteAgentWithIdentityCleanup(auditableCtx(t), goneCaller(t), "org-a", "project-a", "agent-a")

	require.NoError(t, err)
	require.True(t, handled)
	require.Empty(t, outside)
	// production waits on stage in the first attempt; the second runs the whole cleanup again.
	require.Equal(t, []string{"development", "stage", "development", "stage", "production"}, f.providerEnvs())
	require.Len(t, oc.DeleteComponentCalls(), 1, "the component is deleted only once the cleanup succeeds")
}

func TestDeleteAgentGivesUpAfterTheIdentityCleanupRetryBudget(t *testing.T) {
	shrinkIdentityDeleteRetryDelay(t)
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeExternal)
	f.deleteErr["stage"] = errors.New("stage unavailable")
	oc := deletableAgentOC(func(context.Context) error {
		t.Fatal("component must remain when identity cleanup fails")
		return nil
	})
	manager := deletionManager(oc, f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider()))

	handled, _, err := manager.deleteAgentWithIdentityCleanup(auditableCtx(t), goneCaller(t), "org-a", "project-a", "agent-a")

	require.Error(t, err)
	require.True(t, handled)
	stageAttempts := 0
	for _, env := range f.providerEnvs() {
		if env == "stage" {
			stageAttempts++
		}
	}
	require.Equal(t, agentIdentityDeleteAttempts, stageAttempts)
}

func TestDeleteAgentDoesNotRetryAnIdentityConflict(t *testing.T) {
	shrinkIdentityDeleteRetryDelay(t)
	f := newCPDeleteFixture(t, models.AgentProvisioningTypeExternal)
	// The source environment left the pipeline: the identity must be reconciled by hand.
	f.renameSeedEnvironment("development", "dev-retired")
	svc := f.service(f.failIfUsedResolver(), thundersvc.IdentityManagementCPManaged, f.deletionProvider()).(*agentThunderProvisioningService)
	oc := deletableAgentOC(func(context.Context) error {
		t.Fatal("component must remain when identity cleanup conflicts")
		return nil
	})
	manager := deletionManager(oc, svc)

	_, _, err := manager.deleteAgentWithIdentityCleanup(auditableCtx(t), goneCaller(t), "org-a", "project-a", "agent-a")

	require.ErrorIs(t, err, utils.ErrConflict)
	require.Len(t, svc.repo.(*repomocks.AgentThunderClientRepositoryMock).FindByAgentCalls(), 1, "a conflict must not be retried")
}

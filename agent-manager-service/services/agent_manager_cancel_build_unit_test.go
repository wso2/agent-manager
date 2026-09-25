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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	cancelBuildOuID    = "ou-1"
	cancelBuildProject = "proj-a"
	cancelBuildAgent   = "agent-a"
	cancelBuildName    = "agent-a-1700000000000"
)

// cancelBuildClient builds an OpenChoreoClient mock whose org/component lookups
// succeed and whose GetBuild reports the given status. CancelBuildFunc is left
// for the caller: nil means the test asserts that cancel is never reached.
func cancelBuildClient(buildStatus string) *clientmocks.OpenChoreoClientMock {
	return &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, _ string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: cancelBuildOuID}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				UUID:         "agent-uuid",
				Name:         cancelBuildAgent,
				Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
			}, nil
		},
		GetBuildFunc: func(_ context.Context, _, _, _, _ string) (*models.BuildDetailsResponse, error) {
			return &models.BuildDetailsResponse{
				BuildResponse: models.BuildResponse{Name: cancelBuildName, Status: buildStatus},
			}, nil
		},
	}
}

// newCancelBuildService builds the service with a cancelled-build repo that
// accepts writes and reports no history, which is what most cases need.
func newCancelBuildService(oc *clientmocks.OpenChoreoClientMock) *agentManagerService {
	return newCancelBuildServiceWithRepo(oc, &repomocks.CancelledBuildRepositoryMock{
		RecordFunc: func(_ context.Context, _ *models.CancelledBuild) error { return nil },
		ListByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.CancelledBuild, error) {
			return []models.CancelledBuild{}, nil
		},
	})
}

func newCancelBuildServiceWithRepo(
	oc *clientmocks.OpenChoreoClientMock, repo *repomocks.CancelledBuildRepositoryMock,
) *agentManagerService {
	return &agentManagerService{ocClient: oc, cancelledBuildRepo: repo, logger: discardLogger()}
}

func TestAgentManagerService_CancelBuild_CancelsRunningBuild(t *testing.T) {
	oc := cancelBuildClient("Running")
	var cancelled string
	oc.CancelBuildFunc = func(_ context.Context, _, _, _, buildName string) error {
		cancelled = buildName
		return nil
	}

	err := newCancelBuildService(oc).CancelBuild(
		auditableCtx(t), cancelBuildOuID, cancelBuildProject, cancelBuildAgent, cancelBuildName)

	require.NoError(t, err)
	assert.Equal(t, cancelBuildName, cancelled)
}

func TestAgentManagerService_CancelBuild_CancelsPendingBuild(t *testing.T) {
	oc := cancelBuildClient("Pending")
	oc.CancelBuildFunc = func(_ context.Context, _, _, _, _ string) error { return nil }

	err := newCancelBuildService(oc).CancelBuild(
		auditableCtx(t), cancelBuildOuID, cancelBuildProject, cancelBuildAgent, cancelBuildName)

	assert.NoError(t, err)
}

// A terminal build has nothing left to stop, and cancelling deletes the
// WorkflowRun — so the request must be refused before any delete is issued.
// CancelBuildFunc is deliberately nil: reaching it panics the test.
func TestAgentManagerService_CancelBuild_RejectsTerminalBuilds(t *testing.T) {
	for _, status := range []string{"Completed", "Succeeded", "Failed"} {
		t.Run(status, func(t *testing.T) {
			oc := cancelBuildClient(status)

			err := newCancelBuildService(oc).CancelBuild(
				auditableCtx(t), cancelBuildOuID, cancelBuildProject, cancelBuildAgent, cancelBuildName)

			assert.ErrorIs(t, err, utils.ErrBuildNotCancellable)
			assert.Empty(t, oc.CancelBuildCalls())
		})
	}
}

func TestAgentManagerService_CancelBuild_MapsMissingBuildToBuildNotFound(t *testing.T) {
	oc := cancelBuildClient("Running")
	oc.GetBuildFunc = func(_ context.Context, _, _, _, _ string) (*models.BuildDetailsResponse, error) {
		return nil, utils.ErrNotFound
	}

	err := newCancelBuildService(oc).CancelBuild(
		auditableCtx(t), cancelBuildOuID, cancelBuildProject, cancelBuildAgent, cancelBuildName)

	assert.ErrorIs(t, err, utils.ErrBuildNotFound)
	assert.Empty(t, oc.CancelBuildCalls())
}

// A transport failure from OpenChoreo must surface as itself, not be flattened
// into "build not found" — that would tell the caller to stop retrying.
func TestAgentManagerService_CancelBuild_DoesNotMaskRealErrorsAsNotFound(t *testing.T) {
	boom := errors.New("openchoreo unreachable")
	oc := cancelBuildClient("Running")
	oc.CancelBuildFunc = func(_ context.Context, _, _, _, _ string) error { return boom }

	err := newCancelBuildService(oc).CancelBuild(
		auditableCtx(t), cancelBuildOuID, cancelBuildProject, cancelBuildAgent, cancelBuildName)

	assert.ErrorIs(t, err, boom)
	assert.NotErrorIs(t, err, utils.ErrBuildNotFound)
	assert.NotErrorIs(t, err, utils.ErrBuildNotCancellable)
}

// Builds only exist for internally provisioned agents; an external agent must be
// refused before the build is ever looked up.
func TestAgentManagerService_CancelBuild_RejectsNonInternalAgents(t *testing.T) {
	oc := cancelBuildClient("Running")
	oc.GetComponentFunc = func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
		return &models.AgentResponse{
			UUID:         "agent-uuid",
			Name:         cancelBuildAgent,
			Provisioning: models.Provisioning{Type: "external"},
		}, nil
	}

	err := newCancelBuildService(oc).CancelBuild(
		auditableCtx(t), cancelBuildOuID, cancelBuildProject, cancelBuildAgent, cancelBuildName)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported for agent type")
	assert.Empty(t, oc.GetBuildCalls())
	assert.Empty(t, oc.CancelBuildCalls())
}

func TestAgentManagerService_CancelBuild_MapsMissingAgentToAgentNotFound(t *testing.T) {
	oc := cancelBuildClient("Running")
	oc.GetComponentFunc = func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
		return nil, utils.ErrNotFound
	}

	err := newCancelBuildService(oc).CancelBuild(
		auditableCtx(t), cancelBuildOuID, cancelBuildProject, cancelBuildAgent, cancelBuildName)

	assert.ErrorIs(t, err, utils.ErrAgentNotFound)
	assert.Empty(t, oc.CancelBuildCalls())
}

// --- cancelled_builds shim -------------------------------------------------

func TestAgentManagerService_CancelBuild_RecordsCancelledBuild(t *testing.T) {
	oc := cancelBuildClient("Running")
	oc.GetBuildFunc = func(_ context.Context, _, _, _, _ string) (*models.BuildDetailsResponse, error) {
		return &models.BuildDetailsResponse{
			BuildResponse: models.BuildResponse{
				UUID:            "run-uid",
				Name:            cancelBuildName,
				Status:          "Running",
				StartedAt:       time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC),
				BuildParameters: models.BuildParameters{Language: "python", Branch: "main"},
			},
		}, nil
	}
	oc.CancelBuildFunc = func(_ context.Context, _, _, _, _ string) error { return nil }

	var recorded *models.CancelledBuild
	repo := &repomocks.CancelledBuildRepositoryMock{
		RecordFunc: func(_ context.Context, rec *models.CancelledBuild) error {
			recorded = rec
			return nil
		},
	}

	err := newCancelBuildServiceWithRepo(oc, repo).CancelBuild(
		auditableCtx(t), cancelBuildOuID, cancelBuildProject, cancelBuildAgent, cancelBuildName)

	require.NoError(t, err)
	require.NotNil(t, recorded)
	assert.Equal(t, cancelBuildName, recorded.BuildName)
	assert.Equal(t, "run-uid", recorded.BuildUUID)
	// The status stored is the one the build held when cancelled, not "Cancelled".
	assert.Equal(t, "Running", recorded.StatusAtCancel)
	assert.Equal(t, "python", recorded.BuildParameters.Language)
	require.NotNil(t, recorded.StartedAt)
}

// The WorkflowRun is already deleted by the time the record is written, so a
// repo failure must not turn a cancel that happened into a reported failure.
func TestAgentManagerService_CancelBuild_SucceedsWhenRecordingFails(t *testing.T) {
	oc := cancelBuildClient("Running")
	oc.CancelBuildFunc = func(_ context.Context, _, _, _, _ string) error { return nil }
	repo := &repomocks.CancelledBuildRepositoryMock{
		RecordFunc: func(_ context.Context, _ *models.CancelledBuild) error {
			return errors.New("db down")
		},
	}

	err := newCancelBuildServiceWithRepo(oc, repo).CancelBuild(
		auditableCtx(t), cancelBuildOuID, cancelBuildProject, cancelBuildAgent, cancelBuildName)

	assert.NoError(t, err)
}

// Nothing is recorded when the cancel itself failed — the build is still there.
func TestAgentManagerService_CancelBuild_DoesNotRecordWhenCancelFails(t *testing.T) {
	oc := cancelBuildClient("Running")
	oc.CancelBuildFunc = func(_ context.Context, _, _, _, _ string) error {
		return errors.New("openchoreo unreachable")
	}
	repo := &repomocks.CancelledBuildRepositoryMock{} // RecordFunc nil → panics if called

	err := newCancelBuildServiceWithRepo(oc, repo).CancelBuild(
		auditableCtx(t), cancelBuildOuID, cancelBuildProject, cancelBuildAgent, cancelBuildName)

	require.Error(t, err)
	assert.Empty(t, repo.RecordCalls())
}

func TestAgentManagerService_MergeCancelledBuilds_AppendsCancelledBuilds(t *testing.T) {
	repo := &repomocks.CancelledBuildRepositoryMock{
		ListByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.CancelledBuild, error) {
			return []models.CancelledBuild{{BuildName: "build-cancelled", StatusAtCancel: "Running"}}, nil
		},
	}
	svc := newCancelBuildServiceWithRepo(&clientmocks.OpenChoreoClientMock{}, repo)

	merged := svc.mergeCancelledBuilds(context.Background(), cancelBuildOuID, cancelBuildProject, cancelBuildAgent,
		[]*models.BuildResponse{{Name: "build-live", Status: "Completed"}})

	require.Len(t, merged, 2)
	assert.Equal(t, "build-cancelled", merged[1].Name)
	// The merged entry reports Cancelled, not the status held at cancel time.
	assert.Equal(t, models.BuildStatusCancelled, merged[1].Status)
}

// If the WorkflowRun is somehow still live, OpenChoreo is authoritative and the
// build must not appear twice.
func TestAgentManagerService_MergeCancelledBuilds_SkipsStillLiveBuilds(t *testing.T) {
	repo := &repomocks.CancelledBuildRepositoryMock{
		ListByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.CancelledBuild, error) {
			return []models.CancelledBuild{{BuildName: "build-1", StatusAtCancel: "Running"}}, nil
		},
	}
	svc := newCancelBuildServiceWithRepo(&clientmocks.OpenChoreoClientMock{}, repo)

	merged := svc.mergeCancelledBuilds(context.Background(), cancelBuildOuID, cancelBuildProject, cancelBuildAgent,
		[]*models.BuildResponse{{Name: "build-1", Status: "Running"}})

	require.Len(t, merged, 1)
	assert.Equal(t, "Running", merged[0].Status)
}

// A history read failure degrades to live builds only rather than failing a
// listing OpenChoreo answered fine.
func TestAgentManagerService_MergeCancelledBuilds_DegradesOnRepoError(t *testing.T) {
	repo := &repomocks.CancelledBuildRepositoryMock{
		ListByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.CancelledBuild, error) {
			return nil, errors.New("db down")
		},
	}
	svc := newCancelBuildServiceWithRepo(&clientmocks.OpenChoreoClientMock{}, repo)
	live := []*models.BuildResponse{{Name: "build-live"}}

	merged := svc.mergeCancelledBuilds(context.Background(), cancelBuildOuID, cancelBuildProject, cancelBuildAgent, live)

	assert.Equal(t, live, merged)
}

// Opening a cancelled build from history must not 404 just because its
// WorkflowRun is gone.
func TestAgentManagerService_GetBuild_FallsBackToCancelledRecord(t *testing.T) {
	oc := cancelBuildClient("Running")
	oc.GetBuildFunc = func(_ context.Context, _, _, _, _ string) (*models.BuildDetailsResponse, error) {
		return nil, utils.ErrNotFound
	}
	repo := &repomocks.CancelledBuildRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.CancelledBuild, error) {
			return &models.CancelledBuild{BuildName: cancelBuildName, StatusAtCancel: "Running"}, nil
		},
	}

	build, err := newCancelBuildServiceWithRepo(oc, repo).GetBuild(
		context.Background(), cancelBuildOuID, cancelBuildProject, cancelBuildAgent, cancelBuildName)

	require.NoError(t, err)
	require.NotNil(t, build)
	assert.Equal(t, models.BuildStatusCancelled, build.Status)
}

// A build that was never cancelled and is not in OpenChoreo is genuinely gone.
func TestAgentManagerService_GetBuild_ReportsNotFoundWhenNeverCancelled(t *testing.T) {
	oc := cancelBuildClient("Running")
	oc.GetBuildFunc = func(_ context.Context, _, _, _, _ string) (*models.BuildDetailsResponse, error) {
		return nil, utils.ErrNotFound
	}
	repo := &repomocks.CancelledBuildRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, _ string) (*models.CancelledBuild, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}

	_, err := newCancelBuildServiceWithRepo(oc, repo).GetBuild(
		context.Background(), cancelBuildOuID, cancelBuildProject, cancelBuildAgent, cancelBuildName)

	assert.ErrorIs(t, err, utils.ErrBuildNotFound)
}

// Only a not-found is worth a history lookup; a transport failure is reported
// as itself rather than being answered from the shim.
func TestAgentManagerService_GetBuild_DoesNotFallBackOnTransportError(t *testing.T) {
	boom := errors.New("openchoreo unreachable")
	oc := cancelBuildClient("Running")
	oc.GetBuildFunc = func(_ context.Context, _, _, _, _ string) (*models.BuildDetailsResponse, error) {
		return nil, boom
	}
	repo := &repomocks.CancelledBuildRepositoryMock{} // GetFunc nil → panics if consulted

	_, err := newCancelBuildServiceWithRepo(oc, repo).GetBuild(
		context.Background(), cancelBuildOuID, cancelBuildProject, cancelBuildAgent, cancelBuildName)

	assert.ErrorIs(t, err, boom)
	assert.Empty(t, repo.GetCalls())
}

// A cancelled build belongs in the history at the point it actually ran, not
// pinned to the end of the list where pagination would strand it.
func TestAgentManagerService_MergeCancelledBuilds_SortsNewestFirst(t *testing.T) {
	cancelledStart := time.Date(2026, 9, 21, 11, 0, 0, 0, time.UTC)
	repo := &repomocks.CancelledBuildRepositoryMock{
		ListByAgentFunc: func(_ context.Context, _, _, _ string) ([]models.CancelledBuild, error) {
			return []models.CancelledBuild{{
				BuildName:      "build-middle",
				StatusAtCancel: "Running",
				StartedAt:      &cancelledStart,
			}}, nil
		},
	}
	svc := newCancelBuildServiceWithRepo(&clientmocks.OpenChoreoClientMock{}, repo)

	merged := svc.mergeCancelledBuilds(context.Background(), cancelBuildOuID, cancelBuildProject, cancelBuildAgent,
		[]*models.BuildResponse{
			{Name: "build-newest", StartedAt: time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)},
			{Name: "build-oldest", StartedAt: time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)},
		})

	require.Len(t, merged, 3)
	assert.Equal(t, []string{"build-newest", "build-middle", "build-oldest"},
		[]string{merged[0].Name, merged[1].Name, merged[2].Name})
}

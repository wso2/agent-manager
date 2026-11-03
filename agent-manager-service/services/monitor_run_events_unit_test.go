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

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

type runEventPublisher func(context.Context, events.Event)

func (p runEventPublisher) Publish(ctx context.Context, e events.Event) { p(ctx, e) }

func TestMonitorRunEventRetriesScoreReadBeforeTerminalPersistence(t *testing.T) {
	scoreError := errors.New("scores temporarily unavailable")
	persisted := false
	published := false
	monitor := &models.Monitor{ID: uuid.New(), OUID: "org"}
	run := &models.MonitorRun{ID: uuid.New(), MonitorID: monitor.ID, Status: models.RunStatusRunning}
	scoreRepo := &repomocks.ScoreRepositoryMock{
		GetEvaluatorsByMonitorAndRunIDFunc: func(uuid.UUID, uuid.UUID) ([]models.MonitorRunEvaluator, error) {
			if scoreError != nil {
				return nil, scoreError
			}
			return []models.MonitorRunEvaluator{}, nil
		},
	}
	repo := &repomocks.MonitorRepositoryMock{
		GetMonitorByIDFunc: func(uuid.UUID) (*models.Monitor, error) { return monitor, nil },
		UpdateMonitorRunFunc: func(_ *models.MonitorRun, updates map[string]interface{}) error {
			persisted = true
			require.Equal(t, models.RunStatusSuccess, updates["status"])
			return nil
		},
	}
	oc := &clientmocks.OpenChoreoClientMock{
		GetWorkflowRunFunc: func(context.Context, string, string) (*client.WorkflowRunResponse, error) {
			return &client.WorkflowRunResponse{Status: "Succeeded"}, nil
		},
	}
	scheduler := newScheduler(oc, &fakeProvisioner{IsThunderModeFunc: func() bool { return false }}, &fakeMonitorExecutor{}, repo)
	scheduler.runObserver = NewMonitorRunEvents(runEventPublisher(func(_ context.Context, e events.Event) {
		require.True(t, persisted, "must publish only after status is persisted")
		require.Contains(t, e.Data, "scores", "empty scores are valid for a no-trace run")
		require.NotNil(t, e.Data.(map[string]any)["run"].(map[string]any)["completedAt"])
		published = true
	}), scoreRepo, discardLogger())
	require.ErrorIs(t, scheduler.syncSingleRunStatus(context.Background(), run), scoreError)
	require.False(t, persisted)
	require.False(t, published)
	require.Equal(t, models.RunStatusRunning, run.Status)
	scoreError = nil
	require.NoError(t, scheduler.syncSingleRunStatus(context.Background(), run))
	require.True(t, published)
}

func TestMonitorRunEventDoesNotPublishOnStatusWriteFailure(t *testing.T) {
	writeError := errors.New("write failed")
	monitor := &models.Monitor{ID: uuid.New(), OUID: "org"}
	run := &models.MonitorRun{ID: uuid.New(), MonitorID: monitor.ID}
	repo := &repomocks.MonitorRepositoryMock{
		GetMonitorByIDFunc:   func(uuid.UUID) (*models.Monitor, error) { return monitor, nil },
		UpdateMonitorRunFunc: func(*models.MonitorRun, map[string]interface{}) error { return writeError },
	}
	oc := &clientmocks.OpenChoreoClientMock{
		GetWorkflowRunFunc: func(context.Context, string, string) (*client.WorkflowRunResponse, error) {
			return &client.WorkflowRunResponse{Status: "Failed"}, nil
		},
	}
	scheduler := newScheduler(oc, &fakeProvisioner{IsThunderModeFunc: func() bool { return false }}, &fakeMonitorExecutor{}, repo)
	scheduler.runObserver = NewMonitorRunEvents(runEventPublisher(func(context.Context, events.Event) {
		t.Fatal("must not publish when terminal persistence fails")
	}), &repomocks.ScoreRepositoryMock{}, discardLogger())
	require.ErrorIs(t, scheduler.syncSingleRunStatus(context.Background(), run), writeError)
}

func TestDisabledMonitorEventsDoNotQueryScores(t *testing.T) {
	observer := NewMonitorRunEvents(events.NoopPublisher{}, &repomocks.ScoreRepositoryMock{}, discardLogger())
	publish, err := observer.PrepareMonitorRunFinished(context.Background(), &models.Monitor{}, &models.MonitorRun{}, models.RunStatusSuccess, "")
	require.NoError(t, err)
	publish()
}

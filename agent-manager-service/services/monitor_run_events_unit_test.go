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

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

type runEventPublisher func(context.Context, events.Event) error

func (p runEventPublisher) Publish(ctx context.Context, e events.Event) error { return p(ctx, e) }

func TestMonitorRunEventScoreFailureDoesNotBlockTerminalPersistence(t *testing.T) {
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
		ListUnqueuedTerminalRunsFunc: func(context.Context, int) ([]models.MonitorRun, error) { return []models.MonitorRun{*run}, nil },
		GetMonitorByIDFunc:           func(uuid.UUID) (*models.Monitor, error) { return monitor, nil },
		MarkRunEventQueuedFunc:       func(context.Context, uuid.UUID) error { return nil },
		UpdateMonitorRunFunc: func(_ *models.MonitorRun, updates map[string]interface{}) error {
			persisted = true
			run.Status = models.RunStatusSuccess
			at := updates["completed_at"].(time.Time)
			run.CompletedAt = &at
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
	scheduler.runObserver = NewMonitorRunEvents(runEventPublisher(func(_ context.Context, e events.Event) error {
		require.True(t, persisted, "must publish only after status is persisted")
		require.Contains(t, e.Data, "scores", "empty scores are valid for a no-trace run")
		require.NotNil(t, e.Data.(map[string]any)["run"].(map[string]any)["completedAt"])
		published = true
		return nil
	}), scoreRepo, repo, discardLogger())
	require.NoError(t, scheduler.syncSingleRunStatus(context.Background(), run))
	require.True(t, persisted)
	require.False(t, published)
	require.Equal(t, models.RunStatusSuccess, run.Status)
	scoreError = nil
	require.NoError(t, scheduler.runObserver.(*monitorRunEvents).Reconcile(context.Background()))
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
	scheduler.runObserver = NewMonitorRunEvents(runEventPublisher(func(context.Context, events.Event) error {
		t.Fatal("must not publish when terminal persistence fails")
		return nil
	}), &repomocks.ScoreRepositoryMock{}, &repomocks.MonitorRepositoryMock{}, discardLogger())
	require.ErrorIs(t, scheduler.syncSingleRunStatus(context.Background(), run), writeError)
}

func TestDisabledMonitorEventsDoNotQueryScores(t *testing.T) {
	observer := NewMonitorRunEvents(events.NoopPublisher{}, &repomocks.ScoreRepositoryMock{}, &repomocks.MonitorRepositoryMock{}, discardLogger())
	publish, err := observer.PrepareMonitorRunFinished(context.Background(), &models.Monitor{}, &models.MonitorRun{}, models.RunStatusSuccess, "")
	require.NoError(t, err)
	publish()
}

func TestMonitorRunEventsReplayAfterScoreReadFailure(t *testing.T) {
	ctx := context.Background()
	at := time.Now().Add(-time.Hour)
	monitor := &models.Monitor{ID: uuid.New(), OUID: "ou-1", ProjectName: "p", AgentName: "a", Name: "quality"}
	run := models.MonitorRun{ID: uuid.New(), MonitorID: monitor.ID, Status: models.RunStatusSuccess, CompletedAt: &at}
	unavailable := true
	marked := false
	var published []events.Event
	scores := &repomocks.ScoreRepositoryMock{
		GetEvaluatorsByMonitorAndRunIDFunc: func(uuid.UUID, uuid.UUID) ([]models.MonitorRunEvaluator, error) {
			if unavailable {
				return nil, errors.New("temporary DB failure")
			}
			return []models.MonitorRunEvaluator{{EvaluatorName: "quality", Aggregations: map[string]interface{}{"mean": 0.2}}}, nil
		},
	}
	repo := &repomocks.MonitorRepositoryMock{
		ListUnqueuedTerminalRunsFunc: func(context.Context, int) ([]models.MonitorRun, error) { return []models.MonitorRun{run}, nil },
		GetMonitorByIDFunc:           func(uuid.UUID) (*models.Monitor, error) { return monitor, nil },
		MarkRunEventQueuedFunc:       func(context.Context, uuid.UUID) error { marked = true; return nil },
	}
	pub := runEventPublisher(func(_ context.Context, e events.Event) error { published = append(published, e); return nil })
	observer := NewMonitorRunEvents(pub, scores, repo, discardLogger()).(*monitorRunEvents)
	_, err := observer.PrepareMonitorRunFinished(ctx, monitor, &run, run.Status, "")
	require.Error(t, err)
	assert.Empty(t, published)
	assert.False(t, marked)
	// A later scheduler tick, including after a restart, reads the terminal source.
	unavailable = false
	require.NoError(t, observer.Reconcile(ctx))
	require.Len(t, published, 1)
	assert.Equal(t, "evt_monrun_"+run.ID.String(), published[0].ID)
	assert.True(t, published[0].Time.Equal(at), "replay keeps completion time for threshold ordering")
	assert.True(t, marked)
	data := published[0].Data.(map[string]any)
	assert.Len(t, data["scores"], 1)
}

func TestMonitorRunEventsReplayContinuesPastAStuckRun(t *testing.T) {
	ctx := context.Background()
	monitor := &models.Monitor{ID: uuid.New(), OUID: "ou-1", ProjectName: "p", AgentName: "a", Name: "quality"}
	stuck := models.MonitorRun{ID: uuid.New(), MonitorID: monitor.ID, Status: models.RunStatusSuccess}
	next := models.MonitorRun{ID: uuid.New(), MonitorID: monitor.ID, Status: models.RunStatusSuccess}
	scores := &repomocks.ScoreRepositoryMock{
		GetEvaluatorsByMonitorAndRunIDFunc: func(_ uuid.UUID, runID uuid.UUID) ([]models.MonitorRunEvaluator, error) {
			if runID == stuck.ID {
				return nil, errors.New("temporary DB failure")
			}
			return []models.MonitorRunEvaluator{}, nil
		},
	}
	var marked []uuid.UUID
	repo := &repomocks.MonitorRepositoryMock{
		ListUnqueuedTerminalRunsFunc: func(context.Context, int) ([]models.MonitorRun, error) {
			return []models.MonitorRun{stuck, next}, nil
		},
		GetMonitorByIDFunc:     func(uuid.UUID) (*models.Monitor, error) { return monitor, nil },
		MarkRunEventQueuedFunc: func(_ context.Context, id uuid.UUID) error { marked = append(marked, id); return nil },
	}
	var published []events.Event
	pub := runEventPublisher(func(_ context.Context, e events.Event) error { published = append(published, e); return nil })
	observer := NewMonitorRunEvents(pub, scores, repo, discardLogger()).(*monitorRunEvents)

	err := observer.Reconcile(ctx)
	require.Error(t, err, "the stuck run is reported")
	assert.Contains(t, err.Error(), stuck.ID.String())
	require.Len(t, published, 1, "the run after it is still replayed")
	assert.Equal(t, "evt_monrun_"+next.ID.String(), published[0].ID)
	assert.Equal(t, []uuid.UUID{next.ID}, marked, "the stuck run stays unqueued for the next poll")
}

func TestMonitorRunEventsDoesNotMarkRejectedPublication(t *testing.T) {
	repo := &repomocks.MonitorRepositoryMock{} // marking would panic
	pub := runEventPublisher(func(context.Context, events.Event) error { return errors.New("outbox unavailable") })
	observer := NewMonitorRunEvents(pub, &repomocks.ScoreRepositoryMock{}, repo, discardLogger())
	publish, err := observer.PrepareMonitorRunFinished(context.Background(), &models.Monitor{}, &models.MonitorRun{ID: uuid.New()}, models.RunStatusFailed, "failed")
	require.NoError(t, err)
	publish()
}

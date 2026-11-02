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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

type runEventPublisher struct {
	publish func(context.Context, events.Event) error
}

func (p runEventPublisher) Publish(ctx context.Context, e events.Event) error {
	return p.publish(ctx, e)
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
	pub := runEventPublisher{publish: func(_ context.Context, e events.Event) error { published = append(published, e); return nil }}
	observer := NewMonitorRunEvents(pub, scores, repo, discardLogger()).(*monitorRunEvents)
	observer.OnMonitorRunFinished(ctx, monitor, &run, run.Status, "")
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

func TestMonitorRunEventsDoesNotMarkRejectedPublication(t *testing.T) {
	repo := &repomocks.MonitorRepositoryMock{} // marking would panic
	pub := runEventPublisher{publish: func(context.Context, events.Event) error { return errors.New("outbox unavailable") }}
	observer := NewMonitorRunEvents(pub, &repomocks.ScoreRepositoryMock{}, repo, discardLogger())
	observer.OnMonitorRunFinished(context.Background(), &models.Monitor{}, &models.MonitorRun{ID: uuid.New()}, models.RunStatusFailed, "failed")
}

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

package reconcile

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo/gwrepomocks"
)

// syncBuffer is a log sink the worker's goroutines may write while the test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// failingPlacementKind cannot list its unplaced records.
type failingPlacementKind struct{ stubKind }

func (failingPlacementKind) UnplacedRecords(context.Context, string, *uuid.UUID, string, int) ([]Record, error) {
	return nil, errors.New("listing failed")
}

func (failingPlacementKind) Targets(context.Context, Record) (uuid.UUID, bool, error) {
	return uuid.Nil, false, nil
}

func (failingPlacementKind) PlacedOn(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) (bool, error) {
	return false, nil
}

func newLoopWorker(t *testing.T, kinds Kinds) (*Worker, *syncBuffer) {
	t.Helper()
	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	core := NewCore(nil, nil, nil, kinds, Config{
		RenderInterval: time.Hour, PlaceInterval: time.Hour, DriftInterval: time.Hour,
	}, logger)
	return NewWorker(core, logger), logs
}

// A placement pass that fails, fleet-wide or for a newly mapped environment, is logged and the worker keeps running.
// Only the placement loops run: the core has no database, which the drift loop would reach on its next interval
// boundary.
func TestWorkerLogsAFailedPlacementPass(t *testing.T) {
	worker, logs := newLoopWorker(t, Kinds{failingPlacementKind{}})
	worker.placeAll(context.Background())
	worker.done.Add(1)
	go worker.placeRequested(context.Background())
	t.Cleanup(func() { _ = worker.Stop() })

	worker.RequestPlaceEnv("ou-1", uuid.New())
	require.Eventually(t, func() bool {
		out := logs.String()
		return strings.Contains(out, "Placement pass failed") &&
			strings.Contains(out, "Failed to place gateway artifacts in a newly mapped environment")
	}, 5*time.Second, 10*time.Millisecond, logs.String())
}

// Requested placements past the queue are dropped with a warning; the slow placement pass covers them.
func TestRequestPlaceEnvDropsWhenTheQueueIsFull(t *testing.T) {
	worker, logs := newLoopWorker(t, Kinds{stubKind{}})
	for range placeQueue {
		worker.RequestPlaceEnv("ou-1", uuid.New())
	}
	assert.NotContains(t, logs.String(), "Placement queue is full")
	worker.RequestPlaceEnv("ou-1", uuid.New())
	assert.Contains(t, logs.String(), "Placement queue is full")
}

// The loop runs on its interval and on a wake-up, and stops when the worker stops. Nudges while one is pending
// coalesce into one run.
func TestLoopRunsOnTickAndOnWake(t *testing.T) {
	worker, _ := newLoopWorker(t, Kinds{stubKind{}})
	var runs atomic.Int32
	run := func(context.Context) { runs.Add(1) }

	worker.done.Add(1)
	go worker.loop(context.Background(), time.Hour, worker.nudgeCh, run)
	worker.core.Nudge()
	worker.Nudge()
	require.Eventually(t, func() bool { return runs.Load() >= 1 }, 5*time.Second, 5*time.Millisecond, "a nudge runs it")
	require.NoError(t, worker.Stop())

	ticked, _ := newLoopWorker(t, Kinds{stubKind{}})
	var ticks atomic.Int32
	ticked.done.Add(1)
	go ticked.loop(context.Background(), 5*time.Millisecond, nil, func(context.Context) { ticks.Add(1) })
	require.Eventually(t, func() bool { return ticks.Load() >= 2 }, 5*time.Second, 5*time.Millisecond, "the interval runs it")
	require.NoError(t, ticked.Stop())

	ctx, cancel := context.WithCancel(context.Background())
	cancelled, _ := newLoopWorker(t, Kinds{stubKind{}})
	cancelled.done.Add(1)
	go cancelled.loop(ctx, time.Hour, nil, run)
	cancel()
	cancelled.done.Wait()
}

// A render pass that cannot claim its due rows is logged, and one whose write-back fails is logged per row. A row of
// a kind no reconciler handles is left waiting for later, not rendered.
func TestRunOnceLogsClaimAndWriteBackFailures(t *testing.T) {
	logs := &syncBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	repo := &gwrepomocks.DeploymentRepositoryMock{
		ClaimDueRendersFunc: func(context.Context, time.Time, time.Duration, int) ([]gwmodels.ClaimedRender, error) {
			return nil, errors.New("claim failed")
		},
	}
	core := NewCore(nil, repo, nil, Kinds{stubKind{}}, Config{Lease: time.Minute}, logger)
	worker := NewWorker(core, logger)

	worker.RunOnce(context.Background())
	assert.Contains(t, logs.String(), "Failed to claim due renders")

	var outcome gwrepo.RenderOutcome
	repo.ClaimDueRendersFunc = func(context.Context, time.Time, time.Duration, int) ([]gwmodels.ClaimedRender, error) {
		return []gwmodels.ClaimedRender{{
			Placement: gwmodels.Placement{
				ArtifactRef: gwmodels.ArtifactRef{ArtifactUUID: uuid.New(), Kind: "NoSuchKind"},
				GatewayUUID: uuid.New(),
			},
			ClaimedAt: time.Now(),
		}}, nil
	}
	repo.RecordRenderOutcomeFunc = func(_ context.Context, _ gwmodels.ClaimedRender, o gwrepo.RenderOutcome) error {
		outcome = o
		return errors.New("write-back failed")
	}

	worker.RunOnce(context.Background())
	assert.Equal(t, gwrepo.RenderWaiting, outcome.Outcome, "an unregistered kind's render waits")
	assert.Contains(t, outcome.Reason, `no reconciler for artifact kind "NoSuchKind"`)
	require.NotNil(t, outcome.NextAttemptAt)
	assert.Contains(t, logs.String(), "Failed to write back a render")
}

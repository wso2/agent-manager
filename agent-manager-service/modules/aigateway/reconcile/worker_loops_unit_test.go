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

// A placement pass that fails, at start or for a newly mapped environment, is logged and the worker keeps running.
func TestWorkerLogsAFailedPlacementPass(t *testing.T) {
	worker, logs := newLoopWorker(t, Kinds{failingPlacementKind{}})
	require.NoError(t, worker.Start(context.Background()))
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

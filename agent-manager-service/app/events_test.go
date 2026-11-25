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

package app

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

type startupBus struct {
	events.Bus
	setup func(context.Context) error
}

func (b startupBus) Setup(ctx context.Context) error { return b.setup(ctx) }

type startupComponent struct{ start func(context.Context) error }

func (s startupComponent) Start(ctx context.Context) error { return s.start(ctx) }
func (s startupComponent) Stop() error                     { return nil }

func TestEventsStartupRetriesConsumerFailures(t *testing.T) {
	var setup, dispatch atomic.Int32
	deps := &wiring.AppParams{
		EventBus: startupBus{setup: func(context.Context) error { setup.Add(1); return nil }},
		EventDispatcher: startupComponent{start: func(ctx context.Context) error {
			require.NoError(t, ctx.Err())
			if dispatch.Add(1) == 1 {
				return errors.New("transient")
			}
			return nil
		}},
		EventPublisher: events.NoopPublisher{},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	select {
	case <-startEvents(ctx, deps):
	case <-ctx.Done():
		t.Fatal("startup did not retry")
	}
	assert.EqualValues(t, 1, setup.Load())
	assert.EqualValues(t, 2, dispatch.Load())
}

func TestEventsStartupCancellationStopsRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	entered := make(chan struct{})
	deps := &wiring.AppParams{
		EventBus:        startupBus{setup: func(context.Context) error { close(entered); return errors.New("unavailable") }},
		EventDispatcher: startupComponent{start: func(context.Context) error { t.Error("started after cancellation"); return nil }},
	}
	done := startEvents(ctx, deps)
	<-entered
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("startup ignored cancellation")
	}
}

type closingBus struct {
	events.Bus
	closed *atomic.Bool
}

func (b closingBus) Close() { b.closed.Store(true) }

type stoppingDispatcher struct {
	startupComponent
	stopped *atomic.Bool
	err     error
}

func (d stoppingDispatcher) Stop() error { d.stopped.Store(true); return d.err }

// Shutdown stops the dispatcher and closes the bus, even when the dispatcher
// reports an error stopping; with events off there is nothing to stop.
func TestStopEvents(t *testing.T) {
	stopEvents(&wiring.AppParams{})

	for _, stopErr := range []error{nil, errors.New("stuck")} {
		var stopped, closed atomic.Bool
		stopEvents(&wiring.AppParams{
			EventBus:        closingBus{closed: &closed},
			EventDispatcher: stoppingDispatcher{stopped: &stopped, err: stopErr},
			EventPublisher:  events.NoopPublisher{},
		})
		assert.True(t, stopped.Load())
		assert.True(t, closed.Load(), "the bus is closed even if the dispatcher failed to stop")
	}
}

// The configured log level decides what is written; an unknown one falls back
// to INFO rather than silencing or flooding the log.
func TestSetupLoggerLevels(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	ctx := context.Background()
	for level, want := range map[string]slog.Level{
		"DEBUG": slog.LevelDebug, "INFO": slog.LevelInfo, "WARN": slog.LevelWarn, "ERROR": slog.LevelError, "loud": slog.LevelInfo,
	} {
		setupLogger(&config.Config{LogLevel: level})
		assert.True(t, slog.Default().Enabled(ctx, want), level)
		assert.False(t, slog.Default().Enabled(ctx, want-1), level)
	}
}

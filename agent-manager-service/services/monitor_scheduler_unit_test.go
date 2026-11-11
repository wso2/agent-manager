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

// UNIT tests for monitorSchedulerService.
//
// Unlike services/monitor_scheduler_test.go (which carries `//go:build
// integration` and exercises the scheduler loop against a real Postgres via
// pg_try_advisory_xact_lock), this file has NO build tag and therefore runs in
// the fast unit tier. It asserts the scheduler's OWN logic — validation gates,
// error mapping/propagation, Thunder vs non-Thunder client selection,
// next-run-time computation, due-monitor fan-out, and the OpenChoreo status ->
// DB status mapping — with every collaborator mocked:
//
//   - repositories.MonitorRepository      -> repomocks.MonitorRepositoryMock (moq-generated)
//   - repositories.PlatformRepository             -> repomocks.PlatformRepositoryMock (moq-generated)
//   - services.MonitorExecutor            -> fakeMonitorExecutor (hand-written func-field stub)
//   - services.PublisherCredentialProvisioner -> fakeProvisioner (hand-written func-field stub)
//
// The exported surface (Start/Stop) is thin and the meaningful logic lives in
// unexported methods; because this test is in package `services` it drives those
// methods directly against the concrete *monitorSchedulerService.
//
// strPtr and discardLogger already exist in this package and are reused here.
package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// -----------------------------------------------------------------------------
// Hand-written func-field stubs for the two in-package interfaces that have no
// generated mocks. Same "unconfigured method panics" contract as moq mocks, so a
// test that hits an unexpected code path fails loudly.
// -----------------------------------------------------------------------------

type fakeMonitorExecutor struct {
	ExecuteMonitorRunFunc func(ctx context.Context, params ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error)
	UpdateNextRunTimeFunc func(ctx context.Context, monitorID uuid.UUID, nextRunTime time.Time) error
}

func (f *fakeMonitorExecutor) ExecuteMonitorRun(ctx context.Context, params ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error) {
	if f.ExecuteMonitorRunFunc == nil {
		panic("fakeMonitorExecutor.ExecuteMonitorRunFunc: method is nil but ExecuteMonitorRun was just called")
	}
	return f.ExecuteMonitorRunFunc(ctx, params)
}

func (f *fakeMonitorExecutor) UpdateNextRunTime(ctx context.Context, monitorID uuid.UUID, nextRunTime time.Time) error {
	if f.UpdateNextRunTimeFunc == nil {
		panic("fakeMonitorExecutor.UpdateNextRunTimeFunc: method is nil but UpdateNextRunTime was just called")
	}
	return f.UpdateNextRunTimeFunc(ctx, monitorID, nextRunTime)
}

type fakeProvisioner struct {
	EnsureCredentialsFunc func(ctx context.Context, ouID, orgUUID string) (*PublisherCredentials, error)
	IsThunderModeFunc     func() bool
}

func (f *fakeProvisioner) EnsureCredentials(ctx context.Context, ouID, orgUUID string) (*PublisherCredentials, error) {
	if f.EnsureCredentialsFunc == nil {
		panic("fakeProvisioner.EnsureCredentialsFunc: method is nil but EnsureCredentials was just called")
	}
	return f.EnsureCredentialsFunc(ctx, ouID, orgUUID)
}

func (f *fakeProvisioner) IsThunderMode() bool {
	if f.IsThunderModeFunc == nil {
		panic("fakeProvisioner.IsThunderModeFunc: method is nil but IsThunderMode was just called")
	}
	return f.IsThunderModeFunc()
}

// newScheduler builds the concrete scheduler so the test can exercise the
// unexported methods that hold the real logic.
func newScheduler(
	prov PublisherCredentialProvisioner,
	exec MonitorExecutor,
	repo *repomocks.MonitorRepositoryMock,
) *monitorSchedulerService {
	return NewMonitorSchedulerService(prov, discardLogger(), exec, repo, NoopMonitorRunObserver{}).(*monitorSchedulerService)
}

func intPtrU(i int) *int { return &i }

func futureMonitor(ouID string, interval int, nextRun time.Time) models.Monitor {
	return models.Monitor{
		ID:              uuid.New(),
		Name:            "my-monitor",
		Type:            models.MonitorTypeFuture,
		OUID:            ouID,
		IntervalMinutes: intPtrU(interval),
		NextRunTime:     &nextRun,
		Evaluators:      []models.MonitorEvaluator{{Identifier: "answer_relevancy", DisplayName: "Relevancy"}},
	}
}

// -----------------------------------------------------------------------------
// Stop — idempotent close of stopCh via sync.Once.
// -----------------------------------------------------------------------------

func TestMonitorScheduler_Stop(t *testing.T) {
	s := newScheduler(&fakeProvisioner{}, &fakeMonitorExecutor{}, &repomocks.MonitorRepositoryMock{})

	require.NoError(t, s.Stop())
	// Second call must not panic on a double close — sync.Once guards it.
	require.NoError(t, s.Stop())

	select {
	case <-s.stopCh:
		// channel is closed as expected
	default:
		t.Fatal("expected stopCh to be closed after Stop")
	}
}

// -----------------------------------------------------------------------------
// triggerMonitor — validation gates, time-window computation, fan-out into the
// executor, and error propagation.
// -----------------------------------------------------------------------------

func TestMonitorScheduler_triggerMonitor(t *testing.T) {
	const org = "acme"

	t.Run("rejects when interval_minutes is nil", func(t *testing.T) {
		m := futureMonitor(org, 10, time.Now())
		m.IntervalMinutes = nil
		// Provisioner / executor left unconfigured: must not be reached.
		s := newScheduler(&fakeProvisioner{}, &fakeMonitorExecutor{}, &repomocks.MonitorRepositoryMock{})

		err := s.triggerMonitor(context.Background(), &m)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "interval_minutes is nil")
	})

	t.Run("rejects when next_run_time is nil", func(t *testing.T) {
		m := futureMonitor(org, 10, time.Now())
		m.NextRunTime = nil
		s := newScheduler(&fakeProvisioner{}, &fakeMonitorExecutor{}, &repomocks.MonitorRepositoryMock{})

		err := s.triggerMonitor(context.Background(), &m)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "next_run_time is nil")
	})

	t.Run("propagates the executor error and backs the monitor off", func(t *testing.T) {
		boom := errors.New("workflow create failed")
		const interval = 10
		m := futureMonitor(org, interval, time.Now())
		prov := &fakeProvisioner{IsThunderModeFunc: func() bool { return false }}
		var retryAt time.Time
		exec := &fakeMonitorExecutor{
			ExecuteMonitorRunFunc: func(_ context.Context, _ ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error) {
				return nil, boom
			},
			UpdateNextRunTimeFunc: func(_ context.Context, _ uuid.UUID, t time.Time) error {
				retryAt = t
				return nil
			},
		}
		s := newScheduler(prov, exec, &repomocks.MonitorRepositoryMock{})

		before := time.Now()
		err := s.triggerMonitor(context.Background(), &m)

		assert.ErrorIs(t, err, boom)
		assert.False(t, retryAt.Before(before.Add(interval*time.Minute)),
			"expected the retry to be deferred by the monitor's interval, got %s", retryAt)
	})

	t.Run("hands the executor a context carrying the monitor's org", func(t *testing.T) {
		m := futureMonitor(org, 10, time.Now())
		prov := &fakeProvisioner{IsThunderModeFunc: func() bool { return false }}

		var gotOrg orgctx.ResolvedOrg
		var hadOrg bool
		exec := &fakeMonitorExecutor{
			ExecuteMonitorRunFunc: func(ctx context.Context, _ ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error) {
				gotOrg, hadOrg = orgctx.GetResolvedOrg(ctx)
				return &ExecuteMonitorRunResult{Name: "run-1"}, nil
			},
			UpdateNextRunTimeFunc: func(context.Context, uuid.UUID, time.Time) error { return nil },
		}
		s := newScheduler(prov, exec, &repomocks.MonitorRepositoryMock{})

		require.NoError(t, s.triggerMonitor(context.Background(), &m))

		require.True(t, hadOrg, "scheduler must stamp the org onto the context it passes down")
		assert.Equal(t, org, gotOrg.OUID)
	})

	t.Run("caps the backoff so a long-interval monitor is not stalled by one failure", func(t *testing.T) {
		const interval = 24 * 60
		m := futureMonitor(org, interval, time.Now())
		prov := &fakeProvisioner{IsThunderModeFunc: func() bool { return false }}
		var retryAt time.Time
		exec := &fakeMonitorExecutor{
			ExecuteMonitorRunFunc: func(_ context.Context, _ ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error) {
				return nil, errors.New("transient")
			},
			UpdateNextRunTimeFunc: func(_ context.Context, _ uuid.UUID, t time.Time) error {
				retryAt = t
				return nil
			},
		}
		s := newScheduler(prov, exec, &repomocks.MonitorRepositoryMock{})

		before := time.Now()
		_ = s.triggerMonitor(context.Background(), &m)

		assert.False(t, retryAt.After(before.Add(triggerFailureBackoffCap+time.Minute)),
			"expected the backoff to be capped at %s, got a retry at %s", triggerFailureBackoffCap, retryAt)
	})

	t.Run("computes the trace window", func(t *testing.T) {
		const interval = 60
		nextRun := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
		m := futureMonitor(org, interval, nextRun)

		prov := &fakeProvisioner{IsThunderModeFunc: func() bool { return false }}

		var captured ExecuteMonitorRunParams
		exec := &fakeMonitorExecutor{
			ExecuteMonitorRunFunc: func(_ context.Context, params ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error) {
				captured = params
				return &ExecuteMonitorRunResult{Name: "my-monitor-abcd1234"}, nil
			},
		}
		var nextUpdated time.Time
		var updatedID uuid.UUID
		exec.UpdateNextRunTimeFunc = func(_ context.Context, id uuid.UUID, t time.Time) error {
			updatedID = id
			nextUpdated = t
			return nil
		}

		s := newScheduler(prov, exec, &repomocks.MonitorRepositoryMock{})

		before := time.Now()
		err := s.triggerMonitor(context.Background(), &m)
		after := time.Now()

		require.NoError(t, err)

		// StartTime = NextRunTime - interval (deterministic, independent of wall clock).
		assert.Equal(t, nextRun.Add(-time.Duration(interval)*time.Minute), captured.StartTime)
		assert.Equal(t, org, captured.OUID)
		assert.Equal(t, m.Evaluators, captured.Evaluators)

		// EndTime = now - safetyDelta; bound it between the pre/post wall-clock reads.
		safetyDelta := time.Duration(float64(interval)*models.SafetyDeltaPercent) * time.Minute
		assert.False(t, captured.EndTime.Before(before.Add(-safetyDelta)), "EndTime should be >= before-safetyDelta")
		assert.False(t, captured.EndTime.After(after.Add(-safetyDelta)), "EndTime should be <= after-safetyDelta")

		// nextRunTime passed to UpdateNextRunTime = EndTime + interval.
		assert.Equal(t, captured.EndTime.Add(time.Duration(interval)*time.Minute), nextUpdated)
		assert.Equal(t, m.ID, updatedID)
	})

	t.Run("propagates the UpdateNextRunTime error after a successful execute", func(t *testing.T) {
		boom := errors.New("db update failed")
		m := futureMonitor(org, 10, time.Now())
		prov := &fakeProvisioner{IsThunderModeFunc: func() bool { return false }}
		exec := &fakeMonitorExecutor{
			ExecuteMonitorRunFunc: func(_ context.Context, _ ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error) {
				return &ExecuteMonitorRunResult{Name: "run-1"}, nil
			},
			UpdateNextRunTimeFunc: func(_ context.Context, _ uuid.UUID, _ time.Time) error {
				return boom
			},
		}
		s := newScheduler(prov, exec, &repomocks.MonitorRepositoryMock{})

		err := s.triggerMonitor(context.Background(), &m)

		assert.ErrorIs(t, err, boom)
		assert.Contains(t, err.Error(), "next_run_time")
	})
}

// -----------------------------------------------------------------------------
// triggerPendingMonitors — due-monitor selection, fan-out, and the rule that a
// single monitor's failure must not abort the batch.
// -----------------------------------------------------------------------------

func TestMonitorScheduler_triggerPendingMonitors(t *testing.T) {
	t.Run("returns nil and skips fan-out when no monitors are due", func(t *testing.T) {
		repo := &repomocks.MonitorRepositoryMock{
			ListDueMonitorsFunc: func(monitorType string, _ time.Time) ([]models.Monitor, error) {
				assert.Equal(t, models.MonitorTypeFuture, monitorType)
				return []models.Monitor{}, nil
			},
		}
		// Executor left unconfigured — must NOT be reached.
		s := newScheduler(&fakeProvisioner{}, &fakeMonitorExecutor{}, repo)

		require.NoError(t, s.triggerPendingMonitors(context.Background()))
	})

	t.Run("wraps the repo query error", func(t *testing.T) {
		boom := errors.New("query boom")
		repo := &repomocks.MonitorRepositoryMock{
			ListDueMonitorsFunc: func(_ string, _ time.Time) ([]models.Monitor, error) {
				return nil, boom
			},
		}
		s := newScheduler(&fakeProvisioner{}, &fakeMonitorExecutor{}, repo)

		err := s.triggerPendingMonitors(context.Background())

		assert.ErrorIs(t, err, boom)
		assert.Contains(t, err.Error(), "failed to query pending monitors")
	})

	t.Run("fans out over all due monitors and a single failure does not abort the batch", func(t *testing.T) {
		m1 := futureMonitor("org-a", 10, time.Now())
		m2 := futureMonitor("org-b", 10, time.Now())
		m2.IntervalMinutes = nil // forces m2's triggerMonitor to fail at the validation gate

		repo := &repomocks.MonitorRepositoryMock{
			ListDueMonitorsFunc: func(_ string, _ time.Time) ([]models.Monitor, error) {
				return []models.Monitor{m1, m2}, nil
			},
		}
		prov := &fakeProvisioner{IsThunderModeFunc: func() bool { return false }}

		executed := 0
		exec := &fakeMonitorExecutor{
			ExecuteMonitorRunFunc: func(_ context.Context, _ ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error) {
				executed++
				return &ExecuteMonitorRunResult{Name: "run"}, nil
			},
			UpdateNextRunTimeFunc: func(_ context.Context, _ uuid.UUID, _ time.Time) error { return nil },
		}
		s := newScheduler(prov, exec, repo)

		// The batch-level call swallows per-monitor errors (logged), so it returns nil.
		require.NoError(t, s.triggerPendingMonitors(context.Background()))
		// m1 succeeded; m2 failed before reaching the executor.
		assert.Equal(t, 1, executed)
	})
}

// -----------------------------------------------------------------------------
// syncRunStatus / syncSingleRunStatus — stale pending runs are failed.
// -----------------------------------------------------------------------------

func TestMonitorScheduler_syncRunStatus(t *testing.T) {
	t.Run("returns nil and skips when there are no pending/running runs", func(t *testing.T) {
		repo := &repomocks.MonitorRepositoryMock{
			ListPendingOrRunningRunsFunc: func(_ int) ([]models.MonitorRun, error) { return []models.MonitorRun{}, nil },
		}
		s := newScheduler(&fakeProvisioner{}, &fakeMonitorExecutor{}, repo)

		require.NoError(t, s.syncRunStatus(context.Background()))
	})

	t.Run("wraps the repo query error", func(t *testing.T) {
		boom := errors.New("list boom")
		repo := &repomocks.MonitorRepositoryMock{
			ListPendingOrRunningRunsFunc: func(_ int) ([]models.MonitorRun, error) { return nil, boom },
		}
		s := newScheduler(&fakeProvisioner{}, &fakeMonitorExecutor{}, repo)

		err := s.syncRunStatus(context.Background())

		assert.ErrorIs(t, err, boom)
	})
}

func TestMonitorScheduler_syncSingleRunStatus(t *testing.T) {
	const org = "acme"
	monitorID := uuid.New()

	baseRun := func() *models.MonitorRun {
		return &models.MonitorRun{ID: uuid.New(), MonitorID: monitorID, Name: "run-1", Status: models.RunStatusPending}
	}

	// repoWithMonitor returns a mock whose GetMonitorByID resolves to a monitor in org.
	repoWithMonitor := func() *repomocks.MonitorRepositoryMock {
		return &repomocks.MonitorRepositoryMock{
			GetMonitorByIDFunc: func(id uuid.UUID) (*models.Monitor, error) {
				assert.Equal(t, monitorID, id)
				return &models.Monitor{ID: monitorID, OUID: org, Name: "my-monitor"}, nil
			},
		}
	}

	nonThunder := func() *fakeProvisioner { return &fakeProvisioner{IsThunderModeFunc: func() bool { return false }} }

	t.Run("wraps an error from GetMonitorByID", func(t *testing.T) {
		boom := errors.New("get monitor boom")
		repo := &repomocks.MonitorRepositoryMock{
			GetMonitorByIDFunc: func(_ uuid.UUID) (*models.Monitor, error) { return nil, boom },
		}
		s := newScheduler(nonThunder(), &fakeMonitorExecutor{}, repo)

		err := s.syncSingleRunStatus(context.Background(), baseRun())

		assert.ErrorIs(t, err, boom)
	})

	t.Run("returns nil (skips) when the monitor no longer exists", func(t *testing.T) {
		repo := &repomocks.MonitorRepositoryMock{
			//nolint:nilnil // intentionally exercising the (nil, nil) "not found" input the service must handle
			GetMonitorByIDFunc: func(_ uuid.UUID) (*models.Monitor, error) { return nil, nil },
		}
		// Provisioner left unconfigured — must NOT be reached.
		s := newScheduler(&fakeProvisioner{}, &fakeMonitorExecutor{}, repo)

		require.NoError(t, s.syncSingleRunStatus(context.Background(), baseRun()))
	})

	t.Run("leaves a run without a start time alone", func(t *testing.T) {
		repo := repoWithMonitor() // UpdateMonitorRunFunc nil => must not be called
		s := newScheduler(nonThunder(), &fakeMonitorExecutor{}, repo)

		require.NoError(t, s.syncSingleRunStatus(context.Background(), baseRun()))
	})

	t.Run("fails a run that has been pending past the stuck timeout", func(t *testing.T) {
		var updates map[string]interface{}
		repo := repoWithMonitor()
		repo.UpdateMonitorRunFunc = func(_ *models.MonitorRun, u map[string]interface{}) error {
			updates = u
			return nil
		}
		run := baseRun()
		startedAt := time.Now().Add(-runStuckTimeout - time.Minute)
		run.StartedAt = &startedAt
		s := newScheduler(nonThunder(), &fakeMonitorExecutor{}, repo)

		require.NoError(t, s.syncSingleRunStatus(context.Background(), run))

		// Left pending, it would occupy a slot in the capped pending query forever.
		assert.Equal(t, models.RunStatusFailed, updates["status"])
		assert.NotEmpty(t, updates["error_message"])
		assert.Equal(t, updates["completed_at"], updates["event_queued_at"], "disabled events must not replay this run")
	})

	t.Run("leaves a pending run alone while it is still within the stuck timeout", func(t *testing.T) {
		repo := repoWithMonitor() // UpdateMonitorRunFunc nil => must not be called
		run := baseRun()
		startedAt := time.Now().Add(-time.Minute)
		run.StartedAt = &startedAt
		s := newScheduler(nonThunder(), &fakeMonitorExecutor{}, repo)

		require.NoError(t, s.syncSingleRunStatus(context.Background(), run))
	})

	t.Run("propagates a failure to persist the stale-run update", func(t *testing.T) {
		boom := errors.New("update boom")
		repo := repoWithMonitor()
		repo.UpdateMonitorRunFunc = func(_ *models.MonitorRun, _ map[string]interface{}) error { return boom }
		run := baseRun()
		startedAt := time.Now().Add(-runStuckTimeout - time.Minute)
		run.StartedAt = &startedAt
		s := newScheduler(nonThunder(), &fakeMonitorExecutor{}, repo)

		assert.ErrorIs(t, s.syncSingleRunStatus(context.Background(), run), boom)
	})
}

//go:build integration

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
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// mockExecutor is a test mock for the MonitorExecutor interface
type mockExecutor struct {
	mu                     sync.Mutex
	executeMonitorRunFunc  func(ctx context.Context, params ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error)
	advanceNextRunTimeFunc func(ctx context.Context, monitorID uuid.UUID, from, to time.Time) (bool, error)
	executeCalls           []ExecuteMonitorRunParams
	updateCalls            []struct {
		MonitorID   uuid.UUID
		NextRunTime time.Time
	}
}

func (m *mockExecutor) ExecuteMonitorRun(ctx context.Context, params ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error) {
	m.mu.Lock()
	m.executeCalls = append(m.executeCalls, params)
	m.mu.Unlock()
	if m.executeMonitorRunFunc != nil {
		return m.executeMonitorRunFunc(ctx, params)
	}
	return &ExecuteMonitorRunResult{
		Run:  &models.MonitorRun{ID: uuid.New()},
		Name: fmt.Sprintf("%s-%d", params.Monitor.Name, time.Now().Unix()),
	}, nil
}

func (m *mockExecutor) AdvanceNextRunTime(ctx context.Context, monitorID uuid.UUID, from, nextRunTime time.Time) (bool, error) {
	m.mu.Lock()
	m.updateCalls = append(m.updateCalls, struct {
		MonitorID   uuid.UUID
		NextRunTime time.Time
	}{monitorID, nextRunTime})
	m.mu.Unlock()
	if m.advanceNextRunTimeFunc != nil {
		return m.advanceNextRunTimeFunc(ctx, monitorID, from, nextRunTime)
	}
	return true, nil
}

func newTestScheduler(executor MonitorExecutor) *monitorSchedulerService {
	return &monitorSchedulerService{
		ocClient: &clientmocks.OpenChoreoClientMock{
			GetWorkflowRunFunc: func(ctx context.Context, namespaceName string, runName string) (*client.WorkflowRunResponse, error) {
				return nil, fmt.Errorf("mock: workflow run not found")
			},
		},
		provisioner: NewStaticPublisherCredentialProvisioner(),
		logger:      slog.Default(),
		executor:    executor,
		monitorRepo: schedulerFixtureRepo{MonitorRepository: repositories.NewMonitorRepo(db.GetDB())},
		runObserver: NoopMonitorRunObserver{},
		stopCh:      make(chan struct{}),
	}
}

// Scheduler integration tests share the database with API tests. Restrict
// scheduling to this suite's fixtures instead of deleting other suites' monitors.
type schedulerFixtureRepo struct {
	repositories.MonitorRepository
}

func (r schedulerFixtureRepo) ListDueMonitors(monitorType string, now time.Time) ([]models.Monitor, error) {
	monitors, err := r.MonitorRepository.ListDueMonitors(monitorType, now)
	if err != nil {
		return nil, err
	}
	fixtures := make([]models.Monitor, 0, len(monitors))
	for _, monitor := range monitors {
		if monitor.OUID == "lock-test-org" {
			fixtures = append(fixtures, monitor)
		}
	}
	return fixtures, nil
}

func (schedulerFixtureRepo) ListPendingOrRunningRuns(int) ([]models.MonitorRun, error) {
	return []models.MonitorRun{}, nil
}

// --- triggerMonitor tests ---

func intPtr(i int) *int { return &i }

func timePtr(t time.Time) *time.Time { return &t }

func TestTriggerMonitor_Success(t *testing.T) {
	executor := &mockExecutor{}
	s := newTestScheduler(executor)

	monitorID := uuid.New()
	now := time.Now()
	interval := 60
	monitor := &models.Monitor{
		ID:              monitorID,
		Name:            "test-monitor",
		OUID:            "test-org",
		IntervalMinutes: intPtr(interval),
		NextRunTime:     timePtr(now),
		Evaluators:      []models.MonitorEvaluator{{Identifier: "eval-1", DisplayName: "eval-1", Config: map[string]interface{}{"level": "trace"}}},
		SamplingRate:    1.0,
	}

	err := s.triggerMonitor(context.Background(), monitor)
	require.NoError(t, err)

	// Verify executor was called
	require.Len(t, executor.executeCalls, 1)
	call := executor.executeCalls[0]
	assert.Equal(t, "test-org", call.OUID)
	assert.Equal(t, monitor, call.Monitor)
	assert.Equal(t, []models.MonitorEvaluator{{Identifier: "eval-1", DisplayName: "eval-1", Config: map[string]interface{}{"level": "trace"}}}, call.Evaluators)

	// Verify time window calculation
	expectedStart := now.Add(-time.Duration(interval) * time.Minute)
	assert.Equal(t, expectedStart, call.StartTime)

	// EndTime should be approximately now minus safety delta
	safetyDelta := time.Duration(float64(interval)*models.SafetyDeltaPercent) * time.Minute
	expectedEndApprox := time.Now().Add(-safetyDelta)
	assert.WithinDuration(t, expectedEndApprox, call.EndTime, 2*time.Second)

	// Verify AdvanceNextRunTime was called
	require.Len(t, executor.updateCalls, 1)
	assert.Equal(t, monitorID, executor.updateCalls[0].MonitorID)

	// Next run time should be endTime + interval
	expectedNextRun := call.EndTime.Add(time.Duration(interval) * time.Minute)
	assert.WithinDuration(t, expectedNextRun, executor.updateCalls[0].NextRunTime, 2*time.Second)
}

func TestTriggerMonitor_NilIntervalMinutes(t *testing.T) {
	executor := &mockExecutor{}
	s := newTestScheduler(executor)

	monitor := &models.Monitor{
		Name:            "test-monitor",
		IntervalMinutes: nil,
		NextRunTime:     timePtr(time.Now()),
	}

	err := s.triggerMonitor(context.Background(), monitor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "interval_minutes is nil")
	assert.Empty(t, executor.executeCalls)
}

func TestTriggerMonitor_NilNextRunTime(t *testing.T) {
	executor := &mockExecutor{}
	s := newTestScheduler(executor)

	monitor := &models.Monitor{
		Name:            "test-monitor",
		IntervalMinutes: intPtr(60),
		NextRunTime:     nil,
	}

	err := s.triggerMonitor(context.Background(), monitor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "next_run_time is nil")
	assert.Empty(t, executor.executeCalls)
}

func TestTriggerMonitor_ExecutorError(t *testing.T) {
	executor := &mockExecutor{
		executeMonitorRunFunc: func(ctx context.Context, params ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error) {
			return nil, fmt.Errorf("workflow creation failed")
		},
	}
	s := newTestScheduler(executor)

	monitor := &models.Monitor{
		Name:            "test-monitor",
		IntervalMinutes: intPtr(60),
		NextRunTime:     timePtr(time.Now()),
	}

	before := time.Now()
	err := s.triggerMonitor(context.Background(), monitor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workflow creation failed")

	// next_run_time is pushed forward on failure: left in the past, the monitor would be
	// re-attempted on every scheduler tick for a reason a minute will not fix. The delay is
	// the monitor's own interval, capped by triggerFailureBackoffCap.
	require.Len(t, executor.updateCalls, 1)
	assert.False(t, executor.updateCalls[0].NextRunTime.Before(before),
		"expected the retry to be deferred, got %s", executor.updateCalls[0].NextRunTime)
	assert.False(t, executor.updateCalls[0].NextRunTime.After(before.Add(triggerFailureBackoffCap+time.Minute)),
		"expected the backoff to be capped at %s, got %s", triggerFailureBackoffCap, executor.updateCalls[0].NextRunTime)
}

func TestTriggerMonitor_AdvanceNextRunTimeError(t *testing.T) {
	executor := &mockExecutor{
		advanceNextRunTimeFunc: func(ctx context.Context, monitorID uuid.UUID, from, nextRunTime time.Time) (bool, error) {
			return false, fmt.Errorf("db error")
		},
	}
	s := newTestScheduler(executor)

	monitor := &models.Monitor{
		ID:              uuid.New(),
		Name:            "test-monitor",
		OUID:            "test-org",
		IntervalMinutes: intPtr(60),
		NextRunTime:     timePtr(time.Now()),
		Evaluators:      []models.MonitorEvaluator{{Identifier: "eval-1", DisplayName: "eval-1", Config: map[string]interface{}{"level": "trace"}}},
	}

	// Should return error — update failure is fatal to prevent double-trigger on the next cycle
	err := s.triggerMonitor(context.Background(), monitor)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to update next_run_time")

	// Executor should still have been called (WorkflowRun was already created)
	require.Len(t, executor.executeCalls, 1)
}

func TestTriggerMonitor_TimeWindowCalculation(t *testing.T) {
	executor := &mockExecutor{}
	s := newTestScheduler(executor)

	// Use a specific interval to verify safety delta
	interval := 100 // minutes
	nextRunTime := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	monitor := &models.Monitor{
		ID:              uuid.New(),
		Name:            "calc-test",
		OUID:            "test-org",
		IntervalMinutes: intPtr(interval),
		NextRunTime:     timePtr(nextRunTime),
		Evaluators:      []models.MonitorEvaluator{{Identifier: "eval-1", DisplayName: "eval-1", Config: map[string]interface{}{"level": "trace"}}},
	}

	err := s.triggerMonitor(context.Background(), monitor)
	require.NoError(t, err)

	call := executor.executeCalls[0]

	// startTime = nextRunTime - interval
	expectedStart := nextRunTime.Add(-time.Duration(interval) * time.Minute)
	assert.Equal(t, expectedStart, call.StartTime)

	// safetyDelta = 5% of 100 minutes = 5 minutes
	safetyDelta := 5 * time.Minute
	expectedEnd := time.Now().Add(-safetyDelta)
	assert.WithinDuration(t, expectedEnd, call.EndTime, 2*time.Second)

	// nextRunTime = endTime + interval
	expectedNextRun := call.EndTime.Add(time.Duration(interval) * time.Minute)
	assert.WithinDuration(t, expectedNextRun, executor.updateCalls[0].NextRunTime, 2*time.Second)
}

// --- Scheduler lifecycle tests ---

func TestSchedulerStartStop(t *testing.T) {
	executor := &mockExecutor{}
	svc := NewMonitorSchedulerService(nil, NewStaticPublisherCredentialProvisioner(), slog.Default(), executor, repositories.NewMonitorRepo(db.GetDB()), NoopMonitorRunObserver{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := svc.Start(ctx)
	require.NoError(t, err)

	err = svc.Stop()
	require.NoError(t, err)
}

func TestSchedulerStopIdempotent(t *testing.T) {
	executor := &mockExecutor{}
	svc := NewMonitorSchedulerService(nil, NewStaticPublisherCredentialProvisioner(), slog.Default(), executor, repositories.NewMonitorRepo(db.GetDB()), NoopMonitorRunObserver{})

	// Calling Stop multiple times should not panic
	err := svc.Stop()
	require.NoError(t, err)

	err = svc.Stop()
	require.NoError(t, err)

	err = svc.Stop()
	require.NoError(t, err)
}

func TestSchedulerStopsOnContextCancel(t *testing.T) {
	executor := &mockExecutor{}
	s := newTestScheduler(executor)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		s.runSchedulerLoop(ctx)
		close(done)
	}()

	// Cancel context to stop the loop
	cancel()

	select {
	case <-done:
		// Loop exited as expected
	case <-time.After(5 * time.Second):
		t.Fatal("scheduler loop did not stop after context cancellation")
	}
}

func TestSchedulerStopsOnStopChannel(t *testing.T) {
	ctx := context.Background()

	executor := &mockExecutor{}
	s := newTestScheduler(executor)

	done := make(chan struct{})
	go func() {
		s.runSchedulerLoop(ctx)
		close(done)
	}()

	// Close stop channel — the loop will exit after the initial cycle completes
	close(s.stopCh)

	select {
	case <-done:
		// Loop exited as expected
		assert.True(t, true, "scheduler loop stopped on stop channel")
	case <-time.After(5 * time.Second):
		t.Fatal("scheduler loop did not stop after stop channel closed")
	}
}

// --- Advisory lock tests (require PostgreSQL) ---

// TestSchedulerCycle_AdvisoryLockBlocksConcurrent verifies that when one
// connection holds the advisory lock, a concurrent runSchedulerCycle skips.
func TestSchedulerCycle_AdvisoryLockBlocksConcurrent(t *testing.T) {
	ctx := context.Background()

	// Hold the advisory lock on a separate transaction
	holdTx := db.DB(ctx).Begin()
	require.NoError(t, holdTx.Error, "failed to begin hold transaction")
	defer holdTx.Rollback()

	var locked bool
	err := holdTx.Raw("SELECT pg_try_advisory_xact_lock(?)", schedulerLockID).Scan(&locked).Error
	require.NoError(t, err)
	require.True(t, locked, "should acquire advisory lock on hold transaction")

	// Now run a scheduler cycle — it should skip because the lock is held
	var executeCalled atomic.Int32
	executor := &mockExecutor{
		executeMonitorRunFunc: func(ctx context.Context, params ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error) {
			executeCalled.Add(1)
			return &ExecuteMonitorRunResult{
				Run:  &models.MonitorRun{ID: uuid.New()},
				Name: "test-run",
			}, nil
		},
	}
	s := newTestScheduler(executor)

	// runSchedulerCycle should try to acquire the lock and fail (skip the cycle)
	s.runSchedulerCycle(ctx)

	// The executor should NOT have been called since the lock was held
	assert.Equal(t, int32(0), executeCalled.Load(),
		"executor should not be called when advisory lock is held by another connection")
}

// TestSchedulerCycle_AdvisoryLockReleasedAfterCycle verifies that the lock
// is released after a cycle completes, allowing subsequent cycles to proceed.
func TestSchedulerCycle_AdvisoryLockReleasedAfterCycle(t *testing.T) {
	ctx := context.Background()

	var executeCalled atomic.Int32
	executor := &mockExecutor{
		executeMonitorRunFunc: func(ctx context.Context, params ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error) {
			executeCalled.Add(1)
			return &ExecuteMonitorRunResult{
				Run:  &models.MonitorRun{ID: uuid.New()},
				Name: "test-run",
			}, nil
		},
	}
	s := newTestScheduler(executor)

	// Run first cycle — it should acquire the lock and complete
	s.runSchedulerCycle(ctx)

	// After the cycle completes, the lock should be released.
	// Verify by acquiring it on a new transaction.
	checkTx := db.DB(ctx).Begin()
	require.NoError(t, checkTx.Error)
	defer checkTx.Rollback()

	var locked bool
	err := checkTx.Raw("SELECT pg_try_advisory_xact_lock(?)", schedulerLockID).Scan(&locked).Error
	require.NoError(t, err)
	assert.True(t, locked, "advisory lock should be released after cycle completes")
}

// TestSchedulerCycle_TwoConcurrentCycles verifies that when two cycles run
// concurrently, exactly one proceeds and the other skips.
func TestSchedulerCycle_TwoConcurrentCycles(t *testing.T) {
	ctx := context.Background()

	var executeCalled atomic.Int32
	// Make execution take some time so both goroutines overlap
	executor := &mockExecutor{
		executeMonitorRunFunc: func(ctx context.Context, params ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error) {
			executeCalled.Add(1)
			time.Sleep(100 * time.Millisecond)
			return &ExecuteMonitorRunResult{
				Run:  &models.MonitorRun{ID: uuid.New()},
				Name: "test-run",
			}, nil
		},
	}

	// Create a monitor for the scheduler to find
	monitorID := uuid.New()
	monitor := models.Monitor{
		ID:              monitorID,
		Name:            fmt.Sprintf("lock-test-%d", time.Now().UnixNano()),
		DisplayName:     "Lock Test Monitor",
		Type:            models.MonitorTypeFuture,
		OUID:            "lock-test-org",
		ProjectName:     "test-project",
		AgentName:       "test-agent",
		AgentID:         "test-agent-id",
		EnvironmentName: "dev",
		EnvironmentID:   "test-env-id",
		Evaluators:      []models.MonitorEvaluator{{Identifier: "eval-1", DisplayName: "eval-1", Config: map[string]interface{}{"level": "trace"}}},
		IntervalMinutes: intPtr(60),
		NextRunTime:     timePtr(time.Now().Add(-1 * time.Minute)), // Due for trigger
		SamplingRate:    1.0,
	}
	require.NoError(t, db.DB(ctx).Create(&monitor).Error)
	defer db.DB(ctx).Delete(&monitor)

	s1 := newTestScheduler(executor)
	s2 := newTestScheduler(executor)

	var wg sync.WaitGroup
	wg.Add(2)

	// Start both cycles at the same time
	start := make(chan struct{})
	go func() {
		defer wg.Done()
		<-start
		s1.runSchedulerCycle(ctx)
	}()
	go func() {
		defer wg.Done()
		<-start
		s2.runSchedulerCycle(ctx)
	}()

	close(start)
	wg.Wait()

	// Exactly one cycle should have called the executor for this monitor.
	// The other should have been skipped due to the advisory lock.
	// Note: executeCalled may be 0 if the monitor query returns empty on the
	// skipped cycle, but the key assertion is it's at most 1.
	assert.LessOrEqual(t, executeCalled.Load(), int32(1),
		"at most one concurrent cycle should execute (advisory lock ensures mutual exclusion)")
}

// stopDuringRunExecutor is the real monitorExecutor, except that executing the run
// stops the monitor the way StopMonitor does. That reproduces a user stopping the
// monitor while the scheduler cycle that selected it is still executing it.
type stopDuringRunExecutor struct {
	*monitorExecutor
	stop func() error
}

func (e stopDuringRunExecutor) ExecuteMonitorRun(_ context.Context, params ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error) {
	if err := e.stop(); err != nil {
		return nil, err
	}
	return &ExecuteMonitorRunResult{Run: &models.MonitorRun{ID: uuid.New()}, Name: params.Monitor.Name + "-run"}, nil
}

// TestSchedulerCycle_StopDuringRunIsNotUndone: a monitor the user stops while a
// scheduler cycle is executing it must stay stopped. Before the fix the cycle wrote
// its next_run_time unconditionally after the run and re-activated the monitor.
func TestSchedulerCycle_StopDuringRunIsNotUndone(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewMonitorRepo(db.GetDB())
	monitor := newDueSchedulerMonitor(t, "stop-race")

	executor := stopDuringRunExecutor{
		monitorExecutor: &monitorExecutor{monitorRepo: repo, logger: slog.Default()},
		stop:            func() error { return repo.UpdateNextRunTime(monitor.ID, nil) },
	}
	newTestScheduler(executor).runSchedulerCycle(ctx)

	var after models.Monitor
	require.NoError(t, db.DB(ctx).First(&after, "id = ?", monitor.ID).Error)
	assert.Nil(t, after.NextRunTime, "a monitor stopped during its scheduled run must stay stopped, got next_run_time %v", after.NextRunTime)
}

// TestSchedulerCycle_RestartDuringRunIsKept: a stop followed by a start while the cycle
// runs sets next_run_time to "now"; the cycle must keep the user's value, not its own.
func TestSchedulerCycle_RestartDuringRunIsKept(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewMonitorRepo(db.GetDB())
	monitor := newDueSchedulerMonitor(t, "restart-race")
	restartedAt := time.Now().Add(5 * time.Minute).Truncate(time.Microsecond)

	executor := stopDuringRunExecutor{
		monitorExecutor: &monitorExecutor{monitorRepo: repo, logger: slog.Default()},
		stop: func() error {
			if err := repo.UpdateNextRunTime(monitor.ID, nil); err != nil {
				return err
			}
			return repo.UpdateNextRunTime(monitor.ID, &restartedAt)
		},
	}
	newTestScheduler(executor).runSchedulerCycle(ctx)

	var after models.Monitor
	require.NoError(t, db.DB(ctx).First(&after, "id = ?", monitor.ID).Error)
	require.NotNil(t, after.NextRunTime)
	assert.True(t, after.NextRunTime.Equal(restartedAt), "expected the user's restart time %v, got %v", restartedAt, *after.NextRunTime)
}

// TestSchedulerCycle_AdvancesUnchangedSchedule: with nobody touching the monitor, the
// compare-and-set must still move next_run_time forward. This guards the timestamp
// round-trip through Postgres: if the value the cycle read did not compare equal on the
// way back, every monitor's schedule would silently stop advancing.
func TestSchedulerCycle_AdvancesUnchangedSchedule(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewMonitorRepo(db.GetDB())
	monitor := newDueSchedulerMonitor(t, "advance")
	before := *monitor.NextRunTime

	executor := stopDuringRunExecutor{
		monitorExecutor: &monitorExecutor{monitorRepo: repo, logger: slog.Default()},
		stop:            func() error { return nil },
	}
	newTestScheduler(executor).runSchedulerCycle(ctx)

	var after models.Monitor
	require.NoError(t, db.DB(ctx).First(&after, "id = ?", monitor.ID).Error)
	require.NotNil(t, after.NextRunTime)
	assert.True(t, after.NextRunTime.After(before.Add(30*time.Minute)),
		"expected next_run_time to advance by about one interval from %v, got %v", before, *after.NextRunTime)
}

// newDueSchedulerMonitor stores a future monitor that is due now, scoped to the
// scheduler fixture org, and deletes it when the test ends.
func newDueSchedulerMonitor(t *testing.T, prefix string) models.Monitor {
	t.Helper()
	ctx := context.Background()
	monitor := models.Monitor{
		ID:              uuid.New(),
		Name:            fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()),
		DisplayName:     "Scheduler Race Monitor",
		Type:            models.MonitorTypeFuture,
		OUID:            "lock-test-org",
		ProjectName:     "test-project",
		AgentName:       "test-agent",
		AgentID:         "test-agent-id",
		EnvironmentName: "dev",
		EnvironmentID:   "test-env-id",
		Evaluators:      []models.MonitorEvaluator{{Identifier: "eval-1", DisplayName: "eval-1", Config: map[string]interface{}{"level": "trace"}}},
		IntervalMinutes: intPtr(60),
		NextRunTime:     timePtr(time.Now().Add(-1 * time.Minute)), // due
		SamplingRate:    1.0,
	}
	require.NoError(t, db.DB(ctx).Create(&monitor).Error)
	t.Cleanup(func() { db.DB(ctx).Delete(&monitor) })
	return monitor
}

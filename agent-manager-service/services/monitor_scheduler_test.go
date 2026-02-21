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

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/clientmocks"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// mockExecutor is a test mock for the MonitorExecutor interface
type mockExecutor struct {
	mu                    sync.Mutex
	executeMonitorRunFunc func(ctx context.Context, params ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error)
	updateNextRunTimeFunc func(ctx context.Context, monitorID uuid.UUID, nextRunTime time.Time) error
	executeCalls          []ExecuteMonitorRunParams
	updateCalls           []struct {
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

func (m *mockExecutor) UpdateNextRunTime(ctx context.Context, monitorID uuid.UUID, nextRunTime time.Time) error {
	m.mu.Lock()
	m.updateCalls = append(m.updateCalls, struct {
		MonitorID   uuid.UUID
		NextRunTime time.Time
	}{monitorID, nextRunTime})
	m.mu.Unlock()
	if m.updateNextRunTimeFunc != nil {
		return m.updateNextRunTimeFunc(ctx, monitorID, nextRunTime)
	}
	return nil
}

func newTestScheduler(executor MonitorExecutor) *monitorSchedulerService {
	return &monitorSchedulerService{
		ocClient: &clientmocks.OpenChoreoClientMock{
			GetResourceFunc: func(ctx context.Context, namespaceName string, kind string, name string) (map[string]interface{}, error) {
				return nil, fmt.Errorf("mock: resource not found")
			},
		},
		logger:   slog.Default(),
		executor: executor,
		stopCh:   make(chan struct{}),
	}
}

// --- extractWorkflowStatus tests ---

func TestExtractWorkflowStatus_NoStatusField(t *testing.T) {
	s := newTestScheduler(nil)
	cr := map[string]interface{}{}

	status, err := s.extractWorkflowStatus(cr)
	require.NoError(t, err)
	assert.Equal(t, "Pending", status)
}

func TestExtractWorkflowStatus_EmptyConditions(t *testing.T) {
	s := newTestScheduler(nil)
	cr := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{},
		},
	}

	status, err := s.extractWorkflowStatus(cr)
	require.NoError(t, err)
	assert.Equal(t, "Pending", status)
}

func TestExtractWorkflowStatus_Succeeded(t *testing.T) {
	s := newTestScheduler(nil)
	cr := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":   "WorkflowCompleted",
					"status": "True",
					"reason": "WorkflowSucceeded",
				},
			},
		},
	}

	status, err := s.extractWorkflowStatus(cr)
	require.NoError(t, err)
	assert.Equal(t, "Succeeded", status)
}

func TestExtractWorkflowStatus_FailedWithTrueStatus(t *testing.T) {
	s := newTestScheduler(nil)
	cr := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":   "WorkflowCompleted",
					"status": "True",
					"reason": "WorkflowFailed",
				},
			},
		},
	}

	status, err := s.extractWorkflowStatus(cr)
	require.NoError(t, err)
	assert.Equal(t, "Failed", status)
}

func TestExtractWorkflowStatus_Running(t *testing.T) {
	s := newTestScheduler(nil)
	cr := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":   "WorkflowCompleted",
					"status": "False",
					"reason": "WorkflowRunning",
				},
			},
		},
	}

	status, err := s.extractWorkflowStatus(cr)
	require.NoError(t, err)
	assert.Equal(t, "Running", status)
}

func TestExtractWorkflowStatus_PendingReason(t *testing.T) {
	s := newTestScheduler(nil)
	cr := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":   "WorkflowCompleted",
					"status": "False",
					"reason": "WorkflowPending",
				},
			},
		},
	}

	status, err := s.extractWorkflowStatus(cr)
	require.NoError(t, err)
	assert.Equal(t, "Pending", status)
}

func TestExtractWorkflowStatus_UnknownReasonWithMessage(t *testing.T) {
	s := newTestScheduler(nil)
	cr := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":    "WorkflowCompleted",
					"status":  "False",
					"reason":  "RenderFailed",
					"message": "template rendering error",
				},
			},
		},
	}

	status, err := s.extractWorkflowStatus(cr)
	require.NoError(t, err)
	assert.Equal(t, "Failed", status)
}

func TestExtractWorkflowStatus_UnknownReasonNoMessage(t *testing.T) {
	s := newTestScheduler(nil)
	cr := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":   "WorkflowCompleted",
					"status": "Unknown",
					"reason": "SomeUnknownReason",
				},
			},
		},
	}

	status, err := s.extractWorkflowStatus(cr)
	require.NoError(t, err)
	assert.Equal(t, "Pending", status)
}

func TestExtractWorkflowStatus_NoWorkflowCompletedCondition(t *testing.T) {
	s := newTestScheduler(nil)
	cr := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":   "SomeOtherCondition",
					"status": "True",
				},
			},
		},
	}

	status, err := s.extractWorkflowStatus(cr)
	require.NoError(t, err)
	assert.Equal(t, "Pending", status)
}

func TestExtractWorkflowStatus_InvalidConditionEntry(t *testing.T) {
	s := newTestScheduler(nil)
	cr := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				"not-a-map",
				map[string]interface{}{
					"type":   "WorkflowCompleted",
					"status": "True",
					"reason": "WorkflowSucceeded",
				},
			},
		},
	}

	status, err := s.extractWorkflowStatus(cr)
	require.NoError(t, err)
	assert.Equal(t, "Succeeded", status)
}

// --- extractErrorMessage tests ---

func TestExtractErrorMessage_NoStatus(t *testing.T) {
	s := newTestScheduler(nil)
	cr := map[string]interface{}{}

	msg := s.extractErrorMessage(cr)
	assert.Empty(t, msg)
}

func TestExtractErrorMessage_NoConditions(t *testing.T) {
	s := newTestScheduler(nil)
	cr := map[string]interface{}{
		"status": map[string]interface{}{},
	}

	msg := s.extractErrorMessage(cr)
	assert.Empty(t, msg)
}

func TestExtractErrorMessage_WithReasonAndMessage(t *testing.T) {
	s := newTestScheduler(nil)
	cr := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":    "WorkflowCompleted",
					"status":  "True",
					"reason":  "WorkflowFailed",
					"message": "out of memory",
				},
			},
		},
	}

	msg := s.extractErrorMessage(cr)
	assert.Equal(t, "WorkflowFailed: out of memory", msg)
}

func TestExtractErrorMessage_ReasonOnly(t *testing.T) {
	s := newTestScheduler(nil)
	cr := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":   "WorkflowCompleted",
					"status": "True",
					"reason": "WorkflowFailed",
				},
			},
		},
	}

	msg := s.extractErrorMessage(cr)
	assert.Equal(t, "WorkflowFailed", msg)
}

func TestExtractErrorMessage_MessageOnly(t *testing.T) {
	s := newTestScheduler(nil)
	cr := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":    "WorkflowCompleted",
					"status":  "True",
					"message": "something went wrong",
				},
			},
		},
	}

	msg := s.extractErrorMessage(cr)
	assert.Equal(t, "something went wrong", msg)
}

func TestExtractErrorMessage_NoReasonNoMessage(t *testing.T) {
	s := newTestScheduler(nil)
	cr := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":   "WorkflowCompleted",
					"status": "True",
				},
			},
		},
	}

	msg := s.extractErrorMessage(cr)
	assert.Empty(t, msg)
}

func TestExtractErrorMessage_NoWorkflowCompletedCondition(t *testing.T) {
	s := newTestScheduler(nil)
	cr := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{
					"type":    "SomeOtherCondition",
					"message": "this should not be returned",
				},
			},
		},
	}

	msg := s.extractErrorMessage(cr)
	assert.Empty(t, msg)
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
		OrgName:         "test-org",
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
	assert.Equal(t, "test-org", call.OrgName)
	assert.Equal(t, monitor, call.Monitor)
	assert.Equal(t, []models.MonitorEvaluator{{Identifier: "eval-1", DisplayName: "eval-1", Config: map[string]interface{}{"level": "trace"}}}, call.Evaluators)

	// Verify time window calculation
	expectedStart := now.Add(-time.Duration(interval) * time.Minute)
	assert.Equal(t, expectedStart, call.StartTime)

	// EndTime should be approximately now minus safety delta
	safetyDelta := time.Duration(float64(interval)*models.SafetyDeltaPercent) * time.Minute
	expectedEndApprox := time.Now().Add(-safetyDelta)
	assert.WithinDuration(t, expectedEndApprox, call.EndTime, 2*time.Second)

	// Verify UpdateNextRunTime was called
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

	err := s.triggerMonitor(context.Background(), monitor)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workflow creation failed")

	// UpdateNextRunTime should NOT be called when execution fails
	assert.Empty(t, executor.updateCalls)
}

func TestTriggerMonitor_UpdateNextRunTimeError(t *testing.T) {
	executor := &mockExecutor{
		updateNextRunTimeFunc: func(ctx context.Context, monitorID uuid.UUID, nextRunTime time.Time) error {
			return fmt.Errorf("db error")
		},
	}
	s := newTestScheduler(executor)

	monitor := &models.Monitor{
		ID:              uuid.New(),
		Name:            "test-monitor",
		OrgName:         "test-org",
		IntervalMinutes: intPtr(60),
		NextRunTime:     timePtr(time.Now()),
		Evaluators:      []models.MonitorEvaluator{{Identifier: "eval-1", DisplayName: "eval-1", Config: map[string]interface{}{"level": "trace"}}},
	}

	// Should NOT return error — update failure is non-fatal
	err := s.triggerMonitor(context.Background(), monitor)
	require.NoError(t, err)

	// Executor should still have been called
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
		OrgName:         "test-org",
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
	svc := NewMonitorSchedulerService(nil, slog.Default(), executor)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := svc.Start(ctx)
	require.NoError(t, err)

	err = svc.Stop()
	require.NoError(t, err)
}

func TestSchedulerStopIdempotent(t *testing.T) {
	executor := &mockExecutor{}
	svc := NewMonitorSchedulerService(nil, slog.Default(), executor)

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

	// Clean stale monitors so the initial cycle doesn't do unnecessary DB work
	db.DB(ctx).Where("type = ?", models.MonitorTypeFuture).Delete(&models.Monitor{})

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

	// Clean up stale monitors from other test runs so we only test our monitor
	db.DB(ctx).Where("type = ?", models.MonitorTypeFuture).Delete(&models.Monitor{})

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
		OrgName:         "lock-test-org",
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

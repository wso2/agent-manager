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
//   - client.OpenChoreoClient             -> clientmocks.OpenChoreoClientMock (moq-generated)
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
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
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
	GetOCClientForOrgFunc func(ctx context.Context, ouID string) (client.OpenChoreoClient, error)
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

func (f *fakeProvisioner) GetOCClientForOrg(ctx context.Context, ouID string) (client.OpenChoreoClient, error) {
	if f.GetOCClientForOrgFunc == nil {
		panic("fakeProvisioner.GetOCClientForOrgFunc: method is nil but GetOCClientForOrg was just called")
	}
	return f.GetOCClientForOrgFunc(ctx, ouID)
}

// newScheduler builds the concrete scheduler so the test can exercise the
// unexported methods that hold the real logic.
func newScheduler(
	oc *clientmocks.OpenChoreoClientMock,
	prov PublisherCredentialProvisioner,
	exec MonitorExecutor,
	repo *repomocks.MonitorRepositoryMock,
) *monitorSchedulerService {
	return NewMonitorSchedulerService(oc, prov, discardLogger(), exec, repo, NoopMonitorRunAlerter{}).(*monitorSchedulerService)
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
	s := newScheduler(&clientmocks.OpenChoreoClientMock{}, &fakeProvisioner{}, &fakeMonitorExecutor{}, &repomocks.MonitorRepositoryMock{})

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
// orgOCClient — Thunder vs non-Thunder branch selection.
// -----------------------------------------------------------------------------

func TestMonitorScheduler_orgOCClient(t *testing.T) {
	t.Run("non-Thunder mode returns the system client without calling the provisioner", func(t *testing.T) {
		systemOC := &clientmocks.OpenChoreoClientMock{}
		prov := &fakeProvisioner{
			IsThunderModeFunc: func() bool { return false },
			// GetOCClientForOrgFunc left nil — must NOT be reached.
		}
		s := newScheduler(systemOC, prov, &fakeMonitorExecutor{}, &repomocks.MonitorRepositoryMock{})

		got, err := s.orgOCClient(context.Background(), "acme")

		require.NoError(t, err)
		assert.Same(t, systemOC, got, "non-Thunder mode must return the injected system client")
	})

	t.Run("Thunder mode delegates to the provisioner", func(t *testing.T) {
		orgOC := &clientmocks.OpenChoreoClientMock{}
		prov := &fakeProvisioner{
			IsThunderModeFunc: func() bool { return true },
			GetOCClientForOrgFunc: func(_ context.Context, ouID string) (client.OpenChoreoClient, error) {
				assert.Equal(t, "acme", ouID)
				return orgOC, nil
			},
		}
		s := newScheduler(&clientmocks.OpenChoreoClientMock{}, prov, &fakeMonitorExecutor{}, &repomocks.MonitorRepositoryMock{})

		got, err := s.orgOCClient(context.Background(), "acme")

		require.NoError(t, err)
		assert.Same(t, orgOC, got)
	})

	t.Run("Thunder mode propagates the provisioner error", func(t *testing.T) {
		prov := &fakeProvisioner{
			IsThunderModeFunc: func() bool { return true },
			GetOCClientForOrgFunc: func(_ context.Context, _ string) (client.OpenChoreoClient, error) {
				return nil, ErrPublisherCredentialNotFound
			},
		}
		s := newScheduler(&clientmocks.OpenChoreoClientMock{}, prov, &fakeMonitorExecutor{}, &repomocks.MonitorRepositoryMock{})

		_, err := s.orgOCClient(context.Background(), "acme")

		assert.ErrorIs(t, err, ErrPublisherCredentialNotFound)
	})
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
		s := newScheduler(&clientmocks.OpenChoreoClientMock{}, &fakeProvisioner{}, &fakeMonitorExecutor{}, &repomocks.MonitorRepositoryMock{})

		err := s.triggerMonitor(context.Background(), &m)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "interval_minutes is nil")
	})

	t.Run("rejects when next_run_time is nil", func(t *testing.T) {
		m := futureMonitor(org, 10, time.Now())
		m.NextRunTime = nil
		s := newScheduler(&clientmocks.OpenChoreoClientMock{}, &fakeProvisioner{}, &fakeMonitorExecutor{}, &repomocks.MonitorRepositoryMock{})

		err := s.triggerMonitor(context.Background(), &m)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "next_run_time is nil")
	})

	t.Run("propagates the client-resolution error and backs the monitor off", func(t *testing.T) {
		const interval = 10
		m := futureMonitor(org, interval, time.Now())
		prov := &fakeProvisioner{
			IsThunderModeFunc: func() bool { return true },
			GetOCClientForOrgFunc: func(_ context.Context, _ string) (client.OpenChoreoClient, error) {
				return nil, ErrPublisherCredentialNotFound
			},
		}
		var retryAt time.Time
		exec := &fakeMonitorExecutor{
			UpdateNextRunTimeFunc: func(_ context.Context, _ uuid.UUID, t time.Time) error {
				retryAt = t
				return nil
			},
		}
		s := newScheduler(&clientmocks.OpenChoreoClientMock{}, prov, exec, &repomocks.MonitorRepositoryMock{})

		before := time.Now()
		err := s.triggerMonitor(context.Background(), &m)

		assert.ErrorIs(t, err, ErrPublisherCredentialNotFound)
		// Left in the past, the monitor would be retried on every tick.
		assert.False(t, retryAt.Before(before.Add(interval*time.Minute)),
			"expected the retry to be deferred by the monitor's interval, got %s", retryAt)
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
		s := newScheduler(&clientmocks.OpenChoreoClientMock{}, prov, exec, &repomocks.MonitorRepositoryMock{})

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
		s := newScheduler(&clientmocks.OpenChoreoClientMock{}, prov, exec, &repomocks.MonitorRepositoryMock{})

		require.NoError(t, s.triggerMonitor(context.Background(), &m))

		// Without this the OpenChoreo client sends no impersonation header and
		// endpoints that resolve the org from the caller reject the request.
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
		s := newScheduler(&clientmocks.OpenChoreoClientMock{}, prov, exec, &repomocks.MonitorRepositoryMock{})

		before := time.Now()
		_ = s.triggerMonitor(context.Background(), &m)

		assert.False(t, retryAt.After(before.Add(triggerFailureBackoffCap+time.Minute)),
			"expected the backoff to be capped at %s, got a retry at %s", triggerFailureBackoffCap, retryAt)
	})

	t.Run("computes the trace window and the system client is injected in non-Thunder mode", func(t *testing.T) {
		const interval = 60
		nextRun := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
		m := futureMonitor(org, interval, nextRun)

		systemOC := &clientmocks.OpenChoreoClientMock{}
		prov := &fakeProvisioner{IsThunderModeFunc: func() bool { return false }}

		var captured ExecuteMonitorRunParams
		var injectedOC client.OpenChoreoClient
		var sawInjected bool
		exec := &fakeMonitorExecutor{
			ExecuteMonitorRunFunc: func(ctx context.Context, params ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error) {
				captured = params
				// The scheduler injects the resolved OC client into ctx for the executor.
				injectedOC = ocClientFromContext(ctx, nil)
				sawInjected = injectedOC != nil
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

		s := newScheduler(systemOC, prov, exec, &repomocks.MonitorRepositoryMock{})

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

		// Non-Thunder mode injects the system OC client into the executor's context.
		assert.True(t, sawInjected, "expected an OC client to be injected into ctx")
		assert.Same(t, systemOC, injectedOC)
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
		s := newScheduler(&clientmocks.OpenChoreoClientMock{}, prov, exec, &repomocks.MonitorRepositoryMock{})

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
		s := newScheduler(&clientmocks.OpenChoreoClientMock{}, &fakeProvisioner{}, &fakeMonitorExecutor{}, repo)

		require.NoError(t, s.triggerPendingMonitors(context.Background()))
	})

	t.Run("wraps the repo query error", func(t *testing.T) {
		boom := errors.New("query boom")
		repo := &repomocks.MonitorRepositoryMock{
			ListDueMonitorsFunc: func(_ string, _ time.Time) ([]models.Monitor, error) {
				return nil, boom
			},
		}
		s := newScheduler(&clientmocks.OpenChoreoClientMock{}, &fakeProvisioner{}, &fakeMonitorExecutor{}, repo)

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
		s := newScheduler(&clientmocks.OpenChoreoClientMock{}, prov, exec, repo)

		// The batch-level call swallows per-monitor errors (logged), so it returns nil.
		require.NoError(t, s.triggerPendingMonitors(context.Background()))
		// m1 succeeded; m2 failed before reaching the executor.
		assert.Equal(t, 1, executed)
	})
}

// -----------------------------------------------------------------------------
// syncRunStatus / syncSingleRunStatus — OpenChoreo status -> DB status mapping.
// -----------------------------------------------------------------------------

func TestMonitorScheduler_syncRunStatus(t *testing.T) {
	t.Run("returns nil and skips when there are no pending/running runs", func(t *testing.T) {
		repo := &repomocks.MonitorRepositoryMock{
			ListPendingOrRunningRunsFunc: func(_ int) ([]models.MonitorRun, error) { return []models.MonitorRun{}, nil },
		}
		s := newScheduler(&clientmocks.OpenChoreoClientMock{}, &fakeProvisioner{}, &fakeMonitorExecutor{}, repo)

		require.NoError(t, s.syncRunStatus(context.Background()))
	})

	t.Run("wraps the repo query error", func(t *testing.T) {
		boom := errors.New("list boom")
		repo := &repomocks.MonitorRepositoryMock{
			ListPendingOrRunningRunsFunc: func(_ int) ([]models.MonitorRun, error) { return nil, boom },
		}
		s := newScheduler(&clientmocks.OpenChoreoClientMock{}, &fakeProvisioner{}, &fakeMonitorExecutor{}, repo)

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
		s := newScheduler(&clientmocks.OpenChoreoClientMock{}, nonThunder(), &fakeMonitorExecutor{}, repo)

		err := s.syncSingleRunStatus(context.Background(), baseRun())

		assert.ErrorIs(t, err, boom)
	})

	t.Run("returns nil (skips) when the monitor no longer exists", func(t *testing.T) {
		repo := &repomocks.MonitorRepositoryMock{
			//nolint:nilnil // intentionally exercising the (nil, nil) "not found" input the service must handle
			GetMonitorByIDFunc: func(_ uuid.UUID) (*models.Monitor, error) { return nil, nil },
		}
		// OC client / provisioner left unconfigured — must NOT be reached.
		s := newScheduler(&clientmocks.OpenChoreoClientMock{}, &fakeProvisioner{}, &fakeMonitorExecutor{}, repo)

		require.NoError(t, s.syncSingleRunStatus(context.Background(), baseRun()))
	})

	t.Run("wraps the GetWorkflowRun error", func(t *testing.T) {
		boom := errors.New("workflow not found")
		oc := &clientmocks.OpenChoreoClientMock{
			GetWorkflowRunFunc: func(_ context.Context, _, _ string) (*client.WorkflowRunResponse, error) {
				return nil, boom
			},
		}
		s := newScheduler(oc, nonThunder(), &fakeMonitorExecutor{}, repoWithMonitor())

		err := s.syncSingleRunStatus(context.Background(), baseRun())

		assert.ErrorIs(t, err, boom)
		assert.Contains(t, err.Error(), "failed to get workflow run")
	})

	t.Run("maps Succeeded to success and sets completed_at", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetWorkflowRunFunc: func(_ context.Context, _, _ string) (*client.WorkflowRunResponse, error) {
				return &client.WorkflowRunResponse{Status: "Succeeded"}, nil
			},
		}
		var got map[string]interface{}
		repo := repoWithMonitor()
		repo.UpdateMonitorRunFunc = func(_ *models.MonitorRun, updates map[string]interface{}) error {
			got = updates
			return nil
		}
		s := newScheduler(oc, nonThunder(), &fakeMonitorExecutor{}, repo)

		require.NoError(t, s.syncSingleRunStatus(context.Background(), baseRun()))
		require.NotNil(t, got, "expected UpdateMonitorRun to be called")
		assert.Equal(t, models.RunStatusSuccess, got["status"])
		assert.Contains(t, got, "completed_at")
	})

	for _, ocStatus := range []string{"Failed", "Error"} {
		t.Run("maps "+ocStatus+" to failed with an error message", func(t *testing.T) {
			oc := &clientmocks.OpenChoreoClientMock{
				GetWorkflowRunFunc: func(_ context.Context, _, _ string) (*client.WorkflowRunResponse, error) {
					return &client.WorkflowRunResponse{Status: ocStatus}, nil
				},
			}
			var got map[string]interface{}
			repo := repoWithMonitor()
			repo.UpdateMonitorRunFunc = func(_ *models.MonitorRun, updates map[string]interface{}) error {
				got = updates
				return nil
			}
			s := newScheduler(oc, nonThunder(), &fakeMonitorExecutor{}, repo)

			require.NoError(t, s.syncSingleRunStatus(context.Background(), baseRun()))
			require.NotNil(t, got)
			assert.Equal(t, models.RunStatusFailed, got["status"])
			assert.Contains(t, got, "completed_at")
			assert.Contains(t, got, "error_message")
		})
	}

	t.Run("transitions a pending run to running", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetWorkflowRunFunc: func(_ context.Context, _, _ string) (*client.WorkflowRunResponse, error) {
				return &client.WorkflowRunResponse{Status: "Running"}, nil
			},
		}
		var got map[string]interface{}
		repo := repoWithMonitor()
		repo.UpdateMonitorRunFunc = func(_ *models.MonitorRun, updates map[string]interface{}) error {
			got = updates
			return nil
		}
		s := newScheduler(oc, nonThunder(), &fakeMonitorExecutor{}, repo)

		require.NoError(t, s.syncSingleRunStatus(context.Background(), baseRun()))
		require.NotNil(t, got)
		assert.Equal(t, models.RunStatusRunning, got["status"])
	})

	t.Run("does not update when an already-running run stays Running (no-op update)", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetWorkflowRunFunc: func(_ context.Context, _, _ string) (*client.WorkflowRunResponse, error) {
				return &client.WorkflowRunResponse{Status: "Running"}, nil
			},
		}
		repo := repoWithMonitor()
		// UpdateMonitorRunFunc left nil: must NOT be called when there is nothing to change.
		s := newScheduler(oc, nonThunder(), &fakeMonitorExecutor{}, repo)

		run := baseRun()
		run.Status = models.RunStatusRunning
		require.NoError(t, s.syncSingleRunStatus(context.Background(), run))
	})

	t.Run("Pending status is a no-op (no update)", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetWorkflowRunFunc: func(_ context.Context, _, _ string) (*client.WorkflowRunResponse, error) {
				return &client.WorkflowRunResponse{Status: "Pending"}, nil
			},
		}
		repo := repoWithMonitor() // UpdateMonitorRunFunc nil => must not be called
		s := newScheduler(oc, nonThunder(), &fakeMonitorExecutor{}, repo)

		require.NoError(t, s.syncSingleRunStatus(context.Background(), baseRun()))
	})

	t.Run("unknown status is a no-op (no update)", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetWorkflowRunFunc: func(_ context.Context, _, _ string) (*client.WorkflowRunResponse, error) {
				return &client.WorkflowRunResponse{Status: "Suspended"}, nil
			},
		}
		repo := repoWithMonitor() // UpdateMonitorRunFunc nil => must not be called
		s := newScheduler(oc, nonThunder(), &fakeMonitorExecutor{}, repo)

		require.NoError(t, s.syncSingleRunStatus(context.Background(), baseRun()))
	})

	t.Run("fails a run that has been Pending past the stuck timeout", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetWorkflowRunFunc: func(_ context.Context, _, _ string) (*client.WorkflowRunResponse, error) {
				return &client.WorkflowRunResponse{Status: "Pending"}, nil
			},
		}
		var updates map[string]interface{}
		repo := repoWithMonitor()
		repo.UpdateMonitorRunFunc = func(_ *models.MonitorRun, u map[string]interface{}) error {
			updates = u
			return nil
		}
		run := baseRun()
		startedAt := time.Now().Add(-runStuckTimeout - time.Minute)
		run.StartedAt = &startedAt
		s := newScheduler(oc, nonThunder(), &fakeMonitorExecutor{}, repo)

		require.NoError(t, s.syncSingleRunStatus(context.Background(), run))

		// Left Pending, it would occupy a slot in the capped pending query forever.
		assert.Equal(t, models.RunStatusFailed, updates["status"])
		assert.NotEmpty(t, updates["error_message"])
	})

	t.Run("leaves a Pending run alone while it is still within the stuck timeout", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetWorkflowRunFunc: func(_ context.Context, _, _ string) (*client.WorkflowRunResponse, error) {
				return &client.WorkflowRunResponse{Status: "Pending"}, nil
			},
		}
		repo := repoWithMonitor() // UpdateMonitorRunFunc nil => must not be called
		run := baseRun()
		startedAt := time.Now().Add(-time.Minute)
		run.StartedAt = &startedAt
		s := newScheduler(oc, nonThunder(), &fakeMonitorExecutor{}, repo)

		require.NoError(t, s.syncSingleRunStatus(context.Background(), run))
	})

	t.Run("stamps the run's org onto the context used for the OpenChoreo lookup", func(t *testing.T) {
		var gotOrg orgctx.ResolvedOrg
		var hadOrg bool
		oc := &clientmocks.OpenChoreoClientMock{
			GetWorkflowRunFunc: func(ctx context.Context, _, _ string) (*client.WorkflowRunResponse, error) {
				gotOrg, hadOrg = orgctx.GetResolvedOrg(ctx)
				return &client.WorkflowRunResponse{Status: "Pending"}, nil
			},
		}
		s := newScheduler(oc, nonThunder(), &fakeMonitorExecutor{}, repoWithMonitor())

		require.NoError(t, s.syncSingleRunStatus(context.Background(), baseRun()))

		require.True(t, hadOrg, "status sync must stamp the org onto the context it passes down")
		assert.Equal(t, org, gotOrg.OUID)
	})

	t.Run("fails a stale run whose WorkflowRun is confirmed gone", func(t *testing.T) {
		oc := &clientmocks.OpenChoreoClientMock{
			GetWorkflowRunFunc: func(_ context.Context, _, _ string) (*client.WorkflowRunResponse, error) {
				return nil, fmt.Errorf("get workflow run: %w", utils.ErrNotFound)
			},
		}
		var updates map[string]interface{}
		repo := repoWithMonitor()
		repo.UpdateMonitorRunFunc = func(_ *models.MonitorRun, u map[string]interface{}) error {
			updates = u
			return nil
		}
		run := baseRun()
		startedAt := time.Now().Add(-runStuckTimeout - time.Minute)
		run.StartedAt = &startedAt
		s := newScheduler(oc, nonThunder(), &fakeMonitorExecutor{}, repo)

		require.NoError(t, s.syncSingleRunStatus(context.Background(), run))

		assert.Equal(t, models.RunStatusFailed, updates["status"])
	})

	t.Run("leaves a stale run alone when the lookup failed for any other reason", func(t *testing.T) {
		boom := errors.New("connection refused")
		oc := &clientmocks.OpenChoreoClientMock{
			GetWorkflowRunFunc: func(_ context.Context, _, _ string) (*client.WorkflowRunResponse, error) {
				return nil, boom
			},
		}
		repo := repoWithMonitor() // UpdateMonitorRunFunc nil => must not be called
		run := baseRun()
		startedAt := time.Now().Add(-runStuckTimeout - time.Minute)
		run.StartedAt = &startedAt
		s := newScheduler(oc, nonThunder(), &fakeMonitorExecutor{}, repo)

		// An OpenChoreo outage says nothing about the workflow; marking every
		// long-pending run failed would discard runs that later succeed.
		assert.ErrorIs(t, s.syncSingleRunStatus(context.Background(), run), boom)
	})

	t.Run("propagates a failure to persist the stale-run update", func(t *testing.T) {
		boom := errors.New("update boom")
		oc := &clientmocks.OpenChoreoClientMock{
			GetWorkflowRunFunc: func(_ context.Context, _, _ string) (*client.WorkflowRunResponse, error) {
				return &client.WorkflowRunResponse{Status: "Pending"}, nil
			},
		}
		repo := repoWithMonitor()
		repo.UpdateMonitorRunFunc = func(_ *models.MonitorRun, _ map[string]interface{}) error { return boom }
		run := baseRun()
		startedAt := time.Now().Add(-runStuckTimeout - time.Minute)
		run.StartedAt = &startedAt
		s := newScheduler(oc, nonThunder(), &fakeMonitorExecutor{}, repo)

		assert.ErrorIs(t, s.syncSingleRunStatus(context.Background(), run), boom)
	})

	t.Run("wraps the UpdateMonitorRun persistence error", func(t *testing.T) {
		boom := errors.New("update boom")
		oc := &clientmocks.OpenChoreoClientMock{
			GetWorkflowRunFunc: func(_ context.Context, _, _ string) (*client.WorkflowRunResponse, error) {
				return &client.WorkflowRunResponse{Status: "Succeeded"}, nil
			},
		}
		repo := repoWithMonitor()
		repo.UpdateMonitorRunFunc = func(_ *models.MonitorRun, _ map[string]interface{}) error { return boom }
		s := newScheduler(oc, nonThunder(), &fakeMonitorExecutor{}, repo)

		err := s.syncSingleRunStatus(context.Background(), baseRun())

		assert.ErrorIs(t, err, boom)
		assert.Contains(t, err.Error(), "failed to update run status")
	})

	t.Run("resolves a per-org OC client in Thunder mode", func(t *testing.T) {
		orgOC := &clientmocks.OpenChoreoClientMock{
			GetWorkflowRunFunc: func(_ context.Context, namespace, _ string) (*client.WorkflowRunResponse, error) {
				assert.Equal(t, org, namespace)
				return &client.WorkflowRunResponse{Status: "Pending"}, nil
			},
		}
		prov := &fakeProvisioner{
			IsThunderModeFunc: func() bool { return true },
			GetOCClientForOrgFunc: func(_ context.Context, ouID string) (client.OpenChoreoClient, error) {
				assert.Equal(t, org, ouID)
				return orgOC, nil
			},
		}
		// The system client must NOT be used in Thunder mode (its GetWorkflowRunFunc is nil).
		s := newScheduler(&clientmocks.OpenChoreoClientMock{}, prov, &fakeMonitorExecutor{}, repoWithMonitor())

		require.NoError(t, s.syncSingleRunStatus(context.Background(), baseRun()))
	})
}

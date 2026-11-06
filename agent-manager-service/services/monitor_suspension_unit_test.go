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
	"github.com/stretchr/testify/require"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/workloadpolicy"
)

func TestMonitorScheduler_SuspensionPreservesSchedule(t *testing.T) {
	scheduler := NewMonitorSchedulerService(&clientmocks.OpenChoreoClientMock{}, &fakeProvisioner{}, discardLogger(), &fakeMonitorExecutor{}, &repomocks.MonitorRepositoryMock{}, NoopMonitorRunObserver{}).(*monitorSchedulerService)
	due := time.Now().Add(-time.Hour)
	interval := 15
	monitor := &models.Monitor{OUID: "org-a", Name: "scheduled", IntervalMinutes: &interval, NextRunTime: &due}
	ctx := workloadpolicy.WithGuard(context.Background(), func(_ context.Context, org string) error {
		require.Equal(t, "org-a", org)
		return workloadpolicy.ErrSuspended
	})
	require.NoError(t, scheduler.triggerMonitor(ctx, monitor))
	require.Equal(t, due, *monitor.NextRunTime)
	require.Equal(t, 15, *monitor.IntervalMinutes)
}

func TestMonitorScheduler_AdmissionFailureDoesNotRun(t *testing.T) {
	scheduler := NewMonitorSchedulerService(&clientmocks.OpenChoreoClientMock{}, &fakeProvisioner{}, discardLogger(), &fakeMonitorExecutor{}, &repomocks.MonitorRepositoryMock{}, NoopMonitorRunObserver{}).(*monitorSchedulerService)
	unavailable := errors.New("lifecycle unavailable")
	ctx := workloadpolicy.WithGuard(context.Background(), func(context.Context, string) error { return unavailable })
	require.ErrorIs(t, scheduler.triggerMonitor(ctx, &models.Monitor{OUID: "org-a"}), unavailable)
}

func TestMonitorExecutor_SuspensionBeforeLLMOrWorkflow(t *testing.T) {
	executor := NewMonitorExecutor(nil, discardLogger(), nil, nil, nil, nil, nil, nil, nil, nil)
	ctx := workloadpolicy.WithGuard(context.Background(), func(context.Context, string) error { return workloadpolicy.ErrSuspended })
	_, err := executor.ExecuteMonitorRun(ctx, ExecuteMonitorRunParams{OUID: "org-a", Monitor: &models.Monitor{ID: uuid.New()}})
	require.ErrorIs(t, err, workloadpolicy.ErrSuspended)
}

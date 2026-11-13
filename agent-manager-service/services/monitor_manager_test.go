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

// UNIT tests for monitorManagerService.GetMonitorRunLogs. Run logs came from
// the evaluation workflow runtime, which is gone, so a found run reports
// ErrNotImplemented while lookup failures keep their own sentinels. No build
// tag, runs in the fast unit tier with every collaborator mocked.
package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func TestGetMonitorRunLogs_FoundRunIsNotImplemented(t *testing.T) {
	monitor := &models.Monitor{ID: uuid.New(), Name: "monitor"}
	run := &models.MonitorRun{ID: uuid.New(), MonitorID: monitor.ID, Name: "workflowrun-abc123"}
	monitorRepo := &repomocks.MonitorRepositoryMock{
		GetMonitorByNameFunc: func(_, _, _, _ string) (*models.Monitor, error) {
			return monitor, nil
		},
		GetMonitorRunByIDFunc: func(_, _ uuid.UUID) (*models.MonitorRun, error) {
			return run, nil
		},
	}
	s := &monitorManagerService{logger: discardLogger(), monitorRepo: monitorRepo}

	_, err := s.GetMonitorRunLogs(context.Background(), "org", "org-handle", "proj", "agent", "monitor", run.ID.String())
	if !errors.Is(err, utils.ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
}

func TestGetMonitorRunLogs_MonitorNotFoundMapsToErrMonitorNotFound(t *testing.T) {
	monitorRepo := &repomocks.MonitorRepositoryMock{
		GetMonitorByNameFunc: func(_, _, _, _ string) (*models.Monitor, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	s := &monitorManagerService{logger: discardLogger(), monitorRepo: monitorRepo}

	_, err := s.GetMonitorRunLogs(context.Background(), "org", "org-handle", "proj", "agent", "monitor", uuid.New().String())
	if !errors.Is(err, utils.ErrMonitorNotFound) {
		t.Fatalf("expected ErrMonitorNotFound, got %v", err)
	}
}

func TestGetMonitorRunLogs_RunNotFoundMapsToErrMonitorRunNotFound(t *testing.T) {
	monitor := &models.Monitor{ID: uuid.New(), Name: "monitor"}
	monitorRepo := &repomocks.MonitorRepositoryMock{
		GetMonitorByNameFunc: func(_, _, _, _ string) (*models.Monitor, error) {
			return monitor, nil
		},
		GetMonitorRunByIDFunc: func(_, _ uuid.UUID) (*models.MonitorRun, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	s := &monitorManagerService{logger: discardLogger(), monitorRepo: monitorRepo}

	_, err := s.GetMonitorRunLogs(context.Background(), "org", "org-handle", "proj", "agent", "monitor", uuid.New().String())
	if !errors.Is(err, utils.ErrMonitorRunNotFound) {
		t.Fatalf("expected ErrMonitorRunNotFound, got %v", err)
	}
}

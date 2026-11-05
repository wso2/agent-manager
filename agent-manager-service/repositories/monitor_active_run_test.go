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

package repositories

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func TestMonitorRepo_HasPendingOrRunningRun(t *testing.T) {
	gdb := db.GetDB()
	repo := NewMonitorRepo(gdb)

	newMonitor := func(t *testing.T) uuid.UUID {
		t.Helper()
		interval := 10
		m := &models.Monitor{
			Name: "active-run-" + uuid.NewString()[:8], Type: models.MonitorTypeFuture,
			OUID: "ou-" + uuid.NewString()[:8], ProjectName: "p", AgentName: "a", AgentID: "a-id",
			EnvironmentName: "e", EnvironmentID: "e-id", IntervalMinutes: &interval,
		}
		require.NoError(t, repo.CreateMonitor(m))
		t.Cleanup(func() {
			gdb.Exec(`DELETE FROM monitor_runs WHERE monitor_id = ?`, m.ID)
			gdb.Exec(`DELETE FROM monitors WHERE id = ?`, m.ID)
		})
		return m.ID
	}
	addRun := func(t *testing.T, monitorID uuid.UUID, status string) {
		t.Helper()
		now := time.Now()
		require.NoError(t, repo.CreateMonitorRun(&models.MonitorRun{
			MonitorID: monitorID, Name: "run-" + uuid.NewString()[:8],
			TraceStart: now.Add(-time.Hour), TraceEnd: now, Status: status,
		}))
	}

	t.Run("false with no runs", func(t *testing.T) {
		active, err := repo.HasPendingOrRunningRun(newMonitor(t))
		require.NoError(t, err)
		assert.False(t, active)
	})

	t.Run("false when every run has finished", func(t *testing.T) {
		id := newMonitor(t)
		addRun(t, id, models.RunStatusSuccess)
		addRun(t, id, models.RunStatusFailed)

		active, err := repo.HasPendingOrRunningRun(id)
		require.NoError(t, err)
		assert.False(t, active)
	})

	t.Run("true for a pending or running run", func(t *testing.T) {
		for _, status := range []string{models.RunStatusPending, models.RunStatusRunning} {
			id := newMonitor(t)
			addRun(t, id, models.RunStatusSuccess)
			addRun(t, id, status)

			active, err := repo.HasPendingOrRunningRun(id)
			require.NoError(t, err)
			assert.True(t, active, status)
		}
	})

	t.Run("ignores another monitor's active run", func(t *testing.T) {
		busy, idle := newMonitor(t), newMonitor(t)
		addRun(t, busy, models.RunStatusRunning)

		active, err := repo.HasPendingOrRunningRun(idle)
		require.NoError(t, err)
		assert.False(t, active)
	})
}

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

//go:build integration

package repositories

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func TestAgentVariableOwnership_SerializesAndReadsCommittedNames(t *testing.T) {
	database := db.GetDB()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// A private schema keeps the test independent of application fixtures and migrations.
	schema := "rename_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, database.WithContext(ctx).Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() { database.Exec("DROP SCHEMA " + schema + " CASCADE") })
	require.NoError(t, database.Exec("CREATE TABLE "+schema+".agent_configurations (uuid uuid PRIMARY KEY, ou_id text, project_name text, agent_id text)").Error)
	require.NoError(t, database.Exec("CREATE TABLE "+schema+".agent_env_config_variables_mapping (config_uuid uuid, variable_name text)").Error)
	configID := uuid.New()
	require.NoError(t, database.Exec("INSERT INTO "+schema+".agent_configurations VALUES (?, 'org', 'project', 'agent')", configID).Error)
	require.NoError(t, database.Exec("INSERT INTO "+schema+".agent_env_config_variables_mapping VALUES (?, 'OLD_URL')", configID).Error)
	begin := func() *gorm.DB {
		tx := database.WithContext(ctx).Begin()
		require.NoError(t, tx.Error)
		t.Cleanup(func() { tx.Rollback() })
		require.NoError(t, tx.Exec("SET LOCAL search_path TO "+schema).Error)
		return tx
	}
	repo := NewAgentEnvConfigVariableRepository(database)
	first := begin()
	rows, err := repo.LockAndListByAgent(ctx, first, "org", "project", "agent")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "OLD_URL", rows[0].VariableName)
	// Different agents, projects and organizations neither contend nor leak ownership.
	for _, scope := range [][3]string{{"other", "project", "agent"}, {"org", "other", "agent"}, {"org", "project", "other"}} {
		other := begin()
		scopedCtx, scopedCancel := context.WithTimeout(ctx, time.Second)
		otherRows, otherErr := repo.LockAndListByAgent(scopedCtx, other, scope[0], scope[1], scope[2])
		scopedCancel()
		require.NoError(t, otherErr)
		require.Empty(t, otherRows)
		require.NoError(t, other.Rollback().Error)
	}
	second := begin()
	var pid int
	require.NoError(t, second.Raw("SELECT pg_backend_pid()").Scan(&pid).Error)
	type outcome struct {
		rows []models.AgentEnvConfigVariable
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		got, readErr := repo.LockAndListByAgent(ctx, second, "org", "project", "agent")
		done <- outcome{rows: got, err: readErr}
	}()
	require.Eventually(t, func() bool {
		var waiting bool
		queryErr := database.WithContext(ctx).Raw("SELECT EXISTS (SELECT 1 FROM pg_locks WHERE pid = ? AND locktype = 'advisory' AND NOT granted)", pid).Scan(&waiting).Error
		return queryErr == nil && waiting
	}, 3*time.Second, 10*time.Millisecond, "competing rename must wait for the owner transaction")
	require.NoError(t, first.Exec("UPDATE agent_env_config_variables_mapping SET variable_name = 'CLAIMED_URL' WHERE config_uuid = ?", configID).Error)
	require.NoError(t, first.Commit().Error)
	select {
	case got := <-done:
		require.NoError(t, got.err)
		require.Len(t, got.rows, 1)
		require.Equal(t, "CLAIMED_URL", got.rows[0].VariableName, "ownership read must see the winning rename, not the pre-lock snapshot")
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.NoError(t, second.Rollback().Error)
}

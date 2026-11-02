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

package dbmigrations

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wso2/agent-manager/agent-manager-service/db"
)

func TestEventOutboxMigrationPreservesRuns(t *testing.T) {
	database := db.GetDB()
	schema := "migration_" + uuid.New().String()[:8]
	require.NoError(t, database.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() { assert.NoError(t, database.Exec("DROP SCHEMA "+schema+" CASCADE").Error) })
	tx := database.Begin()
	defer tx.Rollback()
	require.NoError(t, tx.Exec("SET LOCAL search_path TO "+schema+", public").Error)
	require.NoError(t, tx.Exec(`CREATE TABLE monitor_runs(id TEXT PRIMARY KEY,status TEXT,completed_at TIMESTAMPTZ);
 INSERT INTO monitor_runs(id,status) VALUES ('old-success','success'),('old-failed','failed'),('running','running')`).Error)
	require.NoError(t, migration054.Migrate(tx))
	var marked int64
	require.NoError(t, tx.Table("monitor_runs").Where("event_queued_at IS NOT NULL").Count(&marked).Error)
	assert.EqualValues(t, 2, marked, "old terminal runs are not replayed")
	require.NoError(t, tx.Exec("INSERT INTO monitor_runs(id,status) VALUES ('new-success','success')").Error)
	var pending int64
	require.NoError(t, tx.Table("monitor_runs").Where("event_queued_at IS NULL").Count(&pending).Error)
	assert.EqualValues(t, 2, pending, "new outcomes and in-flight runs remain replayable")
	require.NoError(t, tx.Exec("INSERT INTO event_outbox(id,body) VALUES ('event','{}')").Error)
}

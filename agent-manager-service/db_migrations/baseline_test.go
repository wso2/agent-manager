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
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db/dberr"
	"github.com/wso2/agent-manager/agent-manager-service/db_migrations/schema"
)

func TestSplitStatements(t *testing.T) {
	body := "-- header\n\nCREATE TABLE a (\n    id TEXT\n);\nCREATE INDEX i ON a (id);\n-- trailing comment\n"
	assert.Equal(t, []string{"CREATE TABLE a (\n    id TEXT\n)", "CREATE INDEX i ON a (id)"}, splitStatements(body))
}

func TestEveryDialectHasTheSameMigrations(t *testing.T) {
	var ids [][]string
	for _, dialect := range schema.Dialects {
		list, err := sqlMigrationsFor(dialect)
		require.NoError(t, err)
		var got []string
		for _, m := range list {
			got = append(got, m.ID)
		}
		ids = append(ids, got)
	}
	for _, got := range ids[1:] {
		assert.Equal(t, ids[0], got, "schema rule 11: same versions on every database")
	}
	assert.Equal(t, "001", ids[0][0])
}

func openMigratedSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "amp.db") + "?_pragma=foreign_keys(1)&_time_format=sqlite"
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	require.NoError(t, MigrateBaseline(gormDB, config.DBDriverSQLite))
	return gormDB
}

func TestSQLiteBaselineAppliesOnce(t *testing.T) {
	gormDB := openMigratedSQLite(t)
	require.NoError(t, MigrateBaseline(gormDB, config.DBDriverSQLite), "a second run must be a no-op")

	var tables int64
	require.NoError(t, gormDB.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name NOT IN ('migration_history')").Scan(&tables).Error)
	s, err := schema.Lite()
	require.NoError(t, err)
	assert.EqualValues(t, len(s.Tables), tables)

	var locks []string
	require.NoError(t, gormDB.Raw("SELECT name FROM locks ORDER BY name").Scan(&locks).Error)
	assert.Equal(t, []string{"agent_identity.reconciler", "gateway.drift_scan"}, locks)
}

func TestSQLiteBaselineEnforcesKeys(t *testing.T) {
	assertBaselineKeys(t, openMigratedSQLite(t))
}

// assertBaselineKeys checks the keys behave as the schema design says on any database: cascades follow
// org › project › agent, and a child can never point at another org's parent.
func assertBaselineKeys(t *testing.T, gormDB *gorm.DB) {
	t.Helper()
	now := time.Now().UTC()
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	exec := func(sql string, args ...any) error { return gormDB.Exec(sql, args...).Error }

	orgA, orgB, project, agent := id(), id(), id(), id()
	for handle, org := range map[string]string{"org-a": orgA, "org-b": orgB} {
		require.NoError(t, exec("INSERT INTO organizations (id, handle, display_name, created_at, updated_at) VALUES (?, ?, 'Org', ?, ?)", org, handle, now, now))
	}
	require.NoError(t, exec("INSERT INTO projects (id, org_id, name, display_name, created_at, updated_at) VALUES (?, ?, 'p', 'P', ?, ?)", project, orgA, now, now))
	insertAgent := func(agentID, orgID, name string) error {
		return exec(`INSERT INTO agents (id, org_id, project_id, name, display_name, provisioning_type, agent_type, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'A', 'external', 'external-agent-api', ?, ?)`, agentID, orgID, project, name, now, now)
	}
	require.NoError(t, insertAgent(agent, orgA, "a"))

	assert.True(t, dberr.IsForeignKeyViolation(insertAgent(id(), orgB, "b")), "an agent cannot sit in another org's project")
	assert.True(t, dberr.IsDuplicateKey(insertAgent(id(), orgA, "a")), "agent names are unique per org")
	assert.Error(t, exec("INSERT INTO agents (id, org_id, project_id, name, display_name, provisioning_type, agent_type, created_at, updated_at) VALUES (?, ?, ?, 'c', 'C', 'hosted', 'x', ?, ?)", id(), orgA, project, now, now), "check constraint")

	require.NoError(t, exec("INSERT INTO agent_labels (agent_id, org_id, label_key, label_value) VALUES (?, ?, 'team', 'core')", agent, orgA))
	assert.True(t, dberr.IsForeignKeyViolation(exec("DELETE FROM projects WHERE id = ?", project)), "a project with agents cannot be deleted")
	require.NoError(t, exec("DELETE FROM agents WHERE id = ?", agent))
	var labels int64
	require.NoError(t, gormDB.Raw("SELECT count(*) FROM agent_labels").Scan(&labels).Error)
	assert.Zero(t, labels, "labels cascade from the agent")

	insertGateway := func(properties string) error {
		return exec(`INSERT INTO gateways (id, org_id, name, display_name, functionality_type, hosting, vhost, is_critical, is_active, properties, created_at, updated_at)
			VALUES (?, ?, ?, 'G', 'both', 'managed', 'h', ?, ?, ?, ?, ?)`, id(), orgA, id(), false, true, properties, now, now)
	}
	require.NoError(t, insertGateway(`{"replicas": 1}`))
	assert.Error(t, insertGateway("not json"), "JSON is validated")
}

func TestWaitForBaseline(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "amp.db") + "?_pragma=foreign_keys(1)&_time_format=sqlite"
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err = waitFor(ctx, gormDB, "001", 20*time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "latest applied: none")

	require.NoError(t, MigrateBaseline(gormDB, config.DBDriverSQLite))
	assert.NoError(t, waitFor(context.Background(), gormDB, "001", 20*time.Millisecond))
}

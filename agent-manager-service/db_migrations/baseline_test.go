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
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db_migrations/schema"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dberr"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/sqlitedialect"
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
	dsn := "file:" + filepath.Join(t.TempDir(), "amp.db") + "?_pragma=foreign_keys(1)&_txlock=immediate"
	gormDB, err := gorm.Open(sqlitedialect.Open(dsn), &gorm.Config{Logger: logger.Discard})
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
	assert.Equal(t, []string{"agent_identity.reconciler"}, locks)
}

// The organization is the token's: the schema keeps no table of them, and everything is in the one baseline.
func TestSQLiteBaselineHasNoOrganizationsTable(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "amp.db") + "?_pragma=foreign_keys(1)&_txlock=immediate"
	gormDB, err := gorm.Open(sqlitedialect.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	list, err := sqlMigrationsFor(config.DBDriverSQLite)
	require.NoError(t, err)
	require.Len(t, list, 1, "the baseline is the only migration")

	require.NoError(t, MigrateBaseline(gormDB, config.DBDriverSQLite))
	assert.False(t, gormDB.Migrator().HasTable("organizations"))
	assert.True(t, gormDB.Migrator().HasTable("secret_jobs"))
}

// A database created before organizations left the schema still has the table and the foreign keys to it, which
// every new organization's first write would trip over; migrating refuses it with a clear reason instead.
func TestSQLiteMigrateRefusesADatabaseWithOrganizations(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "amp.db") + "?_pragma=foreign_keys(1)&_txlock=immediate"
	gormDB, err := gorm.Open(sqlitedialect.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	require.NoError(t, gormDB.Exec("CREATE TABLE organizations (id TEXT PRIMARY KEY)").Error)

	err = MigrateBaseline(gormDB, config.DBDriverSQLite)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reset")
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
	require.NoError(t, exec("INSERT INTO projects (id, org_id, name, display_name, created_at, updated_at) VALUES (?, ?, 'p', 'P', ?, ?)", project, orgA, now, now))
	insertAgent := func(agentID, orgID, name string) error {
		return exec(`INSERT INTO agents (id, org_id, project_id, name, display_name, provisioning_type, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'A', 'external', ?, ?)`, agentID, orgID, project, name, now, now)
	}
	require.NoError(t, insertAgent(agent, orgA, "a"))

	assert.True(t, dberr.IsForeignKeyViolation(insertAgent(id(), orgB, "b")), "an agent cannot sit in another org's project")
	assert.True(t, dberr.IsDuplicateKey(insertAgent(id(), orgA, "a")), "agent names are unique per org")
	assert.Error(t, exec("INSERT INTO agents (id, org_id, project_id, name, display_name, provisioning_type, created_at, updated_at) VALUES (?, ?, ?, 'c', 'C', 'hosted', ?, ?)", id(), orgA, project, now, now), "check constraint")

	require.NoError(t, exec("INSERT INTO agent_labels (agent_id, org_id, label_key, label_value) VALUES (?, ?, 'team', 'core')", agent, orgA))
	assert.True(t, dberr.IsForeignKeyViolation(exec("DELETE FROM projects WHERE id = ?", project)), "a project with agents cannot be deleted")
	require.NoError(t, exec("DELETE FROM agents WHERE id = ?", agent))
	var labels int64
	require.NoError(t, gormDB.Raw("SELECT count(*) FROM agent_labels").Scan(&labels).Error)
	assert.Zero(t, labels, "labels cascade from the agent")

	insertUsage := func(dimensions string) error {
		return exec(`INSERT INTO usage_events (id, org_id, occurred_at, source, feature_code, product_version, dimensions)
			VALUES (?, ?, ?, 'api', 'f', 'v', ?)`, id(), orgA, now, dimensions)
	}
	require.NoError(t, insertUsage(`{"replicas": 1}`))
	assert.Error(t, insertUsage("not json"), "JSON is validated")
}

func TestWaitForBaseline(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "amp.db") + "?_pragma=foreign_keys(1)&_txlock=immediate"
	gormDB, err := gorm.Open(sqlitedialect.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err = waitFor(ctx, gormDB, "001", 20*time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Contains(t, err.Error(), "latest applied: none")

	require.NoError(t, MigrateBaseline(gormDB, config.DBDriverSQLite))
	assert.NoError(t, waitFor(context.Background(), gormDB, "001", 20*time.Millisecond))
}

func TestHasHistory(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "amp.db") + "?_pragma=foreign_keys(1)&_txlock=immediate"
	gormDB, err := gorm.Open(sqlitedialect.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	ctx := context.Background()

	has, err := HasHistory(ctx, gormDB)
	require.NoError(t, err)
	assert.False(t, has, "an empty database is a new install")

	require.NoError(t, MigrateBaseline(gormDB, config.DBDriverSQLite))
	has, err = HasHistory(ctx, gormDB)
	require.NoError(t, err)
	assert.True(t, has)
}

// A failed catalog query is an error, never "not migrated": the server would
// otherwise treat an existing install as new.
func TestHasHistoryReportsCatalogErrors(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "amp.db") + "?_pragma=foreign_keys(1)&_txlock=immediate"
	gormDB, err := gorm.Open(sqlitedialect.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	require.NoError(t, MigrateBaseline(gormDB, config.DBDriverSQLite))
	failCatalog := func(db *gorm.DB) {
		if strings.Contains(db.Statement.SQL.String(), "sqlite_master") {
			_ = db.AddError(errors.New("catalog unavailable"))
		}
	}
	require.NoError(t, gormDB.Callback().Row().Before("gorm:row").Register("test:fail_catalog_row", failCatalog))
	require.NoError(t, gormDB.Callback().Raw().Before("gorm:raw").Register("test:fail_catalog_raw", failCatalog))
	require.NoError(t, gormDB.Callback().Query().Before("gorm:query").Register("test:fail_catalog_query", failCatalog))

	_, err = HasHistory(context.Background(), gormDB)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "catalog unavailable")
}

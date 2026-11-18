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

package aigateway

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/wso2/agent-manager/agent-manager-service/db/sqlitedialect"
	dbmigrations "github.com/wso2/agent-manager/agent-manager-service/db_migrations"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// migrated is a scratch SQLite database with the core's baseline and the
// module's migrations.
func migrated(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + filepath.Join(t.TempDir(), "amp.db") + "?_pragma=foreign_keys(1)&_txlock=immediate&_time_format=sqlite"
	g, err := gorm.Open(sqlitedialect.Open(dsn), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := g.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, dbmigrations.MigrateBaseline(g, "sqlite"))
	_, err = modhost.NewWith(g, "sqlite", slog.New(slog.DiscardHandler), []modkit.Module{module{}}).Migrate(context.Background())
	require.NoError(t, err)
	return g
}

// TestFinalizeHooksCoverEveryReference is the finalize contract: every
// module table that names an agent or an environment is cleaned by the hook,
// directly or through a foreign key that cascades from a table it cleans, so
// no module row is left pointing at a deleted core row.
func TestFinalizeHooksCoverEveryReference(t *testing.T) {
	g := migrated(t)
	var tables []string
	require.NoError(t, g.Raw(`SELECT name FROM sqlite_master WHERE type = 'table' AND name LIKE 'mod_ai_gateway_%'`).Scan(&tables).Error)
	require.Len(t, tables, 30)
	cascadesFrom := func(table string) []string {
		var fks []struct {
			Table    string `gorm:"column:table"`
			OnDelete string `gorm:"column:on_delete"`
		}
		require.NoError(t, g.Raw(`SELECT "table", on_delete FROM pragma_foreign_key_list(?)`, table).Scan(&fks).Error)
		var out []string
		for _, fk := range fks {
			if fk.OnDelete == "CASCADE" {
				out = append(out, fk.Table)
			}
		}
		return out
	}
	var covered func(table string, cleaned []string) bool
	covered = func(table string, cleaned []string) bool {
		if slices.Contains(cleaned, table) {
			return true
		}
		return slices.ContainsFunc(cascadesFrom(table), func(parent string) bool { return covered(parent, cleaned) })
	}
	for _, table := range tables {
		var columns []string
		require.NoError(t, g.Raw(`SELECT name FROM pragma_table_info(?)`, table).Scan(&columns).Error)
		if slices.Contains(columns, "agent_id") {
			assert.True(t, covered(table, agentTables), "%s names an agent but the agent finalize hook leaves its rows", table)
		}
		if slices.Contains(columns, "environment_id") {
			assert.True(t, covered(table, environmentTables), "%s names an environment but the environment finalize hook leaves its rows", table)
		}
		if slices.Contains(columns, "project_id") {
			assert.True(t, slices.ContainsFunc(projectTables, func(p struct{ table, what string }) bool { return p.table == table }),
				"%s names a project but the project finalize hook does not guard it", table)
		}
	}
}

func TestFinalizeHooks(t *testing.T) {
	g := migrated(t)
	ctx := context.Background()
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	now := time.Now().UTC()
	exec := func(sql string, args ...any) { require.NoError(t, g.Exec(sql, args...).Error) }
	count := func(sql string, args ...any) int64 {
		var n int64
		require.NoError(t, g.Raw(sql, args...).Scan(&n).Error)
		return n
	}
	org, project, agent, env := id(), id(), id(), id()
	exec(`INSERT INTO organizations (id, handle, display_name, created_at, updated_at) VALUES (?, 'o', 'O', ?, ?)`, org, now, now)
	exec(`INSERT INTO projects (id, org_id, name, display_name, created_at, updated_at) VALUES (?, ?, 'p', 'P', ?, ?)`, project, org, now, now)
	exec(`INSERT INTO environments (id, org_id, name, display_name, is_production, created_at, updated_at) VALUES (?, ?, 'e', 'E', ?, ?, ?)`, env, org, false, now, now)

	t.Run("agent", func(t *testing.T) {
		exec(`INSERT INTO mod_ai_gateway_agent_gateway_settings (agent_id, org_id, environment_id, api_key_security, oauth_enabled, oauth_issuers,
			oauth_audiences, oauth_header_name, oauth_header_prefix, oauth_forward_token, cors_enabled, cors_allow_origins, cors_allow_methods,
			cors_allow_headers, cors_allow_credentials, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, '[]', '[]', 'h', 'p', ?, ?, '[]', '[]', '[]', ?, ?, ?)`,
			agent, org, env, true, false, false, true, false, now, now)
		require.NoError(t, g.Transaction(func(tx *gorm.DB) error {
			return finalizeAgent(ctx, modkit.HookRequest{Tx: tx, OrgID: org, ID: agent})
		}))
		assert.Zero(t, count(`SELECT COUNT(*) FROM mod_ai_gateway_agent_gateway_settings WHERE agent_id = ?`, agent))
	})

	t.Run("project", func(t *testing.T) {
		req := modkit.HookRequest{OrgID: org, ID: project, Object: map[string]any{"name": "p"}}
		require.NoError(t, g.Transaction(func(tx *gorm.DB) error { req.Tx = tx; return finalizeProject(ctx, req) }))
		// A proxy of the project refuses the delete, as the foreign key did.
		exec(`PRAGMA foreign_keys = OFF`)
		exec(`INSERT INTO mod_ai_gateway_llm_proxies (id, org_id, project_id, provider_id, status, configuration) VALUES (?, ?, ?, ?, 'CREATED', '{}')`,
			id(), org, project, id())
		exec(`PRAGMA foreign_keys = ON`)
		err := g.Transaction(func(tx *gorm.DB) error { req.Tx = tx; return finalizeProject(ctx, req) })
		var refused *modkit.Error
		require.True(t, errors.As(err, &refused), "%v", err)
		assert.Equal(t, http.StatusConflict, refused.Status)
		assert.Contains(t, refused.Message, "1 LLM proxy(ies)")
	})

	t.Run("environment", func(t *testing.T) {
		gateway, artifact := id(), id()
		exec(`INSERT INTO mod_ai_gateway_gateways (id, org_id, name, display_name, functionality_type, hosting, vhost, is_critical, is_active,
			properties, created_at, updated_at) VALUES (?, ?, 'gw', 'GW', 'both', 'managed', 'h', ?, ?, '{}', ?, ?)`, gateway, org, false, true, now, now)
		exec(`INSERT INTO mod_ai_gateway_gateway_environments (gateway_id, environment_id, org_id, created_at) VALUES (?, ?, ?, ?)`, gateway, env, org, now)
		exec(`INSERT INTO mod_ai_gateway_artifacts (id, org_id, kind, handle, name, version, in_catalog, created_at, updated_at)
			VALUES (?, ?, 'LlmProvider', 'h', 'n', 'v1', ?, ?, ?)`, artifact, org, false, now, now)
		exec(`INSERT INTO mod_ai_gateway_artifact_gateway_status (artifact_id, gateway_id, org_id, status, status_desired, performed_at, attempt_count, updated_at)
			VALUES (?, ?, ?, 'PENDING', 'DEPLOYED', ?, 0, ?)`, artifact, gateway, org, now, now)
		req := modkit.HookRequest{OrgID: org, ID: env, Object: map[string]any{"name": "e"}}
		err := g.Transaction(func(tx *gorm.DB) error { req.Tx = tx; return finalizeEnvironment(ctx, req) })
		var refused *modkit.Error
		require.True(t, errors.As(err, &refused), "a gateway of the environment serves an artifact: %v", err)
		assert.Equal(t, http.StatusConflict, refused.Status)

		exec(`DELETE FROM mod_ai_gateway_artifact_gateway_status`)
		require.NoError(t, g.Transaction(func(tx *gorm.DB) error { req.Tx = tx; return finalizeEnvironment(ctx, req) }))
		assert.Zero(t, count(`SELECT COUNT(*) FROM mod_ai_gateway_gateway_environments WHERE environment_id = ?`, env), "its mappings go")
		assert.EqualValues(t, 1, count(`SELECT COUNT(*) FROM mod_ai_gateway_gateways WHERE id = ?`, gateway), "the gateway stays")
	})
}

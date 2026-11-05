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
)

func TestMigration055AddsNullableProjectUUID(t *testing.T) {
	g := scratchDatabase(t)
	require.NoError(t, migrateTo(g, 54))
	artifact := uuid.New()
	exec(t, g, `INSERT INTO artifacts (uuid, handle, name, version, kind, ou_id, in_catalog)
		VALUES (?, 'org-proxy-055', 'org-proxy-055', 'v1', 'McpProxy', 'org-055', true)`, artifact)
	exec(t, g, `INSERT INTO mcp_proxies (uuid, configuration) VALUES (?, '{}')`, artifact)

	require.NoError(t, migrateTo(g, 55))

	var cols []struct {
		ColumnName string `gorm:"column:column_name"`
		IsNullable string `gorm:"column:is_nullable"`
		DataType   string `gorm:"column:data_type"`
	}
	require.NoError(t, g.Raw(`SELECT column_name, is_nullable, data_type FROM information_schema.columns
		WHERE table_name = 'mcp_proxies' AND column_name IN ('project_uuid', 'project_name')
		ORDER BY column_name`).Scan(&cols).Error)
	require.Len(t, cols, 2)
	assert.Equal(t, "project_name", cols[0].ColumnName)
	assert.Equal(t, "YES", cols[0].IsNullable)
	assert.Equal(t, "project_uuid", cols[1].ColumnName)
	assert.Equal(t, "YES", cols[1].IsNullable)
	assert.Equal(t, "uuid", cols[1].DataType)

	var row struct {
		ProjectUUID *string `gorm:"column:project_uuid"`
		ProjectName *string `gorm:"column:project_name"`
	}
	require.NoError(t, g.Table("mcp_proxies").Where("uuid = ?", artifact).Take(&row).Error)
	assert.Nil(t, row.ProjectUUID, "existing proxies stay organization-level")
	assert.Nil(t, row.ProjectName)

	var idx int64
	require.NoError(t, g.Raw(`SELECT count(*) FROM pg_indexes WHERE indexname = 'idx_mcp_proxies_project_uuid'`).Scan(&idx).Error)
	assert.EqualValues(t, 1, idx)

	require.NoError(t, migrateTo(g, 55), "re-running the migration must be a no-op")

	// The two columns are set together or not at all.
	project := uuid.New()
	require.NoError(t, g.Exec(`UPDATE mcp_proxies SET project_uuid = ?, project_name = 'orders' WHERE uuid = ?`,
		project, artifact).Error)
	require.NoError(t, g.Exec(`UPDATE mcp_proxies SET project_uuid = ? WHERE uuid = ?`, uuid.New(), artifact).Error,
		"moving a project proxy to another project UUID keeps its name")
	for name, sql := range map[string]string{
		"uuid without name": `UPDATE mcp_proxies SET project_name = NULL WHERE uuid = ?`,
		"name without uuid": `UPDATE mcp_proxies SET project_uuid = NULL WHERE uuid = ?`,
	} {
		err := g.Exec(sql, artifact).Error
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "chk_mcp_proxies_project_scope", name)
	}
}

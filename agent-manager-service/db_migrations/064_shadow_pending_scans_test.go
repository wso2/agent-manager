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

func TestMigration064AddsPendingScansAndOptions(t *testing.T) {
	g := scratchDatabase(t)
	require.NoError(t, migrateTo(g, 63))
	insert := `INSERT INTO shadow_scans (id, ou_id, status, connection_ids, results, findings_count, requested_by, attempts)
		VALUES (?, 'org-064', ?, '[]', '[]', 0, '', 0)`
	require.Error(t, g.Exec(insert, uuid.New(), "pending").Error, "pending is not a status before 064")
	old := uuid.New()
	exec(t, g, insert, old, "succeeded")

	require.NoError(t, migrateTo(g, 64))

	pending := uuid.New()
	exec(t, g, insert, pending, "pending")
	var options string
	require.NoError(t, g.Raw(`SELECT options::text FROM shadow_scans WHERE id = ?`, old).Scan(&options).Error)
	assert.JSONEq(t, `{}`, options, "existing scans get empty options")
	require.Error(t, g.Exec(insert, uuid.New(), "replaced").Error, "only the known statuses are allowed")
	var idx int64
	require.NoError(t, g.Raw(`SELECT count(*) FROM pg_indexes WHERE indexname = 'idx_shadow_scans_pending'`).Scan(&idx).Error)
	assert.EqualValues(t, 1, idx)

	require.NoError(t, migration064.Migrate(g), "running the migration again must succeed")
	exec(t, g, insert, uuid.New(), "pending")
}

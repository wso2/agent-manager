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

func TestMigration053_PreservesExistingAgentIdentityBindings(t *testing.T) {
	g := scratchDatabase(t)
	require.NoError(t, migrateTo(g, 52))
	id := uuid.New()
	exec(t, g, `INSERT INTO agent_thunder_clients
		(id, ou_id, project_name, agent_name, environment_name, provisioning_type,
		 thunder_agent_id, thunder_client_id, secret_ref_path, status)
		VALUES (?, 'org-053', 'default', 'agent-053', 'development', 'internal',
		 'thunder-agent-053', 'client-053', 'secret-ref-053', 'completed')`, id)

	require.NoError(t, migrateTo(g, 53))
	var row struct {
		ThunderAgentID  string  `gorm:"column:thunder_agent_id"`
		ThunderClientID string  `gorm:"column:thunder_client_id"`
		SecretRefPath   string  `gorm:"column:secret_ref_path"`
		PromotionSource string  `gorm:"column:promotion_source"`
		SyncAfter       *string `gorm:"column:sync_after"`
	}
	require.NoError(t, g.Table("agent_thunder_clients").Where("id = ?", id).Take(&row).Error)
	assert.Equal(t, "thunder-agent-053", row.ThunderAgentID)
	assert.Equal(t, "client-053", row.ThunderClientID)
	assert.Equal(t, "secret-ref-053", row.SecretRefPath)
	assert.Empty(t, row.PromotionSource)
	assert.Nil(t, row.SyncAfter)

	require.NoError(t, migrateTo(g, 53), "re-running the migration must not alter existing bindings")
	var count int64
	require.NoError(t, g.Table("agent_thunder_clients").Where("id = ?", id).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

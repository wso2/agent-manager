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

func TestMigration068DropsCustomInventoryCredentials(t *testing.T) {
	g := scratchDatabase(t)
	require.NoError(t, migrateTo(g, 67))
	custom, openai := uuid.New(), uuid.New()
	insert := `INSERT INTO shadow_connections (id, ou_id, name, provider, config, secret_encrypted, secret_hints,
			last_tested_at, last_test_result)
		VALUES (?, 'org-068', ?, ?, ?::jsonb, '\x01'::bytea, '{"apiKey": "k-12...7890"}', NOW(), '{"checks": []}')`
	exec(t, g, insert, custom, "Inventory", "custom", `{"agentsUrl": "https://agents.example.com/v1/agents", "endpoints": {}}`)
	exec(t, g, insert, openai, "OpenAI", "openai", `{"scopedKeyIds": ["proj_1"]}`)
	agent := uuid.New()
	exec(t, g, `INSERT INTO shadow_findings (id, ou_id, connection_id, provider, kind, external_id)
		VALUES (?, 'org-068', ?, 'custom', 'platform_agent', 'inv-1')`, agent, custom)

	require.NoError(t, migrateTo(g, 68))

	type row struct {
		SecretEncrypted []byte  `gorm:"column:secret_encrypted"`
		SecretHints     string  `gorm:"column:secret_hints"`
		LastTestedAt    *string `gorm:"column:last_tested_at"`
		LastTestResult  *string `gorm:"column:last_test_result"`
		Config          string  `gorm:"column:config"`
		LastUploadAt    *string `gorm:"column:last_upload_at"`
	}
	get := func(id uuid.UUID) row {
		var r row
		require.NoError(t, g.Table("shadow_connections").Where("id = ?", id).Take(&r).Error)
		return r
	}
	c := get(custom)
	assert.Nil(t, c.SecretEncrypted, "the API key is deleted")
	assert.JSONEq(t, `{}`, c.SecretHints)
	assert.Nil(t, c.LastTestedAt)
	assert.Nil(t, c.LastTestResult)
	assert.JSONEq(t, `{"endpoints": {}}`, c.Config, "the Agents URL is deleted")
	assert.Nil(t, c.LastUploadAt, "no file uploaded yet")

	other := get(openai)
	assert.Equal(t, []byte{0x01}, other.SecretEncrypted, "cloud platforms keep their secret")
	assert.JSONEq(t, `{"scopedKeyIds": ["proj_1"]}`, other.Config)
	assert.NotNil(t, other.LastTestResult)

	var agents int64
	require.NoError(t, g.Raw(`SELECT COUNT(*) FROM shadow_findings WHERE id = ?`, agent).Scan(&agents).Error)
	assert.Equal(t, int64(1), agents, "the inventory's agents are kept")

	exec(t, g, `UPDATE shadow_connections SET last_upload_at = NOW() WHERE id = ?`, custom)
	require.NoError(t, migration068.Migrate(g), "running the migration again must succeed")
	assert.Nil(t, get(custom).SecretEncrypted)
	assert.NotNil(t, get(custom).LastUploadAt, "running it again keeps the upload time")
}

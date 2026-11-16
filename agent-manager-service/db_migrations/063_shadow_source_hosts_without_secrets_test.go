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

func TestMigration063DropsSourceHostSecrets(t *testing.T) {
	g := scratchDatabase(t)
	require.NoError(t, migrateTo(g, 62))
	gh, ado, openai := uuid.New(), uuid.New(), uuid.New()
	insert := `INSERT INTO shadow_connections (id, ou_id, name, provider, config, secret_encrypted, secret_hints, last_test_result)
		VALUES (?, 'org-063', ?, ?, ?::jsonb, '\x01'::bytea, '{"personalAccessToken": "gith...wxyz"}', '{"checks": []}')`
	exec(t, g, insert, gh, "GitHub", "github", `{"organization": "acme", "allRepositories": true}`)
	exec(t, g, insert, ado, "Azure DevOps", "azure_devops", `{"organization": "contoso", "useEntra": true, "tenantId": "t", "clientId": "c"}`)
	exec(t, g, insert, openai, "OpenAI", "openai", `{"tenantId": "kept"}`)

	require.NoError(t, migrateTo(g, 63))

	type row struct {
		SecretEncrypted []byte  `gorm:"column:secret_encrypted"`
		SecretHints     string  `gorm:"column:secret_hints"`
		LastTestResult  *string `gorm:"column:last_test_result"`
		Config          string  `gorm:"column:config"`
	}
	get := func(id uuid.UUID) row {
		var r row
		require.NoError(t, g.Table("shadow_connections").Where("id = ?", id).Take(&r).Error)
		return r
	}
	for _, id := range []uuid.UUID{gh, ado} {
		r := get(id)
		assert.Nil(t, r.SecretEncrypted, "a source host stores no secret")
		assert.JSONEq(t, `{}`, r.SecretHints)
		assert.Nil(t, r.LastTestResult)
	}
	assert.JSONEq(t, `{"organization": "acme", "allRepositories": true, "privateRepositories": true}`, get(gh).Config,
		"a GitHub connection saved with a token is private")
	assert.JSONEq(t, `{"organization": "contoso"}`, get(ado).Config, "Entra settings are removed")

	other := get(openai)
	assert.Equal(t, []byte{0x01}, other.SecretEncrypted, "cloud platforms keep their secret")
	assert.JSONEq(t, `{"tenantId": "kept"}`, other.Config)
	assert.NotNil(t, other.LastTestResult)

	require.NoError(t, migration063.Migrate(g), "running the migration again must succeed")
	assert.Nil(t, get(gh).SecretEncrypted)
	assert.Equal(t, []byte{0x01}, get(openai).SecretEncrypted)
}

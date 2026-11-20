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

func TestMigration067RemovesAzureDevOps(t *testing.T) {
	g := scratchDatabase(t)
	require.NoError(t, migrateTo(g, 66))
	gh, ado := uuid.New(), uuid.New()
	conn := `INSERT INTO shadow_connections (id, ou_id, name, provider, config) VALUES (?, 'org-067', ?, ?, '{}'::jsonb)`
	exec(t, g, conn, gh, "GitHub", "github")
	exec(t, g, conn, ado, "Azure DevOps", "azure_devops")
	finding := `INSERT INTO shadow_findings (id, ou_id, connection_id, provider, kind, external_id) VALUES (?, 'org-067', ?, ?, 'code_agent', ?)`
	ghAgent, adoAgent := uuid.New(), uuid.New()
	exec(t, g, finding, ghAgent, gh, "github", "repo:agent.py:A")
	exec(t, g, finding, adoAgent, ado, "azure_devops", "Proj/Repo:agent.py:B")
	exec(t, g, `INSERT INTO shadow_finding_signals (ou_id, finding_id, code, source) VALUES ('org-067', ?, 'azure_devops.d2_agent_import', 'agent.py')`, adoAgent)

	require.NoError(t, migrateTo(g, 67))

	count := func(q string, args ...any) int64 {
		var n int64
		require.NoError(t, g.Raw(q, args...).Scan(&n).Error)
		return n
	}
	assert.Zero(t, count(`SELECT COUNT(*) FROM shadow_connections WHERE provider = 'azure_devops'`))
	assert.Zero(t, count(`SELECT COUNT(*) FROM shadow_findings WHERE provider = 'azure_devops'`), "its agents go with it")
	assert.Zero(t, count(`SELECT COUNT(*) FROM shadow_finding_signals WHERE finding_id = ?`, adoAgent), "and their evidence")
	assert.Equal(t, int64(1), count(`SELECT COUNT(*) FROM shadow_connections WHERE id = ?`, gh), "GitHub is kept")
	assert.Equal(t, int64(1), count(`SELECT COUNT(*) FROM shadow_findings WHERE id = ?`, ghAgent))

	require.NoError(t, migration067.Migrate(g), "running the migration again must succeed")
}

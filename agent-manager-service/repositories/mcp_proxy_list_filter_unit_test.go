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

package repositories

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// dryRunDB builds statements without a database connection.
func dryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	g, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=localhost user=x dbname=x"}),
		&gorm.Config{DryRun: true, DisableAutomaticPing: true})
	require.NoError(t, err)
	return g
}

func listSQL(t *testing.T, f MCPProxyListFilter) (string, []interface{}) {
	t.Helper()
	var out []*models.MCPProxy
	stmt := f.apply(dryRunDB(t).Model(&models.MCPProxy{}).
		Joins("JOIN artifacts a ON mcp_proxies.uuid = a.uuid"), "org-1").Find(&out).Statement
	return stmt.SQL.String(), stmt.Vars
}

func TestMCPProxyListFilterDefaultsToOrgOwnedOnly(t *testing.T) {
	sql, vars := listSQL(t, MCPProxyListFilter{})
	assert.Contains(t, sql, "mcp_proxies.project_uuid IS NULL")
	assert.NotContains(t, sql, "LIMIT")
	assert.Equal(t, []interface{}{"org-1", "McpProxy"}, vars)
}

func TestMCPProxyListFilterProjectOnly(t *testing.T) {
	p := "p-1"
	sql, vars := listSQL(t, MCPProxyListFilter{ProjectUUID: &p})
	assert.Contains(t, sql, "mcp_proxies.project_uuid = $3")
	assert.NotContains(t, sql, "IS NULL")
	assert.Equal(t, "p-1", vars[2])
}

func TestMCPProxyListFilterProjectPlusOrgOwned(t *testing.T) {
	p := "p-1"
	sql, vars := listSQL(t, MCPProxyListFilter{ProjectUUID: &p, IncludeOrgOwned: true})
	assert.Contains(t, sql, "(mcp_proxies.project_uuid = $3 OR mcp_proxies.project_uuid IS NULL)")
	assert.Equal(t, "p-1", vars[2])
}

func TestMCPProxyListFilterIgnoresIncludeOrgOwnedWithoutProject(t *testing.T) {
	sql, _ := listSQL(t, MCPProxyListFilter{IncludeOrgOwned: true})
	assert.Contains(t, sql, "mcp_proxies.project_uuid IS NULL")
}

func TestMCPProxyListFilterSourceComponentAndEndpoint(t *testing.T) {
	sql, vars := listSQL(t, MCPProxyListFilter{Component: "comp", Endpoint: "ep"})
	assert.Contains(t, sql, "mcp_proxies.configuration->'source'->>'componentName' = $3")
	assert.Contains(t, sql, "mcp_proxies.configuration->'source'->>'endpointName' = $4")
	assert.Equal(t, []interface{}{"org-1", "McpProxy", "comp", "ep"}, vars)
}

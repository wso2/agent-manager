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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration059_CreatesOrgBuildCredentialsWithOneRowPerOrg(t *testing.T) {
	g := scratchDatabase(t)
	require.NoError(t, migrateTo(g, 59))

	exec(t, g, `INSERT INTO org_build_credentials (ou_id, org_uuid, client_id, secret_ref_name)
		VALUES ('org-059', 'org-059', 'amp-build-org-059', 'amp-build-org-059-secrets')`)

	var row struct {
		ClientID      string `gorm:"column:client_id"`
		SecretRefName string `gorm:"column:secret_ref_name"`
	}
	require.NoError(t, g.Table("org_build_credentials").Where("ou_id = ?", "org-059").Take(&row).Error)
	assert.Equal(t, "amp-build-org-059", row.ClientID)
	assert.Equal(t, "amp-build-org-059-secrets", row.SecretRefName)

	err := g.Exec(`INSERT INTO org_build_credentials (ou_id, client_id, secret_ref_name)
		VALUES ('org-059', 'other-client', 'other-ref')`).Error
	require.Error(t, err, "a second build credential for the same org must violate uq_org_build_creds_ou_id")

	require.NoError(t, migrateTo(g, 59), "re-running the migration must not alter existing rows")
	var count int64
	require.NoError(t, g.Table("org_build_credentials").Where("ou_id = ?", "org-059").Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

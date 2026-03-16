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
	"gorm.io/gorm"
)

// Replace the table-level unique constraint on (organization_name, name) with a partial
// unique index that excludes soft-deleted rows.  Without this, a gateway that has been
// soft-deleted (deleted_at IS NOT NULL) blocks re-creation with the same name in the
// same organisation.
var migration009 = migration{
	ID: 9,
	Migrate: func(db *gorm.DB) error {
		sql := `
			ALTER TABLE gateways DROP CONSTRAINT IF EXISTS uq_gateway_org_name;

			CREATE UNIQUE INDEX IF NOT EXISTS uq_gateway_org_name_active
				ON gateways(organization_name, name)
				WHERE deleted_at IS NULL;
		`
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, sql)
		})
	},
}

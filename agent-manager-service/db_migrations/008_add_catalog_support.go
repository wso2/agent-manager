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

// Add catalog support and agent configuration tables
var migration008 = migration{
	ID: 8,
	Migrate: func(db *gorm.DB) error {
		addCatalogSupportSQL := `
			-- Add in_catalog column to artifacts table
			ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS in_catalog BOOLEAN DEFAULT FALSE;

			-- Create index for catalog queries
			CREATE INDEX IF NOT EXISTS idx_artifacts_in_catalog ON artifacts(in_catalog, organization_uuid);
		`
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, addCatalogSupportSQL)
		})
	},
}

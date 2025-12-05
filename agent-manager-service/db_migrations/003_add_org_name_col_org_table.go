// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

// add org_name column to organizations table
var migration003 = migration{
	ID: 3,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			// Add the column without NOT NULL constraint first
			addColumn := `ALTER TABLE organizations ADD COLUMN org_name VARCHAR(100);`
			if err := runSQL(tx, addColumn); err != nil {
				return err
			}

			// Set default values from open_choreo_org_name
			updateValues := `UPDATE organizations SET org_name = open_choreo_org_name WHERE org_name IS NULL;`
			if err := runSQL(tx, updateValues); err != nil {
				return err
			}

			// Now add NOT NULL and UNIQUE constraints
			addConstraints := `ALTER TABLE organizations 
				ALTER COLUMN org_name SET NOT NULL,
				ADD CONSTRAINT organizations_org_name_unique UNIQUE (org_name),
				ADD CONSTRAINT chk_organizations_name_match CHECK (org_name = open_choreo_org_name);`
			return runSQL(tx, addConstraints)
		})
	},
}

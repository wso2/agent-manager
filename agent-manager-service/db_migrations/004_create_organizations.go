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

// Create organizations table for storing organization metadata
var migration004 = migration{
	ID: 4,
	Migrate: func(db *gorm.DB) error {
		createOrganizationsSQL := `
			CREATE TABLE organizations (
				uuid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				name VARCHAR(100) NOT NULL UNIQUE,
				handle VARCHAR(100) NOT NULL UNIQUE,
				region VARCHAR(50) NOT NULL DEFAULT 'US',
				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
				deleted_at TIMESTAMP
			);

			CREATE INDEX idx_organizations_name ON organizations(name);
			CREATE INDEX idx_organizations_handle ON organizations(handle);
			CREATE INDEX idx_organizations_deleted ON organizations(deleted_at);
		`
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, createOrganizationsSQL)
		})
	},
}

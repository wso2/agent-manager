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

// Create organizations table for storing organization UUID mappings
// Organizations are managed by OpenChoreo, but we need to maintain UUIDs locally
// since OpenChoreo doesn't provide organization UUIDs
var migration002 = migration{
	ID: 2,
	Migrate: func(db *gorm.DB) error {
		createOrganizationsSQL := `
			CREATE TABLE organizations (
				uuid UUID PRIMARY KEY NOT NULL,
				name VARCHAR(100) NOT NULL UNIQUE,
				created_at TIMESTAMP NOT NULL DEFAULT NOW()
			);

			CREATE INDEX idx_organizations_name ON organizations(name);
		`
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, createOrganizationsSQL)
		})
	},
}

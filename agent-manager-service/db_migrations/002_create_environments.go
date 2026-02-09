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

// Create environments table for logical grouping of gateways by deployment stage
var migration002 = migration{
	ID: 2,
	Migrate: func(db *gorm.DB) error {
		createEnvironmentsSQL := `
			CREATE TABLE environments (
				uuid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				organization_name VARCHAR(100) NOT NULL,
				name VARCHAR(64) NOT NULL,
				display_name VARCHAR(128) NOT NULL,
				description TEXT,
				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
				deleted_at TIMESTAMP,

				UNIQUE(organization_name, name)
			);

			CREATE INDEX idx_environments_org ON environments(organization_name);
			CREATE INDEX idx_environments_deleted ON environments(deleted_at);
		`
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, createEnvironmentsSQL)
		})
	},
}

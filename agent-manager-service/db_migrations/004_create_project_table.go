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

// create table projects
var migration004 = migration{
	ID: 4,
	Migrate: func(db *gorm.DB) error {
		createTable := `CREATE TABLE projects
(
    id   UUID PRIMARY KEY,
	name         VARCHAR(100) NOT NULL,
	org_id       UUID NOT NULL,
	open_choreo_project VARCHAR(100) NOT NULL,
	display_name VARCHAR(100),
	description  TEXT,
	created_at   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	deleted_at   TIMESTAMPTZ,
	CONSTRAINT fk_projects_org_id FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT chk_projects_name_open_choreo_match CHECK (name = open_choreo_project)
)`

		createIndex := `CREATE UNIQUE INDEX uk_projects_name_org_id ON projects(name, org_id) WHERE deleted_at IS NULL`

		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, createTable, createIndex)
		})
	},
}

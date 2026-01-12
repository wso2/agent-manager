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

// create table agents
var migration002 = migration{
	ID: 2,
	Migrate: func(db *gorm.DB) error {
		createTable := `CREATE TABLE agents
(
   id      UUID PRIMARY KEY,
   name          VARCHAR(100) NOT NULL,
   display_name  VARCHAR(100) NOT NULL,
   provisioning_type    VARCHAR(100) NOT NULL,
   description   TEXT,
   project_name   VARCHAR(100) NOT NULL,
   org_name        VARCHAR(100) NOT NULL,
   created_at    TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
   updated_at    TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
   deleted_at    TIMESTAMPTZ,
   CONSTRAINT provisioning_type_enum check (provisioning_type in ('internal', 'external'))
)`

		createIndex := `CREATE UNIQUE INDEX uk_agents_name_project_org ON agents(name, project_name, org_name) WHERE deleted_at IS NULL`

		return db.Transaction(func(tx *gorm.DB) error {
			if err := runSQL(tx, createTable, createIndex); err != nil {
				return err
			}
			return nil
		})
	},
}

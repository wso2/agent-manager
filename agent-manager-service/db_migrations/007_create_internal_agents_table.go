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

// create table internal_agents
var migration007 = migration{
	ID: 7,
	Migrate: func(db *gorm.DB) error {
		createTable := `CREATE TABLE internal_agents
(
   id            UUID PRIMARY KEY,
   agent_subtype VARCHAR(100) NOT NULL,
   language      VARCHAR(100) NOT NULL,
   CONSTRAINT fk_internal_agents_id FOREIGN KEY (id) REFERENCES agents(id) ON DELETE CASCADE
)`

		return db.Transaction(func(tx *gorm.DB) error {
			if err := runSQL(tx, createTable); err != nil {
				return err
			}
			return nil
		})
	},
}

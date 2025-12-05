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

// create table organizations
var migration002 = migration{
	ID: 2,
	Migrate: func(db *gorm.DB) error {
		createTable := `CREATE TABLE organizations
(
    id              UUID PRIMARY KEY,
    open_choreo_org_name VARCHAR(100) NOT NULL UNIQUE,
    user_idp_id         UUID NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
)`

		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, createTable)
		})
	},
}

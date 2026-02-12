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

var migration003 = migration{
	ID: 3,
	Migrate: func(db *gorm.DB) error {
		sql := `
		CREATE TABLE gateway_environment_mappings (
				id SERIAL PRIMARY KEY,
				gateway_uuid UUID NOT NULL,
				environment_uuid UUID NOT NULL REFERENCES environments(uuid) ON DELETE CASCADE,
				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				UNIQUE(gateway_uuid, environment_uuid)
		);
		CREATE INDEX idx_gem_gateway ON gateway_environment_mappings(gateway_uuid);
		CREATE INDEX idx_gem_environment ON gateway_environment_mappings(environment_uuid);
	`
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, sql)
		})
	},
}

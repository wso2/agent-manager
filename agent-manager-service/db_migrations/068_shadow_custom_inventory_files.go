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

import "gorm.io/gorm"

// migration068 turns Bring your own inventory (provider custom) from an API
// Agent Manager called into files the customer uploads. Its connections no
// longer call anything, so their stored Agents URL, API key (the encrypted
// secret holds only that key) and connection test results are deleted.
// Their agents are kept until the first uploaded file replaces them. A
// connection records when a file was last uploaded (last_upload_at).
var migration068 = migration{
	ID: 68,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx,
				`ALTER TABLE shadow_connections ADD COLUMN IF NOT EXISTS last_upload_at TIMESTAMPTZ`,
				`UPDATE shadow_connections
				SET secret_encrypted = NULL, secret_hints = '{}'::jsonb,
					last_tested_at = NULL, last_test_result = NULL,
					config = config - 'agentsUrl'
				WHERE provider = 'custom'`,
			)
		})
	},
}

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

// migration063 stops storing a credential for source repository hosts
// (GitHub, Azure DevOps): the source code scan runs on the customer's machine
// and asks for the Git token there. Their stored tokens, Entra sign-in
// settings and connection test results are deleted. GitHub connections saved
// with a token are marked as private, so the scan command asks for one.
var migration063 = migration{
	ID: 63,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx,
				`ALTER TABLE shadow_connections ALTER COLUMN secret_encrypted DROP NOT NULL`,
				`UPDATE shadow_connections
				SET secret_encrypted = NULL, secret_hints = '{}'::jsonb,
					last_tested_at = NULL, last_test_result = NULL,
					config = config - 'useEntra' - 'tenantId' - 'clientId'
				WHERE provider IN ('github', 'azure_devops')`,
				`UPDATE shadow_connections
				SET config = config || '{"privateRepositories": true}'::jsonb
				WHERE provider = 'github'`,
			)
		})
	},
}

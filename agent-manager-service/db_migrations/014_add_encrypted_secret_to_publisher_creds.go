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

// Add client_secret_encrypted column to org_publisher_credentials so the scheduler
// can decrypt and use the org's publisher app secret to obtain org-bound OC tokens.
var migration014 = migration{
	ID: 14,
	Migrate: func(db *gorm.DB) error {
		return db.Exec(`
			ALTER TABLE org_publisher_credentials
			ADD COLUMN IF NOT EXISTS client_secret_encrypted BYTEA
		`).Error
	},
}

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

// migration065 adds the pending scan status and the scan options. A pending
// scan is a source code scan created by Generate command: it waits for its
// scan command to publish results, so the worker never claims it and it does
// not count as the org's active scan. Options hold the choices made in
// Generate command (AI review, LLM judge). The index finds the pending scans
// to expire.
var migration065 = migration{
	ID: 65,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx,
				`ALTER TABLE shadow_scans DROP CONSTRAINT IF EXISTS shadow_scans_status_check`,
				`ALTER TABLE shadow_scans ADD CONSTRAINT shadow_scans_status_check
				CHECK (status IN ('pending', 'queued', 'running', 'succeeded', 'partial', 'failed'))`,
				`ALTER TABLE shadow_scans ADD COLUMN IF NOT EXISTS options JSONB NOT NULL DEFAULT '{}'`,
				`CREATE INDEX IF NOT EXISTS idx_shadow_scans_pending
				ON shadow_scans (created_at) WHERE status = 'pending'`,
			)
		})
	},
}

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

// migration063 records onboarding as an external agent and drops the review
// status. An agent onboarded through Shadow Agent Discovery is linked to the
// externally-hosted agent registered for it (project and agent name, who and
// when); every connection's finding of the same agent carries the link. The
// review status (new, reviewed, dismissed) is no longer used.
var migration063 = migration{
	ID: 63,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx,
				`ALTER TABLE shadow_findings
					ADD COLUMN IF NOT EXISTS linked_project TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS linked_agent TEXT NOT NULL DEFAULT '',
					ADD COLUMN IF NOT EXISTS onboarded_at TIMESTAMPTZ,
					ADD COLUMN IF NOT EXISTS onboarded_by TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE shadow_findings DROP COLUMN IF EXISTS status`,
			)
		})
	},
}

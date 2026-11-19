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

// migration066 lets an onboarded agent be removed from the Unregistered
// Agents list. hidden_at (and who: hidden_by) is set on every connection's
// finding of the agent; a later scan keeps it, so the agent stays out of the
// list while it is still listed by its platform.
var migration066 = migration{
	ID: 66,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx,
				`ALTER TABLE shadow_findings
					ADD COLUMN IF NOT EXISTS hidden_at TIMESTAMPTZ,
					ADD COLUMN IF NOT EXISTS hidden_by TEXT NOT NULL DEFAULT ''`,
			)
		})
	},
}

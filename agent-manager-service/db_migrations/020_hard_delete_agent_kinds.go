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

// Agent kinds are now hard-deleted (consistent with OpenChoreo's component lifecycle).
// Purge any existing soft-deleted kind rows, drop the deleted_at column entirely, and
// replace the partial unique index with a plain one since deleted_at no longer exists.
var migration020 = migration{
	ID: 20,
	Migrate: func(db *gorm.DB) error {
		// Only purge soft-deleted rows if the column still exists (idempotent for envs
		// where this migration was previously applied as migration 018 or 019).
		var columnExists bool
		if err := db.Raw(`SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'agent_kinds' AND column_name = 'deleted_at'
		)`).Scan(&columnExists).Error; err != nil {
			return err
		}
		if columnExists {
			if err := runSQL(db, `DELETE FROM agent_kinds WHERE deleted_at IS NOT NULL`); err != nil {
				return err
			}
		}

		return runSQL(
			db,
			// Drop the constraint first (covers both partial-index and plain-constraint forms)
			`ALTER TABLE agent_kinds DROP CONSTRAINT IF EXISTS uq_agent_kinds_org_name`,
			// Drop any plain index that may have been created separately
			`DROP INDEX IF EXISTS idx_agent_kinds_org_name`,
			// Drop the soft-delete column
			`ALTER TABLE agent_kinds DROP COLUMN IF EXISTS deleted_at`,
			// Recreate the unique constraint on the now-clean table
			`ALTER TABLE agent_kinds ADD CONSTRAINT uq_agent_kinds_org_name UNIQUE (org_name, name)`,
			// Add flexible interface metadata bag to kind versions
			`ALTER TABLE agent_kind_versions ADD COLUMN IF NOT EXISTS metadata JSONB`,
		)
	},
}

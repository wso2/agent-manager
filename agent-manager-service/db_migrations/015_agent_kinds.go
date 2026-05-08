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

var migration015 = migration{
	ID: 15,
	Migrate: func(db *gorm.DB) error {
		createAgentKindsTable := `
		CREATE TABLE IF NOT EXISTS agent_kinds (
			id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name         VARCHAR(63)  NOT NULL,
			display_name VARCHAR(255) NOT NULL DEFAULT '',
			description  TEXT,

			-- Source agent reference (the agent that published this kind)
			org_name     VARCHAR(255) NOT NULL,
			project_name VARCHAR(63)  NOT NULL,
			agent_name   VARCHAR(63)  NOT NULL,

			-- Timestamps
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at   TIMESTAMPTZ,

			CONSTRAINT uq_agent_kinds_org_name UNIQUE (org_name, name)
		)`

		createAgentKindVersionsTable := `
		CREATE TABLE IF NOT EXISTS agent_kind_versions (
			id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			agent_kind_id UUID         NOT NULL,
			version       VARCHAR(30)  NOT NULL,
			build_name    VARCHAR(255) NOT NULL,
			config_schema JSONB        NOT NULL DEFAULT '[]',
			created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

			CONSTRAINT fk_agent_kind_versions_kind FOREIGN KEY (agent_kind_id)
				REFERENCES agent_kinds(id) ON DELETE CASCADE,
			CONSTRAINT uq_agent_kind_versions_version UNIQUE (agent_kind_id, version)
		)`

		createIndexes := []string{
			`CREATE INDEX IF NOT EXISTS idx_agent_kinds_org     ON agent_kinds         (org_name) WHERE deleted_at IS NULL`,
			`CREATE INDEX IF NOT EXISTS idx_agent_kind_versions ON agent_kind_versions (agent_kind_id)`,
		}

		createTrigger := `
		CREATE OR REPLACE FUNCTION update_agent_kind_updated_at()
		RETURNS TRIGGER AS $$
		BEGIN
			NEW.updated_at = NOW();
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;

		DROP TRIGGER IF EXISTS trg_agent_kind_updated_at ON agent_kinds;
		CREATE TRIGGER trg_agent_kind_updated_at
			BEFORE UPDATE ON agent_kinds
			FOR EACH ROW
			EXECUTE FUNCTION update_agent_kind_updated_at()`

		return db.Transaction(func(tx *gorm.DB) error {
			if err := runSQL(tx, createAgentKindsTable); err != nil {
				return err
			}
			if err := runSQL(tx, createAgentKindVersionsTable); err != nil {
				return err
			}
			for _, idx := range createIndexes {
				if err := runSQL(tx, idx); err != nil {
					return err
				}
			}
			return runSQL(tx, createTrigger)
		})
	},
}

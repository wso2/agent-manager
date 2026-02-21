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

// Create agent_configs table for per-environment agent configuration storage
var migration007 = migration{
	ID: 7,
	Migrate: func(db *gorm.DB) error {
		createAgentConfigsTable := `
		CREATE TABLE IF NOT EXISTS agent_configs (
			id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),

			-- Agent identification
			org_name                    VARCHAR(255) NOT NULL,
			project_name                VARCHAR(63) NOT NULL,
			agent_name                  VARCHAR(63) NOT NULL,

			-- Environment identification
			environment_name            VARCHAR(63) NOT NULL,

			-- Configuration settings
			enable_auto_instrumentation BOOLEAN NOT NULL DEFAULT true,

			-- Timestamps
			created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

			-- Unique constraint: one config per agent per environment (must include project_name)
			CONSTRAINT uq_agent_config_agent_env UNIQUE (org_name, project_name, agent_name, environment_name)
		)`

		createIndexes := []string{
			`CREATE INDEX IF NOT EXISTS idx_agent_configs_agent ON agent_configs (org_name, project_name, agent_name)`,
		}

		createTrigger := `
		CREATE OR REPLACE FUNCTION update_agent_config_updated_at()
		RETURNS TRIGGER AS $$
		BEGIN
			NEW.updated_at = NOW();
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;

		DROP TRIGGER IF EXISTS trg_agent_config_updated_at ON agent_configs;
		CREATE TRIGGER trg_agent_config_updated_at
			BEFORE UPDATE ON agent_configs
			FOR EACH ROW
			EXECUTE FUNCTION update_agent_config_updated_at()
		`

		return db.Transaction(func(tx *gorm.DB) error {
			if err := runSQL(tx, createAgentConfigsTable); err != nil {
				return err
			}
			for _, idx := range createIndexes {
				if err := runSQL(tx, idx); err != nil {
					return err
				}
			}
			if err := runSQL(tx, createTrigger); err != nil {
				return err
			}
			return nil
		})
	},
}

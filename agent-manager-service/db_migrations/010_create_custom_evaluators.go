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

// Create custom_evaluators table for user-defined evaluators
var migration010 = migration{
	ID: 10,
	Migrate: func(db *gorm.DB) error {
		createCustomEvaluatorsTable := `
		CREATE TABLE IF NOT EXISTS custom_evaluators (
			id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			org_name          VARCHAR(255) NOT NULL,
			identifier        VARCHAR(128) NOT NULL,
			display_name      VARCHAR(128) NOT NULL,
			description       VARCHAR(512) NOT NULL DEFAULT '',
			version           VARCHAR(20) NOT NULL DEFAULT '1.0',

			-- Evaluator type: 'code' for Python source, 'llm_judge' for prompt template
			type              VARCHAR(20) NOT NULL CHECK (type IN ('code', 'llm_judge')),

			-- Evaluation level
			level             VARCHAR(20) NOT NULL CHECK (level IN ('trace', 'agent', 'llm')),

			-- Source: Python code (code type) or prompt template (llm_judge type)
			source            TEXT NOT NULL,

			-- Configuration parameter schema
			config_schema     JSONB NOT NULL DEFAULT '[]',

			-- Tags for categorization and filtering
			tags              JSONB NOT NULL DEFAULT '[]',

			-- Timestamps & soft delete
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at        TIMESTAMPTZ
		)`

		createPartialUnique := `
		CREATE UNIQUE INDEX IF NOT EXISTS uq_custom_evaluator_org_identifier
			ON custom_evaluators (org_name, identifier)
			WHERE deleted_at IS NULL`

		createIndexes := []string{
			`CREATE INDEX IF NOT EXISTS idx_custom_evaluator_org ON custom_evaluators (org_name)`,
			`CREATE INDEX IF NOT EXISTS idx_custom_evaluator_type ON custom_evaluators (type)`,
			`CREATE INDEX IF NOT EXISTS idx_custom_evaluator_deleted_at ON custom_evaluators (deleted_at)`,
		}

		createTrigger := `
		CREATE OR REPLACE FUNCTION update_custom_evaluator_updated_at()
		RETURNS TRIGGER AS $$
		BEGIN
			NEW.updated_at = NOW();
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;

		DROP TRIGGER IF EXISTS trg_custom_evaluator_updated_at ON custom_evaluators;
		CREATE TRIGGER trg_custom_evaluator_updated_at
			BEFORE UPDATE ON custom_evaluators
			FOR EACH ROW
			EXECUTE FUNCTION update_custom_evaluator_updated_at()
		`

		return db.Transaction(func(tx *gorm.DB) error {
			if err := runSQL(tx, createCustomEvaluatorsTable); err != nil {
				return err
			}
			if err := runSQL(tx, createPartialUnique); err != nil {
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

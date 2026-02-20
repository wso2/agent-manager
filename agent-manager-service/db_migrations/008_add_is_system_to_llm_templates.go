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

// Add is_system flag to llm_provider_templates table
var migration008 = migration{
	ID: 8,
	Migrate: func(db *gorm.DB) error {
		addIsSystemFlagSQL := `
			-- Add is_system column to distinguish built-in vs user templates
			ALTER TABLE llm_provider_templates ADD COLUMN is_system BOOLEAN NOT NULL DEFAULT false;

			-- Add index for performance
			CREATE INDEX idx_llm_provider_templates_is_system ON llm_provider_templates(is_system);

			-- Drop old unique constraint
			ALTER TABLE llm_provider_templates DROP CONSTRAINT IF EXISTS uq_llm_template_handle_org;

			-- Add new unique constraint allowing same handle for system vs user templates
			ALTER TABLE llm_provider_templates ADD CONSTRAINT uq_llm_template_handle_org_system
				UNIQUE(organization_name, handle, is_system);
		`

		changeTemplateReferenceSQL := `
			-- Drop the foreign key constraint to llm_provider_templates
			ALTER TABLE llm_providers DROP CONSTRAINT IF EXISTS fk_llm_provider_template;

			-- Add new template_handle column
			ALTER TABLE llm_providers ADD COLUMN template_handle VARCHAR(255);

			-- Migrate existing data: copy template handle from llm_provider_templates table
			-- This assumes template_uuid currently references a valid template
			UPDATE llm_providers p
			SET template_handle = t.handle
			FROM llm_provider_templates t
			WHERE p.template_uuid = t.uuid;

			-- Make template_handle NOT NULL after data migration
			ALTER TABLE llm_providers ALTER COLUMN template_handle SET NOT NULL;

			-- Drop the old template_uuid column
			ALTER TABLE llm_providers DROP COLUMN template_uuid;

			-- Create index on template_handle for performance
			CREATE INDEX idx_llm_providers_template_handle ON llm_providers(template_handle);
		`
		return db.Transaction(func(tx *gorm.DB) error {
			err := runSQL(tx, changeTemplateReferenceSQL)
			if err != nil {
				return err
			}

			err = runSQL(tx, addIsSystemFlagSQL)
			if err != nil {
				return err
			}
			return nil
		})
	},
}

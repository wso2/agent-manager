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

// Create LLM provider tables for AI gateway functionality
var migration005 = migration{
	ID: 5,
	Migrate: func(db *gorm.DB) error {
		createLLMProvidersSQL := `
			-- LLM Provider Templates table
			CREATE TABLE llm_provider_templates (
				uuid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				organization_uuid UUID NOT NULL,
				handle VARCHAR(255) NOT NULL,
				name VARCHAR(253) NOT NULL,
				description TEXT,
				created_by VARCHAR(255),
				configuration TEXT NOT NULL,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

				CONSTRAINT fk_llm_template_organization FOREIGN KEY (organization_uuid)
					REFERENCES organizations(uuid) ON DELETE CASCADE,
				CONSTRAINT uq_llm_template_handle_org UNIQUE(organization_uuid, handle)
			);

			-- LLM Providers table
			CREATE TABLE llm_providers (
				uuid UUID PRIMARY KEY,
				description TEXT,
				created_by VARCHAR(255),
				template_uuid UUID NOT NULL,
				openapi_spec TEXT,
				model_list TEXT,
				status VARCHAR(20) NOT NULL DEFAULT 'CREATED',
				configuration JSONB NOT NULL,

				CONSTRAINT fk_llm_provider_artifact FOREIGN KEY (uuid)
					REFERENCES artifacts(uuid) ON DELETE CASCADE,
				CONSTRAINT fk_llm_provider_template FOREIGN KEY (template_uuid)
					REFERENCES llm_provider_templates(uuid) ON DELETE RESTRICT
			);

			-- LLM Proxies table
			CREATE TABLE llm_proxies (
				uuid UUID PRIMARY KEY,
				project_uuid UUID NOT NULL,
				description TEXT,
				created_by VARCHAR(255),
				provider_uuid UUID NOT NULL,
				openapi_spec TEXT,
				status VARCHAR(20) NOT NULL DEFAULT 'CREATED',
				configuration JSONB NOT NULL,

				CONSTRAINT fk_llm_proxy_artifact FOREIGN KEY (uuid)
					REFERENCES artifacts(uuid) ON DELETE CASCADE,
				CONSTRAINT fk_llm_proxy_provider FOREIGN KEY (provider_uuid)
					REFERENCES llm_providers(uuid) ON DELETE RESTRICT
			);

			-- Indexes for performance
			CREATE INDEX idx_llm_provider_templates_org ON llm_provider_templates(organization_uuid);
			CREATE INDEX idx_llm_providers_template ON llm_providers(template_uuid);
			CREATE INDEX idx_llm_proxies_project ON llm_proxies(project_uuid);
			CREATE INDEX idx_llm_proxies_provider ON llm_proxies(provider_uuid);
		`
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, createLLMProvidersSQL)
		})
	},
}

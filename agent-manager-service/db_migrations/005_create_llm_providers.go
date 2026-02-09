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

// Create org_llm_providers table for organization-level LLM provider registry
var migration005 = migration{
	ID: 5,
	Migrate: func(db *gorm.DB) error {
		createOrgLLMProvidersSQL := `
			CREATE TABLE org_llm_providers (
				uuid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				organization_name VARCHAR(100) NOT NULL,
				handle VARCHAR(64) NOT NULL,
				display_name VARCHAR(128) NOT NULL,
				template VARCHAR(64) NOT NULL,
				configuration JSONB NOT NULL,
				status VARCHAR(32) NOT NULL,
				approved_by VARCHAR(255),
				approved_at TIMESTAMP,
				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
				deleted_at TIMESTAMP,
				created_by VARCHAR(255) NOT NULL,

				UNIQUE(organization_name, handle)
			);

			CREATE INDEX idx_org_llm_providers_org ON org_llm_providers(organization_name);
			CREATE INDEX idx_org_llm_providers_status ON org_llm_providers(status);
			CREATE INDEX idx_org_llm_providers_template ON org_llm_providers(template);
			CREATE INDEX idx_org_llm_providers_deleted ON org_llm_providers(deleted_at);
			CREATE INDEX idx_org_llm_providers_configuration ON org_llm_providers USING GIN (configuration);
		`
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, createOrgLLMProvidersSQL)
		})
	},
}

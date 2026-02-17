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

// Migration006CreateLLMProviderEnvironmentMapping creates the llm_provider_gateway_mappings table
var migration008 = migration{
	ID: 8,
	Migrate: func(db *gorm.DB) error {
		createGwEnvMapping := `
			CREATE TABLE IF NOT EXISTS llm_provider_gateway_mappings (
				llm_provider_uuid UUID NOT NULL,
				gateway_uuid UUID NOT NULL,

				-- Foreign key constraints
				CONSTRAINT fk_mapping_provider FOREIGN KEY (llm_provider_uuid)
					REFERENCES llm_providers(uuid) ON DELETE CASCADE,

				-- Unique constraint: one mapping per provider-gateway pair
				CONSTRAINT uq_provider_gateway UNIQUE (llm_provider_uuid, gateway_uuid)
			);

			-- Indexes for efficient queries
			CREATE INDEX IF NOT EXISTS idx_mapping_provider ON llm_provider_gateway_mappings(llm_provider_uuid);
			CREATE INDEX IF NOT EXISTS idx_mapping_gateway ON llm_provider_gateway_mappings(gateway_uuid);
		`

		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, createGwEnvMapping)
		})
	},
}

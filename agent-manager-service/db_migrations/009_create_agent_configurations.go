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

// Create agent configuration tables
var migration011 = migration{
	ID: 11,
	Migrate: func(db *gorm.DB) error {
		// Agent Configurations table
		createAgentConfigurationsTable := `
		CREATE TABLE IF NOT EXISTS agent_configurations (
			uuid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name VARCHAR(255) NOT NULL,
			description TEXT,
			agent_id VARCHAR(255) NOT NULL,
			type VARCHAR(50) NOT NULL DEFAULT 'llm',
			organization_name VARCHAR(255) NOT NULL,
			project_name VARCHAR(255) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

			CONSTRAINT chk_agent_config_type CHECK (type IN ('llm', 'mcp', 'other')),
			CONSTRAINT uq_agent_config_name UNIQUE(agent_id, name, organization_name)
		)`

		// Environment Agent Model Mapping table
		createEnvAgentModelMappingTable := `
		CREATE TABLE IF NOT EXISTS env_agent_model_mapping (
			id SERIAL PRIMARY KEY,
			config_uuid UUID NOT NULL,
			environment_uuid UUID NOT NULL,
			llm_proxy_uuid UUID NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

			CONSTRAINT fk_env_mapping_config FOREIGN KEY (config_uuid)
				REFERENCES agent_configurations(uuid) ON DELETE CASCADE,
			CONSTRAINT fk_env_mapping_proxy FOREIGN KEY (llm_proxy_uuid)
				REFERENCES llm_proxies(uuid) ON DELETE CASCADE,
			CONSTRAINT uq_env_mapping UNIQUE(config_uuid, environment_uuid)
		)`

		// Agent Environment Config Variables Mapping table
		createAgentEnvConfigVariablesTable := `
		CREATE TABLE IF NOT EXISTS agent_env_config_variables_mapping (
			id SERIAL PRIMARY KEY,
			config_uuid UUID NOT NULL,
			environment_uuid UUID NOT NULL,
			variable_name VARCHAR(255) NOT NULL,
			secret_reference TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

			CONSTRAINT fk_env_var_config FOREIGN KEY (config_uuid)
				REFERENCES agent_configurations(uuid) ON DELETE CASCADE,
			CONSTRAINT uq_env_var UNIQUE(config_uuid, environment_uuid, variable_name)
		)`

		// Create indexes
		createIndexes := []string{
			// agent_configurations indexes
			`CREATE INDEX IF NOT EXISTS idx_agent_config_agent ON agent_configurations(agent_id)`,
			`CREATE INDEX IF NOT EXISTS idx_agent_config_org_project ON agent_configurations(organization_name, project_name)`,

			// env_agent_model_mapping indexes
			`CREATE INDEX IF NOT EXISTS idx_env_mapping_config ON env_agent_model_mapping(config_uuid)`,
			`CREATE INDEX IF NOT EXISTS idx_env_mapping_environment ON env_agent_model_mapping(environment_uuid)`,
			`CREATE INDEX IF NOT EXISTS idx_env_mapping_proxy ON env_agent_model_mapping(llm_proxy_uuid)`,

			// agent_env_config_variables_mapping indexes
			`CREATE INDEX IF NOT EXISTS idx_env_var_config ON agent_env_config_variables_mapping(config_uuid)`,
			`CREATE INDEX IF NOT EXISTS idx_env_var_environment ON agent_env_config_variables_mapping(environment_uuid)`,
		}

		// Create comments
		createComments := []string{
			`COMMENT ON COLUMN env_agent_model_mapping.environment_uuid IS 'Environment UUID from OpenChoreo (validated at runtime, no FK constraint)'`,
			`COMMENT ON COLUMN agent_env_config_variables_mapping.environment_uuid IS 'Environment UUID from OpenChoreo (validated at runtime, no FK constraint)'`,
			`COMMENT ON COLUMN agent_env_config_variables_mapping.secret_reference IS 'Secret reference like choreo:///default/secret/my-config-prod-url (NOT actual secret)'`,
		}

		return db.Transaction(func(tx *gorm.DB) error {
			// Create tables
			if err := runSQL(tx, createAgentConfigurationsTable); err != nil {
				return err
			}
			if err := runSQL(tx, createEnvAgentModelMappingTable); err != nil {
				return err
			}
			if err := runSQL(tx, createAgentEnvConfigVariablesTable); err != nil {
				return err
			}

			// Create indexes
			for _, idx := range createIndexes {
				if err := runSQL(tx, idx); err != nil {
					return err
				}
			}

			// Add comments
			for _, comment := range createComments {
				if err := runSQL(tx, comment); err != nil {
					return err
				}
			}

			return nil
		})
	},
}

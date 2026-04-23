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

// Create monitor_llm_mapping table — links monitors to LLM proxies,
// mirroring the env_agent_model_mapping pattern used by agents.
var migration012 = migration{
	ID: 12,
	Migrate: func(db *gorm.DB) error {
		createMonitorLLMMappingTable := `
		CREATE TABLE IF NOT EXISTS monitor_llm_mapping (
			id                   SERIAL PRIMARY KEY,
			monitor_id           UUID NOT NULL,
			llm_proxy_uuid       UUID NOT NULL,
			policy_configuration JSONB DEFAULT '[]'::jsonb,
			created_at           TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

			CONSTRAINT fk_monitor_llm_mapping_monitor FOREIGN KEY (monitor_id)
				REFERENCES monitors(id) ON DELETE CASCADE,
			CONSTRAINT fk_monitor_llm_mapping_proxy FOREIGN KEY (llm_proxy_uuid)
				REFERENCES llm_proxies(uuid) ON DELETE CASCADE,
			CONSTRAINT uq_monitor_llm_proxy UNIQUE(monitor_id, llm_proxy_uuid)
		)`

		createIndexes := []string{
			`CREATE INDEX IF NOT EXISTS idx_monitor_llm_mapping_monitor ON monitor_llm_mapping(monitor_id)`,
			`CREATE INDEX IF NOT EXISTS idx_monitor_llm_mapping_proxy ON monitor_llm_mapping(llm_proxy_uuid)`,
		}

		return db.Transaction(func(tx *gorm.DB) error {
			if err := runSQL(tx, createMonitorLLMMappingTable); err != nil {
				return err
			}
			for _, idx := range createIndexes {
				if err := runSQL(tx, idx); err != nil {
					return err
				}
			}
			return nil
		})
	},
}

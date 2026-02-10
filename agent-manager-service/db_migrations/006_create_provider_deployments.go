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

// Create provider_gateway_deployments table for tracking provider deployments to gateways
var migration006 = migration{
	ID: 6,
	Migrate: func(db *gorm.DB) error {
		createProviderDeploymentsSQL := `
			CREATE TABLE provider_gateway_deployments (
				id SERIAL PRIMARY KEY,
				provider_uuid UUID NOT NULL,
				gateway_uuid UUID NOT NULL,
				deployment_id VARCHAR(255) NOT NULL,
				environment VARCHAR(64),
				configuration_version INTEGER NOT NULL DEFAULT 1,
				gateway_specific_overrides JSONB,
				status VARCHAR(32) NOT NULL,
				deployed_at TIMESTAMP,
				error_message TEXT,
				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMP NOT NULL DEFAULT NOW(),

				UNIQUE(provider_uuid, gateway_uuid),
				FOREIGN KEY (provider_uuid) REFERENCES llm_providers(uuid) ON DELETE CASCADE,
				FOREIGN KEY (gateway_uuid) REFERENCES gateways(uuid) ON DELETE CASCADE
			);

			CREATE INDEX idx_pgd_provider ON provider_gateway_deployments(provider_uuid);
			CREATE INDEX idx_pgd_gateway ON provider_gateway_deployments(gateway_uuid);
			CREATE INDEX idx_pgd_status ON provider_gateway_deployments(status);
			CREATE INDEX idx_pgd_environment ON provider_gateway_deployments(environment);
		`
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, createProviderDeploymentsSQL)
		})
	},
}

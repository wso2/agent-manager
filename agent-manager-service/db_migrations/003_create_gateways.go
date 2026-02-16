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

// Create gateways and gateway_tokens tables for API Platform integration
var migration003 = migration{
	ID: 3,
	Migrate: func(db *gorm.DB) error {
		createGatewaysSQL := `
			-- Gateways table
			CREATE TABLE gateways (
				uuid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				organization_uuid UUID NOT NULL,
				name VARCHAR(255) NOT NULL,
				display_name VARCHAR(255) NOT NULL,
				description TEXT,
				properties JSONB NOT NULL DEFAULT '{}'::jsonb,
				vhost VARCHAR(255) NOT NULL,
				is_critical BOOLEAN DEFAULT FALSE,
				gateway_functionality_type VARCHAR(20) DEFAULT 'regular' NOT NULL,
				is_active BOOLEAN DEFAULT FALSE,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				deleted_at TIMESTAMP,

				CONSTRAINT fk_gateway_organization FOREIGN KEY (organization_uuid)
					REFERENCES organizations(uuid) ON DELETE CASCADE,
				CONSTRAINT uq_gateway_org_name UNIQUE(organization_uuid, name),
				CONSTRAINT chk_gateway_functionality_type
					CHECK (gateway_functionality_type IN ('regular', 'ai'))
			);

			-- Gateway Tokens table
			CREATE TABLE gateway_tokens (
				uuid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				gateway_uuid UUID NOT NULL,
				token_hash VARCHAR(255) NOT NULL,
				salt VARCHAR(255) NOT NULL,
				token_prefix VARCHAR(36) NOT NULL,
				status VARCHAR(10) NOT NULL DEFAULT 'active',
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				revoked_at TIMESTAMP,

				CONSTRAINT fk_gateway_token_gateway FOREIGN KEY (gateway_uuid)
					REFERENCES gateways(uuid) ON DELETE CASCADE,
				CONSTRAINT chk_gateway_token_status
					CHECK (status IN ('active', 'revoked')),
				CONSTRAINT chk_gateway_token_revoked
					CHECK (revoked_at IS NULL OR status = 'revoked')
			);

			-- Indexes for gateways
			CREATE INDEX idx_gateways_org ON gateways(organization_uuid);
			CREATE INDEX idx_gateways_active ON gateways(is_active);
			CREATE INDEX idx_gateways_deleted ON gateways(deleted_at) WHERE deleted_at IS NOT NULL;
			CREATE INDEX idx_gateways_functionality_type ON gateways(gateway_functionality_type);

			-- Indexes for gateway_tokens
			CREATE INDEX idx_gateway_tokens_gateway ON gateway_tokens(gateway_uuid);
			CREATE INDEX idx_gateway_tokens_status ON gateway_tokens(gateway_uuid, status);
			CREATE INDEX idx_gateway_tokens_active ON gateway_tokens(status) WHERE status = 'active';
			CREATE UNIQUE INDEX IF NOT EXISTS idx_gateway_tokens_prefix_active
				ON gateway_tokens(token_prefix) WHERE status = 'active';

			-- Recreate gateway_environment_mappings table
			CREATE TABLE gateway_environment_mappings (
				id SERIAL PRIMARY KEY,
				gateway_uuid UUID NOT NULL,
				environment_uuid UUID NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				CONSTRAINT fk_gateway_env_mapping_gateway FOREIGN KEY (gateway_uuid)
					REFERENCES gateways(uuid) ON DELETE CASCADE,
				UNIQUE(gateway_uuid, environment_uuid)
			);
			CREATE INDEX idx_gem_gateway ON gateway_environment_mappings(gateway_uuid);
			CREATE INDEX idx_gem_environment ON gateway_environment_mappings(environment_uuid);

			-- Comments for documentation
			COMMENT ON TABLE gateways IS 'API Platform gateways for routing and managing APIs';
			COMMENT ON COLUMN gateways.gateway_functionality_type IS 'Type of gateway: regular, ai, or event';
			COMMENT ON COLUMN gateways.properties IS 'Gateway-specific configuration properties';
			COMMENT ON COLUMN gateways.is_active IS 'Whether the gateway is currently active and connected';

			COMMENT ON TABLE gateway_tokens IS 'Authentication tokens for gateway connectivity';
			COMMENT ON COLUMN gateway_tokens.token_hash IS 'SHA256 hash of the token';
			COMMENT ON COLUMN gateway_tokens.salt IS 'Salt used for token hashing';
			COMMENT ON COLUMN gateway_tokens.status IS 'Token status: active or revoked';

			COMMENT ON TABLE gateway_environment_mappings IS 'Maps gateways to environments';
		`
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, createGatewaysSQL)
		})
	},
}

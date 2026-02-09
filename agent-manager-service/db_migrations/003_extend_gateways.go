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

// Create gateways table with gateway management fields and credential storage
var migration003 = migration{
	ID: 3,
	Migrate: func(db *gorm.DB) error {
		sql := `
			CREATE TYPE gateway_type AS ENUM ('INGRESS', 'EGRESS');
			CREATE TYPE gateway_status AS ENUM ('ACTIVE', 'INACTIVE', 'PROVISIONING', 'ERROR');

			CREATE TABLE gateways (
				uuid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				organization_name VARCHAR(100) NOT NULL,
				name VARCHAR(64) NOT NULL,
				display_name VARCHAR(128) NOT NULL,
				gateway_type gateway_type NOT NULL DEFAULT 'EGRESS',
				control_plane_url TEXT,
				vhost VARCHAR(253) NOT NULL,
				region VARCHAR(64),
				is_critical BOOLEAN NOT NULL DEFAULT false,
				status gateway_status NOT NULL DEFAULT 'ACTIVE',
				adapter_config JSONB,
				credentials_encrypted BYTEA,
				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
				deleted_at TIMESTAMP,

				UNIQUE(organization_name, name)
			);

			CREATE INDEX idx_gateways_org ON gateways(organization_name);
			CREATE INDEX idx_gateways_type ON gateways(gateway_type);
			CREATE INDEX idx_gateways_status ON gateways(status);
			CREATE INDEX idx_gateways_deleted ON gateways(deleted_at);
		`
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, sql)
		})
	},
}

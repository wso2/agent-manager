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

// Create developer portals and publication tables
var migration007 = migration{
	ID: 7,
	Migrate: func(db *gorm.DB) error {
		createDevPortalsSQL := `
			-- DevPortals table
			CREATE TABLE devportals (
				uuid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				organization_uuid UUID NOT NULL,
				name VARCHAR(100) NOT NULL,
				identifier VARCHAR(100) NOT NULL,
				api_url VARCHAR(255) NOT NULL,
				hostname VARCHAR(255) NOT NULL,
				api_key VARCHAR(255) NOT NULL,
				header_key_name VARCHAR(100) DEFAULT 'x-wso2-api-key',
				is_active BOOLEAN DEFAULT FALSE,
				is_enabled BOOLEAN DEFAULT FALSE,
				is_default BOOLEAN DEFAULT FALSE,
				visibility VARCHAR(20) NOT NULL DEFAULT 'private',
				description VARCHAR(500),
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

				CONSTRAINT fk_devportal_organization FOREIGN KEY (organization_uuid)
					REFERENCES organizations(uuid) ON DELETE CASCADE,
				CONSTRAINT uq_devportal_org_api_url UNIQUE(organization_uuid, api_url),
				CONSTRAINT uq_devportal_org_hostname UNIQUE(organization_uuid, hostname)
			);

			-- API-DevPortal Publication Tracking Table
			CREATE TABLE publication_mappings (
				api_uuid UUID NOT NULL,
				devportal_uuid UUID NOT NULL,
				organization_uuid UUID NOT NULL,
				status VARCHAR(20) NOT NULL CHECK (status IN ('published', 'failed', 'publishing')),
				api_version VARCHAR(50),
				devportal_ref_id VARCHAR(100),

				-- Gateway endpoints for sandbox and production
				sandbox_endpoint_url VARCHAR(500) NOT NULL,
				production_endpoint_url VARCHAR(500) NOT NULL,

				-- Timestamps
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

				-- Constraints
				CONSTRAINT pk_publication_mappings PRIMARY KEY (api_uuid, devportal_uuid, organization_uuid),
				CONSTRAINT fk_publication_api FOREIGN KEY (api_uuid)
					REFERENCES rest_apis(uuid) ON DELETE CASCADE,
				CONSTRAINT fk_publication_devportal FOREIGN KEY (devportal_uuid)
					REFERENCES devportals(uuid) ON DELETE CASCADE,
				CONSTRAINT fk_publication_organization FOREIGN KEY (organization_uuid)
					REFERENCES organizations(uuid) ON DELETE CASCADE,
				CONSTRAINT uq_publication_api_devportal_org UNIQUE (api_uuid, devportal_uuid, organization_uuid)
			);

			-- Indexes for performance
			CREATE INDEX idx_devportals_org ON devportals(organization_uuid);
			CREATE INDEX idx_devportals_active ON devportals(organization_uuid, is_active);
			CREATE UNIQUE INDEX idx_devportals_default_per_org ON devportals(organization_uuid) WHERE is_default = TRUE;
			CREATE INDEX idx_publication_mappings_api ON publication_mappings(api_uuid);
			CREATE INDEX idx_publication_mappings_devportal ON publication_mappings(devportal_uuid);
			CREATE INDEX idx_publication_mappings_org ON publication_mappings(organization_uuid);
		`
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, createDevPortalsSQL)
		})
	},
}

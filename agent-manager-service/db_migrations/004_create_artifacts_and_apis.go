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

// Create artifacts, REST APIs, and deployment tables
var migration004 = migration{
	ID: 4,
	Migrate: func(db *gorm.DB) error {
		createArtifactsAndAPIsSQL := `
			-- Artifacts table
			CREATE TABLE artifacts (
				uuid UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				handle VARCHAR(255) NOT NULL,
				name VARCHAR(255) NOT NULL,
				version VARCHAR(30) NOT NULL,
				kind VARCHAR(20) NOT NULL,
				organization_uuid UUID NOT NULL,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

				CONSTRAINT fk_artifact_organization FOREIGN KEY (organization_uuid)
					REFERENCES organizations(uuid) ON DELETE RESTRICT,
				CONSTRAINT uq_artifact_handle_org UNIQUE(handle, organization_uuid),
				CONSTRAINT uq_artifact_name_version_org UNIQUE(name, version, organization_uuid)
			);

			-- Deployments table (immutable deployment artifacts)
			CREATE TABLE deployments (
				deployment_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				name VARCHAR(255) NOT NULL,
				artifact_uuid UUID NOT NULL,
				organization_uuid UUID NOT NULL,
				gateway_uuid UUID NOT NULL,
				base_deployment_id UUID,
				content BYTEA NOT NULL,
				metadata TEXT, -- JSON object as TEXT
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

				CONSTRAINT fk_deployment_artifact FOREIGN KEY (artifact_uuid)
					REFERENCES artifacts(uuid) ON DELETE CASCADE,
				CONSTRAINT fk_deployment_organization FOREIGN KEY (organization_uuid)
					REFERENCES organizations(uuid) ON DELETE CASCADE,
				CONSTRAINT fk_deployment_gateway FOREIGN KEY (gateway_uuid)
					REFERENCES gateways(uuid) ON DELETE CASCADE,
				CONSTRAINT fk_deployment_base FOREIGN KEY (base_deployment_id)
					REFERENCES deployments(deployment_id) ON DELETE SET NULL
			);

			-- Deployment Status table (current deployment state)
			CREATE TABLE deployment_status (
				artifact_uuid UUID NOT NULL,
				organization_uuid UUID NOT NULL,
				gateway_uuid UUID NOT NULL,
				deployment_id UUID NOT NULL,
				status VARCHAR(20) NOT NULL DEFAULT 'DEPLOYED',
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

				CONSTRAINT pk_deployment_status PRIMARY KEY (artifact_uuid, organization_uuid, gateway_uuid),
				CONSTRAINT fk_deployment_status_artifact FOREIGN KEY (artifact_uuid)
					REFERENCES artifacts(uuid) ON DELETE CASCADE,
				CONSTRAINT fk_deployment_status_organization FOREIGN KEY (organization_uuid)
					REFERENCES organizations(uuid) ON DELETE CASCADE,
				CONSTRAINT fk_deployment_status_gateway FOREIGN KEY (gateway_uuid)
					REFERENCES gateways(uuid) ON DELETE CASCADE,
				CONSTRAINT fk_deployment_status_deployment FOREIGN KEY (deployment_id)
					REFERENCES deployments(deployment_id) ON DELETE CASCADE,
				CONSTRAINT chk_deployment_status CHECK (status IN ('DEPLOYED', 'UNDEPLOYED'))
			);

			-- Association Mappings table (for gateways and dev portals)
			CREATE TABLE association_mappings (
				id SERIAL PRIMARY KEY,
				artifact_uuid UUID NOT NULL,
				organization_uuid UUID NOT NULL,
				resource_uuid UUID NOT NULL,
				association_type VARCHAR(20) NOT NULL,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

				CONSTRAINT fk_association_artifact FOREIGN KEY (artifact_uuid)
					REFERENCES artifacts(uuid) ON DELETE CASCADE,
				CONSTRAINT fk_association_organization FOREIGN KEY (organization_uuid)
					REFERENCES organizations(uuid) ON DELETE CASCADE,
				CONSTRAINT uq_association_artifact_resource_type UNIQUE(artifact_uuid, resource_uuid, association_type, organization_uuid),
				CONSTRAINT chk_association_type CHECK (association_type IN ('gateway', 'dev_portal'))
			);

			-- Indexes for performance
			CREATE INDEX idx_artifacts_org ON artifacts(organization_uuid);
			CREATE INDEX idx_deployments_artifact_gateway ON deployments(artifact_uuid, gateway_uuid);
			CREATE INDEX idx_deployments_created_at ON deployments(artifact_uuid, gateway_uuid, created_at);
			CREATE INDEX idx_deployments_org_gateway_created ON deployments(artifact_uuid, organization_uuid, gateway_uuid, created_at DESC);
			CREATE INDEX idx_deployment_status_deployment ON deployment_status(deployment_id);
			CREATE INDEX idx_deployment_status_status ON deployment_status(status);
			CREATE INDEX idx_association_artifact_resource_type ON association_mappings(artifact_uuid, association_type, organization_uuid);
			CREATE INDEX idx_association_resource ON association_mappings(association_type, resource_uuid, organization_uuid);
			CREATE INDEX idx_association_org ON association_mappings(organization_uuid);
		`
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, createArtifactsAndAPIsSQL)
		})
	},
}

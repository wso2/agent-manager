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

import "gorm.io/gorm"

// Organizations, projects, environments and agents move from OpenChoreo into
// this database. IDs and timestamps are set by the application.
var migration060 = migration{
	ID: 60,
	Migrate: func(db *gorm.DB) error {
		return db.Exec(`
CREATE TABLE IF NOT EXISTS organizations (
 id UUID PRIMARY KEY,
 handle VARCHAR(63) NOT NULL,
 display_name VARCHAR(255) NOT NULL,
 description VARCHAR(1000),
 thunder_ou_id VARCHAR(64),
 created_by VARCHAR(255),
 created_at TIMESTAMPTZ NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL,
 CONSTRAINT uq_organizations_handle UNIQUE (handle));
CREATE INDEX IF NOT EXISTS idx_organizations_thunder_ou_id ON organizations(thunder_ou_id);

CREATE TABLE IF NOT EXISTS environments (
 id UUID PRIMARY KEY,
 org_id UUID NOT NULL REFERENCES organizations(id),
 name VARCHAR(63) NOT NULL,
 display_name VARCHAR(255) NOT NULL,
 is_production BOOLEAN NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL,
 CONSTRAINT uq_environments_org_name UNIQUE (org_id, name),
 CONSTRAINT uq_environments_org_id UNIQUE (org_id, id));

CREATE TABLE IF NOT EXISTS projects (
 id UUID PRIMARY KEY,
 org_id UUID NOT NULL REFERENCES organizations(id),
 name VARCHAR(63) NOT NULL,
 display_name VARCHAR(255) NOT NULL,
 description VARCHAR(1000),
 created_by VARCHAR(255),
 created_at TIMESTAMPTZ NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL,
 CONSTRAINT uq_projects_org_name UNIQUE (org_id, name),
 CONSTRAINT uq_projects_org_id UNIQUE (org_id, id));

CREATE TABLE IF NOT EXISTS agents (
 id UUID PRIMARY KEY,
 org_id UUID NOT NULL,
 project_id UUID NOT NULL,
 name VARCHAR(63) NOT NULL,
 display_name VARCHAR(255) NOT NULL,
 description VARCHAR(1000),
 provisioning_type VARCHAR(16) NOT NULL,
 agent_type VARCHAR(32) NOT NULL,
 sub_type VARCHAR(32),
 created_by VARCHAR(255),
 created_at TIMESTAMPTZ NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL,
 CONSTRAINT fk_agents_project FOREIGN KEY (org_id, project_id) REFERENCES projects(org_id, id),
 CONSTRAINT uq_agents_org_name UNIQUE (org_id, name),
 CONSTRAINT uq_agents_org_id UNIQUE (org_id, id));
CREATE INDEX IF NOT EXISTS idx_agents_project ON agents(org_id, project_id);

CREATE TABLE IF NOT EXISTS agent_labels (
 agent_id UUID NOT NULL,
 org_id UUID NOT NULL,
 label_key VARCHAR(63) NOT NULL,
 label_value VARCHAR(63),
 PRIMARY KEY (agent_id, label_key),
 CONSTRAINT fk_agent_labels_agent FOREIGN KEY (org_id, agent_id) REFERENCES agents(org_id, id) ON DELETE CASCADE);
CREATE INDEX IF NOT EXISTS idx_agent_labels_lookup ON agent_labels(org_id, label_key, label_value);
`).Error
	},
}

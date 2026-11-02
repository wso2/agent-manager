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

// a2a_proxies marks an environment as gateway-proxied and holds its encrypted upstream auth.
// a2a_publications gets a source and an environment-UUID key next to its name key.
var migration048 = migration{
	ID: 48,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			createProxies := `
			CREATE TABLE IF NOT EXISTS a2a_proxies (
				id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				ou_id                VARCHAR(255) NOT NULL,
				project_name         VARCHAR(255) NOT NULL,
				agent_name           VARCHAR(255) NOT NULL,
				environment_uuid     UUID NOT NULL,
				environment_name     VARCHAR(255) NOT NULL,
				upstream_auth_header VARCHAR(255) NOT NULL DEFAULT '',
				upstream_auth_secret TEXT NOT NULL DEFAULT '',
				created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),

				CONSTRAINT uq_a2a_proxies_agent_env
					UNIQUE (ou_id, project_name, agent_name, environment_uuid)
			)`
			if err := runSQL(tx, createProxies); err != nil {
				return err
			}

			addSource := `
			ALTER TABLE a2a_publications
				ADD COLUMN IF NOT EXISTS source VARCHAR(32) NOT NULL DEFAULT 'platform'`
			if err := runSQL(tx, addSource); err != nil {
				return err
			}

			// The name key stays until upstream/main pods, which upsert ON CONFLICT on it, are gone.
			// TODO: drop uq_a2a_publications_agent_env in the next release's migration.
			addUUIDKey := `
			ALTER TABLE a2a_publications ADD CONSTRAINT uq_a2a_publications_agent_env_uuid
				UNIQUE (ou_id, project_name, agent_name, environment_uuid)`
			return runSQL(tx, addUUIDKey)
		})
	},
}

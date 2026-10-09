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

// migration048 records the environments an agent's MCP connection was deliberately
// removed from.
//
// Until now a connection's environment scope was inferred from its env var rows: an
// environment with rows was in scope, one without was not. That cannot tell "never
// included" apart from "removed on purpose" — `amctl agent mcp unset --env stage` deletes
// stage's rows, and so does nothing at all for a stage that joined the deployment pipeline
// after the connection was attached. The binding reconcile therefore had to choose between
// resurrecting deliberate removals and ignoring every environment added later, and it chose
// the latter, which left a connection attached before its environments existed unbindable
// without a detach and re-attach.
//
// Recording the removal itself breaks the tie: a connection is in scope in every
// environment of its project's pipeline except the ones listed here.
//
// Same shape as agent_mcp_config_proxy (migration047): the composite foreign key onto
// agent_configurations (uuid, type_id), with type_id pinned to MCP, makes it the
// database's rule that only an MCP configuration can carry an exclusion, and ON DELETE
// CASCADE drops them with the configuration.
//
// No backfill. Pre-migration, a deliberate removal left no trace beyond missing rows, so
// there is nothing to reconstruct one from; existing connections start with an empty
// exclusion set. An environment unset before this migration could therefore be re-bound
// by the reconcile once its proxy serves it — re-run the unset to record it.
var migration048 = migration{
	ID: 48,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(
				tx,
				`CREATE TABLE IF NOT EXISTS agent_mcp_config_env_exclusion (
					config_uuid UUID NOT NULL,
					environment_uuid UUID NOT NULL,
					type_id INTEGER NOT NULL DEFAULT 2 CHECK (type_id = 2),
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

					PRIMARY KEY (config_uuid, environment_uuid),
					CONSTRAINT fk_mcp_env_exclusion_config
						FOREIGN KEY (config_uuid, type_id)
						REFERENCES agent_configurations (uuid, type_id)
						ON DELETE CASCADE
				)`,
			)
		})
	},
}

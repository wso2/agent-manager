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

// migration044 records the MCP proxy an agent's MCP connection references, in a side table
// keyed by the configuration.
//
// Until now the only link from an agent configuration to an org-level MCP proxy was the
// per-environment env_agent_mcp_mapping row. That made the link environment-scoped even
// though the connection is environment-agnostic by construction (every environment maps to
// the same proxy), so a configuration with no mapping in a given environment — the state
// left behind when the proxy has no endpoint there yet — was unreachable from the proxy
// side. Nothing could backfill a binding it could not find, and the connection stayed dead
// until someone detached and re-attached it.
//
// The reference lives in its own table rather than as a column on agent_configurations,
// because that table is polymorphic: type_id already distinguishes LLM (1), MCP (2) and
// agent (3) configurations. An MCP-only column there would be NULL for two thirds of the
// rows, and nothing would stop an LLM configuration from carrying one. A side table keyed
// by the configuration keeps agent_configurations generic and leaves room for the same
// shape per type (an agent_llm_config_provider, say) without widening the shared row.
//
// Two constraints carry invariants a column could only document:
//
//   - CHECK (type_id = 2), with the composite foreign key onto (uuid, type_id), makes
//     "only an MCP configuration may reference an MCP proxy" the database's rule rather
//     than a convention readers have to trust. The redundant type_id column is what the
//     composite key needs in order to express it, and uq_agent_config_uuid_type is what
//     lets it be referenced at all — Postgres requires a unique constraint over exactly
//     the referenced columns, and the bare primary key on uuid does not qualify.
//   - mcp_proxy_uuid is NOT NULL with ON DELETE RESTRICT. The row's presence is the whole
//     signal, so there is no NULL to interpret, and a proxy cannot be deleted while a
//     configuration still references it. MCPProxyService.Delete checks for references
//     first, but it reads them outside the delete transaction; a configuration committed
//     between that read and the delete would otherwise be stranded with neither a mapping
//     row nor a proxy to reconcile against. RESTRICT makes that the database's invariant
//     to keep rather than the service's to race, and MCPProxyRepo.Delete already maps the
//     resulting 23503 violation onto ErrMCPProxyHasMappings, so the caller-facing error is
//     unchanged.
//
// ON DELETE CASCADE on the configuration side: the reference means nothing without the
// configuration it describes.
var migration044 = migration{
	ID: 44,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			if err := runSQL(
				tx,
				// uuid is already the primary key, so this adds no uniqueness. It exists
				// only as the target the composite foreign key below must reference.
				`ALTER TABLE agent_configurations
					DROP CONSTRAINT IF EXISTS uq_agent_config_uuid_type`,
				`ALTER TABLE agent_configurations
					ADD CONSTRAINT uq_agent_config_uuid_type UNIQUE (uuid, type_id)`,
				`CREATE TABLE IF NOT EXISTS agent_mcp_config_proxy (
					config_uuid UUID PRIMARY KEY,
					type_id INTEGER NOT NULL DEFAULT 2 CHECK (type_id = 2),
					mcp_proxy_uuid UUID NOT NULL
						REFERENCES mcp_proxies(uuid) ON DELETE RESTRICT,
					created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

					CONSTRAINT fk_mcp_ref_config
						FOREIGN KEY (config_uuid, type_id)
						REFERENCES agent_configurations (uuid, type_id)
						ON DELETE CASCADE
				)`,
				`CREATE INDEX IF NOT EXISTS idx_agent_mcp_config_proxy_proxy
					ON agent_mcp_config_proxy (mcp_proxy_uuid)`,
			); err != nil {
				return err
			}

			// Backfill from the mapping rows, the pre-migration source of truth. HAVING
			// COUNT(DISTINCT ...) = 1 restricts this to configurations whose environments
			// all agree on one proxy; a divergent configuration gets no row rather than
			// having one of its proxies guessed for it, and readers fall back to its
			// mapping rows exactly as before.
			//
			// The single agreed value is taken with array_agg rather than MIN, which
			// Postgres does not define for uuid. Given the HAVING clause the aggregated
			// array holds exactly one element, so indexing it is a projection, not a
			// choice between candidates.
			return runSQL(
				tx,
				`INSERT INTO agent_mcp_config_proxy (config_uuid, type_id, mcp_proxy_uuid)
				 SELECT agreed.config_uuid, 2, agreed.mcp_proxy_uuid
				 FROM (
					SELECT m.config_uuid,
					       (array_agg(DISTINCT m.mcp_proxy_uuid))[1] AS mcp_proxy_uuid
					FROM env_agent_mcp_mapping m
					JOIN agent_configurations c ON c.uuid = m.config_uuid
					WHERE c.type_id = 2
					GROUP BY m.config_uuid
					HAVING COUNT(DISTINCT m.mcp_proxy_uuid) = 1
				 ) AS agreed
				 ON CONFLICT (config_uuid) DO NOTHING`,
			)
		})
	},
}

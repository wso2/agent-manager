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

// migration044 records the MCP proxy an agent's MCP connection references on the
// configuration row itself.
//
// Until now the only link from an agent configuration to an org-level MCP proxy was
// the per-environment env_agent_mcp_mapping row. That made the link environment-scoped
// even though the connection is environment-agnostic by construction (every environment
// maps to the same proxy), so a configuration with no mapping in a given environment —
// the state left behind when the proxy has no endpoint there yet — was unreachable from
// the proxy side. Nothing could backfill a binding it could not find, and the connection
// stayed dead until someone detached and re-attached it.
//
// mcp_proxy_uuid makes the reference environment-independent, so the binding reconcile
// can start from "which configurations reference this proxy" instead of "which
// configurations already have a mapping row for it".
//
// Nullable on purpose. A configuration whose environments name DIFFERENT proxies records
// no single environment-agnostic intent, so it is left NULL and keeps resolving through
// its mapping rows exactly as before.
//
// ON DELETE RESTRICT, not SET NULL. MCPProxyService.Delete refuses a proxy still
// referenced here, but it reads the references outside the delete transaction, so a
// configuration committed between that read and the delete would slip through. SET NULL
// would then clear the committed reference and strand the connection with neither a
// mapping row nor a proxy to reconcile against — the exact state this column exists to
// make repairable. RESTRICT makes the invariant the database's to keep rather than the
// service's to race, and MCPProxyRepo.Delete already maps the resulting 23503 violation
// onto ErrMCPProxyHasMappings, so the caller-facing error is unchanged.
var migration044 = migration{
	ID: 44,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			if err := runSQL(
				tx,
				`ALTER TABLE agent_configurations ADD COLUMN IF NOT EXISTS mcp_proxy_uuid UUID`,
				`ALTER TABLE agent_configurations DROP CONSTRAINT IF EXISTS fk_agent_config_mcp_proxy`,
				`ALTER TABLE agent_configurations ADD CONSTRAINT fk_agent_config_mcp_proxy
					FOREIGN KEY (mcp_proxy_uuid) REFERENCES mcp_proxies(uuid) ON DELETE RESTRICT`,
				`CREATE INDEX IF NOT EXISTS idx_agent_configurations_mcp_proxy_uuid
					ON agent_configurations (mcp_proxy_uuid) WHERE mcp_proxy_uuid IS NOT NULL`,
			); err != nil {
				return err
			}

			// Backfill from the mapping rows, which are the pre-migration source of truth.
			// HAVING COUNT(DISTINCT ...) = 1 restricts this to configurations whose
			// environments all agree on one proxy; a divergent configuration stays NULL
			// rather than having one of its proxies guessed for it.
			//
			// The single agreed value is taken with array_agg rather than MIN, which
			// Postgres does not define for uuid. Given the HAVING clause the aggregated
			// array holds exactly one element, so indexing it is a projection, not a
			// choice between candidates.
			return runSQL(
				tx,
				`UPDATE agent_configurations c
				 SET mcp_proxy_uuid = agreed.mcp_proxy_uuid
				 FROM (
					SELECT config_uuid, (array_agg(DISTINCT mcp_proxy_uuid))[1] AS mcp_proxy_uuid
					FROM env_agent_mcp_mapping
					GROUP BY config_uuid
					HAVING COUNT(DISTINCT mcp_proxy_uuid) = 1
				 ) AS agreed
				 WHERE c.uuid = agreed.config_uuid AND c.mcp_proxy_uuid IS NULL`,
			)
		})
	},
}

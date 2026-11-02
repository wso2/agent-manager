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

// migration051 moves gateway deployments onto one ack-driven status model.
//
//   - deployment_status records what the gateway holds: six states, the desired state, the gateway's reason,
//     the transition's performed_at, and the render queue. Existing DEPLOYED and UNDEPLOYED rows keep their
//     meaning. status_desired defaults to DEPLOYED so an insert that doesn't name it still passes the CHECKs.
//   - a2a_publications becomes platform_agent_exposures: the record that AMS exposes a platform-hosted agent in
//     an environment. External agents are recorded by a2a_proxies, and publication state moves to
//     deployment_status, so external rows, orphans and the queue columns go. Every platform row is kept. A
//     publication still pending or failed over a deployed agent leaves that agent's rows due for a redeploy.
//   - association_mappings was never written, and organization_name has held an empty string since 029.
//   - event hub events get a per-gateway sequence, numbered under the gateway-state row lock, for the poller's cursor.
//
// Old and new instances can't share the migrated schema: stop the old ones before this runs.
var migration051 = migration{
	ID: 51,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			for _, step := range [][]string{
				deploymentStatusLifecycle(),
				platformAgentExposures(),
				dropOrganizationName(),
				{`DROP TABLE association_mappings`},
				eventHubSequence(),
			} {
				if err := runSQL(tx, step...); err != nil {
					return err
				}
			}
			return nil
		})
	},
}

func deploymentStatusLifecycle() []string {
	return []string{
		`ALTER TABLE deployment_status
			DROP CONSTRAINT chk_deployment_status,
			ADD COLUMN status_desired  VARCHAR(20) NOT NULL DEFAULT 'DEPLOYED',
			ADD COLUMN status_reason   TEXT NOT NULL DEFAULT '',
			ADD COLUMN performed_at    TIMESTAMPTZ,
			ADD COLUMN next_attempt_at TIMESTAMPTZ,
			ADD COLUMN attempt_count   INTEGER NOT NULL DEFAULT 0,
			ADD COLUMN render_reason   TEXT NOT NULL DEFAULT '',
			ALTER COLUMN deployment_id DROP NOT NULL`,
		// updated_at is a zone-less TIMESTAMP written in the session's zone.
		`UPDATE deployment_status SET
			status_desired = status,
			performed_at = date_trunc('milliseconds',
				COALESCE(updated_at AT TIME ZONE current_setting('TimeZone'), NOW()))`,
		`ALTER TABLE deployment_status
			ALTER COLUMN performed_at SET NOT NULL,
			ADD CONSTRAINT chk_deployment_status CHECK (status IN
				('PENDING', 'DEPLOYING', 'DEPLOYED', 'UNDEPLOYING', 'UNDEPLOYED', 'FAILED')),
			ADD CONSTRAINT chk_deployment_status_desired CHECK (
				(status_desired = 'DEPLOYED' AND status IN ('PENDING', 'DEPLOYING', 'DEPLOYED', 'FAILED'))
				OR (status_desired = 'UNDEPLOYED' AND status IN ('UNDEPLOYING', 'UNDEPLOYED', 'FAILED'))),
			ADD CONSTRAINT chk_deployment_status_deployment_id CHECK (
				deployment_id IS NOT NULL OR status IN ('PENDING', 'UNDEPLOYED'))`,
		`CREATE INDEX idx_deployment_status_next_attempt ON deployment_status (next_attempt_at)
			WHERE next_attempt_at IS NOT NULL`,
		`CREATE INDEX idx_deployment_status_gateway ON deployment_status (gateway_uuid, ou_id)`,
	}
}

func platformAgentExposures() []string {
	return []string{
		// Before the external rows go: their cards need the agent too.
		`ALTER TABLE a2a_agent_cards ADD COLUMN agent_uuid UUID`,
		`UPDATE a2a_agent_cards c SET agent_uuid = p.agent_uuid
			FROM a2a_publications p
			WHERE p.ou_id = c.ou_id AND p.project_name = c.project_name
				AND p.agent_name = c.agent_name AND p.environment_uuid = c.environment_uuid`,

		// A pending or failed publication of an agent already deployed owes a redeploy: queue it on the agent's rows
		// before its status goes, so the worker re-renders it.
		`UPDATE deployment_status s SET next_attempt_at = NOW(), attempt_count = 0, render_reason = ''
			FROM a2a_publications p
			WHERE p.artifact_uuid = s.artifact_uuid AND p.ou_id = s.ou_id
				AND p.status IN ('pending', 'failed') AND s.status_desired = 'DEPLOYED'`,

		`DELETE FROM a2a_publications WHERE source = 'external'`,
		`DELETE FROM a2a_publications p WHERE NOT EXISTS (SELECT 1 FROM artifacts a WHERE a.uuid = p.artifact_uuid)`,
		`DELETE FROM a2a_publications WHERE id NOT IN (
			SELECT DISTINCT ON (artifact_uuid) id FROM a2a_publications
			ORDER BY artifact_uuid, updated_at DESC, created_at DESC, id DESC)`,

		`DROP INDEX idx_a2a_publications_due`,
		`ALTER TABLE a2a_publications
			DROP CONSTRAINT uq_a2a_publications_agent_env,
			DROP COLUMN status,
			DROP COLUMN attempt_count,
			DROP COLUMN last_error,
			DROP COLUMN next_attempt_at,
			DROP COLUMN published_upstream_url,
			DROP COLUMN source,
			ADD COLUMN protocol VARCHAR(16) NOT NULL DEFAULT 'a2a',
			ADD CONSTRAINT chk_platform_agent_exposures_protocol CHECK (protocol IN ('a2a')),
			ADD CONSTRAINT uq_platform_agent_exposures_artifact UNIQUE (artifact_uuid),
			ADD CONSTRAINT fk_platform_agent_exposures_artifact FOREIGN KEY (artifact_uuid)
				REFERENCES artifacts(uuid) ON DELETE CASCADE,
			ALTER COLUMN created_at TYPE TIMESTAMPTZ,
			ALTER COLUMN updated_at TYPE TIMESTAMPTZ`,

		`ALTER TABLE a2a_publications RENAME TO platform_agent_exposures`,
		`ALTER TABLE platform_agent_exposures RENAME CONSTRAINT a2a_publications_pkey TO platform_agent_exposures_pkey`,
		`ALTER TABLE platform_agent_exposures
			RENAME CONSTRAINT uq_a2a_publications_agent_env_uuid TO uq_platform_agent_exposures_agent_env`,
		`CREATE INDEX idx_platform_agent_exposures_env ON platform_agent_exposures (ou_id, environment_uuid)`,
	}
}

// dropOrganizationName drops the column from the tables whose keys 030 moved to ou_id. Postgres would silently
// drop any index or constraint still on the column with it, so the guard refuses rather than lose a key. NOT NULL
// constraints, which Postgres 18 lists in pg_constraint, aren't keys and go with the column.
func dropOrganizationName() []string {
	return []string{
		`DO $$
		DECLARE
			leftover text;
		BEGIN
			SELECT string_agg(name, ', ') INTO leftover FROM (
				SELECT c.conrelid::regclass || '.' || c.conname AS name
				FROM pg_constraint c
				JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY (c.conkey)
				WHERE a.attname = 'organization_name' AND c.contype <> 'n'
					AND c.conrelid IN ('artifacts'::regclass, 'deployments'::regclass, 'deployment_status'::regclass)
				UNION
				SELECT i.indrelid::regclass || '.' || i.indexrelid::regclass
				FROM pg_index i
				JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY (i.indkey)
				WHERE a.attname = 'organization_name'
					AND i.indrelid IN ('artifacts'::regclass, 'deployments'::regclass, 'deployment_status'::regclass)
			) refs;
			IF leftover IS NOT NULL THEN
				RAISE EXCEPTION 'organization_name is still part of %; move it to ou_id first (see migration 030)', leftover;
			END IF;
		END $$`,
		`ALTER TABLE artifacts DROP COLUMN organization_name`,
		`ALTER TABLE deployments DROP COLUMN organization_name`,
		`ALTER TABLE deployment_status DROP COLUMN organization_name`,
	}
}

func eventHubSequence() []string {
	return []string{
		`ALTER TABLE eventhub_gateway_states ADD COLUMN seq BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE eventhub_events ADD COLUMN seq BIGINT`,
		`UPDATE eventhub_events e SET seq = numbered.seq
			FROM (
				SELECT event_id, row_number() OVER (PARTITION BY gateway_id ORDER BY processed_timestamp, event_id) AS seq
				FROM eventhub_events
			) numbered
			WHERE e.event_id = numbered.event_id`,
		`UPDATE eventhub_gateway_states s SET seq = latest.seq
			FROM (SELECT gateway_id, max(seq) AS seq FROM eventhub_events GROUP BY gateway_id) latest
			WHERE s.gateway_id = latest.gateway_id`,
		`ALTER TABLE eventhub_events ALTER COLUMN seq SET NOT NULL`,
		`CREATE INDEX idx_eventhub_events_gateway_seq ON eventhub_events (gateway_id, seq)`,
	}
}

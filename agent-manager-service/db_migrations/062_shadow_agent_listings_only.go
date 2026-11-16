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

// migration062 narrows Shadow Agent Discovery to the agents the platforms
// list (their agent listing APIs only): findings of other kinds (API keys,
// IAM principals, projects and workspaces, tool infrastructure) and the
// evidence of other sources (usage, CloudTrail, metrics, logs, deep
// inspection) are deleted, with the scores, verdicts, usage, keys,
// onboarding and credential retirement that came with them. A finding gains
// "present": false once a complete listing of its connection no longer
// returns the agent.
var migration062 = migration{
	ID: 62,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx,
				`DELETE FROM shadow_findings WHERE kind NOT IN ('platform_agent', 'runtime')`,
				// A runtime that was not READY had its own code; every runtime
				// is now one kind of evidence, with its status observed. A
				// runtime that had both (it became READY) keeps the READY row.
				`DELETE FROM shadow_finding_signals n
				WHERE n.code = 'bedrock.agentcore_runtime_not_ready' AND EXISTS (
					SELECT 1 FROM shadow_finding_signals r
					WHERE r.finding_id = n.finding_id AND r.source = n.source AND r.code = 'bedrock.agentcore_runtime')`,
				`UPDATE shadow_finding_signals SET code = 'bedrock.agentcore_runtime'
				WHERE code = 'bedrock.agentcore_runtime_not_ready'`,
				`DELETE FROM shadow_finding_signals WHERE code NOT IN (
					'openai.agent_object', 'anthropic.managed_agent',
					'bedrock.agentcore_runtime', 'bedrock.registry_agent_record', 'bedrock.classic_agent',
					'google.agent_runtime_engine', 'google.registry_agent',
					'foundry.agent_object', 'foundry.classic_agent', 'foundry.external_possible_duplicate')`,
				// Only agent evidence counts now: a finding with none left is not an agent.
				`DELETE FROM shadow_findings f WHERE NOT EXISTS (
					SELECT 1 FROM shadow_finding_signals s
					WHERE s.finding_id = f.id AND s.code <> 'foundry.external_possible_duplicate')`,
				`ALTER TABLE shadow_finding_signals
					DROP COLUMN IF EXISTS tier,
					DROP COLUMN IF EXISTS weight,
					DROP COLUMN IF EXISTS condition`,
				`DROP INDEX IF EXISTS idx_shadow_findings_org`,
				`ALTER TABLE shadow_findings
					DROP COLUMN IF EXISTS score,
					DROP COLUMN IF EXISTS verdict,
					DROP COLUMN IF EXISTS first_used_at,
					DROP COLUMN IF EXISTS last_used_at,
					DROP COLUMN IF EXISTS linked_project,
					DROP COLUMN IF EXISTS linked_agent,
					DROP COLUMN IF EXISTS onboarded_at,
					DROP COLUMN IF EXISTS onboarded_by,
					DROP COLUMN IF EXISTS retirement_state,
					DROP COLUMN IF EXISTS retirement_marked_at,
					DROP COLUMN IF EXISTS retirement_marked_by,
					DROP COLUMN IF EXISTS retirement_verified_at,
					ADD COLUMN IF NOT EXISTS present BOOLEAN NOT NULL DEFAULT TRUE`,
				`CREATE INDEX IF NOT EXISTS idx_shadow_findings_agent
				ON shadow_findings (ou_id, provider, kind, external_id)`,
				`UPDATE shadow_findings SET status = 'reviewed' WHERE status IN ('onboarded', 'retired')`,
				// Owner hints came from API keys, audit logs and CloudTrail, which
				// are no longer read; the agent listings set them again.
				`UPDATE shadow_findings SET owner_hint = '{}'::jsonb WHERE owner_hint <> '{}'::jsonb`,
				// Only possible duplicates across connections remain as relations.
				`UPDATE shadow_findings SET related = COALESCE((
					SELECT jsonb_agg(r) FROM jsonb_array_elements(related) r
					WHERE r->>'relation' = 'possible_duplicate'), '[]'::jsonb)
				WHERE related <> '[]'::jsonb`,
				`DROP TABLE IF EXISTS shadow_usage_daily`,
				`DROP TABLE IF EXISTS shadow_api_keys`,
				`ALTER TABLE shadow_scans DROP COLUMN IF EXISTS lookback_days`,
				// Connection settings of the sources no longer read.
				`UPDATE shadow_connections SET config = config - 'deepInspection' WHERE config ? 'deepInspection'`,
				`UPDATE shadow_connections SET config = jsonb_set(config, '{disabledServices}', COALESCE((
					SELECT jsonb_agg(v) FROM jsonb_array_elements(config->'disabledServices') v
					WHERE v #>> '{}' IN ('scanAgentCore', 'scanAgentRegistry', 'scanClassicAgents')), '[]'::jsonb))
				WHERE jsonb_typeof(config->'disabledServices') = 'array'`,
				`UPDATE shadow_connections SET config = jsonb_set(config, '{endpoints}', COALESCE((
					SELECT jsonb_object_agg(e.key, e.value) FROM jsonb_each(config->'endpoints') e
					WHERE split_part(e.key, '@', 1) NOT IN ('bedrock', 'cloudwatch', 'logs', 'cloudtrail')), '{}'::jsonb))
				WHERE provider = 'bedrock' AND jsonb_typeof(config->'endpoints') = 'object'`,
				`UPDATE shadow_connections SET config = jsonb_set(config, '{endpoints}', COALESCE((
					SELECT jsonb_object_agg(e.key, e.value) FROM jsonb_each(config->'endpoints') e
					WHERE split_part(e.key, '@', 1) NOT IN ('monitoring', 'logging')), '{}'::jsonb))
				WHERE provider = 'google' AND jsonb_typeof(config->'endpoints') = 'object'`,
				// The Anthropic cloud variants were removed earlier; their
				// endpoints may still be stored.
				`UPDATE shadow_connections SET config = jsonb_set(config, '{endpoints}', COALESCE((
					SELECT jsonb_object_agg(e.key, e.value) FROM jsonb_each(config->'endpoints') e
					WHERE e.key = 'api'), '{}'::jsonb))
				WHERE provider = 'anthropic' AND jsonb_typeof(config->'endpoints') = 'object'`,
			)
		})
	},
}

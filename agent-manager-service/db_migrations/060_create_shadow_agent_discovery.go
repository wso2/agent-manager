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

// migration060 creates the Shadow Agent Discovery schema: the provider
// connections an org scans, the scans themselves, and what they found.
//
// Data retention (see docs/shadow-agent-discovery.md): no table holds a
// provider secret in plain text, a prompt, a completion or a tool payload.
// Keys are recorded by ID, name, redacted hint, status and timestamps only;
// usage is kept as daily aggregates per key and model.
var migration060 = migration{
	ID: 60,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx, `
			CREATE TABLE IF NOT EXISTS shadow_connections (
				id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				ou_id            TEXT NOT NULL,
				name             TEXT NOT NULL,
				provider         TEXT NOT NULL,
				config           JSONB NOT NULL DEFAULT '{}',
				secret_encrypted BYTEA NOT NULL,
				secret_hints     JSONB NOT NULL DEFAULT '{}',
				last_tested_at   TIMESTAMPTZ,
				last_test_result JSONB,
				created_by       TEXT NOT NULL DEFAULT '',
				created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`,
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_shadow_connections_name
				ON shadow_connections (ou_id, lower(name))`,
				`CREATE TABLE IF NOT EXISTS shadow_scans (
				id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				ou_id          TEXT NOT NULL,
				status         TEXT NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'partial', 'failed')),
				lookback_days  INTEGER NOT NULL,
				connection_ids JSONB NOT NULL DEFAULT '[]',
				results        JSONB NOT NULL DEFAULT '[]',
				findings_count INTEGER NOT NULL DEFAULT 0,
				requested_by   TEXT NOT NULL DEFAULT '',
				claimed_until  TIMESTAMPTZ,
				attempts       INTEGER NOT NULL DEFAULT 0,
				started_at     TIMESTAMPTZ,
				finished_at    TIMESTAMPTZ,
				created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`,
				`CREATE INDEX IF NOT EXISTS idx_shadow_scans_org_created
				ON shadow_scans (ou_id, created_at DESC)`,
				`CREATE INDEX IF NOT EXISTS idx_shadow_scans_due
				ON shadow_scans (status, claimed_until)`,
				// At most one queued or running scan per org, enforced by the
				// database rather than a read-then-write check.
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_shadow_scans_one_active
				ON shadow_scans (ou_id) WHERE status IN ('queued', 'running')`,
				`CREATE TABLE IF NOT EXISTS shadow_findings (
				id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				ou_id                  TEXT NOT NULL,
				connection_id          UUID NOT NULL REFERENCES shadow_connections(id) ON DELETE CASCADE,
				provider               TEXT NOT NULL,
				kind                   TEXT NOT NULL,
				external_id            TEXT NOT NULL,
				display_name           TEXT NOT NULL DEFAULT '',
				scope                  TEXT NOT NULL DEFAULT '',
				state                  TEXT NOT NULL DEFAULT '',
				score                  INTEGER NOT NULL DEFAULT 0,
				verdict                TEXT NOT NULL DEFAULT 'llm_consumer',
				status                 TEXT NOT NULL DEFAULT 'new',
				owner_hint             JSONB NOT NULL DEFAULT '{}',
				attributes             JSONB NOT NULL DEFAULT '{}',
				related                JSONB NOT NULL DEFAULT '[]',
				provider_created_at    TIMESTAMPTZ,
				first_used_at          TIMESTAMPTZ,
				last_used_at           TIMESTAMPTZ,
				first_seen_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				last_seen_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				last_scan_id           UUID,
				linked_project         TEXT NOT NULL DEFAULT '',
				linked_agent           TEXT NOT NULL DEFAULT '',
				onboarded_at           TIMESTAMPTZ,
				onboarded_by           TEXT NOT NULL DEFAULT '',
				retirement_state       TEXT NOT NULL DEFAULT '',
				retirement_marked_at   TIMESTAMPTZ,
				retirement_marked_by   TEXT NOT NULL DEFAULT '',
				retirement_verified_at TIMESTAMPTZ,
				created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE (ou_id, connection_id, kind, external_id)
			)`,
				`CREATE INDEX IF NOT EXISTS idx_shadow_findings_org
				ON shadow_findings (ou_id, verdict, score DESC)`,
				`CREATE TABLE IF NOT EXISTS shadow_finding_signals (
				id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				ou_id       TEXT NOT NULL,
				finding_id  UUID NOT NULL REFERENCES shadow_findings(id) ON DELETE CASCADE,
				code        TEXT NOT NULL,
				tier        TEXT NOT NULL,
				weight      INTEGER NOT NULL,
				source      TEXT NOT NULL,
				condition   TEXT NOT NULL DEFAULT '',
				observed    JSONB NOT NULL DEFAULT '{}',
				scan_id     UUID,
				detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE (finding_id, code, source)
			)`,
				`CREATE TABLE IF NOT EXISTS shadow_api_keys (
				id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				ou_id               TEXT NOT NULL,
				connection_id       UUID NOT NULL REFERENCES shadow_connections(id) ON DELETE CASCADE,
				finding_id          UUID REFERENCES shadow_findings(id) ON DELETE SET NULL,
				key_id              TEXT NOT NULL,
				name                TEXT NOT NULL DEFAULT '',
				redacted_hint       TEXT NOT NULL DEFAULT '',
				status              TEXT NOT NULL DEFAULT '',
				scope               TEXT NOT NULL DEFAULT '',
				owner_hint          JSONB NOT NULL DEFAULT '{}',
				provider_created_at TIMESTAMPTZ,
				last_used_at        TIMESTAMPTZ,
				last_seen_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				present             BOOLEAN NOT NULL DEFAULT TRUE,
				created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE (ou_id, connection_id, key_id)
			)`,
				`CREATE INDEX IF NOT EXISTS idx_shadow_api_keys_finding
				ON shadow_api_keys (finding_id)`,
				`CREATE TABLE IF NOT EXISTS shadow_usage_daily (
				ou_id        TEXT NOT NULL,
				finding_id   UUID NOT NULL REFERENCES shadow_findings(id) ON DELETE CASCADE,
				key_id       TEXT NOT NULL DEFAULT '',
				model        TEXT NOT NULL DEFAULT '',
				day          DATE NOT NULL,
				requests     BIGINT NOT NULL DEFAULT 0,
				input_tokens BIGINT NOT NULL DEFAULT 0,
				output_tokens BIGINT NOT NULL DEFAULT 0,
				cost_amount  NUMERIC(20, 6),
				currency     TEXT NOT NULL DEFAULT '',
				updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (finding_id, key_id, model, day)
			)`,
				`CREATE INDEX IF NOT EXISTS idx_shadow_usage_daily_org
				ON shadow_usage_daily (ou_id, day)`,
			)
		})
	},
}

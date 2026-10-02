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

// Alerting: one HTTP endpoint per org, per-monitor alert rules, and an outbox
// of deliveries the dispatcher drains. Header values and the signing secret
// are stored encrypted with the service encryption key.
var migration047 = migration{
	ID: 47,
	Migrate: func(db *gorm.DB) error {
		return db.Exec(`
		CREATE TABLE IF NOT EXISTS alert_endpoints (
			ou_id                    TEXT PRIMARY KEY,
			url                      TEXT NOT NULL,
			enabled                  BOOLEAN NOT NULL DEFAULT TRUE,
			header_names             JSONB NOT NULL DEFAULT '[]',
			headers_encrypted        BYTEA,
			signing_secret_encrypted BYTEA NOT NULL,
			consecutive_failures     INTEGER NOT NULL DEFAULT 0,
			last_success_at          TIMESTAMPTZ,
			last_failure_at          TIMESTAMPTZ,
			created_by               TEXT NOT NULL DEFAULT '',
			created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS monitor_alert_configs (
			monitor_id               UUID PRIMARY KEY REFERENCES monitors(id) ON DELETE CASCADE,
			enabled                  BOOLEAN NOT NULL DEFAULT FALSE,
			alert_on_run_failure     BOOLEAN NOT NULL DEFAULT TRUE,
			thresholds               JSONB NOT NULL DEFAULT '[]',
			cooldown_minutes         INTEGER NOT NULL DEFAULT 60,
			consecutive_breaches     INTEGER NOT NULL DEFAULT 1,
			consecutive_breach_count INTEGER NOT NULL DEFAULT 0,
			last_alerted_at          TIMESTAMPTZ,
			created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS alert_deliveries (
			id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			ou_id              TEXT NOT NULL,
			event_id           TEXT NOT NULL UNIQUE,
			event_type         TEXT NOT NULL,
			monitor_id         UUID,
			monitor_name       TEXT NOT NULL DEFAULT '',
			payload            JSONB NOT NULL,
			status             TEXT NOT NULL DEFAULT 'pending',
			attempts           INTEGER NOT NULL DEFAULT 0,
			next_attempt_at    TIMESTAMPTZ,
			last_response_code INTEGER,
			last_error         TEXT NOT NULL DEFAULT '',
			delivered_at       TIMESTAMPTZ,
			created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_alert_deliveries_due
			ON alert_deliveries (next_attempt_at) WHERE status = 'pending';
		CREATE INDEX IF NOT EXISTS idx_alert_deliveries_org_created
			ON alert_deliveries (ou_id, created_at DESC);
		`).Error
	},
}

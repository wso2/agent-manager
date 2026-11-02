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

// alert_channels are the destinations events are delivered to, at org,
// project or agent scope; kind decides how (webhook today) and config holds
// the kind's settings. alert_deliveries records the outcome of each event
// sent to each channel.
var migration051 = migration{
	ID: 51,
	Migrate: func(db *gorm.DB) error {
		return db.Exec(`
		CREATE TABLE IF NOT EXISTS alert_channels (
			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			ou_id            TEXT NOT NULL,
			scope            TEXT NOT NULL CHECK (scope IN ('org', 'project', 'agent')),
			project_name     TEXT NOT NULL DEFAULT '',
			agent_name       TEXT NOT NULL DEFAULT '',
			environments     JSONB NOT NULL DEFAULT '[]',
			name             TEXT NOT NULL,
			description      TEXT NOT NULL DEFAULT '',
			kind             TEXT NOT NULL,
			config           JSONB NOT NULL DEFAULT '{}',
			event_types      JSONB NOT NULL DEFAULT '[]',
			enabled          BOOLEAN NOT NULL DEFAULT TRUE,
			secret_encrypted BYTEA,
			created_by       TEXT NOT NULL DEFAULT '',
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_alert_channels_target
			ON alert_channels (ou_id, scope, project_name, agent_name);

		CREATE TABLE IF NOT EXISTS alert_deliveries (
			id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			channel_id    UUID NOT NULL REFERENCES alert_channels(id) ON DELETE CASCADE,
			ou_id         TEXT NOT NULL,
			event_id      TEXT NOT NULL,
			event_type    TEXT NOT NULL,
			status        TEXT NOT NULL,
			attempts      INTEGER NOT NULL DEFAULT 0,
			response_code INTEGER,
			last_error    TEXT NOT NULL DEFAULT '',
			delivered_at  TIMESTAMPTZ,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (channel_id, event_id)
		);
		CREATE INDEX IF NOT EXISTS idx_alert_deliveries_channel_created
			ON alert_deliveries (channel_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_alert_deliveries_created
			ON alert_deliveries (created_at);
		`).Error
	},
}

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

// migration053 splits alert channels into event destinations, which org
// admins register once, and event subscriptions, which pick events and a
// destination at org, project or agent level. event_deliveries replaces
// alert_deliveries with one row per event and destination.
//
// Each alert channel becomes a destination and a subscription sharing its ID,
// so existing channels keep working: an org channel's destination is
// available to org subscriptions only, a project or agent channel's to its
// project. A channel could only hold event types of its own scope, which
// reach a subscription at that scope exactly as they reached the channel.
var migration053 = migration{
	ID: 53,
	Migrate: func(db *gorm.DB) error {
		return db.Exec(`
		CREATE TABLE IF NOT EXISTS event_destinations (
			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			ou_id            TEXT NOT NULL,
			name             TEXT NOT NULL,
			description      TEXT NOT NULL DEFAULT '',
			kind             TEXT NOT NULL,
			config           JSONB NOT NULL DEFAULT '{}',
			secret_encrypted BYTEA,
			availability     TEXT NOT NULL DEFAULT 'all' CHECK (availability IN ('all', 'org', 'projects')),
			projects         JSONB NOT NULL DEFAULT '[]',
			created_by       TEXT NOT NULL DEFAULT '',
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CHECK (availability = 'projects' OR projects = '[]'::jsonb)
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_event_destinations_name
			ON event_destinations (ou_id, lower(name));

		CREATE TABLE IF NOT EXISTS event_subscriptions (
			id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			ou_id          TEXT NOT NULL,
			scope          TEXT NOT NULL CHECK (scope IN ('org', 'project', 'agent')),
			project_name   TEXT NOT NULL DEFAULT '',
			agent_name     TEXT NOT NULL DEFAULT '',
			name           TEXT NOT NULL,
			description    TEXT NOT NULL DEFAULT '',
			destination_id UUID NOT NULL REFERENCES event_destinations(id) ON DELETE RESTRICT,
			event_types    JSONB NOT NULL DEFAULT '[]',
			environments   JSONB NOT NULL DEFAULT '[]',
			enabled        BOOLEAN NOT NULL DEFAULT TRUE,
			created_by     TEXT NOT NULL DEFAULT '',
			created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_event_subscriptions_target
			ON event_subscriptions (ou_id, project_name, agent_name);
		CREATE INDEX IF NOT EXISTS idx_event_subscriptions_destination
			ON event_subscriptions (destination_id);

		CREATE TABLE IF NOT EXISTS event_deliveries (
			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			ou_id            TEXT NOT NULL,
			destination_id   UUID NOT NULL REFERENCES event_destinations(id) ON DELETE CASCADE,
			subscription_ids JSONB NOT NULL DEFAULT '[]',
			event_id         TEXT NOT NULL,
			event_type       TEXT NOT NULL,
			event_scope      TEXT NOT NULL DEFAULT '',
			project_name     TEXT NOT NULL DEFAULT '',
			agent_name       TEXT NOT NULL DEFAULT '',
			environment      TEXT NOT NULL DEFAULT '',
			status           TEXT NOT NULL,
			attempts         INTEGER NOT NULL DEFAULT 0,
			response_code    INTEGER,
			last_error       TEXT NOT NULL DEFAULT '',
			delivered_at     TIMESTAMPTZ,
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (destination_id, event_id)
		);
		CREATE INDEX IF NOT EXISTS idx_event_deliveries_org_created
			ON event_deliveries (ou_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_event_deliveries_location_created
			ON event_deliveries (ou_id, project_name, agent_name, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_event_deliveries_destination_created
			ON event_deliveries (destination_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_event_deliveries_subscriptions
			ON event_deliveries USING GIN (subscription_ids jsonb_path_ops);
		CREATE INDEX IF NOT EXISTS idx_event_deliveries_created
			ON event_deliveries (created_at);

		-- Channel names were unique nowhere; destination names are unique per
		-- org, so a repeated name gets the start of the channel's ID.
		INSERT INTO event_destinations (
			id, ou_id, name, description, kind, config, secret_encrypted,
			availability, projects, created_by, created_at, updated_at)
		SELECT id, ou_id,
			CASE WHEN rn = 1 THEN name ELSE name || ' (' || left(id::text, 8) || ')' END,
			description, kind, config, secret_encrypted,
			CASE WHEN scope = 'org' THEN 'org' ELSE 'projects' END,
			CASE WHEN scope = 'org' THEN '[]'::jsonb ELSE jsonb_build_array(project_name) END,
			created_by, created_at, updated_at
		FROM (
			SELECT c.*, row_number() OVER (
				PARTITION BY c.ou_id, lower(c.name) ORDER BY c.created_at, c.id) AS rn
			FROM alert_channels c
		) numbered;

		INSERT INTO event_subscriptions (
			id, ou_id, scope, project_name, agent_name, name, description,
			destination_id, event_types, environments, enabled, created_by,
			created_at, updated_at)
		SELECT id, ou_id, scope, project_name, agent_name, name, description,
			id, event_types, environments, enabled, created_by,
			created_at, updated_at
		FROM alert_channels;

		INSERT INTO event_deliveries (
			id, ou_id, destination_id, subscription_ids, event_id, event_type,
			event_scope, project_name, agent_name, status, attempts,
			response_code, last_error, delivered_at, created_at, updated_at)
		SELECT d.id, d.ou_id, d.channel_id, jsonb_build_array(d.channel_id::text),
			d.event_id, d.event_type, c.scope, c.project_name, c.agent_name,
			d.status, d.attempts, d.response_code, d.last_error, d.delivered_at,
			d.created_at, d.updated_at
		FROM alert_deliveries d
		JOIN alert_channels c ON c.id = d.channel_id;

		DROP TABLE IF EXISTS alert_deliveries;
		DROP TABLE IF EXISTS alert_channels;
		`).Error
	},
}

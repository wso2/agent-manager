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

// Dispatcher database, migration 1: webhook endpoints at org, project or
// agent scope, and the outcome of each delivery. These tables live in the
// dispatcher's own database; in "all" mode that is the service database.
var dispatcherMigration001 = migration{
	ID: 1,
	Migrate: func(db *gorm.DB) error {
		return db.Exec(`
		CREATE TABLE IF NOT EXISTS webhook_endpoints (
			id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			ou_id            TEXT NOT NULL,
			scope            TEXT NOT NULL CHECK (scope IN ('org', 'project', 'agent')),
			project_name     TEXT NOT NULL DEFAULT '',
			agent_name       TEXT NOT NULL DEFAULT '',
			environments     JSONB NOT NULL DEFAULT '[]',
			name             TEXT NOT NULL,
			description      TEXT NOT NULL DEFAULT '',
			url              TEXT NOT NULL,
			event_types      JSONB NOT NULL DEFAULT '[]',
			enabled          BOOLEAN NOT NULL DEFAULT TRUE,
			secret_encrypted BYTEA NOT NULL,
			created_by       TEXT NOT NULL DEFAULT '',
			created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_webhook_endpoints_target
			ON webhook_endpoints (ou_id, scope, project_name, agent_name);

		CREATE TABLE IF NOT EXISTS webhook_deliveries (
			id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			endpoint_id   UUID NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
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
			UNIQUE (endpoint_id, event_id)
		);
		CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_endpoint_created
			ON webhook_deliveries (endpoint_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_created
			ON webhook_deliveries (created_at);
		`).Error
	},
}

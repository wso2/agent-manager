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

// monitor_watches holds each user's watch on a monitor: what about its runs
// they want to hear, and which of their score thresholds the latest run
// breached. notifications is each user's console inbox.
var migration052 = migration{
	ID: 52,
	Migrate: func(db *gorm.DB) error {
		return db.Exec(`
		CREATE TABLE IF NOT EXISTS monitor_watches (
			id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			ou_id             TEXT NOT NULL,
			monitor_id        UUID NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
			user_id           TEXT NOT NULL,
			notify_on_failure BOOLEAN NOT NULL DEFAULT TRUE,
			notify_on_success BOOLEAN NOT NULL DEFAULT FALSE,
			thresholds        JSONB NOT NULL DEFAULT '[]',
			breached          JSONB NOT NULL DEFAULT '[]',
			last_evaluated_at TIMESTAMPTZ,
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (monitor_id, user_id)
		);

		CREATE TABLE IF NOT EXISTS notifications (
			id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			ou_id      TEXT NOT NULL,
			user_id    TEXT NOT NULL,
			kind       TEXT NOT NULL,
			title      TEXT NOT NULL,
			body       TEXT NOT NULL DEFAULT '',
			target     JSONB NOT NULL DEFAULT '{}',
			event_id   TEXT NOT NULL,
			read_at    TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (user_id, event_id, kind)
		);
		CREATE INDEX IF NOT EXISTS idx_notifications_inbox
			ON notifications (ou_id, user_id, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_notifications_unread
			ON notifications (ou_id, user_id) WHERE read_at IS NULL;
		CREATE INDEX IF NOT EXISTS idx_notifications_created
			ON notifications (created_at);
		`).Error
	},
}

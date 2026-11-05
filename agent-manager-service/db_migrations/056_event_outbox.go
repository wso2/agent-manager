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

// Additive migration: old binaries ignore the outbox and run marker. Leave
// these columns/tables in place when rolling back; pending work can resume on
// upgrade. Existing terminal runs are marked to avoid replaying history.
var migration056 = migration{
	ID: 56,
	Migrate: func(db *gorm.DB) error {
		return db.Exec(`
   CREATE TABLE event_outbox (
    id TEXT PRIMARY KEY,
    body JSONB NOT NULL,
    lease TEXT NOT NULL DEFAULT '',
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
   );
   CREATE INDEX idx_event_outbox_available ON event_outbox(available_at, id);
   ALTER TABLE monitor_runs ADD COLUMN event_queued_at TIMESTAMPTZ;
   UPDATE monitor_runs SET event_queued_at = NOW() WHERE status IN ('success','failed');
   CREATE INDEX idx_monitor_runs_event_pending ON monitor_runs(completed_at, id)
    WHERE event_queued_at IS NULL AND status IN ('success','failed');
  `).Error
	},
}

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

// Monitor alerts deliver shared rules to organization-managed destinations.
var migration057 = migration{
	ID: 57,
	Migrate: func(db *gorm.DB) error {
		return db.Exec(`CREATE TABLE IF NOT EXISTS monitor_alerts (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 ou_id TEXT NOT NULL, monitor_id UUID NOT NULL UNIQUE REFERENCES monitors(id) ON DELETE CASCADE,
 destination_ids JSONB NOT NULL, notify_on_failure BOOLEAN NOT NULL, notify_on_success BOOLEAN NOT NULL,
 thresholds JSONB NOT NULL DEFAULT '[]', breached JSONB NOT NULL DEFAULT '[]',
 last_evaluated_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
 ALTER TABLE event_deliveries ADD COLUMN IF NOT EXISTS monitor_alert_id UUID;
 ALTER TABLE event_deliveries ADD COLUMN IF NOT EXISTS source_event_id TEXT;
 ALTER TABLE event_deliveries ADD COLUMN IF NOT EXISTS payload JSONB;
 CREATE INDEX IF NOT EXISTS idx_monitor_alert_delivery_source ON event_deliveries(source_event_id) WHERE monitor_alert_id IS NOT NULL;
 `).Error
	},
}

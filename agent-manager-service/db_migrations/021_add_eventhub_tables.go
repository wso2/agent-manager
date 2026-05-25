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

var migration021 = migration{
	ID: 21,
	Migrate: func(db *gorm.DB) error {
		return db.Exec(`
			CREATE TABLE IF NOT EXISTS eventhub_gateway_states (
				gateway_id  TEXT      PRIMARY KEY,
				version_id  TEXT      NOT NULL DEFAULT '',
				updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
			);

			CREATE TABLE IF NOT EXISTS eventhub_events (
				event_id             TEXT      PRIMARY KEY,
				gateway_id           TEXT      NOT NULL REFERENCES eventhub_gateway_states(gateway_id) ON DELETE CASCADE,
				processed_timestamp  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
				originated_timestamp TIMESTAMP NOT NULL,
				entity_type          TEXT      NOT NULL,
				action               TEXT      NOT NULL,
				entity_id            TEXT      NOT NULL,
				event_data           TEXT      NOT NULL
			);

			CREATE INDEX IF NOT EXISTS idx_eventhub_events_gateway_ts
				ON eventhub_events(gateway_id, processed_timestamp);
		`).Error
	},
}

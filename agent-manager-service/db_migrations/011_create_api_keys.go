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

// Create api_keys table for persisting API keys so gateways can bulk-sync on reconnect
var migration011 = migration{
	ID: 11,
	Migrate: func(db *gorm.DB) error {
		createAPIKeysTable := `
		CREATE TABLE IF NOT EXISTS api_keys (
			uuid              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name              VARCHAR(255) NOT NULL,
			artifact_uuid     UUID NOT NULL REFERENCES artifacts(uuid) ON DELETE CASCADE,
			organization_name VARCHAR(255) NOT NULL,
			api_key_hash      VARCHAR(255) NOT NULL,
			masked_api_key    VARCHAR(255) NOT NULL DEFAULT '',
			status            VARCHAR(50) NOT NULL DEFAULT 'active',
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			expires_at        TIMESTAMPTZ,
			UNIQUE(artifact_uuid, name)
		);

		CREATE INDEX IF NOT EXISTS idx_api_keys_artifact ON api_keys(artifact_uuid);
		CREATE INDEX IF NOT EXISTS idx_api_keys_org ON api_keys(organization_name);
		`
		return db.Exec(createAPIKeysTable).Error
	},
}

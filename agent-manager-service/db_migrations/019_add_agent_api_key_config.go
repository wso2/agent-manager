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

var migration019 = migration{
	ID: 19,
	Migrate: func(db *gorm.DB) error {
		addAgentAPIKeyConfig := `
		ALTER TABLE agent_configs
			ADD COLUMN IF NOT EXISTS enable_api_key_security BOOLEAN NOT NULL DEFAULT true;

		ALTER TABLE api_keys
			ADD COLUMN IF NOT EXISTS display_name VARCHAR(255) NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS purpose SMALLINT;

		ALTER TABLE api_keys ALTER COLUMN purpose DROP DEFAULT;
		ALTER TABLE api_keys ALTER COLUMN purpose TYPE SMALLINT USING
			CASE
				WHEN purpose::text = 'permanent' THEN 1
				WHEN purpose::text = 'test' THEN 2
				WHEN purpose IS NULL THEN 1
				ELSE purpose::text::smallint
			END;
		UPDATE api_keys SET purpose = 1 WHERE purpose IS NULL;
		ALTER TABLE api_keys ALTER COLUMN purpose SET DEFAULT 1;
		ALTER TABLE api_keys ALTER COLUMN purpose SET NOT NULL;

		CREATE INDEX IF NOT EXISTS idx_api_keys_purpose ON api_keys(purpose);
		`
		return db.Exec(addAgentAPIKeyConfig).Error
	},
}

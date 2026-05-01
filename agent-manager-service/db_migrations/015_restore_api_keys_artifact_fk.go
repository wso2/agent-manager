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

// Restore api_keys.artifact_uuid foreign key if an earlier dev migration removed it.
var migration015 = migration{
	ID: 15,
	Migrate: func(db *gorm.DB) error {
		restoreAPIKeysArtifactFK := `
		INSERT INTO artifacts (uuid, handle, name, version, kind, organization_name, in_catalog, created_at, updated_at)
		SELECT
			k.artifact_uuid,
			k.artifact_uuid::text,
			k.artifact_uuid::text,
			'v1',
			'Agent',
			MIN(k.organization_name),
			false,
			NOW(),
			NOW()
		FROM api_keys k
		LEFT JOIN artifacts a ON a.uuid = k.artifact_uuid
		WHERE a.uuid IS NULL
		GROUP BY k.artifact_uuid
		ON CONFLICT (uuid) DO NOTHING;

		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM information_schema.table_constraints tc
				JOIN information_schema.key_column_usage kcu
					ON tc.constraint_name = kcu.constraint_name
					AND tc.table_schema = kcu.table_schema
				WHERE tc.constraint_type = 'FOREIGN KEY'
					AND tc.table_schema = current_schema()
					AND tc.table_name = 'api_keys'
					AND kcu.column_name = 'artifact_uuid'
			) THEN
				ALTER TABLE api_keys
					ADD CONSTRAINT fk_api_keys_artifact
					FOREIGN KEY (artifact_uuid)
					REFERENCES artifacts(uuid)
					ON DELETE CASCADE;
			END IF;
		END $$;
		`
		return db.Exec(restoreAPIKeysArtifactFK).Error
	},
}

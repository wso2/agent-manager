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

// Secrets are stored encrypted in this table by the in-house secret store.
// One row holds every key of one secret (one SecretReference), sealed together.
var migration060 = migration{
	ID: 60,
	Migrate: func(db *gorm.DB) error {
		return db.Exec(`CREATE TABLE IF NOT EXISTS secrets (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 ou_id TEXT NOT NULL,
 name TEXT NOT NULL,
 ciphertext BYTEA NOT NULL,
 hash TEXT NOT NULL,
 keys JSONB NOT NULL DEFAULT '[]',
 labels JSONB NOT NULL DEFAULT '{}',
 managed_by TEXT NOT NULL,
 provider TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 CONSTRAINT uq_secrets_ou_name UNIQUE (ou_id, name));
 `).Error
	},
}

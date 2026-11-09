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

// A project MCP proxy binding's capability discovery: AMS lists the MCP server's tools, prompts and resources through
// the binding's gateway once it is deployed. One row per binding artifact, removed with it.
var migration060 = migration{
	ID: 60,
	Migrate: func(db *gorm.DB) error {
		return db.Exec(`CREATE TABLE IF NOT EXISTS mcp_capability_discoveries (
 artifact_uuid UUID PRIMARY KEY REFERENCES artifacts(uuid) ON DELETE CASCADE,
 ou_id TEXT NOT NULL,
 status TEXT NOT NULL CHECK (status IN ('PENDING', 'DISCOVERED', 'FAILED')),
 attempt_count INTEGER NOT NULL DEFAULT 0,
 next_attempt_at TIMESTAMPTZ,
 last_error TEXT NOT NULL DEFAULT '',
 discovered_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
 CREATE INDEX IF NOT EXISTS idx_mcp_capability_discoveries_due ON mcp_capability_discoveries(next_attempt_at)
 WHERE status = 'PENDING';
 `).Error
	},
}

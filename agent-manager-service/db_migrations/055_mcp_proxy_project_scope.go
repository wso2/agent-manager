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

// migration055 lets an MCP proxy belong to a project.
//
// project_uuid is the OpenChoreo project UUID the proxy is published into and
// project_name is that project's name (kept so reads can return it without an
// OpenChoreo lookup). Both are NULL for an organization-level proxy, which is
// every row that exists today, so existing proxies and the previous version of
// the service, which never reads the columns, are unaffected. They are set
// together or not at all.
var migration055 = migration{
	ID: 55,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(
				tx,
				`ALTER TABLE mcp_proxies ADD COLUMN IF NOT EXISTS project_uuid UUID`,
				`ALTER TABLE mcp_proxies ADD COLUMN IF NOT EXISTS project_name TEXT`,
				`CREATE INDEX IF NOT EXISTS idx_mcp_proxies_project_uuid
					ON mcp_proxies (project_uuid)`,
			)
		})
	},
}

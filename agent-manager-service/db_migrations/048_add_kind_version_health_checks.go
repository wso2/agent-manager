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

// agent_kind_versions gains the health checks the source agent had when the version
// was published, so a catalog agent checks its image the way that image expects.
// NULL for versions published before this, whose images date from when every agent
// had the platform's default health checks.
var migration048 = migration{
	ID: 48,
	Migrate: func(db *gorm.DB) error {
		return db.Exec(`
		ALTER TABLE agent_kind_versions
			ADD COLUMN IF NOT EXISTS health_checks JSONB;
		`).Error
	},
}

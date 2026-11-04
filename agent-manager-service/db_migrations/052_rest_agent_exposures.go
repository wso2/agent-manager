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

// migration052 lets a platform agent exposure carry the rest protocol beside a2a.
var migration052 = migration{
	ID: 52,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(tx,
				`ALTER TABLE platform_agent_exposures
					DROP CONSTRAINT chk_platform_agent_exposures_protocol,
					ADD CONSTRAINT chk_platform_agent_exposures_protocol CHECK (protocol IN ('a2a', 'rest'))`)
		})
	},
}

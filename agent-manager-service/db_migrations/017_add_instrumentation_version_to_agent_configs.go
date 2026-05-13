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

// Add a nullable instrumentation_version column to agent_configs.
// It holds the AMP instrumentation version the customer selected for the agent;
// NULL means "use the platform default".
var migration017 = migration{
	ID: 17,
	Migrate: func(db *gorm.DB) error {
		addColumn := `ALTER TABLE agent_configs ADD COLUMN IF NOT EXISTS instrumentation_version VARCHAR(64)`
		return runSQL(db, addColumn)
	},
}

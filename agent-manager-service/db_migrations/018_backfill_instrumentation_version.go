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

// Backfill agent_configs.instrumentation_version for existing rows that were
// created when the column was nullable and meant "use the platform default".
// Pinning them to the concrete current default (0.2.0) means a later default
// bump won't silently move an existing agent to a new AMP instrumentation
// version. New agents that don't pick a version explicitly still keep
// instrumentation_version NULL going forward.
var migration018 = migration{
	ID: 18,
	Migrate: func(db *gorm.DB) error {
		backfill := `UPDATE agent_configs SET instrumentation_version = '0.2.0' WHERE instrumentation_version IS NULL`
		return runSQL(db, backfill)
	},
}

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

// agent_configs gains the per-environment switch that makes a Ballerina agent
// receive its AgentID credentials as BAL_CONFIG_VAR_AMPAGENTID* (Ballerina
// configurables) instead of AMP_AGENTID_*. Defaults to false so every existing
// agent keeps the plain names: a Ballerina program that does not declare the
// matching configurables fails to start when they are injected.
var migration047 = migration{
	ID: 47,
	Migrate: func(db *gorm.DB) error {
		return db.Exec(`
		ALTER TABLE agent_configs
			ADD COLUMN IF NOT EXISTS agentid_as_bal_configurables BOOLEAN NOT NULL DEFAULT FALSE;
		`).Error
	},
}

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

// Drop the dependencies column and its check constraint from custom_evaluators.
// Custom dependencies are no longer supported; packages are pre-installed in the eval job image.
var migration010 = migration{
	ID: 10,
	Migrate: func(db *gorm.DB) error {
		return db.Exec(`
			ALTER TABLE custom_evaluators DROP CONSTRAINT IF EXISTS chk_custom_eval_dependencies;
			ALTER TABLE custom_evaluators DROP COLUMN IF EXISTS dependencies;
		`).Error
	},
}

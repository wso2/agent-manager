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

// Add secret_kv_path and secret_key columns to monitor_llm_mapping so the executor
// can pass the correct remoteRef fields to the workflow, instead of recomputing
// the raw OpenBao KV path which cannot be used to mount secrets into pods.
var migration015 = migration{
	ID: 15,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE monitor_llm_mapping
				ADD COLUMN IF NOT EXISTS secret_kv_path TEXT NOT NULL DEFAULT '',
				ADD COLUMN IF NOT EXISTS secret_key    TEXT NOT NULL DEFAULT ''
			`).Error; err != nil {
				return err
			}
			return nil
		})
	},
}

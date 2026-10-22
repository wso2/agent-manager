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

// a2a_publications records the transports each row last published. Existing rows stay NULL.
var migration050 = migration{
	ID: 50,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(AddA2APublicationTransportsColumn)
	},
}

// AddA2APublicationTransportsColumn adds the nullable published_transports column; rerunning it is a no-op.
func AddA2APublicationTransportsColumn(tx *gorm.DB) error {
	return runSQL(tx, `
		ALTER TABLE a2a_publications
			ADD COLUMN IF NOT EXISTS published_transports JSONB`)
}

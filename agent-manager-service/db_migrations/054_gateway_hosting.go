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

// migration054 records whether a gateway is run by the platform or by the customer.
//
// Every existing row is managed, which is the column default, so the migration
// needs no backfill and the previous version of the service, which never reads
// the column, is unaffected.
var migration054 = migration{
	ID: 54,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(
				tx,
				`ALTER TABLE gateways
					ADD COLUMN hosting VARCHAR(16) NOT NULL DEFAULT 'managed',
					ADD CONSTRAINT chk_gateways_hosting CHECK (hosting IN ('managed','self_hosted'))`,
			)
		})
	},
}

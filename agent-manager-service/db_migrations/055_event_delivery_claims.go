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

// claimed_until is the lease a delivery worker takes before sending, so a
// job the bus redelivers while the first worker is still sending is not
// sent twice. Additive: rows start unclaimed.
var migration055 = migration{
	ID: 55,
	Migrate: func(db *gorm.DB) error {
		return db.Exec(`ALTER TABLE event_deliveries ADD COLUMN IF NOT EXISTS claimed_until TIMESTAMPTZ`).Error
	},
}

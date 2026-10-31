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

// migration051 lets an agent's identity follow its project pipeline by promotion.
//
// promotion_source records, for a binding whose environment receives its
// identity by promotion, the environment it is promoted from. Without it the
// reconciler would have to read the project's pipeline to order the promotions.
// Empty (the default) means the identity is created directly in that
// environment, which is what every existing row is.
//
// sync_after marks a completed promoted binding whose carried set (roles,
// groups, resource servers) changed. The reconciler re-promotes it once the time
// has passed, and clears it only if it was not moved since. A request moves it
// later, so a burst of changes becomes one promotion.
//
// Both columns are additive and backward compatible: promotion_source has an
// empty default and sync_after is nullable, so existing rows and the previous
// version of the service, which never reads them, are unaffected. The partial
// index covers only the rows that are waiting, so the reconciler's poll stays
// cheap however many bindings exist.
var migration051 = migration{
	ID: 51,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return runSQL(
				tx,
				`ALTER TABLE agent_thunder_clients
					ADD COLUMN IF NOT EXISTS promotion_source TEXT NOT NULL DEFAULT ''`,
				`ALTER TABLE agent_thunder_clients
					ADD COLUMN IF NOT EXISTS sync_after TIMESTAMPTZ`,
				`CREATE INDEX IF NOT EXISTS idx_agent_thunder_clients_sync_after
					ON agent_thunder_clients (sync_after)
					WHERE sync_after IS NOT NULL`,
			)
		})
	},
}

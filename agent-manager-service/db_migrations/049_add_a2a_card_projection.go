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

// a2a_agent_cards stores a fingerprint and search text derived from each card,
// and every fetched card is requeued once so the reconciler fills them in.
var migration049 = migration{
	ID: 49,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			if err := AddA2ACardProjectionColumns(tx); err != nil {
				return err
			}
			return RequeueFetchedA2ACards(tx)
		})
	},
}

// AddA2ACardProjectionColumns adds the card fingerprint and search text columns; rerunning it is a no-op.
func AddA2ACardProjectionColumns(tx *gorm.DB) error {
	return runSQL(tx,
		`ALTER TABLE a2a_agent_cards
			ADD COLUMN IF NOT EXISTS card_fingerprint VARCHAR(64) NOT NULL DEFAULT ''`,
		`ALTER TABLE a2a_agent_cards
			ADD COLUMN IF NOT EXISTS card_search_text TEXT NOT NULL DEFAULT ''`,
	)
}

// RequeueFetchedA2ACards makes every fetched card due now with a fresh attempt budget.
// The card, its hash and fetch time are kept, so readers keep the last good card until the refetch lands.
func RequeueFetchedA2ACards(tx *gorm.DB) error {
	return runSQL(tx, `
		UPDATE a2a_agent_cards
		SET status = 'pending', attempt_count = 0, next_attempt_at = NOW()
		WHERE status = 'fetched' AND card IS NOT NULL`)
}

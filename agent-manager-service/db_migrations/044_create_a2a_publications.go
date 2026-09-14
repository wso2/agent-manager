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

// a2a_publications is the queue of A2A agents whose gateway Agent resource is
// still to be emitted.
//
// One row per (org, project, agent, environment): a redeploy resets the
// existing row to pending rather than adding a second, because publication is
// idempotent — it always emits the agent's current state, so two outstanding
// rows for the same pair would only do the same work twice.
//
// The (status, next_attempt_at) index is what the reconciler's due-scan reads;
// without it the scan degrades to a full table scan every tick.
var migration044 = migration{
	ID: 44,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			createTable := `
			CREATE TABLE IF NOT EXISTS a2a_publications (
				id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				ou_id            VARCHAR(255) NOT NULL,
				project_name     VARCHAR(255) NOT NULL,
				agent_name       VARCHAR(255) NOT NULL,
				environment_name VARCHAR(255) NOT NULL,
				environment_uuid UUID NOT NULL,
				artifact_uuid    UUID NOT NULL,
				status           VARCHAR(32) NOT NULL DEFAULT 'pending',
				attempt_count    INTEGER NOT NULL DEFAULT 0,
				last_error       TEXT NOT NULL DEFAULT '',
				next_attempt_at  TIMESTAMPTZ,
				created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

				CONSTRAINT uq_a2a_publications_agent_env
					UNIQUE (ou_id, project_name, agent_name, environment_name)
			)`
			if err := runSQL(tx, createTable); err != nil {
				return err
			}

			createIndex := `
			CREATE INDEX IF NOT EXISTS idx_a2a_publications_due
				ON a2a_publications (status, next_attempt_at)`
			return runSQL(tx, createIndex)
		})
	},
}

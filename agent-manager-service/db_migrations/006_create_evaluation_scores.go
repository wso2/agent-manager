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

// Create monitor_run_evaluators and scores tables (redesigned schema)
var migration006 = migration{
	ID: 6,
	Migrate: func(db *gorm.DB) error {
		createMonitorRunEvaluatorsTable := `
		CREATE TABLE IF NOT EXISTS monitor_run_evaluators (
			id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			monitor_run_id    UUID NOT NULL REFERENCES monitor_runs(id) ON DELETE CASCADE,
			monitor_id        UUID NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,

			-- Evaluator identity
			evaluator_name    VARCHAR(255) NOT NULL,
			display_name      VARCHAR(255) NOT NULL,
			level             VARCHAR(10) NOT NULL CHECK (level IN ('trace', 'agent', 'llm')),

			-- Aggregated results (flexible)
			aggregations      JSONB NOT NULL DEFAULT '{}',
			count             INT NOT NULL DEFAULT 0,
			skipped_count     INT NOT NULL DEFAULT 0,

			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),

			CONSTRAINT uq_run_evaluator UNIQUE (monitor_run_id, display_name)
		)`

		createScoresTable := `
		CREATE TABLE IF NOT EXISTS scores (
			id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			run_evaluator_id  UUID NOT NULL REFERENCES monitor_run_evaluators(id) ON DELETE CASCADE,
			monitor_id        UUID NOT NULL,

			-- Item identification
			trace_id          VARCHAR(255) NOT NULL,
			span_id           VARCHAR(255),

			-- Score data (NULL when skipped)
			score             DECIMAL(5,4) CHECK (score IS NULL OR (score >= 0 AND score <= 1)),
			explanation       TEXT,

			-- Trace timestamp for time-series
			trace_timestamp   TIMESTAMPTZ NOT NULL,

			-- Extra context
			metadata          JSONB DEFAULT '{}',
			skip_reason       TEXT,

			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`

		createIndexes := []string{
			// monitor_run_evaluators indexes
			`CREATE INDEX IF NOT EXISTS idx_run_eval_monitor ON monitor_run_evaluators (monitor_id)`,
			`CREATE INDEX IF NOT EXISTS idx_run_eval_run ON monitor_run_evaluators (monitor_run_id)`,

			// scores indexes
			`CREATE INDEX IF NOT EXISTS idx_score_monitor_time ON scores (monitor_id, trace_timestamp)`,
			`CREATE INDEX IF NOT EXISTS idx_score_trace ON scores (trace_id)`,
			`CREATE INDEX IF NOT EXISTS idx_score_trace_span ON scores (trace_id, span_id) WHERE span_id IS NOT NULL`,
			`CREATE INDEX IF NOT EXISTS idx_score_run_eval ON scores (run_evaluator_id)`,

			// Unique constraint: treat NULL span_id values as equal so that re-evaluating the
			// same (run_evaluator_id, trace_id) row upserts instead of inserting a duplicate.
			// NULLS NOT DISTINCT requires PostgreSQL 15+. This project targets PostgreSQL 16.
			`CREATE UNIQUE INDEX IF NOT EXISTS uq_score_per_item ON scores (run_evaluator_id, trace_id, span_id) NULLS NOT DISTINCT`,
		}

		return db.Transaction(func(tx *gorm.DB) error {
			// Create new tables
			if err := runSQL(tx, createMonitorRunEvaluatorsTable); err != nil {
				return err
			}
			if err := runSQL(tx, createScoresTable); err != nil {
				return err
			}

			// Create indexes
			for _, idx := range createIndexes {
				if err := runSQL(tx, idx); err != nil {
					return err
				}
			}

			return nil
		})
	},
}

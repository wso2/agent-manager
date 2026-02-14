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

// Create monitors and monitor_runs tables
var migration006 = migration{
	ID: 6,
	Migrate: func(db *gorm.DB) error {
		createMonitorsTable := `
		CREATE TABLE IF NOT EXISTS monitors (
			id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name                  VARCHAR(63) NOT NULL,
			display_name          VARCHAR(128) NOT NULL DEFAULT '',
			type                  VARCHAR(20) NOT NULL CHECK (type IN ('future', 'past')),

			-- References
			org_name              VARCHAR(255) NOT NULL,
			project_name          VARCHAR(63) NOT NULL,
			agent_name            VARCHAR(63) NOT NULL,
			agent_id              VARCHAR(255) NOT NULL,
			environment_name      VARCHAR(63) NOT NULL,
			environment_id        VARCHAR(255) NOT NULL,

			-- Evaluator config
			evaluators            JSONB NOT NULL DEFAULT '[]',

			-- Scheduling (future monitors only, NULL for past monitors)
			interval_minutes      INT,
			next_run_time         TIMESTAMPTZ,

			-- Time range (past monitors only, NULL for future monitors)
			trace_start           TIMESTAMPTZ,
			trace_end             TIMESTAMPTZ,

			-- Sampling
			sampling_rate         DECIMAL(3,2) NOT NULL DEFAULT 1.00
			                      CHECK (sampling_rate > 0 AND sampling_rate <= 1),

			-- Timestamps
			created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),

			CONSTRAINT uq_monitor_name_org UNIQUE (name, org_name)
		)`

		createMonitorRunsTable := `
		CREATE TABLE IF NOT EXISTS monitor_runs (
			id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			monitor_id            UUID NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,

			-- WorkflowRun name for querying OpenChoreo API
			name                  VARCHAR(255) NOT NULL,

			-- Evaluator snapshot (preserves which evaluators were used for this run)
			evaluators            JSONB NOT NULL DEFAULT '[]',

			-- Trace time window (the range of data this run evaluates)
			trace_start            TIMESTAMPTZ NOT NULL,
			trace_end              TIMESTAMPTZ NOT NULL,

			-- Execution timestamps (when the job actually ran)
			started_at            TIMESTAMPTZ,
			completed_at          TIMESTAMPTZ,

			-- Result
			status                VARCHAR(20) NOT NULL DEFAULT 'pending'
			                      CHECK (status IN ('pending', 'running', 'success', 'failed')),
			error_message         TEXT,

			created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`

		createIndexes := []string{
			`CREATE INDEX IF NOT EXISTS idx_monitor_org ON monitors (org_name)`,
			`CREATE INDEX IF NOT EXISTS idx_monitor_agent ON monitors (agent_id)`,
			`CREATE INDEX IF NOT EXISTS idx_monitor_type ON monitors (type)`,
			`CREATE INDEX IF NOT EXISTS idx_run_monitor ON monitor_runs (monitor_id)`,
			`CREATE INDEX IF NOT EXISTS idx_run_status ON monitor_runs (status)`,
			`CREATE INDEX IF NOT EXISTS idx_run_time_window ON monitor_runs (monitor_id, trace_start, trace_end)`,
			`CREATE INDEX IF NOT EXISTS idx_run_name ON monitor_runs (name)`,
		}

		createTrigger := `
		CREATE OR REPLACE FUNCTION update_monitor_updated_at()
		RETURNS TRIGGER AS $$
		BEGIN
			NEW.updated_at = NOW();
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;

		DROP TRIGGER IF EXISTS trg_monitor_updated_at ON monitors;
		CREATE TRIGGER trg_monitor_updated_at
			BEFORE UPDATE ON monitors
			FOR EACH ROW
			EXECUTE FUNCTION update_monitor_updated_at()
		`

		return db.Transaction(func(tx *gorm.DB) error {
			if err := runSQL(tx, createMonitorsTable); err != nil {
				return err
			}
			if err := runSQL(tx, createMonitorRunsTable); err != nil {
				return err
			}
			for _, idx := range createIndexes {
				if err := runSQL(tx, idx); err != nil {
					return err
				}
			}
			if err := runSQL(tx, createTrigger); err != nil {
				return err
			}
			return nil
		})
	},
}

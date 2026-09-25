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

// cancelled_builds is a SHIM, not a build mirror.
//
// Builds have no AMS-side record at all: they are derived entirely from
// OpenChoreo WorkflowRuns, which list/get read through on every request.
// Cancelling a build deletes its WorkflowRun, because OpenChoreo's
// WorkflowRunSpec exposes no suspend or cancel verb — so without this table a
// cancelled build vanishes from history the moment the next poll lands, and the
// developer who cancelled it has no way to see that it ever ran.
//
// Only cancelled builds get a row. Every other build stays exclusively in
// OpenChoreo, which is why this is a shim and not a second source of truth:
// when OpenChoreo grows a cancel verb that leaves the run in a terminal
// Cancelled state, this table and the merge in ListAgentBuilds are deleted and
// nothing else changes.
//
// build_parameters is a JSONB snapshot rather than a column per field because
// the shape it mirrors (models.BuildParameters) belongs to OpenChoreo and is
// expected to be thrown away with this table — pinning it into ten columns
// would buy queryability nobody needs at the cost of a migration every time
// OpenChoreo adds a build field.
//
// Rows are scoped by (ou_id, project_name, agent_name), matching how builds are
// addressed everywhere else, with uq_cancelled_builds_name making a cancel
// idempotent: the same build cannot be recorded cancelled twice.
var migration044 = migration{
	ID: 44,
	Migrate: func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			createTable := `
			CREATE TABLE IF NOT EXISTS cancelled_builds (
				id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				ou_id             VARCHAR(255) NOT NULL,
				project_name      VARCHAR(255) NOT NULL,
				agent_name        VARCHAR(255) NOT NULL,
				build_name        VARCHAR(255) NOT NULL,
				build_uuid        VARCHAR(255) NOT NULL DEFAULT '',
				status_at_cancel  VARCHAR(64) NOT NULL,
				build_parameters  JSONB NOT NULL DEFAULT '{}'::jsonb,
				started_at        TIMESTAMPTZ,
				cancelled_by      VARCHAR(255) NOT NULL DEFAULT '',
				cancelled_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

				CONSTRAINT uq_cancelled_builds_name UNIQUE (ou_id, project_name, agent_name, build_name)
			)`
			if err := runSQL(tx, createTable); err != nil {
				return err
			}

			// ListAgentBuilds reads every cancelled build for one agent on each
			// call, so the agent triple is the only access path that matters.
			createIndex := `
			CREATE INDEX IF NOT EXISTS idx_cancelled_builds_agent
				ON cancelled_builds (ou_id, project_name, agent_name)`

			return runSQL(tx, createIndex)
		})
	},
}

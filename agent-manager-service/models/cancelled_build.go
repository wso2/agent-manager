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

package models

import (
	"time"

	"github.com/google/uuid"
)

// BuildStatusCancelled is the status reported for a build that was cancelled.
// It is AMS's own value: OpenChoreo never produces it, because cancelling
// deletes the WorkflowRun rather than moving it into a terminal state.
const BuildStatusCancelled = "Cancelled"

// CancelledBuild is the record of a build that was cancelled before it
// finished. Cancellation deletes the underlying WorkflowRun, so this row is the
// only surviving evidence that the build existed — see migration044 for why it
// is a shim rather than a build mirror.
//
// It deliberately does NOT carry the build's logs or its produced image. Logs
// die with the WorkflowRun and cannot be recovered here, and a cancelled build
// never produced an image to record.
type CancelledBuild struct {
	ID          uuid.UUID `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OUID        string    `gorm:"column:ou_id;not null;uniqueIndex:uq_cancelled_builds_name"`
	ProjectName string    `gorm:"column:project_name;not null;uniqueIndex:uq_cancelled_builds_name"`
	AgentName   string    `gorm:"column:agent_name;not null;uniqueIndex:uq_cancelled_builds_name"`
	BuildName   string    `gorm:"column:build_name;not null;uniqueIndex:uq_cancelled_builds_name"`
	// BuildUUID is the deleted WorkflowRun's UID, kept so a cancelled build
	// keeps the same identity it had while it was running.
	BuildUUID string `gorm:"column:build_uuid;not null;default:''"`
	// StatusAtCancel is the status the build held when it was cancelled
	// (Pending or Running) — not BuildStatusCancelled, which is what the build
	// is reported as afterwards.
	StatusAtCancel  string          `gorm:"column:status_at_cancel;not null"`
	BuildParameters BuildParameters `gorm:"column:build_parameters;type:jsonb;serializer:json;not null;default:'{}'"`
	StartedAt       *time.Time      `gorm:"column:started_at"`
	// CancelledBy is the caller's token subject, empty when the cancel came
	// from a surface that carries no user identity.
	CancelledBy string    `gorm:"column:cancelled_by;not null;default:''"`
	CancelledAt time.Time `gorm:"column:cancelled_at;not null;default:NOW()"`
}

// TableName returns the table name for the CancelledBuild model.
func (CancelledBuild) TableName() string { return "cancelled_builds" }

// ToBuildResponse renders the record in the same shape as a live build, so a
// cancelled build can be merged into a build listing without the caller needing
// to know it came from a different source.
func (c *CancelledBuild) ToBuildResponse() *BuildResponse {
	var startedAt time.Time
	if c.StartedAt != nil {
		startedAt = *c.StartedAt
	}
	cancelledAt := c.CancelledAt
	return &BuildResponse{
		UUID:            c.BuildUUID,
		Name:            c.BuildName,
		AgentName:       c.AgentName,
		ProjectName:     c.ProjectName,
		Status:          BuildStatusCancelled,
		StartedAt:       startedAt,
		EndedAt:         &cancelledAt,
		BuildParameters: c.BuildParameters,
	}
}

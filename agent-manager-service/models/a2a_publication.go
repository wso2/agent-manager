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

// A2APublicationStatus is where one agent-environment pair stands in the
// publish-to-gateway cycle.
type A2APublicationStatus string

const (
	// A2APublicationStatusPending means the deploy has been recorded but the
	// Agent resource has not been emitted yet — normally because the release
	// binding has not published its ServiceURL.
	A2APublicationStatusPending A2APublicationStatus = "pending"
	// A2APublicationStatusPublished means the Agent resource was written to a
	// deployments row and broadcast.
	A2APublicationStatusPublished A2APublicationStatus = "published"
	// A2APublicationStatusFailed means the attempt budget was exhausted. The row
	// is kept so an operator can see which agent never reached its gateway.
	A2APublicationStatusFailed A2APublicationStatus = "failed"
)

// A2APublication is one agent-environment pair's outstanding gateway
// publication.
//
// This table exists because upstream.url is read from the release binding's
// status, which is only populated once the binding reconciles — several minutes
// after the deploy call returns. Publishing inline would emit an Agent with an
// empty upstream that routes nowhere, and holding the deploy request open until
// the binding is ready would make every deploy slow and still lose the work on
// a process restart. Recording the intent durably is what lets a background
// reconciler finish the job.
type A2APublication struct {
	ID              uuid.UUID `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OUID            string    `gorm:"column:ou_id;not null"`
	ProjectName     string    `gorm:"column:project_name;not null"`
	AgentName       string    `gorm:"column:agent_name;not null"`
	EnvironmentName string    `gorm:"column:environment_name;not null"`
	EnvironmentUUID uuid.UUID `gorm:"column:environment_uuid;type:uuid;not null"`
	// ArtifactUUID is the per-environment models.KindAgent artifact row. It is
	// the agentId the gateway is told about and the key its API keys are bound
	// to, so it is captured at enqueue time rather than re-derived later.
	ArtifactUUID  uuid.UUID            `gorm:"column:artifact_uuid;type:uuid;not null"`
	Status        A2APublicationStatus `gorm:"column:status;not null;default:'pending'"`
	AttemptCount  int                  `gorm:"column:attempt_count;not null;default:0"`
	LastError     string               `gorm:"column:last_error;not null;default:''"`
	NextAttemptAt *time.Time           `gorm:"column:next_attempt_at"`
	CreatedAt     time.Time            `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt     time.Time            `gorm:"column:updated_at;not null;default:NOW()"`
}

func (A2APublication) TableName() string { return "a2a_publications" }

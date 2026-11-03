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

// Deployment represents an immutable artifact deployment
// Status and UpdatedAt are populated from deployment_status table via JOIN
// If Status is nil, the deployment is ARCHIVED (not currently active or undeployed)
type Deployment struct {
	DeploymentID     uuid.UUID              `gorm:"column:deployment_id;primaryKey" json:"deploymentId"`
	Name             string                 `gorm:"column:name" json:"name"`
	ArtifactUUID     uuid.UUID              `gorm:"column:artifact_uuid" json:"artifactId"`
	OUID             string                 `gorm:"column:ou_id" json:"organizationId"`
	GatewayUUID      uuid.UUID              `gorm:"column:gateway_uuid" json:"gatewayId"`
	BaseDeploymentID *uuid.UUID             `gorm:"column:base_deployment_id" json:"baseDeploymentId,omitempty"`
	Content          []byte                 `gorm:"column:content" json:"-"`
	Metadata         map[string]interface{} `gorm:"column:metadata;type:text;serializer:json" json:"metadata,omitempty"`
	CreatedAt        time.Time              `gorm:"column:created_at" json:"createdAt"`

	// Lifecycle state fields (from deployment_status table via JOIN)
	// nil values indicate ARCHIVED state (no record in status table)
	// Read-only fields - populated via JOIN, never inserted/updated directly
	Status    *DeploymentStatus `gorm:"column:status;->" json:"status,omitempty"`
	UpdatedAt *time.Time        `gorm:"column:status_updated_at;->" json:"updatedAt,omitempty"`
	// The rest of the status row, for UserStatus.
	StatusDesired *DeploymentStatus `gorm:"column:status_desired;->" json:"-"`
	StatusReason  *string           `gorm:"column:status_reason;->" json:"-"`
	NextAttemptAt *time.Time        `gorm:"column:next_attempt_at;->" json:"-"`
	RenderReason  *string           `gorm:"column:render_reason;->" json:"-"`
}

// SetStatusRow records row, the status row pointing at the deployment, on it.
func (d *Deployment) SetStatusRow(row *DeploymentStatusRecord) {
	d.Status = &row.Status
	d.StatusDesired = &row.StatusDesired
	d.StatusReason = &row.StatusReason
	d.NextAttemptAt = row.NextAttemptAt
	d.RenderReason = &row.RenderReason
}

// Current reports whether a status row points at the deployment and the deployment is placed or held: the row
// heads for DEPLOYED or has a deploy owed (placed), or it isn't UNDEPLOYED (the gateway may still hold it).
// Deleting it would cascade the row, and any owed deploy, away.
func (d *Deployment) Current() bool {
	if d.Status == nil || *d.Status == DeploymentStatusArchived {
		return false
	}
	placed := (d.StatusDesired != nil && *d.StatusDesired == DeploymentStatusDeployed) || d.NextAttemptAt != nil
	return placed || *d.Status != DeploymentStatusUndeployed
}

// UserStatus is what the user is shown for the deployment: ARCHIVED when no status row points at it, otherwise
// what UserStatusOf derives from the row.
func (d *Deployment) UserStatus() UserStatus {
	if d.Status == nil || *d.Status == DeploymentStatusArchived || d.StatusDesired == nil {
		return UserStatus{Status: DeploymentStatusArchived}
	}
	return UserStatusOf(DeploymentStatusRecord{
		Status:        *d.Status,
		StatusDesired: *d.StatusDesired,
		StatusReason:  stringOrEmpty(d.StatusReason),
		NextAttemptAt: d.NextAttemptAt,
		RenderReason:  stringOrEmpty(d.RenderReason),
	})
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// TableName returns the table name for the Deployment model
func (Deployment) TableName() string {
	return "deployments"
}

// DeploymentStatus represents the status of a deployment
// Note: ARCHIVED is a derived state (not stored in database)
type DeploymentStatus string

const (
	// DeploymentStatusPending is a deploy owed to a gateway that has never been sent anything for the artifact.
	DeploymentStatusPending     DeploymentStatus = "PENDING"
	DeploymentStatusDeploying   DeploymentStatus = "DEPLOYING"
	DeploymentStatusDeployed    DeploymentStatus = "DEPLOYED"
	DeploymentStatusUndeploying DeploymentStatus = "UNDEPLOYING"
	DeploymentStatusUndeployed  DeploymentStatus = "UNDEPLOYED"
	DeploymentStatusFailed      DeploymentStatus = "FAILED"
	DeploymentStatusArchived    DeploymentStatus = "ARCHIVED" // Derived state: exists in history but not in status table
)

// DeploymentStatusRecord is one artifact's state on one gateway, and the render queue for its next deploy.
type DeploymentStatusRecord struct {
	ArtifactUUID uuid.UUID `gorm:"column:artifact_uuid;primaryKey" json:"artifactId"`
	OUID         string    `gorm:"column:ou_id;primaryKey" json:"organizationId"`
	GatewayUUID  uuid.UUID `gorm:"column:gateway_uuid;primaryKey" json:"gatewayId"`
	// DeploymentID is nil while nothing has been sent (PENDING, or UNDEPLOYED reached from PENDING), and on an
	// UNDEPLOYED row whose deployment was deleted.
	DeploymentID *uuid.UUID       `gorm:"column:deployment_id" json:"deploymentId"`
	Status       DeploymentStatus `gorm:"column:status" json:"status"`
	// StatusDesired is DEPLOYED or UNDEPLOYED: where the current transition is heading.
	StatusDesired DeploymentStatus `gorm:"column:status_desired" json:"statusDesired"`
	// StatusReason is the gateway's error code, or DEPLOYMENT_TIMEOUT.
	StatusReason string `gorm:"column:status_reason" json:"statusReason"`
	// PerformedAt is when the current transition started; always written through NewPerformedAt.
	PerformedAt time.Time `gorm:"column:performed_at" json:"performedAt"`
	// NextAttemptAt is non-nil while a deploy is owed to the gateway.
	NextAttemptAt *time.Time `gorm:"column:next_attempt_at" json:"-"`
	AttemptCount  int        `gorm:"column:attempt_count" json:"-"`
	RenderReason  string     `gorm:"column:render_reason" json:"-"`
	UpdatedAt     time.Time  `gorm:"column:updated_at" json:"updatedAt"`
}

// TableName returns the table name for the DeploymentStatusRecord model
func (DeploymentStatusRecord) TableName() string {
	return "deployment_status"
}

// NewPerformedAt is t as the gateway stores it: UTC at millisecond precision. A zero t becomes now, because the
// gateway reads a zero performedAt as "now", which always wins.
func NewPerformedAt(t time.Time) time.Time {
	if t.IsZero() {
		t = time.Now()
	}
	return t.UTC().Truncate(time.Millisecond)
}

// UserStatus is what a user is shown for one artifact on one gateway.
type UserStatus struct {
	Status DeploymentStatus
	Reason string
}

// UserStatusOf derives the shown status from a row; it is the only place that does. A deploy that is owed shows
// DEPLOYING; a redeploy that was given up keeps the gateway's status with the reason it didn't happen. PENDING is
// never shown.
func UserStatusOf(r DeploymentStatusRecord) UserStatus {
	redeployOwed := r.NextAttemptAt != nil
	redeployGivenUp := !redeployOwed && r.RenderReason != ""
	switch {
	case redeployOwed:
		return UserStatus{Status: DeploymentStatusDeploying, Reason: r.RenderReason}
	case redeployGivenUp && r.Status == DeploymentStatusPending:
		return UserStatus{Status: DeploymentStatusFailed, Reason: r.RenderReason}
	case redeployGivenUp:
		return UserStatus{Status: r.Status, Reason: r.RenderReason}
	case r.Status == DeploymentStatusPending:
		return UserStatus{Status: DeploymentStatusDeploying, Reason: r.StatusReason}
	default:
		return UserStatus{Status: r.Status, Reason: r.StatusReason}
	}
}

// ShownStatusRecord is the row a reader of an environment-addressed artifact shows: the one heading for DEPLOYED
// or owed a deploy (an artifact is placed on one gateway of its environment), else the first; nil when there is
// none, which means no gateway in the environment took the artifact.
func ShownStatusRecord(rows []DeploymentStatusRecord) *DeploymentStatusRecord {
	for i := range rows {
		if rows[i].StatusDesired == DeploymentStatusDeployed || rows[i].NextAttemptAt != nil {
			return &rows[i]
		}
	}
	if len(rows) == 0 {
		return nil
	}
	return &rows[0]
}

// IsDeploymentStatusFilter reports whether status is one a deployments list can be filtered by: a stored status, or
// ARCHIVED.
func IsDeploymentStatusFilter(status string) bool {
	switch DeploymentStatus(status) {
	case DeploymentStatusPending, DeploymentStatusDeploying, DeploymentStatusDeployed, DeploymentStatusUndeploying,
		DeploymentStatusUndeployed, DeploymentStatusFailed, DeploymentStatusArchived:
		return true
	default:
		return false
	}
}

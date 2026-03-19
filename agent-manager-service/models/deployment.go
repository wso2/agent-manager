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
	OrganizationName string                 `gorm:"column:organization_name" json:"organizationId"`
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
}

// TableName returns the table name for the Deployment model
func (Deployment) TableName() string {
	return "deployments"
}

// DeploymentStatus represents the status of a deployment
// Note: ARCHIVED is a derived state (not stored in database)
type DeploymentStatus string

const (
	DeploymentStatusDeployed   DeploymentStatus = "DEPLOYED"
	DeploymentStatusUndeployed DeploymentStatus = "UNDEPLOYED"
	DeploymentStatusArchived   DeploymentStatus = "ARCHIVED" // Derived state: exists in history but not in status table
)

// DeploymentStatusRecord represents the current deployment status record
type DeploymentStatusRecord struct {
	ArtifactUUID     uuid.UUID        `gorm:"column:artifact_uuid;primaryKey" json:"artifactId"`
	OrganizationName string           `gorm:"column:organization_name;primaryKey" json:"organizationId"`
	GatewayUUID      uuid.UUID        `gorm:"column:gateway_uuid;primaryKey" json:"gatewayId"`
	DeploymentID     uuid.UUID        `gorm:"column:deployment_id" json:"deploymentId"`
	Status           DeploymentStatus `gorm:"column:status" json:"status"`
	UpdatedAt        time.Time        `gorm:"column:updated_at" json:"updatedAt"`
}

// TableName returns the table name for the DeploymentStatusRecord model
func (DeploymentStatusRecord) TableName() string {
	return "deployment_status"
}

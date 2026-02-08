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

// DeploymentStatus enum
type DeploymentStatus string

const (
	DeploymentStatusPending     DeploymentStatus = "PENDING"
	DeploymentStatusDeploying   DeploymentStatus = "DEPLOYING"
	DeploymentStatusDeployed    DeploymentStatus = "DEPLOYED"
	DeploymentStatusFailed      DeploymentStatus = "FAILED"
	DeploymentStatusUndeploying DeploymentStatus = "UNDEPLOYING"
	DeploymentStatusUndeployed  DeploymentStatus = "UNDEPLOYED"
)

// ProviderDeployment is the database model for provider deployments
type ProviderDeployment struct {
	ID                       int                    `gorm:"column:id;primaryKey"`
	ProviderUUID             uuid.UUID              `gorm:"column:provider_uuid"`
	GatewayUUID              uuid.UUID              `gorm:"column:gateway_uuid"`
	DeploymentID             string                 `gorm:"column:deployment_id"`
	Environment              *string                `gorm:"column:environment"`
	ConfigurationVersion     int                    `gorm:"column:configuration_version"`
	GatewaySpecificOverrides map[string]interface{} `gorm:"column:gateway_specific_overrides;type:jsonb;serializer:json"`
	Status                   string                 `gorm:"column:status"`
	DeployedAt               *time.Time             `gorm:"column:deployed_at"`
	ErrorMessage             *string                `gorm:"column:error_message"`
	CreatedAt                time.Time              `gorm:"column:created_at"`
	UpdatedAt                time.Time              `gorm:"column:updated_at"`
}

func (ProviderDeployment) TableName() string {
	return "provider_gateway_deployments"
}

// ProviderDeploymentResponse is the API response DTO for provider deployments
type ProviderDeploymentResponse struct {
	ID                       int                    `json:"id"`
	ProviderUUID             string                 `json:"providerUuid"`
	GatewayUUID              string                 `json:"gatewayUuid"`
	GatewayName              string                 `json:"gatewayName,omitempty"`
	DeploymentID             string                 `json:"deploymentId"`
	Environment              *string                `json:"environment,omitempty"`
	ConfigurationVersion     int                    `json:"configurationVersion"`
	GatewaySpecificOverrides map[string]interface{} `json:"gatewaySpecificOverrides,omitempty"`
	Status                   string                 `json:"status"`
	DeployedAt               *time.Time             `json:"deployedAt,omitempty"`
	ErrorMessage             *string                `json:"errorMessage,omitempty"`
	CreatedAt                time.Time              `json:"createdAt"`
	UpdatedAt                time.Time              `json:"updatedAt"`
}

// ToResponse converts database model to API response
func (d *ProviderDeployment) ToResponse() *ProviderDeploymentResponse {
	return &ProviderDeploymentResponse{
		ID:                       d.ID,
		ProviderUUID:             d.ProviderUUID.String(),
		GatewayUUID:              d.GatewayUUID.String(),
		DeploymentID:             d.DeploymentID,
		Environment:              d.Environment,
		ConfigurationVersion:     d.ConfigurationVersion,
		GatewaySpecificOverrides: d.GatewaySpecificOverrides,
		Status:                   d.Status,
		DeployedAt:               d.DeployedAt,
		ErrorMessage:             d.ErrorMessage,
		CreatedAt:                d.CreatedAt,
		UpdatedAt:                d.UpdatedAt,
	}
}

// DeployToGatewayRequest is the API request for deploying to a gateway
type DeployToGatewayRequest struct {
	GatewayUUID string                 `json:"gatewayUuid" validate:"required,uuid"`
	Overrides   map[string]interface{} `json:"overrides,omitempty"`
}

// DeployToEnvironmentRequest is the API request for deploying to an environment
type DeployToEnvironmentRequest struct {
	EnvironmentUUID string                 `json:"environmentUuid" validate:"required,uuid"`
	Overrides       map[string]interface{} `json:"overrides,omitempty"`
}

// ProviderDeploymentListResponse is the paginated list response for provider deployments
type ProviderDeploymentListResponse struct {
	Deployments []ProviderDeploymentResponse `json:"deployments"`
	Total       int32                        `json:"total"`
}

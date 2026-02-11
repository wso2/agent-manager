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

// ProviderStatus enum
type ProviderStatus string

const (
	ProviderStatusDraft    ProviderStatus = "DRAFT"
	ProviderStatusPending  ProviderStatus = "PENDING"
	ProviderStatusApproved ProviderStatus = "APPROVED"
	ProviderStatusRejected ProviderStatus = "REJECTED"
	ProviderStatusArchived ProviderStatus = "ARCHIVED"
)

// LLMProvider is the database model for organization LLM providers
type LLMProvider struct {
	UUID             uuid.UUID              `gorm:"column:uuid;primaryKey"`
	OrganizationName string                 `gorm:"column:organization_name"`
	Handle           string                 `gorm:"column:handle"`
	DisplayName      string                 `gorm:"column:display_name"`
	Template         string                 `gorm:"column:template"`
	Configuration    map[string]interface{} `gorm:"column:configuration;type:jsonb;serializer:json"`
	Status           string                 `gorm:"column:status"`
	CreatedAt        time.Time              `gorm:"column:created_at"`
	UpdatedAt        time.Time              `gorm:"column:updated_at"`
	CreatedBy        string                 `gorm:"column:created_by"`
}

func (LLMProvider) TableName() string {
	return "llm_providers"
}

// LLMProviderResponse is the API response DTO
type LLMProviderResponse struct {
	UUID          string                 `json:"uuid"`
	Handle        string                 `json:"handle"`
	DisplayName   string                 `json:"displayName"`
	Template      string                 `json:"template"`
	Configuration map[string]interface{} `json:"configuration"`
	Status        string                 `json:"status"`
	CreatedAt     time.Time              `json:"createdAt"`
	UpdatedAt     time.Time              `json:"updatedAt"`
	CreatedBy     string                 `json:"createdBy"`
}

// CreateProviderRequest is the API request for creating a provider
type CreateProviderRequest struct {
	Handle        string                 `json:"handle" validate:"required,max=64"`
	DisplayName   string                 `json:"displayName" validate:"required,max=128"`
	Template      string                 `json:"template" validate:"required,max=64"`
	Configuration map[string]interface{} `json:"configuration" validate:"required"`
}

// UpdateProviderRequest is the API request for updating a provider
type UpdateProviderRequest struct {
	DisplayName   *string                `json:"displayName,omitempty"`
	Configuration map[string]interface{} `json:"configuration,omitempty"`
	Status        *string                `json:"status,omitempty"`
}

// ToResponse converts the database model to API response
func (p *LLMProvider) ToResponse() *LLMProviderResponse {
	return &LLMProviderResponse{
		UUID:          p.UUID.String(),
		Handle:        p.Handle,
		DisplayName:   p.DisplayName,
		Template:      p.Template,
		Configuration: p.Configuration,
		Status:        p.Status,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
		CreatedBy:     p.CreatedBy,
	}
}

// ProviderListResponse is the paginated list response
type ProviderListResponse struct {
	Providers []LLMProviderResponse `json:"providers"`
	Total     int32                 `json:"total"`
	Limit     int32                 `json:"limit"`
	Offset    int32                 `json:"offset"`
}

// ProviderGatewayDeployment tracks provider deployments to gateways
type ProviderGatewayDeployment struct {
	ID                   int                    `gorm:"column:id;primaryKey"`
	ProviderUUID         uuid.UUID              `gorm:"column:provider_uuid;not null"`
	GatewayUUID          uuid.UUID              `gorm:"column:gateway_uuid;not null"`
	DeploymentID         string                 `gorm:"column:deployment_id;not null"`
	Environment          string                 `gorm:"column:environment"`
	ConfigurationVersion int                    `gorm:"column:configuration_version;not null;default:1"`
	GatewayOverrides     map[string]interface{} `gorm:"column:gateway_specific_overrides;type:jsonb;serializer:json"`
	Status               string                 `gorm:"column:status;not null"`
	DeployedAt           *time.Time             `gorm:"column:deployed_at"`
	ErrorMessage         *string                `gorm:"column:error_message;type:text"`
	CreatedAt            time.Time              `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt            time.Time              `gorm:"column:updated_at;not null;default:now()"`
	Provider             *LLMProvider           `gorm:"foreignKey:ProviderUUID;references:UUID"`
}

func (ProviderGatewayDeployment) TableName() string {
	return "provider_gateway_deployments"
}

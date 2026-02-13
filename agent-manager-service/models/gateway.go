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
	"gorm.io/gorm"
)

// Gateway represents an API Platform gateway instance within an organization
type Gateway struct {
	UUID                     uuid.UUID              `gorm:"column:uuid;primaryKey" json:"id"`
	OrganizationUUID         uuid.UUID              `gorm:"column:organization_uuid" json:"organizationId"`
	Name                     string                 `gorm:"column:name" json:"name"`
	DisplayName              string                 `gorm:"column:display_name" json:"displayName"`
	Description              string                 `gorm:"column:description" json:"description"`
	Properties               map[string]interface{} `gorm:"column:properties;type:jsonb;serializer:json" json:"properties,omitempty"`
	Vhost                    string                 `gorm:"column:vhost" json:"vhost"`
	IsCritical               bool                   `gorm:"column:is_critical" json:"isCritical"`
	GatewayFunctionalityType string                 `gorm:"column:gateway_functionality_type" json:"functionalityType"`
	IsActive                 bool                   `gorm:"column:is_active" json:"isActive"`
	CreatedAt                time.Time              `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt                time.Time              `gorm:"column:updated_at" json:"updatedAt"`
	DeletedAt                gorm.DeletedAt         `gorm:"column:deleted_at;index" json:"-"`
}

// TableName returns the table name for the Gateway model
func (Gateway) TableName() string {
	return "gateways"
}

// GatewayToken represents an authentication token for an API Platform gateway
type GatewayToken struct {
	UUID        uuid.UUID  `gorm:"column:uuid;primaryKey" json:"id"`
	GatewayUUID uuid.UUID  `gorm:"column:gateway_uuid" json:"gatewayId"`
	TokenHash   string     `gorm:"column:token_hash" json:"-"`  // Never expose in JSON responses
	Salt        string     `gorm:"column:salt" json:"-"`        // Never expose in JSON responses
	Status      string     `gorm:"column:status" json:"status"` // "active" or "revoked"
	CreatedAt   time.Time  `gorm:"column:created_at" json:"createdAt"`
	RevokedAt   *time.Time `gorm:"column:revoked_at" json:"revokedAt,omitempty"` // Pointer for NULL support
}

// TableName returns the table name for the GatewayToken model
func (GatewayToken) TableName() string {
	return "gateway_tokens"
}

// IsActive returns true if token status is active
func (t *GatewayToken) IsActive() bool {
	return t.Status == "active"
}

// Revoke marks the token as revoked with current timestamp
func (t *GatewayToken) Revoke() {
	now := time.Now()
	t.Status = "revoked"
	t.RevokedAt = &now
}

// APIGatewayWithDetails represents a gateway with its association and deployment details for an API
type APIGatewayWithDetails struct {
	// Gateway information
	UUID                     uuid.UUID              `json:"id" db:"id"`
	OrganizationUUID         uuid.UUID              `json:"organizationId" db:"organization_id"`
	Name                     string                 `json:"name" db:"name"`
	DisplayName              string                 `json:"displayName" db:"display_name"`
	Description              string                 `json:"description" db:"description"`
	Properties               map[string]interface{} `json:"properties,omitempty" db:"properties"`
	Vhost                    string                 `json:"vhost" db:"vhost"`
	IsCritical               bool                   `json:"isCritical" db:"is_critical"`
	GatewayFunctionalityType string                 `json:"functionalityType" db:"functionality_type"`
	IsActive                 bool                   `json:"isActive" db:"is_active"`
	CreatedAt                time.Time              `json:"createdAt" db:"created_at"`
	UpdatedAt                time.Time              `json:"updatedAt" db:"updated_at"`

	// Association information
	AssociatedAt         time.Time `json:"associatedAt" db:"associated_at"`
	AssociationUpdatedAt time.Time `json:"associationUpdatedAt" db:"association_updated_at"`

	IsDeployed bool `json:"isDeployed" db:"is_deployed"`
	// Deployment information (nullable if not deployed)
	DeploymentID *string    `json:"deploymentId,omitempty" db:"deployment_id"`
	DeployedAt   *time.Time `json:"deployedAt,omitempty" db:"deployed_at"`
}

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

// GatewayType enum
type GatewayType string

const (
	GatewayTypeIngress GatewayType = "INGRESS"
	GatewayTypeEgress  GatewayType = "EGRESS"
)

// GatewayStatus enum
type GatewayStatus string

const (
	GatewayStatusActive       GatewayStatus = "ACTIVE"
	GatewayStatusInactive     GatewayStatus = "INACTIVE"
	GatewayStatusProvisioning GatewayStatus = "PROVISIONING"
	GatewayStatusError        GatewayStatus = "ERROR"
)

// GatewayCredentials holds authentication credentials for gateway access
type GatewayCredentials struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	APIKey   string `json:"apiKey,omitempty"`
	Token    string `json:"token,omitempty"`
}

// GatewayResponse is the API response DTO
type GatewayResponse struct {
	UUID             string                       `json:"uuid"`
	OrganizationName string                       `json:"organizationName"`
	Name             string                       `json:"name"`
	DisplayName      string                       `json:"displayName"`
	GatewayType      string                       `json:"gatewayType"`
	ControlPlaneURL  string                       `json:"controlPlaneUrl,omitempty"`
	VHost            string                       `json:"vhost"`
	Region           string                       `json:"region,omitempty"`
	IsCritical       bool                         `json:"isCritical"`
	Status           string                       `json:"status"`
	AdapterConfig    map[string]interface{}       `json:"adapterConfig,omitempty"`
	CreatedAt        time.Time                    `json:"createdAt"`
	UpdatedAt        time.Time                    `json:"updatedAt"`
	Environments     []GatewayEnvironmentResponse `json:"environments,omitempty"`
}

// CreateGatewayRequest is the API request for registering a gateway
type CreateGatewayRequest struct {
	OrganizationName string                 `json:"organizationName" validate:"required,max=100"`
	Name             string                 `json:"name" validate:"required,max=64"`
	DisplayName      string                 `json:"displayName" validate:"required,max=128"`
	GatewayType      string                 `json:"gatewayType" validate:"required,oneof=INGRESS EGRESS"`
	VHost            string                 `json:"vhost" validate:"required,max=253"`
	Region           string                 `json:"region,omitempty"`
	IsCritical       bool                   `json:"isCritical"`
	AdapterConfig    map[string]interface{} `json:"adapterConfig,omitempty"`
	Credentials      *GatewayCredentials    `json:"credentials,omitempty"`
}

// UpdateGatewayRequest is the API request for updating a gateway
type UpdateGatewayRequest struct {
	DisplayName   *string                `json:"displayName,omitempty"`
	IsCritical    *bool                  `json:"isCritical,omitempty"`
	Status        *string                `json:"status,omitempty"`
	AdapterConfig map[string]interface{} `json:"adapterConfig,omitempty"`
	Credentials   *GatewayCredentials    `json:"credentials,omitempty"`
}

// Gateway is the database model
type Gateway struct {
	UUID                 uuid.UUID              `gorm:"column:uuid;primaryKey"`
	OrganizationName     string                 `gorm:"column:organization_name"`
	Name                 string                 `gorm:"column:name"`
	DisplayName          string                 `gorm:"column:display_name"`
	GatewayType          string                 `gorm:"column:gateway_type"`
	ControlPlaneURL      string                 `gorm:"column:control_plane_url"`
	VHost                string                 `gorm:"column:vhost"`
	Region               string                 `gorm:"column:region"`
	IsCritical           bool                   `gorm:"column:is_critical"`
	Status               string                 `gorm:"column:status"`
	AdapterConfig        map[string]interface{} `gorm:"column:adapter_config;type:jsonb;serializer:json"`
	CredentialsEncrypted []byte                 `gorm:"column:credentials_encrypted"`
	CreatedAt            time.Time              `gorm:"column:created_at"`
	UpdatedAt            time.Time              `gorm:"column:updated_at"`
	DeletedAt            gorm.DeletedAt         `gorm:"column:deleted_at"`
	Environments         []Environment          `gorm:"many2many:gateway_environment_mappings;foreignKey:UUID;joinForeignKey:gateway_uuid;References:UUID;joinReferences:environment_uuid"`
}

// TableName returns the table name for GORM
func (Gateway) TableName() string {
	return "gateways"
}

// ToResponse converts the database model to API response
func (g *Gateway) ToResponse() *GatewayResponse {
	resp := &GatewayResponse{
		UUID:             g.UUID.String(),
		OrganizationName: g.OrganizationName,
		Name:             g.Name,
		DisplayName:      g.DisplayName,
		GatewayType:      g.GatewayType,
		ControlPlaneURL:  g.ControlPlaneURL,
		VHost:            g.VHost,
		Region:           g.Region,
		IsCritical:       g.IsCritical,
		Status:           g.Status,
		AdapterConfig:    g.AdapterConfig,
		CreatedAt:        g.CreatedAt,
		UpdatedAt:        g.UpdatedAt,
	}

	if len(g.Environments) > 0 {
		resp.Environments = make([]GatewayEnvironmentResponse, len(g.Environments))
		for i, env := range g.Environments {
			resp.Environments[i] = *env.ToResponse()
		}
	}

	return resp
}

// GatewayEnvironmentMapping is the junction table model
type GatewayEnvironmentMapping struct {
	ID              int       `gorm:"column:id;primaryKey"`
	GatewayUUID     uuid.UUID `gorm:"column:gateway_uuid"`
	EnvironmentUUID uuid.UUID `gorm:"column:environment_uuid"`
	CreatedAt       time.Time `gorm:"column:created_at"`
}

// TableName returns the table name for GORM
func (GatewayEnvironmentMapping) TableName() string {
	return "gateway_environment_mappings"
}

// GatewayListResponse is the paginated list response
type GatewayListResponse struct {
	Gateways []GatewayResponse `json:"gateways"`
	Total    int32             `json:"total"`
	Limit    int32             `json:"limit"`
	Offset   int32             `json:"offset"`
}

// HealthStatusResponse is the gateway health check response
type HealthStatusResponse struct {
	GatewayID    string `json:"gatewayId"`
	Status       string `json:"status"`
	ResponseTime string `json:"responseTime,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
	CheckedAt    string `json:"checkedAt"`
}

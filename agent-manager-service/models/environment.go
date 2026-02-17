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

// GatewayEnvironmentResponse is the API response DTO
type GatewayEnvironmentResponse struct {
	UUID             string    `json:"uuid"`
	OrganizationName string    `json:"organizationName"`
	Name             string    `json:"name"`
	DisplayName      string    `json:"displayName"`
	Description      string    `json:"description,omitempty"`
	DataplaneRef     string    `json:"dataplaneRef"`
	DNSPrefix        string    `json:"dnsPrefix"`
	IsProduction     bool      `json:"isProduction"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// CreateEnvironmentRequest is the API request for creating an environment
type CreateEnvironmentRequest struct {
	OrganizationName string `json:"organizationName" validate:"required,max=100"`
	Name             string `json:"name" validate:"required,max=64"`
	DisplayName      string `json:"displayName" validate:"required,max=128"`
	Description      string `json:"description,omitempty"`
	DataplaneRef     string `json:"dataplaneRef" validate:"required,max=100"`
	DNSPrefix        string `json:"dnsPrefix" validate:"required,max=100"`
	IsProduction     bool   `json:"isProduction"`
}

// UpdateEnvironmentRequest is the API request for updating an environment
type UpdateEnvironmentRequest struct {
	DisplayName *string `json:"displayName,omitempty"`
	Description *string `json:"description,omitempty"`
}

// Environment is the database model
type Environment struct {
	UUID             uuid.UUID      `gorm:"column:uuid;primaryKey"`
	OrganizationName string         `gorm:"column:organization_name"`
	Name             string         `gorm:"column:name"`
	DisplayName      string         `gorm:"column:display_name"`
	Description      string         `gorm:"column:description"`
	DataplaneRef     string         `gorm:"column:dataplane_ref;default:'default'"`
	DNSPrefix        string         `gorm:"column:dns_prefix;default:'default'"`
	IsProduction     bool           `gorm:"column:is_production;default:false"`
	CreatedAt        time.Time      `gorm:"column:created_at"`
	UpdatedAt        time.Time      `gorm:"column:updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"column:deleted_at"`
}

// EnvironmentListResponse is the paginated list response
type EnvironmentListResponse struct {
	Environments []GatewayEnvironmentResponse `json:"environments"`
	Total        int32                        `json:"total"`
	Limit        int32                        `json:"limit"`
	Offset       int32                        `json:"offset"`
}

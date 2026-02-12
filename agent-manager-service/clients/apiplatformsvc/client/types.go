//
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
//

package client

import "time"

// -----------------------------------------------------------------------------
// Enums and Constants
// -----------------------------------------------------------------------------

// FunctionalityType defines the type of gateway functionality
type FunctionalityType string

const (
	FunctionalityTypeRegular FunctionalityType = "regular"
	FunctionalityTypeAI      FunctionalityType = "ai"
	FunctionalityTypeEvent   FunctionalityType = "event"
)

// -----------------------------------------------------------------------------
// Request Types - used for creating/updating resources via the client
// -----------------------------------------------------------------------------

// CreateGatewayRequest contains data for creating a gateway
type CreateGatewayRequest struct {
	Name              string
	DisplayName       string
	Vhost             string
	FunctionalityType FunctionalityType
	Description       *string
	IsCritical        *bool
	Properties        *map[string]interface{}
}

// UpdateGatewayRequest contains data for updating a gateway
type UpdateGatewayRequest struct {
	DisplayName *string
	Description *string
	IsCritical  *bool
	Properties  *map[string]interface{}
}

// -----------------------------------------------------------------------------
// Response Types - returned by client methods
// -----------------------------------------------------------------------------

// GatewayResponse represents a gateway response from API Platform
type GatewayResponse struct {
	ID                string
	Name              string
	DisplayName       string
	Description       string
	Vhost             string
	FunctionalityType string
	IsCritical        bool
	IsActive          bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Properties        map[string]interface{}
}

// GatewayTokenResponse represents a gateway token rotation response
type GatewayTokenResponse struct {
	GatewayID string
	Token     string
	TokenID   string
	CreatedAt time.Time
	ExpiresAt *time.Time
}

// -----------------------------------------------------------------------------
// Organization Request/Response Types
// -----------------------------------------------------------------------------

// RegisterOrganizationRequest contains data for registering an organization
type RegisterOrganizationRequest struct {
	ID     string
	Name   string
	Handle string
	Region string
}

// OrganizationResponse represents an organization response from API Platform
type OrganizationResponse struct {
	ID        string
	Name      string
	Handle    string
	Region    string
	CreatedAt time.Time
}

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
)

// GatewayEnvironmentResponse is the API response DTO
type GatewayEnvironmentResponse struct {
	UUID             string       `json:"uuid"`
	OrganizationName string       `json:"organizationName"`
	Name             string       `json:"name"`
	DisplayName      string       `json:"displayName"`
	Description      string       `json:"description,omitempty"`
	IsolationTier    string       `json:"isolationTier,omitempty"`
	DataplaneRef     string       `json:"dataplaneRef"`
	DNSPrefix        string       `json:"dnsPrefix"`
	IsProduction     bool         `json:"isProduction"`
	Gateway          *GatewaySpec `json:"gateway,omitempty"`
	CreatedAt        time.Time    `json:"createdAt"`
	UpdatedAt        time.Time    `json:"updatedAt"`
}

// CreateEnvironmentRequest is the API request for creating an environment
type CreateEnvironmentRequest struct {
	OrganizationName string       `json:"organizationName" validate:"required,max=100"`
	Name             string       `json:"name" validate:"required,max=64"`
	DisplayName      string       `json:"displayName" validate:"required,max=128"`
	Description      string       `json:"description,omitempty"`
	IsolationTier    string       `json:"isolationTier,omitempty"`
	DataplaneRef     string       `json:"dataplaneRef,omitempty" validate:"omitempty,max=100"`
	DNSPrefix        string       `json:"dnsPrefix" validate:"required,max=100"`
	IsProduction     bool         `json:"isProduction"`
	Gateway          *GatewaySpec `json:"gateway,omitempty"`
}

// UpdateEnvironmentRequest is the API request for updating an environment
type UpdateEnvironmentRequest struct {
	DisplayName  *string      `json:"displayName,omitempty"`
	Description  *string      `json:"description,omitempty"`
	IsProduction *bool        `json:"isProduction,omitempty"`
	Gateway      *GatewaySpec `json:"gateway,omitempty"`
}

// GatewaySpec is the internal representation of the gateway configuration on an
// environment. Only fields surfaced through the public API are present; runtime
// OC-only fields (Gateway resource Name/Namespace, listener Name) are filled in
// when constructing the OC payload.
type GatewaySpec struct {
	Ingress *GatewayNetworkSpec `json:"ingress,omitempty"`
	Egress  *GatewayNetworkSpec `json:"egress,omitempty"`
}

// GatewayNetworkSpec splits a network direction into external and internal endpoints.
type GatewayNetworkSpec struct {
	External *GatewayEndpointSpec `json:"external,omitempty"`
	Internal *GatewayEndpointSpec `json:"internal,omitempty"`
}

// GatewayEndpointSpec carries the listener configuration for an endpoint.
type GatewayEndpointSpec struct {
	HTTP  *GatewayListenerSpec `json:"http,omitempty"`
	HTTPS *GatewayListenerSpec `json:"https,omitempty"`
	TLS   *GatewayListenerSpec `json:"tls,omitempty"`
}

// GatewayListenerSpec is the listener-level config exposed through the API.
type GatewayListenerSpec struct {
	Port *int32  `json:"port,omitempty"`
	Host *string `json:"host,omitempty"`
}

// EnvironmentListResponse is the paginated list response
type EnvironmentListResponse struct {
	Environments []GatewayEnvironmentResponse `json:"environments"`
	Total        int32                        `json:"total"`
	Limit        int32                        `json:"limit"`
	Offset       int32                        `json:"offset"`
}

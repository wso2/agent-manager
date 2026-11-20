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

package gwmodels

import (
	"strings"
	"time"
)

// DefaultEnvironmentName is the name of every organization's default
// environment.
const DefaultEnvironmentName = "default"

// Default OAuth header values used when a config omits them. They mirror the
// gateway jwt-auth policy defaults so callers send a standard
// `Authorization: Bearer <token>` header.
const (
	DefaultOAuthHeaderName       = "Authorization"
	DefaultOAuthAuthHeaderPrefix = "Bearer"
	// DefaultOAuthForwardToken mirrors the gateway jwt-auth policy default: the
	// validated token header is forwarded to the upstream service unless disabled.
	DefaultOAuthForwardToken = true
)

// ReservedIdentityProviderName is the gateway identity provider used
// internally. It lives in the gateway runtime config but is hidden from users:
// never mirrored into AMS, never shown in the UI, and never offered as an
// agent OAuth issuer.
const ReservedIdentityProviderName = "agent-manager-service"

// systemIdentityProviderNames are the identity providers seeded with the
// platform. They are mirrored with type=system and cannot be deleted.
var systemIdentityProviderNames = map[string]bool{"ThunderKeyManager": true}

// envThunderIdentityProviderPrefix matches the env-Thunder release names
// (amp-thunder-<org>-<env>) the gateway bootstrap seeds as identity providers.
const envThunderIdentityProviderPrefix = "amp-thunder-"

// IsSystemIdentityProvider reports whether the identity provider is a
// platform-seeded system provider.
func IsSystemIdentityProvider(name string) bool {
	return systemIdentityProviderNames[name] || strings.HasPrefix(name, envThunderIdentityProviderPrefix)
}

// GatewayEnvironmentResponse is an environment as a gateway response lists it.
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

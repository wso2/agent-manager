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
	OUID                     string                 `gorm:"column:ou_id" json:"organizationId"`
	Name                     string                 `gorm:"column:name" json:"name"`
	DisplayName              string                 `gorm:"column:display_name" json:"displayName"`
	Description              string                 `gorm:"column:description" json:"description"`
	Properties               map[string]interface{} `gorm:"column:properties;type:jsonb;serializer:json" json:"properties,omitempty"`
	Manifest                 map[string]interface{} `gorm:"column:manifest;type:jsonb;serializer:json" json:"manifest,omitempty"`
	Vhost                    string                 `gorm:"column:vhost" json:"vhost"`
	RuntimeURL               string                 `gorm:"column:runtime_url" json:"runtimeUrl,omitempty"`
	IsCritical               bool                   `gorm:"column:is_critical" json:"isCritical"`
	GatewayFunctionalityType string                 `gorm:"column:gateway_functionality_type" json:"functionalityType"`
	IsActive                 bool                   `gorm:"column:is_active" json:"isActive"`
	Hosting                  string                 `gorm:"column:hosting;default:managed" json:"hosting"`
	CreatedAt                time.Time              `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt                time.Time              `gorm:"column:updated_at" json:"updatedAt"`
	DeletedAt                gorm.DeletedAt         `gorm:"column:deleted_at;index" json:"-"`
}

// TableName returns the table name for the Gateway model
func (Gateway) TableName() string {
	return "gateways"
}

// Gateway hosting. Stored in gateways.hosting: who runs the gateway runtime.
const (
	// GatewayHostingManaged is a gateway the platform provisions and runs.
	GatewayHostingManaged = "managed"
	// GatewayHostingSelfHosted is a gateway the customer runs and connects to the platform.
	GatewayHostingSelfHosted = "self_hosted"
)

// IsManaged reports whether the platform runs this gateway. A stored gateway is always managed or
// self_hosted (migration 054: NOT NULL DEFAULT 'managed' with a CHECK). An empty Hosting only occurs
// in structs built in memory without it, such as test fixtures, and counts as managed, the column
// default. That is also the safe reading for the MCP upstream checks, which run only for managed gateways.
func (g *Gateway) IsManaged() bool {
	return g.Hosting == "" || g.Hosting == GatewayHostingManaged
}

// Gateway placement roles. Stored lowercase in gateways.gateway_functionality_type;
// emitted uppercase on the wire. Roles are naming and placement policy, not capability:
// every gateway has identical runtime capabilities regardless of role.
const (
	// GatewayRoleIngress handles inbound agent traffic. At most one per environment.
	GatewayRoleIngress = "ingress"
	// GatewayRoleEgress hosts outbound LLM/MCP artifacts. Uncapped per environment.
	GatewayRoleEgress = "egress"
	// GatewayRoleBoth does both. Counts against the ingress cap.
	GatewayRoleBoth = "both"
)

// IngressGatewayRoles are the roles that occupy an environment's single ingress slot.
var IngressGatewayRoles = []string{GatewayRoleIngress, GatewayRoleBoth}

// EgressGatewayRoles are the roles eligible to host LLM/MCP artifacts. Not capped:
// "both" plus "egress" in one environment is the supported shape for an existing
// environment that gains egress separation.
var EgressGatewayRoles = []string{GatewayRoleEgress, GatewayRoleBoth}

// IsIngressCapable reports whether this gateway occupies its environment's ingress slot.
func (g *Gateway) IsIngressCapable() bool {
	return g.GatewayFunctionalityType == GatewayRoleIngress || g.GatewayFunctionalityType == GatewayRoleBoth
}

// IsEgressCapable reports whether this gateway is a legal target for LLM/MCP placement.
func (g *Gateway) IsEgressCapable() bool {
	return g.GatewayFunctionalityType == GatewayRoleEgress || g.GatewayFunctionalityType == GatewayRoleBoth
}

// GatewayToken represents an authentication token for an API Platform gateway
type GatewayToken struct {
	UUID        uuid.UUID  `gorm:"column:uuid;primaryKey" json:"id"`
	GatewayUUID uuid.UUID  `gorm:"column:gateway_uuid" json:"gatewayId"`
	TokenPrefix string     `gorm:"column:token_prefix" json:"-"` // First 8 chars of plaintext token for indexed lookup
	TokenHash   string     `gorm:"column:token_hash" json:"-"`   // Never expose in JSON responses
	Salt        string     `gorm:"column:salt" json:"-"`         // Never expose in JSON responses
	Status      string     `gorm:"column:status" json:"status"`  // "active" or "revoked"
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

// Identity provider provenance. System providers are seeded with the platform
// (e.g. ThunderKeyManager) and cannot be deleted; custom providers are added by
// operators via the management script.
const (
	IdentityProviderTypeSystem = "system"
	IdentityProviderTypeCustom = "custom"
)

// GatewayIdentityProvider mirrors a gateway-side jwt-auth keymanagers (token
// issuer) entry. The gateway ConfigMap is the source of truth; this row is a
// read mirror written by the gateway bootstrap and the management script.
type GatewayIdentityProvider struct {
	UUID              uuid.UUID `gorm:"column:uuid;primaryKey" json:"-"`
	GatewayUUID       uuid.UUID `gorm:"column:gateway_uuid" json:"-"`
	Name              string    `gorm:"column:name" json:"name"`
	Issuer            string    `gorm:"column:issuer" json:"issuer"`
	JWKSUri           string    `gorm:"column:jwks_uri" json:"jwksUri"`
	Description       string    `gorm:"column:description" json:"description,omitempty"`
	Type              string    `gorm:"column:type" json:"type"`
	JWKSSkipTLSVerify bool      `gorm:"column:jwks_skip_tls_verify" json:"skipTlsVerify"`
	CreatedAt         time.Time `gorm:"column:created_at" json:"-"`
	UpdatedAt         time.Time `gorm:"column:updated_at" json:"-"`
}

// TableName returns the table name for the GatewayIdentityProvider model
func (GatewayIdentityProvider) TableName() string {
	return "gateway_identity_providers"
}

// APIGatewayWithDetails represents a gateway with its association and deployment details for an API
type APIGatewayWithDetails struct {
	// Gateway information
	UUID                     uuid.UUID              `json:"id" db:"id"`
	OrganizationName         string                 `json:"organizationId" db:"organization_id"`
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

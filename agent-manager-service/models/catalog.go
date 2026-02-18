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

// CatalogEntry represents a resource in the catalog
// This model maps to the artifacts table with in_catalog filter
type CatalogEntry struct {
	UUID             uuid.UUID `gorm:"column:uuid;primaryKey" json:"uuid"`
	Handle           string    `gorm:"column:handle;not null" json:"handle"`
	Name             string    `gorm:"column:name;not null" json:"name"`
	Version          string    `gorm:"column:version;not null" json:"version"`
	Kind             string    `gorm:"column:kind;not null" json:"kind"`
	InCatalog        bool      `gorm:"column:in_catalog" json:"inCatalog"`
	OrganizationUUID uuid.UUID `gorm:"column:organization_uuid;not null" json:"-"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"createdAt"`
}

// TableName returns the table name for catalog queries
func (CatalogEntry) TableName() string {
	return "artifacts"
}

// Catalog resource kind constants
const (
	CatalogKindLLMProvider = "LlmProvider"
	CatalogKindAgent       = "agent"
	CatalogKindMCP         = "mcp"
)

// CatalogLLMProviderEntry represents a comprehensive LLM provider in the catalog
type CatalogLLMProviderEntry struct {
	// Basic Artifact Information
	UUID      uuid.UUID `json:"uuid"`
	Handle    string    `json:"handle"`
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	Kind      string    `json:"kind"`
	InCatalog bool      `json:"inCatalog"`
	CreatedAt time.Time `json:"createdAt"`

	// LLM Provider Details
	Description string `json:"description,omitempty"`
	CreatedBy   string `json:"createdBy,omitempty"`
	Status      string `json:"status"`

	// Configuration Summary
	Template string  `json:"template"`
	Context  *string `json:"context,omitempty"`
	VHost    *string `json:"vhost,omitempty"`

	// Model Providers
	ModelProviders []LLMModelProvider `json:"modelProviders,omitempty"`

	// Security Configuration
	Security *SecuritySummary `json:"security,omitempty"`

	// Rate Limiting Configuration Summary
	RateLimiting *RateLimitingSummary `json:"rateLimiting,omitempty"`

	// Deployment Information
	Deployments []DeploymentSummary `json:"deployments,omitempty"`
}

// SecuritySummary provides security configuration overview
type SecuritySummary struct {
	Enabled       bool   `json:"enabled"`
	APIKeyEnabled bool   `json:"apiKeyEnabled"`
	APIKeyIn      string `json:"apiKeyIn,omitempty"`
}

// RateLimitingSummary provides rate limiting overview
type RateLimitingSummary struct {
	ProviderLevel *RateLimitingScope `json:"providerLevel,omitempty"`
	ConsumerLevel *RateLimitingScope `json:"consumerLevel,omitempty"`
}

// RateLimitingScope summarizes scope-level limits
type RateLimitingScope struct {
	GlobalEnabled       bool     `json:"globalEnabled"`
	ResourceWiseEnabled bool     `json:"resourceWiseEnabled"`
	RequestLimitCount   *int     `json:"requestLimitCount,omitempty"`
	TokenLimitCount     *int     `json:"tokenLimitCount,omitempty"`
	CostLimitAmount     *float64 `json:"costLimitAmount,omitempty"`
}

// DeploymentSummary provides deployment status per environment
type DeploymentSummary struct {
	GatewayID       uuid.UUID        `json:"gatewayId"`
	GatewayName     string           `json:"gatewayName"`
	EnvironmentName string           `json:"environmentName"`
	Status          DeploymentStatus `json:"status"`
	DeployedAt      *time.Time       `json:"deployedAt,omitempty"`
	VHost           string           `json:"vhost,omitempty"`
}

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
	"github.com/google/uuid"
)

// LLMProvider represents an LLM provider entity
type LLMProvider struct {
	UUID          uuid.UUID         `gorm:"column:uuid;primaryKey" json:"uuid"`
	Description   string            `gorm:"column:description" json:"description,omitempty"`
	CreatedBy     string            `gorm:"column:created_by" json:"createdBy,omitempty"`
	TemplateUUID  uuid.UUID         `gorm:"column:template_uuid" json:"templateUuid"`
	OpenAPISpec   string            `gorm:"column:openapi_spec;type:text" json:"openapi,omitempty"`
	ModelList     string            `gorm:"column:model_list;type:text" json:"-"` // TEXT field stores model list
	Status        string            `gorm:"column:status" json:"status"`
	Configuration LLMProviderConfig `gorm:"column:configuration;type:jsonb;serializer:json" json:"configuration"`

	// Relations - populated via joins
	Artifact       *Artifact          `gorm:"foreignKey:UUID;references:UUID" json:"artifact,omitempty"`
	ModelProviders []LLMModelProvider `gorm:"-" json:"modelProviders,omitempty"` // Parsed from ModelList
	InCatalog      bool               `gorm:"-" json:"inCatalog"`                // Populated from Artifact.InCatalog via join
}

// TableName returns the table name for the LLMProvider model
func (LLMProvider) TableName() string {
	return "llm_providers"
}

// LLMProviderConfig represents the LLM provider configuration
type LLMProviderConfig struct {
	Name          string                 `json:"name,omitempty"`
	Version       string                 `json:"version,omitempty"`
	Context       *string                `json:"context,omitempty"`
	VHost         *string                `json:"vhost,omitempty"`
	Template      string                 `json:"template,omitempty"`
	Upstream      *UpstreamConfig        `json:"upstream,omitempty"`
	AccessControl *LLMAccessControl      `json:"accessControl,omitempty"`
	RateLimiting  *LLMRateLimitingConfig `json:"rateLimiting,omitempty"`
	Policies      []LLMPolicy            `json:"policies,omitempty"`
	Security      *SecurityConfig        `json:"security,omitempty"`
}

// LLMModelProvider represents a model provider
type LLMModelProvider struct {
	ID     string     `json:"id"`
	Name   string     `json:"name,omitempty"`
	Models []LLMModel `json:"models,omitempty"`
}

// LLMModel represents an LLM model
type LLMModel struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// LLMAccessControl represents access control configuration
type LLMAccessControl struct {
	Mode       string           `json:"mode"`
	Exceptions []RouteException `json:"exceptions,omitempty"`
}

// RouteException represents a route exception
type RouteException struct {
	Path    string   `json:"path"`
	Methods []string `json:"methods"`
}

// LLMPolicy represents an LLM policy
type LLMPolicy struct {
	Name    string          `json:"name"`
	Version string          `json:"version"`
	Paths   []LLMPolicyPath `json:"paths"`
}

// LLMPolicyPath represents a policy path
type LLMPolicyPath struct {
	Path    string                 `json:"path"`
	Methods []string               `json:"methods"`
	Params  map[string]interface{} `json:"params"`
}

// LLMRateLimitingConfig represents rate limiting configuration
type LLMRateLimitingConfig struct {
	ProviderLevel *RateLimitingScopeConfig `json:"providerLevel,omitempty"`
	ConsumerLevel *RateLimitingScopeConfig `json:"consumerLevel,omitempty"`
}

// RateLimitingScopeConfig represents scope-level rate limiting configuration
type RateLimitingScopeConfig struct {
	Global       *RateLimitingLimitConfig        `json:"global,omitempty"`
	ResourceWise *ResourceWiseRateLimitingConfig `json:"resourceWise,omitempty"`
}

// ResourceWiseRateLimitingConfig represents resource-wise rate limiting configuration
type ResourceWiseRateLimitingConfig struct {
	Default   RateLimitingLimitConfig     `json:"default"`
	Resources []RateLimitingResourceLimit `json:"resources"`
}

// RateLimitingResourceLimit represents a resource-specific rate limit
type RateLimitingResourceLimit struct {
	Resource string                  `json:"resource"`
	Limit    RateLimitingLimitConfig `json:"limit"`
}

// RateLimitingLimitConfig represents rate limit configuration
type RateLimitingLimitConfig struct {
	Request *RequestRateLimit `json:"request,omitempty"`
	Token   *TokenRateLimit   `json:"token,omitempty"`
	Cost    *CostRateLimit    `json:"cost,omitempty"`
}

// RequestRateLimit represents request rate limiting
type RequestRateLimit struct {
	Enabled bool                 `json:"enabled"`
	Count   int                  `json:"count"`
	Reset   RateLimitResetWindow `json:"reset"`
}

// TokenRateLimit represents token rate limiting
type TokenRateLimit struct {
	Enabled bool                 `json:"enabled"`
	Count   int                  `json:"count"`
	Reset   RateLimitResetWindow `json:"reset"`
}

// CostRateLimit represents cost rate limiting
type CostRateLimit struct {
	Enabled bool                 `json:"enabled"`
	Amount  float64              `json:"amount"`
	Reset   RateLimitResetWindow `json:"reset"`
}

// RateLimitResetWindow represents a rate limit reset window
type RateLimitResetWindow struct {
	Duration int    `json:"duration"`
	Unit     string `json:"unit"`
}

// SecurityConfig represents security configuration
type SecurityConfig struct {
	Enabled *bool           `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	APIKey  *APIKeySecurity `json:"apiKey,omitempty" yaml:"apiKey,omitempty"`
}

// APIKeySecurity represents API key security configuration
type APIKeySecurity struct {
	Enabled *bool  `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Key     string `json:"key,omitempty" yaml:"key,omitempty"`
	In      string `json:"in,omitempty" yaml:"in,omitempty"`
}

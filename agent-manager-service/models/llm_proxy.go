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

// LLMProxy represents an LLM proxy entity
type LLMProxy struct {
	UUID          uuid.UUID      `gorm:"column:uuid;primaryKey" json:"uuid"`
	ProjectUUID   uuid.UUID      `gorm:"column:project_uuid" json:"projectId"`
	Description   string         `gorm:"column:description" json:"description,omitempty"`
	CreatedBy     string         `gorm:"column:created_by" json:"createdBy,omitempty"`
	ProviderUUID  uuid.UUID      `gorm:"column:provider_uuid" json:"providerUuid"`
	OpenAPISpec   string         `gorm:"column:openapi_spec;type:text" json:"openapi,omitempty"`
	Status        string         `gorm:"column:status" json:"status"`
	Configuration LLMProxyConfig `gorm:"column:configuration;type:jsonb;serializer:json" json:"configuration"`

	// Computed/derived fields from Artifact table (populated via joins, not stored in llm_proxies table)
	OrganizationUUID string    `gorm:"-" json:"organizationId,omitempty"`
	ID               string    `gorm:"-" json:"id,omitempty"`
	Name             string    `gorm:"-" json:"name,omitempty"`
	Handle           string    `gorm:"-" json:"handle,omitempty"`
	Version          string    `gorm:"-" json:"version,omitempty"`
	CreatedAt        time.Time `gorm:"-" json:"createdAt,omitempty"`
	UpdatedAt        time.Time `gorm:"-" json:"updatedAt,omitempty"`
}

// TableName returns the table name for the LLMProxy model
func (LLMProxy) TableName() string {
	return "llm_proxies"
}

// LLMProxyConfig represents the LLM proxy configuration
type LLMProxyConfig struct {
	Name         string          `json:"name,omitempty"`
	Version      string          `json:"version,omitempty"`
	Context      *string         `json:"context,omitempty"`
	Vhost        *string         `json:"vhost,omitempty"`
	Provider     string          `json:"provider,omitempty"`
	UpstreamAuth *UpstreamAuth   `json:"upstreamAuth,omitempty"`
	Policies     []LLMPolicy     `json:"policies,omitempty"`
	Security     *SecurityConfig `json:"security,omitempty"`
}

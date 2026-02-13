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

// LLMProviderTemplate represents an LLM provider template
// This structure matches api_platform/platform-api/src/internal/model/llm.go:129
type LLMProviderTemplate struct {
	UUID             uuid.UUID `gorm:"column:uuid;primaryKey" json:"uuid"`
	OrganizationUUID uuid.UUID `gorm:"column:organization_uuid" json:"organizationId"`
	Handle           string    `gorm:"column:handle" json:"id"`
	Name             string    `gorm:"column:name" json:"name"`
	Description      string    `gorm:"column:description" json:"description,omitempty"`
	CreatedBy        string    `gorm:"column:created_by" json:"createdBy,omitempty"`
	Configuration    string    `gorm:"column:configuration;type:text" json:"-"` // TEXT field stores raw config as JSON
	CreatedAt        time.Time `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt        time.Time `gorm:"column:updated_at" json:"updatedAt"`

	// Parsed configuration fields (not stored in DB, populated from Configuration field)
	// These match the API Platform structure exactly
	Metadata         *LLMProviderTemplateMetadata `gorm:"-" json:"metadata,omitempty"`
	PromptTokens     *ExtractionIdentifier        `gorm:"-" json:"promptTokens,omitempty"`
	CompletionTokens *ExtractionIdentifier        `gorm:"-" json:"completionTokens,omitempty"`
	TotalTokens      *ExtractionIdentifier        `gorm:"-" json:"totalTokens,omitempty"`
	RemainingTokens  *ExtractionIdentifier        `gorm:"-" json:"remainingTokens,omitempty"`
	RequestModel     *ExtractionIdentifier        `gorm:"-" json:"requestModel,omitempty"`
	ResponseModel    *ExtractionIdentifier        `gorm:"-" json:"responseModel,omitempty"`
}

// TableName returns the table name for the LLMProviderTemplate model
func (LLMProviderTemplate) TableName() string {
	return "llm_provider_templates"
}

// LLMProviderTemplateMetadata represents template metadata
type LLMProviderTemplateMetadata struct {
	EndpointURL    string                   `json:"endpointUrl,omitempty"`
	Auth           *LLMProviderTemplateAuth `json:"auth,omitempty"`
	LogoURL        string                   `json:"logoUrl,omitempty"`
	OpenapiSpecURL string                   `json:"openapiSpecUrl,omitempty"`
}

// LLMProviderTemplateAuth represents template authentication configuration
type LLMProviderTemplateAuth struct {
	Type        string `json:"type,omitempty"`
	Header      string `json:"header,omitempty"`
	ValuePrefix string `json:"valuePrefix,omitempty"`
}

// ExtractionIdentifier represents an extraction identifier for LLM metadata
type ExtractionIdentifier struct {
	Location   string `json:"location"`
	Identifier string `json:"identifier"`
}

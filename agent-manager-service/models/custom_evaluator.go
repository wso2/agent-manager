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
	"regexp"
	"time"

	"github.com/google/uuid"
)

// Custom evaluator types
const (
	CustomEvaluatorTypeCode     = "code"
	CustomEvaluatorTypeLLMJudge = "llm_judge"
)

// Custom evaluator provider names (used in EvaluatorResponse.Provider)
const (
	CustomProviderCode     = "custom_code"
	CustomProviderLLMJudge = "custom_llm_judge"
)

// CustomEvaluator is the GORM model for the custom_evaluators table
type CustomEvaluator struct {
	ID           uuid.UUID              `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OrgName      string                 `gorm:"column:org_name;not null"`
	Identifier   string                 `gorm:"column:identifier;not null"`
	DisplayName  string                 `gorm:"column:display_name;not null"`
	Description  string                 `gorm:"column:description;not null;default:''"`
	Version      string                 `gorm:"column:version;not null;default:'1.0'"`
	Type         string                 `gorm:"column:type;not null"`
	Level        string                 `gorm:"column:level;not null"`
	Source       string                 `gorm:"column:source;not null"`
	ConfigSchema []EvaluatorConfigParam `gorm:"column:config_schema;type:jsonb;serializer:json;not null;default:'[]'"`
	Tags         []string               `gorm:"column:tags;type:jsonb;serializer:json;not null;default:'[]'"`
	CreatedAt    time.Time              `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt    time.Time              `gorm:"column:updated_at;not null;default:NOW()"`
	DeletedAt    *time.Time             `gorm:"column:deleted_at"`
}

func (CustomEvaluator) TableName() string { return "custom_evaluators" }

// ToEvaluatorResponse converts a CustomEvaluator DB record to the shared EvaluatorResponse DTO
func (ce *CustomEvaluator) ToEvaluatorResponse() *EvaluatorResponse {
	provider := CustomProviderCode
	if ce.Type == CustomEvaluatorTypeLLMJudge {
		provider = CustomProviderLLMJudge
	}

	// Build auto-tags from type, then append user-defined tags (deduped)
	var autoTags []string
	if ce.Type == CustomEvaluatorTypeCode {
		autoTags = []string{"code"}
	} else {
		autoTags = []string{"llm-judge"}
	}

	seen := make(map[string]struct{}, len(autoTags)+len(ce.Tags))
	tags := make([]string, 0, len(autoTags)+len(ce.Tags))
	for _, t := range autoTags {
		seen[t] = struct{}{}
		tags = append(tags, t)
	}
	for _, t := range ce.Tags {
		if _, exists := seen[t]; !exists {
			seen[t] = struct{}{}
			tags = append(tags, t)
		}
	}

	// For llm_judge evaluators, prepend base config params that the user hasn't overridden.
	configSchema := ce.ConfigSchema
	if ce.Type == CustomEvaluatorTypeLLMJudge {
		userKeys := make(map[string]struct{}, len(configSchema))
		for _, p := range configSchema {
			userKeys[p.Key] = struct{}{}
		}
		merged := make([]EvaluatorConfigParam, 0, len(LLMJudgeBaseConfigSchema)+len(configSchema))
		for _, p := range LLMJudgeBaseConfigSchema {
			if _, overridden := userKeys[p.Key]; !overridden {
				merged = append(merged, p)
			}
		}
		configSchema = append(merged, configSchema...)
	}
	if configSchema == nil {
		configSchema = []EvaluatorConfigParam{}
	}

	return &EvaluatorResponse{
		ID:           ce.ID,
		Identifier:   ce.Identifier,
		DisplayName:  ce.DisplayName,
		Description:  ce.Description,
		Version:      ce.Version,
		Provider:     provider,
		Level:        ce.Level,
		Tags:         tags,
		IsBuiltin:    false,
		ConfigSchema: configSchema,
		Type:         ce.Type,
		Source:       ce.Source,
	}
}

// IdentifierRegex validates that identifiers are URL-path-safe slugs.
var IdentifierRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// CreateCustomEvaluatorRequest is the request body for creating a custom evaluator
type CreateCustomEvaluatorRequest struct {
	Identifier   string                 `json:"identifier" validate:"omitempty,min=1,max=128"`
	DisplayName  string                 `json:"displayName" validate:"required,min=1,max=128"`
	Description  string                 `json:"description" validate:"max=512"`
	Type         string                 `json:"type" validate:"required,oneof=code llm_judge"`
	Level        string                 `json:"level" validate:"required,oneof=trace agent llm"`
	Source       string                 `json:"source" validate:"required,min=1"`
	ConfigSchema []EvaluatorConfigParam `json:"configSchema,omitempty"`
	Tags         []string               `json:"tags,omitempty"`
}

// UpdateCustomEvaluatorRequest is the request body for updating a custom evaluator
type UpdateCustomEvaluatorRequest struct {
	DisplayName  *string                 `json:"displayName,omitempty" validate:"omitempty,min=1,max=128"`
	Description  *string                 `json:"description,omitempty" validate:"omitempty,max=512"`
	Source       *string                 `json:"source,omitempty" validate:"omitempty,min=1"`
	ConfigSchema *[]EvaluatorConfigParam `json:"configSchema,omitempty"`
	Tags         *[]string               `json:"tags,omitempty"`
}

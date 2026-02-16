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

// Evaluator represents an evaluator definition in the catalog
// Supports both platform-provided builtin evaluators and organization-specific custom evaluators
type Evaluator struct {
	ID          uuid.UUID `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	Identifier  string    `gorm:"column:identifier;not null"`
	DisplayName string    `gorm:"column:display_name;not null"`
	Description string    `gorm:"column:description;not null;default:''"`
	Version     string    `gorm:"column:version;not null;default:'1.0'"`

	// SDK binding
	Provider  string `gorm:"column:provider;not null"`
	ClassName string `gorm:"column:class_name;not null"`

	// Classification
	Tags         []string               `gorm:"column:tags;type:jsonb;serializer:json;not null;default:'[]'"`
	IsBuiltin    bool                   `gorm:"column:is_builtin;not null;default:false"`
	ConfigSchema []EvaluatorConfigParam `gorm:"column:config_schema;type:jsonb;serializer:json;not null;default:'[]'"`

	// Ownership (NULL for builtins)
	OrgID     *uuid.UUID `gorm:"column:org_id;type:uuid"`
	CreatedBy string     `gorm:"column:created_by"`

	// Timestamps
	CreatedAt time.Time `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:NOW()"`
}

// EvaluatorConfigParam represents a single configuration parameter for an evaluator
type EvaluatorConfigParam struct {
	Key         string      `json:"key"`
	Type        string      `json:"type"` // string, integer, float, boolean, array, enum
	Description string      `json:"description"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
	Min         *float64    `json:"min,omitempty"`
	Max         *float64    `json:"max,omitempty"`
	EnumValues  []string    `json:"enumValues,omitempty"`
}

// TableName specifies the table name for the Evaluator model
func (Evaluator) TableName() string {
	return "evaluator_catalog"
}

// BeforeCreate hook is called before creating a new evaluator
func (e *Evaluator) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}

// EvaluatorResponse is the API response DTO for an evaluator
type EvaluatorResponse struct {
	ID           uuid.UUID              `json:"id"`
	Identifier   string                 `json:"identifier"`
	DisplayName  string                 `json:"displayName"`
	Description  string                 `json:"description"`
	Version      string                 `json:"version"`
	Provider     string                 `json:"provider"`
	Tags         []string               `json:"tags"`
	IsBuiltin    bool                   `json:"isBuiltin"`
	ConfigSchema []EvaluatorConfigParam `json:"configSchema"`
	CreatedAt    time.Time              `json:"createdAt"`
	UpdatedAt    time.Time              `json:"updatedAt"`
}

// ToResponse converts an Evaluator model to EvaluatorResponse DTO
func (e *Evaluator) ToResponse() *EvaluatorResponse {
	return &EvaluatorResponse{
		ID:           e.ID,
		Identifier:   e.Identifier,
		DisplayName:  e.DisplayName,
		Description:  e.Description,
		Version:      e.Version,
		Provider:     e.Provider,
		Tags:         e.Tags,
		IsBuiltin:    e.IsBuiltin,
		ConfigSchema: e.ConfigSchema,
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    e.UpdatedAt,
	}
}

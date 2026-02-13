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

package repositories

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

type llmProviderTemplateConfig struct {
	Metadata         *models.LLMProviderTemplateMetadata `json:"metadata,omitempty"`
	PromptTokens     *models.ExtractionIdentifier        `json:"promptTokens,omitempty"`
	CompletionTokens *models.ExtractionIdentifier        `json:"completionTokens,omitempty"`
	TotalTokens      *models.ExtractionIdentifier        `json:"totalTokens,omitempty"`
	RemainingTokens  *models.ExtractionIdentifier        `json:"remainingTokens,omitempty"`
	RequestModel     *models.ExtractionIdentifier        `json:"requestModel,omitempty"`
	ResponseModel    *models.ExtractionIdentifier        `json:"responseModel,omitempty"`
}

// LLMProviderTemplateRepository defines the interface for LLM provider template persistence
type LLMProviderTemplateRepository interface {
	Create(t *models.LLMProviderTemplate) error
	GetByHandle(templateHandle, orgUUID string) (*models.LLMProviderTemplate, error)
	GetByUUID(uuid, orgUUID string) (*models.LLMProviderTemplate, error)
	List(orgUUID string, limit, offset int) ([]*models.LLMProviderTemplate, error)
	Count(orgUUID string) (int, error)
	Update(t *models.LLMProviderTemplate) error
	Delete(templateID, orgUUID string) error
	Exists(templateID, orgUUID string) (bool, error)
}

// LLMProviderTemplateRepo implements LLMProviderTemplateRepository using GORM
type LLMProviderTemplateRepo struct {
	db *gorm.DB
}

// NewLLMProviderTemplateRepo creates a new LLM provider template repository
func NewLLMProviderTemplateRepo(db *gorm.DB) LLMProviderTemplateRepository {
	return &LLMProviderTemplateRepo{db: db}
}

// unmarshalConfig unmarshals the Configuration JSON into the template's parsed fields
func unmarshalConfig(template *models.LLMProviderTemplate) error {
	if template.Configuration == "" {
		return nil
	}

	var cfg llmProviderTemplateConfig
	if err := json.Unmarshal([]byte(template.Configuration), &cfg); err != nil {
		return err
	}

	template.Metadata = cfg.Metadata
	template.PromptTokens = cfg.PromptTokens
	template.CompletionTokens = cfg.CompletionTokens
	template.TotalTokens = cfg.TotalTokens
	template.RemainingTokens = cfg.RemainingTokens
	template.RequestModel = cfg.RequestModel
	template.ResponseModel = cfg.ResponseModel

	return nil
}

// Create inserts a new LLM provider template
func (r *LLMProviderTemplateRepo) Create(t *models.LLMProviderTemplate) error {
	if t.UUID == uuid.Nil {
		t.UUID = uuid.New()
	}
	t.CreatedAt = time.Now()
	t.UpdatedAt = time.Now()

	configJSON, err := json.Marshal(&llmProviderTemplateConfig{
		Metadata:         t.Metadata,
		PromptTokens:     t.PromptTokens,
		CompletionTokens: t.CompletionTokens,
		TotalTokens:      t.TotalTokens,
		RemainingTokens:  t.RemainingTokens,
		RequestModel:     t.RequestModel,
		ResponseModel:    t.ResponseModel,
	})
	if err != nil {
		return err
	}

	t.Configuration = string(configJSON)

	return r.db.Create(t).Error
}

// GetByID retrieves an LLM provider template by ID (handle)
func (r *LLMProviderTemplateRepo) GetByHandle(templateHandle, orgUUID string) (*models.LLMProviderTemplate, error) {
	var template models.LLMProviderTemplate
	err := r.db.Where("handle = ? AND organization_uuid = ?", templateHandle, orgUUID).
		First(&template).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}

	if err := unmarshalConfig(&template); err != nil {
		return nil, err
	}

	return &template, nil
}

// GetByUUID retrieves an LLM provider template by UUID
func (r *LLMProviderTemplateRepo) GetByUUID(uuid, orgUUID string) (*models.LLMProviderTemplate, error) {
	var template models.LLMProviderTemplate
	err := r.db.Where("uuid = ? AND organization_uuid = ?", uuid, orgUUID).
		First(&template).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}

	if err := unmarshalConfig(&template); err != nil {
		return nil, err
	}

	return &template, nil
}

// List retrieves LLM provider templates with pagination
func (r *LLMProviderTemplateRepo) List(orgUUID string, limit, offset int) ([]*models.LLMProviderTemplate, error) {
	var templates []*models.LLMProviderTemplate
	err := r.db.Where("organization_uuid = ?", orgUUID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&templates).Error
	if err != nil {
		return nil, err
	}

	// Unmarshal configuration for each template
	for _, template := range templates {
		if err := unmarshalConfig(template); err != nil {
			return nil, err
		}
	}

	return templates, nil
}

// Count counts LLM provider templates for an organization
func (r *LLMProviderTemplateRepo) Count(orgUUID string) (int, error) {
	var count int64
	err := r.db.Model(&models.LLMProviderTemplate{}).
		Where("organization_uuid = ?", orgUUID).
		Count(&count).Error
	return int(count), err
}

// Update modifies an existing LLM provider template
func (r *LLMProviderTemplateRepo) Update(t *models.LLMProviderTemplate) error {
	t.UpdatedAt = time.Now()
	result := r.db.Model(&models.LLMProviderTemplate{}).
		Where("handle = ? AND organization_uuid = ?", t.Handle, t.OrganizationUUID).
		Updates(map[string]interface{}{
			"name":          t.Name,
			"description":   t.Description,
			"configuration": t.Configuration,
			"updated_at":    t.UpdatedAt,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Delete removes an LLM provider template
func (r *LLMProviderTemplateRepo) Delete(templateID, orgUUID string) error {
	result := r.db.Where("handle = ? AND organization_uuid = ?", templateID, orgUUID).
		Delete(&models.LLMProviderTemplate{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Exists checks if an LLM provider template exists
func (r *LLMProviderTemplateRepo) Exists(templateID, orgUUID string) (bool, error) {
	var count int64
	err := r.db.Model(&models.LLMProviderTemplate{}).
		Where("handle = ? AND organization_uuid = ?", templateID, orgUUID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

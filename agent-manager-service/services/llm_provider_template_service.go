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

package services

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// LLMProviderTemplateService handles LLM provider template business logic
type LLMProviderTemplateService struct {
	templateRepo repositories.LLMProviderTemplateRepository
}

// NewLLMProviderTemplateService creates a new LLM provider template service
func NewLLMProviderTemplateService(templateRepo repositories.LLMProviderTemplateRepository) *LLMProviderTemplateService {
	return &LLMProviderTemplateService{
		templateRepo: templateRepo,
	}
}

// Create creates a new LLM provider template
func (s *LLMProviderTemplateService) Create(orgID, createdBy string, template *models.LLMProviderTemplate) (*models.LLMProviderTemplate, error) {
	if template == nil {
		return nil, utils.ErrInvalidInput
	}
	if template.Handle == "" || template.Name == "" {
		return nil, utils.ErrInvalidInput
	}

	// Check if template already exists
	exists, err := s.templateRepo.Exists(template.Handle, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to check template exists: %w", err)
	}
	if exists {
		return nil, utils.ErrLLMProviderTemplateExists
	}

	// Parse organization UUID
	orgUUID, err := uuid.Parse(orgID)
	if err != nil {
		return nil, fmt.Errorf("invalid organization UUID: %w", err)
	}

	// Set metadata
	template.OrganizationUUID = orgUUID
	template.CreatedBy = createdBy

	// Serialize configuration
	if err := s.serializeConfiguration(template); err != nil {
		return nil, fmt.Errorf("failed to serialize configuration: %w", err)
	}

	// Create template
	if err := s.templateRepo.Create(template); err != nil {
		return nil, fmt.Errorf("failed to create template: %w", err)
	}

	// Fetch created template
	created, err := s.templateRepo.GetByHandle(template.Handle, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch created template: %w", err)
	}
	if created == nil {
		return nil, utils.ErrLLMProviderTemplateNotFound
	}

	// Deserialize configuration
	if err := s.deserializeConfiguration(created); err != nil {
		return nil, fmt.Errorf("failed to deserialize configuration: %w", err)
	}

	return created, nil
}

// List lists all LLM provider templates for an organization
func (s *LLMProviderTemplateService) List(orgID string, limit, offset int) ([]*models.LLMProviderTemplate, int, error) {
	templates, err := s.templateRepo.List(orgID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list templates: %w", err)
	}

	// Deserialize configuration for each template
	for _, t := range templates {
		if err := s.deserializeConfiguration(t); err != nil {
			return nil, 0, fmt.Errorf("failed to deserialize configuration: %w", err)
		}
	}

	totalCount, err := s.templateRepo.Count(orgID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count templates: %w", err)
	}

	return templates, totalCount, nil
}

// Get retrieves an LLM provider template by ID
func (s *LLMProviderTemplateService) Get(orgID, templateID string) (*models.LLMProviderTemplate, error) {
	if templateID == "" {
		return nil, utils.ErrInvalidInput
	}

	template, err := s.templateRepo.GetByHandle(templateID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get template: %w", err)
	}
	if template == nil {
		return nil, utils.ErrLLMProviderTemplateNotFound
	}

	// Deserialize configuration
	if err := s.deserializeConfiguration(template); err != nil {
		return nil, fmt.Errorf("failed to deserialize configuration: %w", err)
	}

	return template, nil
}

// Update updates an existing LLM provider template
func (s *LLMProviderTemplateService) Update(orgID, templateID string, updates *models.LLMProviderTemplate) (*models.LLMProviderTemplate, error) {
	if templateID == "" || updates == nil {
		return nil, utils.ErrInvalidInput
	}
	if updates.Name == "" {
		return nil, utils.ErrInvalidInput
	}

	// Parse organization UUID
	orgUUID, err := uuid.Parse(orgID)
	if err != nil {
		return nil, fmt.Errorf("invalid organization UUID: %w", err)
	}

	// Set metadata for update
	updates.Handle = templateID
	updates.OrganizationUUID = orgUUID

	// Serialize configuration
	if err := s.serializeConfiguration(updates); err != nil {
		return nil, fmt.Errorf("failed to serialize configuration: %w", err)
	}

	// Update template
	if err := s.templateRepo.Update(updates); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrLLMProviderTemplateNotFound
		}
		return nil, fmt.Errorf("failed to update template: %w", err)
	}

	// Fetch updated template
	updated, err := s.templateRepo.GetByHandle(templateID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated template: %w", err)
	}
	if updated == nil {
		return nil, utils.ErrLLMProviderTemplateNotFound
	}

	// Deserialize configuration
	if err := s.deserializeConfiguration(updated); err != nil {
		return nil, fmt.Errorf("failed to deserialize configuration: %w", err)
	}

	return updated, nil
}

// Delete deletes an LLM provider template
func (s *LLMProviderTemplateService) Delete(orgID, templateID string) error {
	if templateID == "" {
		return utils.ErrInvalidInput
	}

	if err := s.templateRepo.Delete(templateID, orgID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrLLMProviderTemplateNotFound
		}
		return fmt.Errorf("failed to delete template: %w", err)
	}

	return nil
}

// serializeConfiguration serializes the template configuration fields into the Configuration JSON field
func (s *LLMProviderTemplateService) serializeConfiguration(template *models.LLMProviderTemplate) error {
	config := map[string]interface{}{}

	if template.Metadata != nil {
		config["metadata"] = template.Metadata
	}
	if template.PromptTokens != nil {
		config["promptTokens"] = template.PromptTokens
	}
	if template.CompletionTokens != nil {
		config["completionTokens"] = template.CompletionTokens
	}
	if template.TotalTokens != nil {
		config["totalTokens"] = template.TotalTokens
	}
	if template.RemainingTokens != nil {
		config["remainingTokens"] = template.RemainingTokens
	}
	if template.RequestModel != nil {
		config["requestModel"] = template.RequestModel
	}
	if template.ResponseModel != nil {
		config["responseModel"] = template.ResponseModel
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return err
	}

	template.Configuration = string(configJSON)
	return nil
}

// deserializeConfiguration deserializes the Configuration JSON field into the template configuration fields
func (s *LLMProviderTemplateService) deserializeConfiguration(template *models.LLMProviderTemplate) error {
	if template.Configuration == "" {
		return nil
	}

	var config struct {
		Metadata         *models.LLMProviderTemplateMetadata `json:"metadata,omitempty"`
		PromptTokens     *models.ExtractionIdentifier        `json:"promptTokens,omitempty"`
		CompletionTokens *models.ExtractionIdentifier        `json:"completionTokens,omitempty"`
		TotalTokens      *models.ExtractionIdentifier        `json:"totalTokens,omitempty"`
		RemainingTokens  *models.ExtractionIdentifier        `json:"remainingTokens,omitempty"`
		RequestModel     *models.ExtractionIdentifier        `json:"requestModel,omitempty"`
		ResponseModel    *models.ExtractionIdentifier        `json:"responseModel,omitempty"`
	}

	if err := json.Unmarshal([]byte(template.Configuration), &config); err != nil {
		return err
	}

	template.Metadata = config.Metadata
	template.PromptTokens = config.PromptTokens
	template.CompletionTokens = config.CompletionTokens
	template.TotalTokens = config.TotalTokens
	template.RemainingTokens = config.RemainingTokens
	template.RequestModel = config.RequestModel
	template.ResponseModel = config.ResponseModel

	return nil
}

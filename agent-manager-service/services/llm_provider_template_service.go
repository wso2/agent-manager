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

	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// LLMProviderTemplateService handles LLM provider template business logic
type LLMProviderTemplateService struct {
	templateRepo  repositories.LLMProviderTemplateRepository
	templateStore *LLMTemplateStore
}

// NewLLMProviderTemplateService creates a new LLM provider template service
func NewLLMProviderTemplateService(
	templateRepo repositories.LLMProviderTemplateRepository,
	templateStore *LLMTemplateStore,
) *LLMProviderTemplateService {
	return &LLMProviderTemplateService{
		templateRepo:  templateRepo,
		templateStore: templateStore,
	}
}

// Create creates a new LLM provider template
func (s *LLMProviderTemplateService) Create(orgName, createdBy string, template *models.LLMProviderTemplate) (*models.LLMProviderTemplate, error) {
	if template == nil {
		return nil, utils.ErrInvalidInput
	}
	if template.Handle == "" || template.Name == "" {
		return nil, utils.ErrInvalidInput
	}

	// Validate template handle format
	if err := utils.ValidateTemplateHandle(template.Handle); err != nil {
		return nil, fmt.Errorf("%w: %v", utils.ErrInvalidInput, err)
	}

	// Check if handle conflicts with built-in template
	if s.templateStore.Exists(template.Handle) {
		return nil, utils.ErrSystemTemplateOverride
	}

	// Check if template already exists
	exists, err := s.templateRepo.Exists(template.Handle, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to check template exists: %w", err)
	}
	if exists {
		return nil, utils.ErrLLMProviderTemplateExists
	}

	// Set metadata - user templates are never system templates
	template.OrganizationName = orgName
	template.CreatedBy = createdBy
	template.IsSystem = false

	// Serialize configuration
	if err := s.serializeConfiguration(template); err != nil {
		return nil, fmt.Errorf("failed to serialize configuration: %w", err)
	}

	// Create template
	if err := s.templateRepo.Create(template); err != nil {
		return nil, fmt.Errorf("failed to create template: %w", err)
	}

	// Fetch created template
	created, err := s.templateRepo.GetByHandle(template.Handle, orgName)
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

// List lists all LLM provider templates for an organization (built-in + user-defined)
func (s *LLMProviderTemplateService) List(orgName string, limit, offset int) ([]*models.LLMProviderTemplate, int, error) {
	// Get built-in templates from in-memory store
	builtInTemplates := s.templateStore.List()

	// Get user templates from database
	userTemplates, err := s.templateRepo.List(orgName, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list user templates: %w", err)
	}

	// Deserialize configuration for each user template
	for _, t := range userTemplates {
		if err := s.deserializeConfiguration(t); err != nil {
			return nil, 0, fmt.Errorf("failed to deserialize configuration: %w", err)
		}
	}

	// Merge built-in and user templates
	// Built-in templates first, then user templates
	allTemplates := make([]*models.LLMProviderTemplate, 0, len(builtInTemplates)+len(userTemplates))
	allTemplates = append(allTemplates, builtInTemplates...)
	allTemplates = append(allTemplates, userTemplates...)

	// Total count is built-in + user templates
	userCount, err := s.templateRepo.Count(orgName)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count user templates: %w", err)
	}
	totalCount := s.templateStore.Count() + userCount

	return allTemplates, totalCount, nil
}

// Get retrieves an LLM provider template by ID (checks built-in first, then user templates)
func (s *LLMProviderTemplateService) Get(orgName, templateID string) (*models.LLMProviderTemplate, error) {
	if templateID == "" {
		return nil, utils.ErrInvalidInput
	}

	// Validate template handle format
	if err := utils.ValidateTemplateHandle(templateID); err != nil {
		return nil, fmt.Errorf("%w: %v", utils.ErrInvalidInput, err)
	}

	// First check built-in templates
	if builtInTemplate := s.templateStore.Get(templateID); builtInTemplate != nil {
		return builtInTemplate, nil
	}

	// Then check user templates in database
	template, err := s.templateRepo.GetByHandle(templateID, orgName)
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
func (s *LLMProviderTemplateService) Update(orgName, templateID string, updates *models.LLMProviderTemplate) (*models.LLMProviderTemplate, error) {
	if templateID == "" || updates == nil {
		return nil, utils.ErrInvalidInput
	}
	if updates.Name == "" {
		return nil, utils.ErrInvalidInput
	}

	// Validate template handle format
	if err := utils.ValidateTemplateHandle(templateID); err != nil {
		return nil, fmt.Errorf("%w: %v", utils.ErrInvalidInput, err)
	}

	// Check if this is a system template (cannot be modified)
	if s.templateStore.Exists(templateID) {
		return nil, utils.ErrSystemTemplateImmutable
	}

	// Set metadata for update
	updates.Handle = templateID
	updates.OrganizationName = orgName
	updates.IsSystem = false // Ensure user templates remain non-system

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
	updated, err := s.templateRepo.GetByHandle(templateID, orgName)
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
func (s *LLMProviderTemplateService) Delete(orgName, templateID string) error {
	if templateID == "" {
		return utils.ErrInvalidInput
	}

	// Validate template handle format
	if err := utils.ValidateTemplateHandle(templateID); err != nil {
		return fmt.Errorf("%w: %v", utils.ErrInvalidInput, err)
	}

	// Check if this is a system template (cannot be deleted)
	if s.templateStore.Exists(templateID) {
		return utils.ErrSystemTemplateImmutable
	}

	if err := s.templateRepo.Delete(templateID, orgName); err != nil {
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

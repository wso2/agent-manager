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
	"fmt"

	"github.com/google/uuid"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
)

// LLMTemplateSeeder handles seeding of LLM provider templates
// Seeding is idempotent: existing templates are not overwritten
type LLMTemplateSeeder struct {
	templateRepo repositories.LLMProviderTemplateRepository
	templates    []*models.LLMProviderTemplate
}

// NewLLMTemplateSeeder creates a new LLM template seeder
func NewLLMTemplateSeeder(templateRepo repositories.LLMProviderTemplateRepository, templates []*models.LLMProviderTemplate) *LLMTemplateSeeder {
	return &LLMTemplateSeeder{
		templateRepo: templateRepo,
		templates:    templates,
	}
}

// SetTemplates sets the templates to seed
func (s *LLMTemplateSeeder) SetTemplates(templates []*models.LLMProviderTemplate) {
	if s != nil {
		s.templates = templates
	}
}

// SeedForOrg seeds default LLM provider templates for a specific organization
func (s *LLMTemplateSeeder) SeedForOrg(orgUUID uuid.UUID) error {
	if s == nil || s.templateRepo == nil {
		return nil
	}
	if orgUUID == uuid.Nil {
		return fmt.Errorf("orgUUID is empty")
	}
	if len(s.templates) == 0 {
		return nil
	}

	orgUUIDStr := orgUUID.String()

	// Get count of existing templates
	totalCount, err := s.templateRepo.Count(orgUUIDStr)
	if err != nil {
		return fmt.Errorf("failed to count existing templates: %w", err)
	}

	// Get all existing templates for this organization
	existing, err := s.templateRepo.List(orgUUIDStr, totalCount, 0)
	if err != nil {
		return fmt.Errorf("failed to list existing templates: %w", err)
	}

	existingByHandle := make(map[string]*models.LLMProviderTemplate, len(existing))
	for _, t := range existing {
		if t == nil {
			continue
		}
		existingByHandle[t.Handle] = t
	}

	// Seed each template
	for _, tpl := range s.templates {
		fmt.Println(tpl)
		if tpl == nil || tpl.Handle == "" {
			continue
		}

		// Check if template already exists
		if current, ok := existingByHandle[tpl.Handle]; ok {
			// Update metadata if needed
			updated := false
			if current.Metadata == nil && tpl.Metadata != nil {
				current.Metadata = tpl.Metadata
				updated = true
			} else if current.Metadata != nil && tpl.Metadata != nil {
				if current.Metadata.OpenapiSpecURL == "" && tpl.Metadata.OpenapiSpecURL != "" {
					current.Metadata.OpenapiSpecURL = tpl.Metadata.OpenapiSpecURL
					updated = true
				}
			}
			if current.Name == "" && tpl.Name != "" {
				current.Name = tpl.Name
				updated = true
			}
			if updated {
				if err := s.templateRepo.Update(current); err != nil {
					return fmt.Errorf("failed to update template metadata for %s: %w", tpl.Handle, err)
				}
			}
			continue
		}

		// Create new template
		toCreate := &models.LLMProviderTemplate{
			OrganizationUUID: orgUUID,
			Handle:           tpl.Handle,
			Name:             tpl.Name,
			Description:      tpl.Description,
			CreatedBy:        tpl.CreatedBy,
			Metadata:         tpl.Metadata,
			PromptTokens:     tpl.PromptTokens,
			CompletionTokens: tpl.CompletionTokens,
			TotalTokens:      tpl.TotalTokens,
			RemainingTokens:  tpl.RemainingTokens,
			RequestModel:     tpl.RequestModel,
			ResponseModel:    tpl.ResponseModel,
			Configuration:    tpl.Configuration,
		}
		fmt.Println("to create", toCreate)
		if err := s.templateRepo.Create(toCreate); err != nil {
			// Be tolerant to concurrent startup / repeated seeding
			exists, existsErr := s.templateRepo.Exists(tpl.Handle, orgUUIDStr)
			if existsErr == nil && exists {
				continue
			}
			return fmt.Errorf("failed to create template %s: %w", tpl.Handle, err)
		}
	}

	return nil
}

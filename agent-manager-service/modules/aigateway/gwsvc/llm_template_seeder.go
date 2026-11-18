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

package gwsvc

import (
	"fmt"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
)

// LLMTemplateSeeder handles seeding of LLM provider templates
// Seeding is idempotent: existing templates are not overwritten
type LLMTemplateSeeder struct {
	templateRepo gwrepo.LLMProviderTemplateRepository
	templates    []*gwmodels.LLMProviderTemplate
}

// NewLLMTemplateSeeder creates a new LLM template seeder
func NewLLMTemplateSeeder(templateRepo gwrepo.LLMProviderTemplateRepository, templates []*gwmodels.LLMProviderTemplate) *LLMTemplateSeeder {
	return &LLMTemplateSeeder{
		templateRepo: templateRepo,
		templates:    templates,
	}
}

// SetTemplates sets the templates to seed
func (s *LLMTemplateSeeder) SetTemplates(templates []*gwmodels.LLMProviderTemplate) {
	if s != nil {
		s.templates = templates
	}
}

// SeedForOrg seeds default LLM provider templates for a specific organization
func (s *LLMTemplateSeeder) SeedForOrg(ouID string) error {
	if s == nil || s.templateRepo == nil {
		return nil
	}

	if len(s.templates) == 0 {
		return nil
	}

	// Get count of existing templates
	totalCount, err := s.templateRepo.Count(ouID)
	if err != nil {
		return fmt.Errorf("failed to count existing templates: %w", err)
	}

	// Get all existing templates for this organization
	existing, err := s.templateRepo.List(ouID, totalCount, 0)
	if err != nil {
		return fmt.Errorf("failed to list existing templates: %w", err)
	}

	existingByHandle := make(map[string]*gwmodels.LLMProviderTemplate, len(existing))
	for _, t := range existing {
		if t == nil {
			continue
		}
		existingByHandle[t.Handle] = t
	}

	// Seed each template
	for _, tpl := range s.templates {
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
				// Backfill only when absent, so an operator-set auth block survives re-seeding.
				if current.Metadata.Auth == nil && tpl.Metadata.Auth != nil {
					current.Metadata.Auth = tpl.Metadata.Auth
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
		toCreate := &gwmodels.LLMProviderTemplate{
			OUID:             ouID,
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

		if err := s.templateRepo.Create(toCreate); err != nil {
			// Be tolerant to concurrent startup / repeated seeding
			exists, existsErr := s.templateRepo.Exists(tpl.Handle, ouID)
			if existsErr == nil && exists {
				continue
			}
			return fmt.Errorf("failed to create template %s: %w", tpl.Handle, err)
		}
	}

	return nil
}

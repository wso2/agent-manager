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
	"sync"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// LLMTemplateStore provides thread-safe in-memory storage for built-in LLM provider templates
type LLMTemplateStore struct {
	templates map[string]*models.LLMProviderTemplate // key: handle
	mu        sync.RWMutex
}

// NewLLMTemplateStore creates a new in-memory template store
func NewLLMTemplateStore() *LLMTemplateStore {
	return &LLMTemplateStore{
		templates: make(map[string]*models.LLMProviderTemplate),
	}
}

// Load loads templates into the store (typically called at startup)
func (s *LLMTemplateStore) Load(templates []*models.LLMProviderTemplate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range templates {
		if t != nil && t.Handle != "" {
			s.templates[t.Handle] = t
		}
	}
}

// Get retrieves a template by handle (returns nil if not found)
// Returns a deep copy to prevent mutation of shared in-memory templates
func (s *LLMTemplateStore) Get(handle string) *models.LLMProviderTemplate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	template := s.templates[handle]
	if template == nil {
		return nil
	}
	// Return a deep copy to prevent external mutations
	return s.copyTemplate(template)
}

// List returns all templates in the store
// Returns deep copies to prevent mutation of shared in-memory templates
func (s *LLMTemplateStore) List() []*models.LLMProviderTemplate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*models.LLMProviderTemplate, 0, len(s.templates))
	for _, t := range s.templates {
		// Create a deep copy of the template to prevent external mutations
		copy := s.copyTemplate(t)
		result = append(result, copy)
	}
	return result
}

// Exists checks if a template with the given handle exists
func (s *LLMTemplateStore) Exists(handle string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.templates[handle]
	return ok
}

// Count returns the number of templates in the store
func (s *LLMTemplateStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.templates)
}

// copyTemplate creates a deep copy of a template to prevent external mutations
// Note: This assumes the template's nested fields (Metadata, ExtractionIdentifier, etc.)
// are value types or are not modified after creation. For truly deep copy of nested
// pointers, additional copying would be needed.
func (s *LLMTemplateStore) copyTemplate(t *models.LLMProviderTemplate) *models.LLMProviderTemplate {
	if t == nil {
		return nil
	}

	// Create a shallow copy of the template struct
	copy := *t

	// Deep copy nested pointer fields if they exist
	if t.Metadata != nil {
		metadataCopy := *t.Metadata
		copy.Metadata = &metadataCopy
	}
	if t.PromptTokens != nil {
		promptTokensCopy := *t.PromptTokens
		copy.PromptTokens = &promptTokensCopy
	}
	if t.CompletionTokens != nil {
		completionTokensCopy := *t.CompletionTokens
		copy.CompletionTokens = &completionTokensCopy
	}
	if t.TotalTokens != nil {
		totalTokensCopy := *t.TotalTokens
		copy.TotalTokens = &totalTokensCopy
	}
	if t.RemainingTokens != nil {
		remainingTokensCopy := *t.RemainingTokens
		copy.RemainingTokens = &remainingTokensCopy
	}
	if t.RequestModel != nil {
		requestModelCopy := *t.RequestModel
		copy.RequestModel = &requestModelCopy
	}
	if t.ResponseModel != nil {
		responseModelCopy := *t.ResponseModel
		copy.ResponseModel = &responseModelCopy
	}

	return &copy
}

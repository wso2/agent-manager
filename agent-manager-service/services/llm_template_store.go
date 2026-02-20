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
func (s *LLMTemplateStore) Get(handle string) *models.LLMProviderTemplate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.templates[handle]
}

// List returns all templates in the store
func (s *LLMTemplateStore) List() []*models.LLMProviderTemplate {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*models.LLMProviderTemplate, 0, len(s.templates))
	for _, t := range s.templates {
		result = append(result, t)
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

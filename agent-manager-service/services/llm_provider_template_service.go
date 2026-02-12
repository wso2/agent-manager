/*
 *  Copyright (c) 2026, WSO2 LLC. (http://www.wso2.org) All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package services

import (
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
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

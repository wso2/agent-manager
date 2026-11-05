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

package handlers

import (
	"context"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

type LLMProviderHandler struct {
	providerSvc *services.LLMProviderService
	configSvc   services.AgentConfigurationService
}

func NewLLMProviderHandler(providerSvc *services.LLMProviderService, configSvc services.AgentConfigurationService) *LLMProviderHandler {
	return &LLMProviderHandler{providerSvc: providerSvc, configSvc: configSvc}
}

func (h *LLMProviderHandler) ListLLMProviders(_ context.Context, ouID string, limit int, offset int) ([]*models.LLMProvider, int, error) {
	return h.providerSvc.List(ouID, limit, offset)
}

// AddLLMProviderToCatalog accepts a handle or a UUID. The catalog update takes
// only the UUID, so the provider is resolved first; that also skips the write
// when the provider is already in the catalog.
func (h *LLMProviderHandler) AddLLMProviderToCatalog(_ context.Context, ouID string, providerID string) (*models.LLMProvider, bool, error) {
	provider, err := h.providerSvc.Get(providerID, ouID)
	if err != nil {
		return nil, false, err
	}
	if provider.InCatalog {
		return provider, false, nil
	}
	updated, err := h.providerSvc.UpdateCatalogStatus(provider.UUID.String(), ouID, true)
	if err != nil {
		return nil, false, err
	}
	return updated, true, nil
}

func (h *LLMProviderHandler) AttachLLMProvider(ctx context.Context, ouID string, projectName string, agentName string, req models.CreateAgentModelConfigRequest) (*models.AgentModelConfigResponse, error) {
	return h.configSvc.Create(ctx, ouID, projectName, agentName, req, jwtassertion.CallerSubject(ctx))
}

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
	"fmt"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

type AgentConfigHandler struct {
	configSvc services.AgentConfigurationService
}

func NewAgentConfigHandler(configSvc services.AgentConfigurationService) *AgentConfigHandler {
	return &AgentConfigHandler{configSvc: configSvc}
}

// CreateAgentConfig records the token subject as the creator, as the REST
// controller does, so configs made over MCP are attributed to the caller.
func (h *AgentConfigHandler) CreateAgentConfig(ctx context.Context, ouID string, projectName string, agentName string, req models.CreateAgentModelConfigRequest) (*models.AgentModelConfigResponse, error) {
	createdBy := jwtassertion.CallerSubject(ctx)
	if createdBy == "" {
		createdBy = "system"
	}
	return h.configSvc.Create(ctx, ouID, projectName, agentName, req, createdBy)
}

// RevokeConfigAPIKeys revokes every API key of an external agent's LLM or MCP
// configuration in envName, as the console's delete does, so a key nobody holds
// is not left valid. The console's Generate then issues one shown once.
func (h *AgentConfigHandler) RevokeConfigAPIKeys(ctx context.Context, ouID, projectName, agentName, configType, configID, envName string) error {
	id, err := uuid.Parse(configID)
	if err != nil {
		return fmt.Errorf("invalid configuration id %q: %w", configID, err)
	}
	list, revoke := h.configSvc.ListLLMConfigAPIKeys, h.configSvc.RevokeLLMConfigAPIKey
	if configType == models.AgentConfigTypeMCP {
		list, revoke = h.configSvc.ListMCPConfigAPIKeys, h.configSvc.RevokeMCPConfigAPIKey
	}
	keys, err := list(ctx, ouID, projectName, agentName, id, envName)
	if err != nil {
		return err
	}
	if keys == nil {
		return nil
	}
	for _, key := range keys.Keys {
		if err := revoke(ctx, ouID, projectName, agentName, id, envName, key.Name); err != nil {
			return err
		}
	}
	return nil
}

func (h *AgentConfigHandler) ProviderEnvironments(ctx context.Context, ouID string, projectName string, agentName string, providerHandle string) ([]string, error) {
	return h.configSvc.ProviderEnvironments(ctx, ouID, projectName, agentName, providerHandle)
}

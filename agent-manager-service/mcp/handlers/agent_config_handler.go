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

type AgentConfigHandler struct {
	configSvc services.AgentConfigurationService
}

func NewAgentConfigHandler(configSvc services.AgentConfigurationService) *AgentConfigHandler {
	return &AgentConfigHandler{configSvc: configSvc}
}

// CreateAgentConfig records the token subject as the creator, as the REST
// controller does, so configs made over MCP are attributed to the caller.
func (h *AgentConfigHandler) CreateAgentConfig(ctx context.Context, ouID string, projectName string, agentName string, req models.CreateAgentModelConfigRequest) (*models.AgentModelConfigResponse, error) {
	createdBy := "system"
	if claims := jwtassertion.GetTokenClaims(ctx); claims != nil && claims.Sub != "" {
		createdBy = claims.Sub
	}
	return h.configSvc.Create(ctx, ouID, projectName, agentName, req, createdBy)
}

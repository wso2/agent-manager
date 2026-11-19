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

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

type MCPProxyHandler struct {
	proxySvc   *services.MCPProxyService
	projectSvc *services.ProjectMCPProxyService
	infraSvc   services.InfraResourceManager
}

func NewMCPProxyHandler(
	proxySvc *services.MCPProxyService, projectSvc *services.ProjectMCPProxyService, infraSvc services.InfraResourceManager,
) *MCPProxyHandler {
	return &MCPProxyHandler{proxySvc: proxySvc, projectSvc: projectSvc, infraSvc: infraSvc}
}

// ListMCPProxies lists the org-level proxies, and with a projectName that project's proxies too, as the agent
// Tools picker does.
func (h *MCPProxyHandler) ListMCPProxies(
	ctx context.Context, ouID string, projectName string, limit int, offset int,
) (*models.MCPProxyListResponse, error) {
	var projectUUID *string
	if projectName != "" {
		project, err := h.infraSvc.GetProject(ctx, ouID, projectName)
		if err != nil {
			return nil, err
		}
		projectUUID = &project.UUID
	}
	return h.proxySvc.List(ctx, ouID, projectUUID, limit, offset)
}

func (h *MCPProxyHandler) GetMCPProxy(ctx context.Context, ouID string, proxyID string) (*models.MCPProxyDTO, error) {
	return h.proxySvc.Get(ctx, ouID, proxyID)
}

// GetProjectMCPProxy returns a project proxy with its environment bindings, which carry its tools.
func (h *MCPProxyHandler) GetProjectMCPProxy(
	ctx context.Context, ouID string, projectName string, proxyID string,
) (*models.ProjectMCPProxyResponse, error) {
	project, err := h.infraSvc.GetProject(ctx, ouID, projectName)
	if err != nil {
		return nil, err
	}
	return h.projectSvc.Get(ctx, ouID, projectName, project.UUID, proxyID)
}

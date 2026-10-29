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
	proxySvc *services.MCPProxyService
}

func NewMCPProxyHandler(proxySvc *services.MCPProxyService) *MCPProxyHandler {
	return &MCPProxyHandler{proxySvc: proxySvc}
}

func (h *MCPProxyHandler) ListMCPProxies(ctx context.Context, ouID string, limit int, offset int) (*models.MCPProxyListResponse, error) {
	return h.proxySvc.List(ctx, ouID, nil, limit, offset)
}

func (h *MCPProxyHandler) FetchMCPServerInfo(ctx context.Context, req *models.MCPServerInfoFetchRequest) (*models.MCPServerInfoFetchResponse, error) {
	return h.proxySvc.FetchServerInfo(ctx, req)
}

func (h *MCPProxyHandler) CreateMCPProxy(ctx context.Context, ouID string, req *models.MCPProxyDTO) (*models.MCPProxyDTO, error) {
	return h.proxySvc.Create(ctx, ouID, createdBySystem, req)
}

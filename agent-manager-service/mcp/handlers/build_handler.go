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

type BuildHandler struct {
	agentSvc services.AgentManagerService
}

func NewBuildHandler(agentSvc services.AgentManagerService) *BuildHandler {
	return &BuildHandler{agentSvc: agentSvc}
}

func (h *BuildHandler) ListAgentBuilds(ctx context.Context, orgName string, projectName string, agentName string, limit int32, offset int32) ([]*models.BuildResponse, int32, error) {
	return h.agentSvc.ListAgentBuilds(ctx, orgName, projectName, agentName, limit, offset)
}

func (h *BuildHandler) GetBuild(ctx context.Context, orgName string, projectName string, agentName string, buildName string) (*models.BuildDetailsResponse, error) {
	return h.agentSvc.GetBuild(ctx, orgName, projectName, agentName, buildName)
}

func (h *BuildHandler) BuildAgent(ctx context.Context, orgName string, projectName string, agentName string, commitId string) (*models.BuildResponse, error) {
	return h.agentSvc.BuildAgent(ctx, orgName, projectName, agentName, commitId)
}

func (h *BuildHandler) GetBuildLogs(ctx context.Context, orgName string, projectName string, agentName string, buildName string) (*models.LogsResponse, error) {
	return h.agentSvc.GetBuildLogs(ctx, orgName, projectName, agentName, buildName)
}

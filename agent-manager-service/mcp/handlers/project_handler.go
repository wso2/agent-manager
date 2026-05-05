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
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// ProjectHandler bridges MCP project tools to the infra resource manager service.
type ProjectHandler struct {
	infraSvc services.InfraResourceManager
}

func NewProjectHandler(infraSvc services.InfraResourceManager) *ProjectHandler {
	return &ProjectHandler{infraSvc: infraSvc}
}

func (h *ProjectHandler) ListProjects(ctx context.Context, orgName string, limit int, offset int) ([]*models.ProjectResponse, int32, error) {
	return h.infraSvc.ListProjects(ctx, orgName, limit, offset)
}

func (h *ProjectHandler) CreateProject(ctx context.Context, orgName string, payload spec.CreateProjectRequest) (*models.ProjectResponse, error) {
	return h.infraSvc.CreateProject(ctx, orgName, payload)
}

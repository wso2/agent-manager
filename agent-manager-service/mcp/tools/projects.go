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

package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type listProjectsInput struct {
	OrgName string `json:"org_name"`
	Limit   *int   `json:"limit,omitempty"`
	Offset  *int   `json:"offset,omitempty"`
}
type createProjectInput struct {
	OrgName     string  `json:"org_name"`
	ProjectName string  `json:"project_name"`
	DisplayName string  `json:"display_name"`
	Description *string `json:"description"`
}

type listProjectItem struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}
type listProjectsOutput struct {
	OrgName  string            `json:"org_name"`
	Total    int32             `json:"total"`
	Projects []listProjectItem `json:"projects"`
}

func (t *Toolsets) registerProjectTools(server *gomcp.Server) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name: "list_projects",
		Description: "List projects in an organization. " +
			"A project is a logical container that groups agents and related resources within an organization. " +
			"Supports pagination with `limit` and `offset`.",
		InputSchema: createSchema(map[string]any{
			"org_name": stringProperty("Optional. Organization name."),
			"limit":    intProperty(fmt.Sprintf("Optional. Max projects to return (default %d, min %d, max %d).", utils.DefaultLimit, utils.MinLimit, utils.MaxLimit)),
			"offset":   intProperty(fmt.Sprintf("Optional. Pagination offset (default %d, min %d).", utils.DefaultOffset, utils.MinOffset)),
		}, nil),
	}, withToolLogging("list_projects", listProjects(t.ProjectToolset)))

	gomcp.AddTool(server, &gomcp.Tool{
		Name: "create_project",
		Description: "Create a new project in an organization. " +
			"A project is a logical container for agents and related resources within an organization.",
		InputSchema: createSchema(map[string]any{
			"org_name":     stringProperty("Optional. Organization name."),
			"project_name": stringProperty("Required. Unique name for the project."),
			"display_name": stringProperty("Required. Project display name."),
			"description":  stringProperty("Optional. Project description."),
		}, []string{"project_name", "display_name"}),
	}, withToolLogging("create_project", createProject(t.ProjectToolset)))
}

func listProjects(handler ProjectToolsetHandler) func(context.Context, *gomcp.CallToolRequest, listProjectsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listProjectsInput) (*gomcp.CallToolResult, any, error) {
		orgName := resolveOrgName(input.OrgName)

		// Apply default limit. Validate bounds.
		limit := utils.DefaultLimit
		if input.Limit != nil {
			limit = *input.Limit
		}
		if limit < utils.MinLimit || limit > utils.MaxLimit {
			return nil, nil, fmt.Errorf("limit must be between %d and %d", utils.MinLimit, utils.MaxLimit)
		}
		// Apply default offset. Validate bounds.
		offset := utils.DefaultOffset
		if input.Offset != nil {
			offset = *input.Offset
		}
		if offset < utils.MinOffset {
			return nil, nil, fmt.Errorf("offset must be >= %d", utils.MinOffset)
		}
		// Calls the service-layer interface
		projects, total, err := handler.ListProjects(ctx, orgName, limit, offset)
		if err != nil {
			return nil, nil, wrapToolError("list_projects", err)
		}
		// Format the response recieved from service layer.
		formatted := make([]listProjectItem, 0, len(projects))
		for _, project := range projects {
			if project == nil {
				continue
			}
			formatted = append(formatted, listProjectItem{
				Name:      project.Name,
				CreatedAt: project.CreatedAt,
			})
		}
		response := listProjectsOutput{
			OrgName:  orgName,
			Total:    total,
			Projects: formatted,
		}
		return handleToolResult(response, nil)
	}
}

func createProject(handler ProjectToolsetHandler) func(context.Context, *gomcp.CallToolRequest, createProjectInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input createProjectInput) (*gomcp.CallToolResult, any, error) {
		orgName := resolveOrgName(input.OrgName)
		projectName := strings.TrimSpace(input.ProjectName)
		displayName := strings.TrimSpace(input.DisplayName)

		if projectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if err := utils.ValidateResourceName(projectName, "project"); err != nil {
			return nil, nil, err
		}
		if displayName == "" {
			return nil, nil, fmt.Errorf("display_name is required")
		}
		req := spec.CreateProjectRequest{
			Name:               projectName,
			DisplayName:        displayName,
			DeploymentPipeline: "default",
			Description:        normalizeOptionalString(input.Description),
		}
		project, err := handler.CreateProject(ctx, orgName, req)
		if err != nil {
			return nil, nil, wrapToolError("create_project", err)
		}
		response := map[string]any{
			"org_name": orgName,
			"project":  utils.ConvertToProjectResponse(project),
		}
		return handleToolResult(response, nil)
	}
}

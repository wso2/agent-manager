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
	DisplayName string  `json:"display_name"`
	Description *string `json:"description"`
}

type listProjectItem struct {
	Name      string    `json:"name"`
	OrgName   string    `json:"orgName"`
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
			"A project is a logical container for agents and related resources, and the project name is generated automatically from the display name.",
		InputSchema: createSchema(map[string]any{
			"org_name":     stringProperty("Optional. Organization name."),
			"display_name": stringProperty("Required. Project display name."),
			"description":  stringProperty("Optional. Project description."),
		}, []string{"display_name"}),
	}, withToolLogging("create_project", createProject(t.ProjectToolset)))
}

func listProjects(handler ProjectToolsetHandler) func(context.Context, *gomcp.CallToolRequest, listProjectsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listProjectsInput) (*gomcp.CallToolResult, any, error) {
		orgName := resolveOrgName(input.OrgName)
		if orgName == "" {
			return nil, nil, fmt.Errorf("org_name is required")
		}
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
		if orgName == "" {
			return nil, nil, fmt.Errorf("org_name is required")
		}
		if strings.TrimSpace(input.DisplayName) == "" {
			return nil, nil, fmt.Errorf("display_name is required")
		}
		resourceReq := spec.ResourceNameRequest{
			DisplayName:  strings.TrimSpace(input.DisplayName),
			ResourceType: "project",
		}
		projectName, err := handler.GenerateName(ctx, orgName, resourceReq)
		if err != nil {
			return nil, nil, wrapToolError("create_project", err)
		}
		req := spec.CreateProjectRequest{
			Name:               projectName,
			DisplayName:        strings.TrimSpace(input.DisplayName),
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

package tools

import (
	"context"
	"fmt"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// input structs
type listBuildsInput struct {
	OrgName     string `json:"org_name"`
	ProjectName string `json:"project_name"`
	AgentName   string `json:"agent_name"`
	Limit       *int   `json:"limit,omitempty"`
	Offset      *int   `json:"offset,omitempty"`
}
type getBuildDetailsInput struct {
	OrgName     string `json:"org_name"`
	ProjectName string `json:"project_name"`
	AgentName   string `json:"agent_name"`
	BuildName   string `json:"build_name"`
}
type buildAgentInput struct {
	OrgName     string  `json:"org_name"`
	ProjectName string  `json:"project_name"`
	AgentName   string  `json:"agent_name"`
	CommitID    *string `json:"commit_id,omitempty"`
}
type getBuildLogsInput struct {
	OrgName     string `json:"org_name"`
	ProjectName string `json:"project_name"`
	AgentName   string `json:"agent_name"`
	BuildName   string `json:"build_name"`
}

// output structs
type listBuildItem struct {
	BuildID           string     `json:"build_id"`
	BuildName         string     `json:"build_name"`
	ProjectName       string     `json:"project_name"`
	AgentName         string     `json:"agent_name"`
	StartedAt         time.Time  `json:"started_at"`
	EndedAt           *time.Time `json:"ended_at,omitempty"`
	ImageID           string     `json:"image_id,omitempty"`
	Status            string     `json:"status"`
	RetryAfterSeconds *int       `json:"retry_after_seconds,omitempty"`
}
type listBuildsOutput struct {
	OrgName     string          `json:"org_name"`
	ProjectName string          `json:"project_name"`
	AgentName   string          `json:"agent_name"`
	Builds      []listBuildItem `json:"builds"`
	Total       int32           `json:"total"`
	Limit       int32           `json:"limit"`
	Offset      int32           `json:"offset"`
	Note        string          `json:"note"`
}
type getBuildDetailsOutput struct {
	OrgName           string                    `json:"org_name"`
	ProjectName       string                    `json:"project_name"`
	AgentName         string                    `json:"agent_name"`
	Build             spec.BuildDetailsResponse `json:"build"`
	RetryAfterSeconds *int                      `json:"retry_after_seconds,omitempty"`
	Note              string                    `json:"note,omitempty"`
}
type buildAgentOutput struct {
	OrgName     string             `json:"org_name"`
	ProjectName string             `json:"project_name"`
	AgentName   string             `json:"agent_name"`
	Build       spec.BuildResponse `json:"build"`
	Note        string             `json:"note,omitempty"`
}
type getBuildLogsOutput struct {
	OrgName     string `json:"org_name"`
	ProjectName string `json:"project_name"`
	AgentName   string `json:"agent_name"`
	BuildName   string `json:"build_name"`
	Logs        any    `json:"logs"`
}

func (t *Toolsets) registerBuildTools(server *gomcp.Server) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name: "list_builds",
		Description: "List builds for an agent. " +
			"A build is a versioned packaging job that turns agent source into a runnable image using a specific commit and build parameters. " +
			"Successful builds trigger deployment automatically, and in-progress builds may take a few minutes to complete.",
		InputSchema: createSchema(map[string]any{
			"org_name":     stringProperty("Optional. Organization name."),
			"project_name": stringProperty("Required. Project name where the agent exists."),
			"agent_name":   stringProperty("Required. Agent name to list builds for."),
			"limit":        intProperty(fmt.Sprintf("Optional. Max builds to return (default %d, min %d, max %d).", utils.DefaultLimit, utils.MinLimit, utils.MaxLimit)),
			"offset":       intProperty(fmt.Sprintf("Optional. Pagination offset (default %d, min %d).", utils.DefaultOffset, utils.MinOffset)),
		}, []string{"project_name", "agent_name"}),
	}, withToolLogging("list_builds", listBuilds(t.BuildToolset)))

	gomcp.AddTool(server, &gomcp.Tool{
		Name: "get_build_details",
		Description: "Return detailed information for a specific build, including status, steps, duration, commit, and build parameters. " +
			"If the build is still running, completion may take a few minutes.",
		InputSchema: createSchema(map[string]any{
			"org_name":     stringProperty("Optional. Organization name."),
			"project_name": stringProperty("Required. Project name where the agent exists."),
			"agent_name":   stringProperty("Required. Agent name that owns the build."),
			"build_name":   stringProperty("Required. Build name to fetch details for."),
		}, []string{"project_name", "agent_name", "build_name"}),
	}, withToolLogging("get_build_details", getBuildDetails(t.BuildToolset)))

	gomcp.AddTool(server, &gomcp.Tool{
		Name: "build_agent",
		Description: "Start a new build for an existing agent. " +
			"A build packages the agent source into a runnable image from a specific commit and build parameters. " +
			"Successful builds trigger deployment automatically.",
		InputSchema: createSchema(map[string]any{
			"org_name":     stringProperty("Optional. Organization name."),
			"project_name": stringProperty("Required. Project name where the agent exists."),
			"agent_name":   stringProperty("Required. Agent name to trigger build for."),
			"commit_id":    stringProperty("Optional. Commit ID to build. Defaults to latest."),
		}, []string{"project_name", "agent_name"}),
	}, withToolLogging("build_agent", buildAgent(t.BuildToolset)))

	gomcp.AddTool(server, &gomcp.Tool{
		Name: "get_build_logs",
		Description: "Return logs for a specific build of an internal agent. " +
			"Build logs are the step-by-step output produced while packaging the agent source into a runnable image.",
		InputSchema: createSchema(map[string]any{
			"org_name":     stringProperty("Optional. Organization name."),
			"project_name": stringProperty("Required. Project name where the agent exists."),
			"agent_name":   stringProperty("Required. Agent name that owns the build."),
			"build_name":   stringProperty("Required. Build name to fetch logs for."),
		}, []string{"project_name", "agent_name", "build_name"}),
	}, withToolLogging("get_build_logs", getBuildLogs(t.BuildToolset)))

}

func listBuilds(handler BuildToolsetHandler) func(context.Context, *gomcp.CallToolRequest, listBuildsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listBuildsInput) (*gomcp.CallToolResult, any, error) {
		if input.ProjectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if input.AgentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}
		orgName := resolveOrgName(input.OrgName)

		limit := utils.DefaultLimit
		if input.Limit != nil {
			limit = *input.Limit
		}
		if limit < utils.MinLimit || limit > utils.MaxLimit {
			return nil, nil, fmt.Errorf("limit must be between %d and %d", utils.MinLimit, utils.MaxLimit)
		}

		offset := utils.DefaultOffset
		if input.Offset != nil {
			offset = *input.Offset
		}
		if offset < utils.MinOffset {
			return nil, nil, fmt.Errorf("offset must be >= %d", utils.MinOffset)
		}

		builds, total, err := handler.ListAgentBuilds(ctx, orgName, input.ProjectName, input.AgentName, int32(limit), int32(offset))
		if err != nil {
			return nil, nil, wrapToolError("list_builds", err)
		}

		response := listBuildsOutput{
			OrgName:     orgName,
			ProjectName: input.ProjectName,
			AgentName:   input.AgentName,
			Builds:      reduceBuildListResponse(builds),
			Total:       total,
			Limit:       int32(limit),
			Offset:      int32(offset),
			Note:        "If a build completes successfully, deployment is triggered automatically. No need to trigger deployment separately.",
		}

		return handleToolResult(response, nil)
	}
}

func getBuildDetails(handler BuildToolsetHandler) func(context.Context, *gomcp.CallToolRequest, getBuildDetailsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getBuildDetailsInput) (*gomcp.CallToolResult, any, error) {
		if input.ProjectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if input.AgentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}
		if input.BuildName == "" {
			return nil, nil, fmt.Errorf("build_name is required")
		}

		orgName := resolveOrgName(input.OrgName)

		result, err := handler.GetBuild(ctx, orgName, input.ProjectName, input.AgentName, input.BuildName)
		if err != nil {
			return nil, nil, wrapToolError("get_build_details", err)
		}

		response := getBuildDetailsOutput{
			OrgName:     orgName,
			ProjectName: input.ProjectName,
			AgentName:   input.AgentName,
			Build:       utils.ConvertToBuildDetailsResponse(result),
		}
		if result != nil && isBuildInProgress(result.Status) {
			retry := buildRetryAfterSeconds
			response.RetryAfterSeconds = &retry
			response.Note = "Build is still in progress. Wait a couple of minutes before checking again."
		}
		return handleToolResult(response, nil)
	}
}

func buildAgent(handler BuildToolsetHandler) func(context.Context, *gomcp.CallToolRequest, buildAgentInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input buildAgentInput) (*gomcp.CallToolResult, any, error) {
		if input.ProjectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if input.AgentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}
		orgName := resolveOrgName(input.OrgName)
		commitID := ""
		if input.CommitID != nil {
			commitID = *input.CommitID
		}
		build, err := handler.BuildAgent(ctx, orgName, input.ProjectName, input.AgentName, commitID)
		if err != nil {
			return nil, nil, wrapToolError("build_agent", err)
		}
		response := buildAgentOutput{
			OrgName:     orgName,
			ProjectName: input.ProjectName,
			AgentName:   input.AgentName,
			Build:       utils.ConvertToBuildResponse(build),
			Note:        "Build started. Use get_build_details or get_build_logs to track progress.",
		}
		return handleToolResult(response, nil)
	}
}

func getBuildLogs(handler BuildToolsetHandler) func(context.Context, *gomcp.CallToolRequest, getBuildLogsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getBuildLogsInput) (*gomcp.CallToolResult, any, error) {
		if input.ProjectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if input.AgentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}
		if input.BuildName == "" {
			return nil, nil, fmt.Errorf("build_name is required")
		}
		orgName := resolveOrgName(input.OrgName)

		result, err := handler.GetBuildLogs(ctx, orgName, input.ProjectName, input.AgentName, input.BuildName)
		if err != nil {
			return nil, nil, wrapToolError("get_build_logs", err)
		}

		response := getBuildLogsOutput{
			OrgName:     orgName,
			ProjectName: input.ProjectName,
			AgentName:   input.AgentName,
			BuildName:   input.BuildName,
			Logs:        reduceLogsResponse(result),
		}
		return handleToolResult(response, nil)
	}
}

// helpers
const buildRetryAfterSeconds = 120

func reduceBuildListResponse(builds []*models.BuildResponse) []listBuildItem {
	if len(builds) == 0 {
		return []listBuildItem{}
	}
	out := make([]listBuildItem, 0, len(builds))
	for _, build := range builds {
		if build == nil {
			continue
		}
		item := listBuildItem{
			BuildID:     build.UUID,
			BuildName:   build.Name,
			ProjectName: build.ProjectName,
			AgentName:   build.AgentName,
			StartedAt:   build.StartedAt,
			EndedAt:     build.EndedAt,
			ImageID:     build.ImageId,
			Status:      build.Status,
		}
		if isBuildInProgress(build.Status) {
			retry := buildRetryAfterSeconds
			item.RetryAfterSeconds = &retry
		}
		out = append(out, item)
	}
	return out
}

func isBuildInProgress(status string) bool {
	switch status {
	case "BuildInitiated", "BuildTriggered", "BuildRunning", "BuildCompleted":
		return true
	default:
		return false
	}
}

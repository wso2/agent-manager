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
		projectName := strings.TrimSpace(input.ProjectName)
		agentName := strings.TrimSpace(input.AgentName)
		if projectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if agentName == "" {
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

		builds, total, err := handler.ListAgentBuilds(ctx, orgName, projectName, agentName, int32(limit), int32(offset))
		if err != nil {
			return nil, nil, wrapToolError("list_builds", err)
		}

		response := listBuildsOutput{
			OrgName:     orgName,
			ProjectName: projectName,
			AgentName:   agentName,
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
		projectName := strings.TrimSpace(input.ProjectName)
		agentName := strings.TrimSpace(input.AgentName)
		buildName := strings.TrimSpace(input.BuildName)
		if projectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if agentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}
		if buildName == "" {
			return nil, nil, fmt.Errorf("build_name is required")
		}

		orgName := resolveOrgName(input.OrgName)

		result, err := handler.GetBuild(ctx, orgName, projectName, agentName, buildName)
		if err != nil {
			return nil, nil, wrapToolError("get_build_details", err)
		}

		response := getBuildDetailsOutput{
			OrgName:     orgName,
			ProjectName: projectName,
			AgentName:   agentName,
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
		projectName := strings.TrimSpace(input.ProjectName)
		agentName := strings.TrimSpace(input.AgentName)
		if projectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if agentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}
		orgName := resolveOrgName(input.OrgName)
		commitID := ""
		if input.CommitID != nil {
			commitID = strings.TrimSpace(*input.CommitID)
		}
		build, err := handler.BuildAgent(ctx, orgName, projectName, agentName, commitID)
		if err != nil {
			return nil, nil, wrapToolError("build_agent", err)
		}
		response := buildAgentOutput{
			OrgName:     orgName,
			ProjectName: projectName,
			AgentName:   agentName,
			Build:       utils.ConvertToBuildResponse(build),
			Note:        "Build started. Check the build details and build logs to track and verify progress.",
		}
		return handleToolResult(response, nil)
	}
}

func getBuildLogs(handler BuildToolsetHandler) func(context.Context, *gomcp.CallToolRequest, getBuildLogsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getBuildLogsInput) (*gomcp.CallToolResult, any, error) {
		projectName := strings.TrimSpace(input.ProjectName)
		agentName := strings.TrimSpace(input.AgentName)
		buildName := strings.TrimSpace(input.BuildName)
		if projectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if agentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}
		if buildName == "" {
			return nil, nil, fmt.Errorf("build_name is required")
		}
		orgName := resolveOrgName(input.OrgName)

		result, err := handler.GetBuildLogs(ctx, orgName, projectName, agentName, buildName)
		if err != nil {
			return nil, nil, wrapToolError("get_build_logs", err)
		}

		response := getBuildLogsOutput{
			OrgName:     orgName,
			ProjectName: projectName,
			AgentName:   agentName,
			BuildName:   buildName,
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
			BuildID:   build.UUID,
			BuildName: build.Name,
			StartedAt: build.StartedAt,
			EndedAt:   build.EndedAt,
			ImageID:   build.ImageId,
			Status:    build.Status,
		}
		if isBuildInProgress(build.Status) {
			retry := buildRetryAfterSeconds
			item.RetryAfterSeconds = &retry
		}
		out = append(out, item)
	}
	return out
}

// Build states form this pipeline:
//
//	BuildInitiated → BuildTriggered → BuildRunning → BuildSucceeded → BuildCompleted
//	                                                                ↘ BuildFailed
//
// Only BuildCompleted and BuildFailed are terminal.
func isBuildInProgress(status string) bool {
	switch status {
	case "BuildInitiated", "BuildTriggered", "BuildRunning", "BuildSucceeded":
		return true
	default:
		return false
	}
}

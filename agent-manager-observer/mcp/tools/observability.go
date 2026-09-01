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

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-observer/controllers"
	"github.com/wso2/agent-manager/agent-manager-observer/middleware"
	"github.com/wso2/agent-manager/agent-manager-observer/rbac"
)

// requireToolPermission builds the per-call authorization guard for a tool,
// applying the same policy as the REST routes (middleware.AuthorizePermission):
// ordinary tokens need the tool's amp scope when RBAC is enabled, and
// publisher-audience (amp-publisher-*) tokens are confined to their implicit
// trace-read permission regardless of the flag. The per-call bearer token is
// read from req.Extra.Header (set by the streamable-HTTP transport for every
// tool call); JWTAuth has already verified it. The handler ctx cannot be used
// instead: the transport derives it from the session-initializing request, so
// middleware.GetTokenClaims(ctx) would reflect the initialize token, not the
// current call's.
func requireToolPermission(rbacEnabled bool, perm rbac.Permission) func(*gomcp.CallToolRequest) error {
	return func(req *gomcp.CallToolRequest) error {
		var authHeader string
		if req != nil && req.Extra != nil && req.Extra.Header != nil {
			authHeader = req.Extra.Header.Get("Authorization")
		}
		claims := middleware.ParseUnverifiedClaims(authHeader)
		if err := middleware.AuthorizePermission(claims, perm, rbacEnabled); err != nil {
			return fmt.Errorf("%w: this tool requires the %s scope", err, perm.Scope())
		}
		return nil
	}
}

// maxLogsLimit is the maximum "limit" get_runtime_logs accepts, mirroring
// maxLogLimit in handlers/handlers.go.
const maxLogsLimit = 10000

type runtimeLogsInput struct {
	Organization string   `json:"organization" jsonschema:"required"`
	Project      string   `json:"project" jsonschema:"required"`
	Agent        string   `json:"agent" jsonschema:"required"`
	Environment  string   `json:"environment" jsonschema:"required"`
	StartTime    string   `json:"start_time,omitempty"`
	EndTime      string   `json:"end_time,omitempty"`
	Limit        *int     `json:"limit,omitempty"`
	SortOrder    string   `json:"sort_order,omitempty"`
	LogLevels    []string `json:"log_levels,omitempty"`
	SearchPhrase string   `json:"search_phrase,omitempty"`
}

type buildLogsInput struct {
	Organization string `json:"organization" jsonschema:"required"`
	BuildName    string `json:"build_name" jsonschema:"required"`
}

type metricsInput struct {
	Organization string `json:"organization" jsonschema:"required"`
	Project      string `json:"project" jsonschema:"required"`
	Agent        string `json:"agent" jsonschema:"required"`
	Environment  string `json:"environment" jsonschema:"required"`
	StartTime    string `json:"start_time,omitempty"`
	EndTime      string `json:"end_time,omitempty"`
}

func (t *Toolsets) registerObservabilityTools(server *gomcp.Server) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name: "get_runtime_logs",
		Description: "Return runtime logs for an agent. " +
			"Runtime logs are the application logs emitted by a deployed agent, and they can be filtered by time window, log level, sort order, or text search.",
	}, withToolLogging("get_runtime_logs", getRuntimeLogs(t.Observability, requireToolPermission(t.RBACEnabled, rbac.LogRead))))

	gomcp.AddTool(server, &gomcp.Tool{
		Name: "get_build_logs",
		Description: "Return logs for a specific build of an internal agent. " +
			"Build logs are the step-by-step output produced while packaging the agent source into a runnable image.",
	}, withToolLogging("get_build_logs", getBuildLogs(t.Observability, requireToolPermission(t.RBACEnabled, rbac.BuildLogRead))))

	gomcp.AddTool(server, &gomcp.Tool{
		Name: "get_metrics",
		Description: "Return CPU and memory usage, request and limit metrics for an agent over a selected time range. " +
			"Metrics describe runtime resource consumption for a deployment in a specific environment.",
	}, withToolLogging("get_metrics", getMetrics(t.Observability, requireToolPermission(t.RBACEnabled, rbac.MetricRead))))
}

func getRuntimeLogs(obs *controllers.ObservabilityController, authorize func(*gomcp.CallToolRequest) error) func(context.Context, *gomcp.CallToolRequest, runtimeLogsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *gomcp.CallToolRequest, input runtimeLogsInput) (*gomcp.CallToolResult, any, error) {
		if err := authorize(req); err != nil {
			return nil, nil, err
		}
		organization, err := requireField(input.Organization, "organization")
		if err != nil {
			return nil, nil, err
		}
		project, err := requireField(input.Project, "project")
		if err != nil {
			return nil, nil, err
		}
		agent, err := requireField(input.Agent, "agent")
		if err != nil {
			return nil, nil, err
		}
		environment, err := requireField(input.Environment, "environment")
		if err != nil {
			return nil, nil, err
		}

		startTime, endTime, err := resolveTimeWindow(input.StartTime, input.EndTime)
		if err != nil {
			return nil, nil, err
		}
		if err := rejectFutureLogStartTime(startTime); err != nil {
			return nil, nil, err
		}

		levels, err := normalizeLogLevels(input.LogLevels)
		if err != nil {
			return nil, nil, err
		}

		sortOrder, err := validateSortOrder(input.SortOrder, "")
		if err != nil {
			return nil, nil, err
		}

		limit, err := validateOptionalLimit(input.Limit, maxLogsLimit)
		if err != nil {
			return nil, nil, err
		}

		params := controllers.LogsQueryParams{
			Organization: organization,
			Project:      project,
			Agent:        agent,
			Environment:  environment,
			StartTime:    startTime,
			EndTime:      endTime,
			SearchPhrase: strings.TrimSpace(input.SearchPhrase),
			LogLevels:    levels,
			Limit:        limit,
			SortOrder:    sortOrder,
		}

		result, err := obs.GetLogs(ctx, params)
		if err != nil {
			return nil, nil, wrapToolError("get_runtime_logs", err)
		}
		return handleToolResult(result, nil)
	}
}

func getBuildLogs(obs *controllers.ObservabilityController, authorize func(*gomcp.CallToolRequest) error) func(context.Context, *gomcp.CallToolRequest, buildLogsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *gomcp.CallToolRequest, input buildLogsInput) (*gomcp.CallToolResult, any, error) {
		if err := authorize(req); err != nil {
			return nil, nil, err
		}
		organization, err := requireField(input.Organization, "organization")
		if err != nil {
			return nil, nil, err
		}
		buildName, err := requireField(input.BuildName, "build_name")
		if err != nil {
			return nil, nil, err
		}

		result, err := obs.GetBuildLogs(ctx, organization, buildName)
		if err != nil {
			return nil, nil, wrapToolError("get_build_logs", err)
		}
		return handleToolResult(result, nil)
	}
}

func getMetrics(obs *controllers.ObservabilityController, authorize func(*gomcp.CallToolRequest) error) func(context.Context, *gomcp.CallToolRequest, metricsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *gomcp.CallToolRequest, input metricsInput) (*gomcp.CallToolResult, any, error) {
		if err := authorize(req); err != nil {
			return nil, nil, err
		}
		organization, err := requireField(input.Organization, "organization")
		if err != nil {
			return nil, nil, err
		}
		project, err := requireField(input.Project, "project")
		if err != nil {
			return nil, nil, err
		}
		agent, err := requireField(input.Agent, "agent")
		if err != nil {
			return nil, nil, err
		}
		environment, err := requireField(input.Environment, "environment")
		if err != nil {
			return nil, nil, err
		}

		startTime, endTime, err := resolveTimeWindow(input.StartTime, input.EndTime)
		if err != nil {
			return nil, nil, err
		}

		params := controllers.MetricsQueryParams{
			Organization: organization,
			Project:      project,
			Agent:        agent,
			Environment:  environment,
			StartTime:    startTime,
			EndTime:      endTime,
		}

		result, err := obs.GetMetrics(ctx, params)
		if err != nil {
			return nil, nil, wrapToolError("get_metrics", err)
		}
		return handleToolResult(result, nil)
	}
}

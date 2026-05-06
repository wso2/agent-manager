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
)


type runtimeLogsInput struct {
	OrgName      string   `json:"org_name"`
	ProjectName  string   `json:"project_name"`
	AgentName    string   `json:"agent_name"`
	Environment  string   `json:"environment"`
	StartTime    string   `json:"start_time"`
	EndTime      string   `json:"end_time"`
	Limit        *int     `json:"limit"`
	SortOrder    string   `json:"sort_order"`
	LogLevels    []string `json:"log_levels"`
	SearchPhrase string   `json:"search_phrase"`
}

type getMetricsInput struct {
	OrgName     string `json:"org_name"`
	ProjectName string `json:"project_name"`
	AgentName   string `json:"agent_name"`
	Environment string `json:"environment"`
    StartTime   string `json:"start_time"`
    EndTime     string `json:"end_time"`
}

func (t *Toolsets) registerObservabilityTools(server *gomcp.Server) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name: "get_runtime_logs",
		Description: "Return runtime logs for an agent. " +
			"Runtime logs are the application logs emitted by a deployed agent, and they can be filtered by time window, log level, sort order, or text search.",
		InputSchema: createSchema(map[string]any{
			"org_name":      stringProperty("Optional. Organization name."),
			"project_name":  stringProperty("Required. Project name where the agent exists."),
			"agent_name":    stringProperty("Required. Agent name to fetch runtime logs for."),
			"environment":   stringProperty("Optional. Environment name."),
			"start_time":    stringProperty("Optional. Start time in RFC3339 format. Defaults to last 24h if omitted."),
			"end_time":      stringProperty("Optional. End time in RFC3339 format. Defaults to now if omitted."),
			"limit":         intProperty("Optional. Maximum number of log entries to retrieve."),
			"sort_order":    stringProperty("Optional. Sort order of the logs: asc or desc."),
			"log_levels":    arrayProperty("Optional. Filter by log levels: DEBUG, INFO, WARN, ERROR.", map[string]any{"type": "string"}),
			"search_phrase": stringProperty("Optional. Search phrase to filter logs by content."),
		}, []string{"project_name", "agent_name"}),
	}, withToolLogging("get_runtime_logs", getRuntimeLogs(t.ObservabilityToolset)))

	gomcp.AddTool(server, &gomcp.Tool{
		Name: "get_metrics",
		Description: "Return CPU and memory usage, request and limit metrics for an agent over a selected time range. " +
			"Metrics describe runtime resource consumption for a deployment in a specific environment.",
		InputSchema: createSchema(map[string]any{
			"org_name":     stringProperty("Optional. Organization name."),
			"project_name": stringProperty("Required. Project name."),
			"agent_name":   stringProperty("Required. Agent name."),
			"environment":  stringProperty("Optional. Environment name."),
			"start_time":   stringProperty("Optional. Start time in RFC3339 format. Defaults to 24h ago."),
			"end_time":     stringProperty("Optional. End time in RFC3339 format. Defaults to current time."),
		}, []string{"project_name", "agent_name"}),
	}, withToolLogging("get_metrics", getMetrics(t.ObservabilityToolset)))
}

func getRuntimeLogs(handler ObservabilityToolsetHandler) func(context.Context, *gomcp.CallToolRequest, runtimeLogsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input runtimeLogsInput) (*gomcp.CallToolResult, any, error) {
		projectName := strings.TrimSpace(input.ProjectName)
		agentName := strings.TrimSpace(input.AgentName)

		if projectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if agentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}
		if input.Limit != nil && (*input.Limit < 1 || *input.Limit > 10000) {
			return nil, nil, fmt.Errorf("limit must be between 1 and 10000")
		}

		orgName := resolveOrgName(input.OrgName)

		env := resolveEnv(input.Environment)
		start, end, err := resolveTimeWindow(input.StartTime, input.EndTime)
		if err != nil {
			return nil, nil, err
		}
		sortOrder := defaultSortOrder(input.SortOrder)

		levels, err := normalizeLogLevels(input.LogLevels)
		if err != nil {
			return nil, nil, err
		}

		var limit *int32
		if input.Limit != nil {
			value := int32(*input.Limit)
			limit = &value
		}

		var search *string
		if strings.TrimSpace(input.SearchPhrase) != "" {
			value := strings.TrimSpace(input.SearchPhrase)
			search = &value
		}

		req := spec.LogFilterRequest{
			EnvironmentName: env,
			StartTime:       start,
			EndTime:         end,
			Limit:           limit,
			SortOrder:       &sortOrder,
			LogLevels:       levels,
			SearchPhrase:    search,
		}

		result, err := handler.GetRuntimeLogs(ctx, orgName, projectName, agentName, req)
		if err != nil {
			return nil, nil, wrapToolError("get_runtime_logs", err)
		}

		reduced := reduceLogsResponse(result)
		return handleToolResult(reduced, nil)
	}
}

func getMetrics(handler ObservabilityToolsetHandler) func(context.Context, *gomcp.CallToolRequest, getMetricsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getMetricsInput) (*gomcp.CallToolResult, any, error) {
		projectName := strings.TrimSpace(input.ProjectName)
		agentName := strings.TrimSpace(input.AgentName)

		if projectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if agentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}

		orgName := resolveOrgName(input.OrgName)

		env := resolveEnv(input.Environment)

		start, end, err := resolveTimeWindow(input.StartTime, input.EndTime)
		if err != nil {
			return nil, nil, err
		}

		payload := spec.MetricsFilterRequest{
			EnvironmentName: env,
			StartTime:       start,
			EndTime:         end,
		}

		result, err := handler.GetMetrics(ctx, orgName, projectName, agentName, payload)
		if err != nil {
			return nil, nil, wrapToolError("get_metrics", err)
		}
		return handleToolResult(result, nil)
	}
}

// helpers

// resolving time window for metrics and runtime logs retrieval tools
func resolveTimeWindow(start, end string) (string, string, error) {
	start = strings.TrimSpace(start)
    end = strings.TrimSpace(end)

	if start == "" && end == "" {
		return defaultWindow()
	}
	if start == "" || end == "" {
		return "", "", fmt.Errorf("start time and end time must be provided together")
	}
	startTime, err := time.Parse(time.RFC3339, start)
	if err != nil {
		return "", "", fmt.Errorf("Invalid start_time format. Use RFC3339")
	}
	endTime, err := time.Parse(time.RFC3339, end)
	if err != nil {
		return "", "", fmt.Errorf("Invalid end_time format. Use RFC3339")
	}

	if !startTime.Before(endTime) {
		return "", "", fmt.Errorf("start_time must be before end_time")
	}
	if endTime.Sub(startTime) > 14*24*time.Hour {
		return "", "", fmt.Errorf("time range cannot exceed 14 days")
	}

	return startTime.UTC().Format(time.RFC3339), endTime.UTC().Format(time.RFC3339), nil
}

func defaultWindow() (string, string, error) {
	end := time.Now().UTC()
	start := end.Add(-24 * time.Hour)
	return start.Format(time.RFC3339), end.Format(time.RFC3339), nil
}

func defaultSortOrder(order string) string {
	switch strings.ToLower(strings.TrimSpace(order)) {
	case "asc":
		return "asc"
	default:
		return "desc"
	}
}

func normalizeLogLevels(levels []string) ([]string, error) {
	if len(levels) == 0 {
		return nil, nil
	}
	allowed := map[string]bool{
		"DEBUG": true,
		"INFO":  true,
		"WARN":  true,
		"ERROR": true,
	}
	out := make([]string, 0, len(levels))
	for _, lvl := range levels {
		value := strings.ToUpper(strings.TrimSpace(lvl))
		if value == "" {
			continue
		}
		if !allowed[value] {
			return nil, fmt.Errorf("invalid log level: %s", lvl)
		}
		out = append(out, value)
	}
	return out, nil
}
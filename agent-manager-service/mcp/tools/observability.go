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

// input structs
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
type listTracesInput struct {
	OrgName     string `json:"org_name"`
	ProjectName string `json:"project_name"`
	AgentName   string `json:"agent_name"`

	Environment string `json:"environment"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	Limit       *int   `json:"limit"`
	Offset      *int   `json:"offset"`
	SortOrder   string `json:"sort_order"`
	IncludeIO   *bool  `json:"include_io"`
}
type getTracesInput struct {
	OrgName     string `json:"org_name"`
	ProjectName string `json:"project_name"`
	AgentName   string `json:"agent_name"`

	Environment string `json:"environment"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	Limit       *int   `json:"limit"`
	Offset      *int   `json:"offset"`
	SortOrder   string `json:"sort_order"`

	Condition    string `json:"condition"`
	MaxLatency   *int   `json:"max_latency"`
	MaxTokens    *int   `json:"max_tokens"`
	MaxSpanCount *int   `json:"max_spans"`
}
type getTraceDetailsInput struct {
	OrgName     string `json:"org_name"`
	ProjectName string `json:"project_name"`
	AgentName   string `json:"agent_name"`
	TraceID     string `json:"trace_id"`
	Environment string `json:"environment"`
}
type getSpanDetailsInput struct {
	OrgName     string `json:"org_name"`
	ProjectName string `json:"project_name"`
	AgentName   string `json:"agent_name"`
	TraceID     string `json:"trace_id"`
	SpanID      string `json:"span_id"`
	Environment string `json:"environment"`
}

const (
	maxTraceListLimit   = 100
	maxTraceExportLimit = 100

	defaultTraceListLimit   = 10
	defaultTraceExportLimit = 100
)

func (t *Toolsets) registerObservabilityTools(server *gomcp.Server) {
	gomcp.AddTool(server, &gomcp.Tool{
		Name: "get_runtime_logs",
		Description: "Return runtime logs for an agent. " +
			"Runtime logs are the application logs emitted by a deployed agent, and they can be filtered by time window, log level, sort order, or text search.",
		InputSchema: createSchema(map[string]any{
			"org_name":      stringProperty("Optional. Organization name."),
			"project_name":  stringProperty("Required. Project name."),
			"agent_name":    stringProperty("Required. Agent name."),
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

	gomcp.AddTool(server, &gomcp.Tool{
		Name: "list_traces",
		Description: "Returns a summary view of recent traces for an agent within a time window. " +
			"A trace is a single end-to-end execution record for an agent request. ",
		InputSchema: createSchema(map[string]any{
			"org_name":     stringProperty("Optional. Organization name."),
			"project_name": stringProperty("Required. Project name."),
			"agent_name":   stringProperty("Required. Agent name."),
			"environment":  stringProperty("Optional. Environment name."),
			"start_time":   stringProperty("Optional. Start time in RFC3339 format. Defaults to 24h ago."),
			"end_time":     stringProperty("Optional. End time in RFC3339 format. Defaults to current time."),
			"limit": map[string]any{
				"type":        "integer",
				"description": "Optional. Max number of traces to return.",
				"minimum":     1,
				"maximum":     maxTraceListLimit,
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Optional. Pagination offset (>= 0).",
				"minimum":     0,
			},
			"sort_order": enumProperty("Optional. Sort order for traces: desc (newest first) or asc (oldest first).", []string{"desc", "asc"}),
			"include_io": map[string]any{
				"type":        "boolean",
				"description": "Optional. Include input/output fields in the traces.",
			},
		}, []string{"project_name", "agent_name"}),
	}, withToolLogging("list_traces", listTraces(t.ObservabilityToolset)))

	gomcp.AddTool(server, &gomcp.Tool{
		Name: "get_traces",
		Description: "Returns the traces for an agent including full span details within a time window. " +
			"A trace is a single end-to-end execution record for an agent which contains spans that record the internal steps of an execution.",
		InputSchema: createSchema(map[string]any{
			"org_name":     stringProperty("Optional. Organization name."),
			"project_name": stringProperty("Required. Project name."),
			"agent_name":   stringProperty("Required. Agent name."),
			"environment":  stringProperty("Optional. Environment name."),
			"start_time":   stringProperty("Optional. Start time in RFC3339 format. Defaults to 24h ago."),
			"end_time":     stringProperty("Optional. End time in RFC3339 format. Defaults to current time."),
			"limit": map[string]any{
				"type":        "integer",
				"description": "Optional. Max number of traces to return (1-1000).",
				"minimum":     1,
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Optional. Pagination offset (>= 0).",
				"minimum":     0,
			},
			"sort_order":  enumProperty("Optional. Sort order for traces: desc (newest first) or asc (oldest first).", []string{"desc", "asc"}),
			"condition":   enumProperty("Optional. Filter condition. Use error_status to return only traces with errors, high_latency for slow traces, high_token_usage for token-heavy traces, tool_call_fails for traces with failed tool calls, excessive_steps for traces with too many spans.", []string{"error_status", "high_latency", "high_token_usage", "tool_call_fails", "excessive_steps"}),
			"max_latency": intProperty("Optional. Max latency threshold in milliseconds for high_latency condition. Defaults to 30000."),
			"max_tokens":  intProperty("Optional. Max token threshold for high_token_usage condition. Defaults to 10000."),
			"max_spans":   intProperty("Optional. Max span count threshold for excessive_steps condition. Defaults to 40."),
		}, []string{"project_name", "agent_name"}),
	}, withToolLogging("get_traces", getTraces(t.ObservabilityToolset)))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "get_trace_details",
		Description: "Return the metadata plus its span list for one trace",
		InputSchema: createSchema(map[string]any{
			"org_name":     stringProperty("Required. Organization name."),
			"project_name": stringProperty("Required. Project name."),
			"agent_name":   stringProperty("Required. Agent name."),
			"trace_id":     stringProperty("Required. Trace ID to fetch."),
			"environment":  stringProperty("Optional. Environment name."),
		}, []string{"project_name", "agent_name", "trace_id"}),
	}, withToolLogging("get_trace_details", getTraceDetails(t.ObservabilityToolset)))

	gomcp.AddTool(server, &gomcp.Tool{
		Name: "get_span_details",
		Description: "Return the execution details for a single span. " +
			"A span is a single step within a trace execution, such as an LLM call, tool invocation, or retriever lookup, capturing its timing, inputs, outputs, and attributes",
		InputSchema: createSchema(map[string]any{
			"org_name":     stringProperty("Optional. Organization name."),
			"project_name": stringProperty("Required. Project name where the agent exists."),
			"agent_name":   stringProperty("Required. Agent name that produced the trace."),
			"trace_id":     stringProperty("Required. Trace ID containing the span."),
			"span_id":      stringProperty("Required. Span ID to fetch."),
			"environment":  stringProperty("Optional. Environment name."),
		}, []string{"project_name", "agent_name", "trace_id", "span_id"}),
	}, withToolLogging("get_span_details", getSpanDetails(t.ObservabilityToolset)))
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

func listTraces(handler ObservabilityToolsetHandler) func(context.Context, *gomcp.CallToolRequest, listTracesInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input listTracesInput) (*gomcp.CallToolResult, any, error) {
		projectName := strings.TrimSpace(input.ProjectName)
		agentName := strings.TrimSpace(input.AgentName)

		// Input validation
		if projectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if agentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}
		if input.Limit != nil && (*input.Limit < 1 || *input.Limit > maxTraceListLimit) {
			return nil, nil, fmt.Errorf("limit must be between 1 and %d", maxTraceListLimit)
		}
		if input.Offset != nil && *input.Offset < 0 {
			return nil, nil, fmt.Errorf("offset must be >= 0")
		}

		orgName := resolveOrgName(input.OrgName)
		env := resolveEnv(input.Environment)

		start, end, err := resolveTraceTimeWindow(input.StartTime, input.EndTime)
		if err != nil {
			return nil, nil, err
		}
		sortOrder := defaultSortOrder(input.SortOrder)

		limit := defaultTraceListLimit
		if input.Limit != nil {
			limit = *input.Limit
		}
		offset := 0
		if input.Offset != nil {
			offset = *input.Offset
		}

		// Call service layer
		result, err := handler.ListTraces(ctx, orgName, projectName, agentName, env, start, end, sortOrder, limit, offset)
		if err != nil {
			return nil, nil, wrapToolError("list_traces", err)
		}

		includeIO := input.IncludeIO != nil && *input.IncludeIO
		reducedTraces := extractTraceOverviews(result, includeIO)
		reducedTraces["org_name"] = orgName
		reducedTraces["project_name"] = projectName
		reducedTraces["agent_name"] = agentName
		reducedTraces["environment"] = env
		reducedTraces["start_time"] = start
		reducedTraces["end_time"] = end
		reducedTraces["limit"] = limit
		reducedTraces["offset"] = offset

		return handleToolResult(reducedTraces, nil)
	}
}

func getTraces(handler ObservabilityToolsetHandler) func(context.Context, *gomcp.CallToolRequest, getTracesInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getTracesInput) (*gomcp.CallToolResult, any, error) {
		projectName := strings.TrimSpace(input.ProjectName)
		agentName := strings.TrimSpace(input.AgentName)

		if projectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if agentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}
		if input.Limit != nil && (*input.Limit < 1 || *input.Limit > maxTraceExportLimit) {
			return nil, nil, fmt.Errorf("limit must be between 1 and %d", maxTraceExportLimit)
		}
		if input.Offset != nil && *input.Offset < 0 {
			return nil, nil, fmt.Errorf("offset must be >= 0")
		}

		orgName := resolveOrgName(input.OrgName)
		env := resolveEnv(input.Environment)

		start, end, err := resolveTraceTimeWindow(input.StartTime, input.EndTime)
		if err != nil {
			return nil, nil, err
		}
		sortOrder := defaultSortOrder(input.SortOrder)

		limit := defaultTraceExportLimit
		if input.Limit != nil {
			limit = *input.Limit
		}
		offset := 0
		if input.Offset != nil {
			offset = *input.Offset
		}

		result, err := handler.ExportTraces(ctx, orgName, projectName, agentName, env, start, end, sortOrder, limit, offset)
		if err != nil {
			return nil, nil, wrapToolError("get_traces", err)
		}

		reducedTraces := extractTracesWithSpans(result, input.Limit)

		// checking for filtering conditions
		if condition := strings.TrimSpace(strings.ToLower(input.Condition)); condition != "" {
			traces, _ := reducedTraces["traces"].([]map[string]any)
			filtered := make([]map[string]any, 0, len(traces))
			for _, trace := range traces {
				if matchesCondition(trace, condition, input) {
					filtered = append(filtered, trace)
				}
			}
			reducedTraces["traces"] = filtered
			reducedTraces["count"] = len(filtered)
		}

		reducedTraces["totalCount"] = result["totalCount"]
		reducedTraces["org_name"] = orgName
		reducedTraces["project_name"] = projectName
		reducedTraces["agent_name"] = agentName
		reducedTraces["environment"] = env
		reducedTraces["start_time"] = start
		reducedTraces["end_time"] = end

		return handleToolResult(reducedTraces, nil)
	}
}

func getTraceDetails(handler ObservabilityToolsetHandler) func(context.Context, *gomcp.CallToolRequest, getTraceDetailsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getTraceDetailsInput) (*gomcp.CallToolResult, any, error) {
		projectName := strings.TrimSpace(input.ProjectName)
		agentName := strings.TrimSpace(input.AgentName)

		if projectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if agentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}
		if input.TraceID == "" {
			return nil, nil, fmt.Errorf("trace_id is required")
		}

		orgName := resolveOrgName(input.OrgName)
		env := resolveEnv(input.Environment)

		result, err := handler.GetTraceDetails(ctx, orgName, projectName, agentName, input.TraceID, env)
		if err != nil {
			return nil, nil, wrapToolError("get_trace_details", err)
		}

		reducedTrace := extractTraceDetails(result, input.TraceID)
		reducedTrace["org_name"] = orgName
		reducedTrace["project_name"] = projectName
		reducedTrace["agent_name"] = agentName
		reducedTrace["environment"] = env

		return handleToolResult(reducedTrace, nil)
	}
}

func getSpanDetails(handler ObservabilityToolsetHandler) func(context.Context, *gomcp.CallToolRequest, getSpanDetailsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *gomcp.CallToolRequest, input getSpanDetailsInput) (*gomcp.CallToolResult, any, error) {
		projectName := strings.TrimSpace(input.ProjectName)
		agentName := strings.TrimSpace(input.AgentName)

		if projectName == "" {
			return nil, nil, fmt.Errorf("project_name is required")
		}
		if agentName == "" {
			return nil, nil, fmt.Errorf("agent_name is required")
		}
		if input.TraceID == "" {
			return nil, nil, fmt.Errorf("trace_id is required")
		}
		if input.SpanID == "" {
			return nil, nil, fmt.Errorf("span_id is required")
		}

		orgName := resolveOrgName(input.OrgName)
		env := resolveEnv(input.Environment)

		result, err := handler.GetSpanDetails(ctx, orgName, projectName, agentName, input.TraceID, input.SpanID, env)
		if err != nil {
			return nil, nil, wrapToolError("get_span_details", err)
		}
		return handleToolResult(result, nil)
	}
}

// helpers

// resolves a time window for logs and metrics (max 14 days).
func resolveTimeWindow(start, end string) (string, string, error) {
	return resolveTimeWindowWithLimit(start, end, 14*24*time.Hour)
}

// resolves a time window for traces (max 30 days).
func resolveTraceTimeWindow(start, end string) (string, string, error) {
	return resolveTimeWindowWithLimit(start, end, 30*24*time.Hour)
}

func resolveTimeWindowWithLimit(start, end string, maxDuration time.Duration) (string, string, error) {
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)

	if start == "" && end == "" {
		return defaultWindow()
	}
	if start == "" {
		return "", "", fmt.Errorf("start time is required when end time is provided")
	}
	if end == "" {
		end = time.Now().UTC().Format(time.RFC3339)
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
	if endTime.Sub(startTime) > maxDuration {
		days := int(maxDuration.Hours() / 24)
		return "", "", fmt.Errorf("time range cannot exceed %d days", days)
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

// helper function to list traces
func extractTraceOverviews(resp map[string]any, includeIO bool) map[string]any {
	if resp == nil {
		return map[string]any{"traces": []map[string]any{}, "count": 0, "totalCount": 0}
	}
	tracesAny := getSlice(resp["traces"])
	traces := make([]map[string]any, 0, len(tracesAny))
	for _, traceAny := range tracesAny {
		traceMap := getMap(traceAny)
		if traceMap == nil {
			continue
		}
		item := map[string]any{
			"traceId":         getString(traceMap["traceId"]),
			"rootSpanId":      getString(traceMap["rootSpanId"]),
			"rootSpanName":    getString(traceMap["rootSpanName"]),
			"rootSpanKind":    getString(traceMap["rootSpanKind"]),
			"startTime":       traceMap["startTime"],
			"endTime":         traceMap["endTime"],
			"durationInNanos": traceMap["durationInNanos"],
			"spanCount":       traceMap["spanCount"],
			"tokenUsage":      traceMap["tokenUsage"],
			"status":          traceMap["status"],
		}
		if includeIO {
			if v, ok := traceMap["input"]; ok {
				item["input"] = v
			}
			if v, ok := traceMap["output"]; ok {
				item["output"] = v
			}
		}
		traces = append(traces, item)
	}
	return map[string]any{
		"traces":     traces,
		"count":      len(traces),
		"totalCount": resp["totalCount"],
	}
}

// helper function to list traces with spans (export endpoint)
func extractTracesWithSpans(resp map[string]any, limit *int) map[string]any {
	tracesAny := getSlice(resp["traces"])
	if limit != nil && *limit < len(tracesAny) {
		tracesAny = tracesAny[:*limit]
	}

	reducedTraces := make([]map[string]any, 0, len(tracesAny))
	for _, traceAny := range tracesAny {
		traceMap := getMap(traceAny)
		if traceMap == nil {
			continue
		}
		spansAny := getSlice(traceMap["spans"])
		reducedSpans := make([]map[string]any, 0, len(spansAny))
		for _, spanAny := range spansAny {
			spanMap := getMap(spanAny)
			if spanMap == nil {
				continue
			}
			reducedSpans = append(reducedSpans, map[string]any{
				"spanId":          getString(spanMap["spanId"]),
				"parentSpanId":    getString(spanMap["parentSpanId"]),
				"name":            getString(spanMap["name"]),
				"durationInNanos": spanMap["durationInNanos"],
				"ampAttributes":   spanMap["ampAttributes"],
			})
		}
		reducedTraces = append(reducedTraces, map[string]any{
			"traceId":         getString(traceMap["traceId"]),
			"rootSpanId":      getString(traceMap["rootSpanId"]),
			"durationInNanos": traceMap["durationInNanos"],
			"spanCount":       traceMap["spanCount"],
			"tokenUsage":      traceMap["tokenUsage"],
			"status":          traceMap["status"],
			"input":           traceMap["input"],
			"output":          traceMap["output"],
			"spans":           reducedSpans,
		})
	}

	return map[string]any{
		"traces": reducedTraces,
		"count":  len(reducedTraces),
	}
}

func matchesCondition(trace map[string]any, condition string, input getTracesInput) bool {
	switch condition {

	case "error_status":
		status := getMap(trace["status"])
		errorCount, _ := status["errorCount"].(float64)
		return errorCount > 0

	case "high_latency":
		maxLatency := 30000.0
		if input.MaxLatency != nil {
			maxLatency = float64(*input.MaxLatency)
		}
		durationNanos, _ := trace["durationInNanos"].(float64)
		return durationNanos/1_000_000 > maxLatency

	case "high_token_usage":
		maxTokens := 10000.0
		if input.MaxTokens != nil {
			maxTokens = float64(*input.MaxTokens)
		}
		tokenUsage := getMap(trace["tokenUsage"])
		totalTokens, _ := tokenUsage["totalTokens"].(float64)
		return totalTokens > maxTokens

	case "tool_call_fails":
		spans, _ := trace["spans"].([]map[string]any)
		for _, spanMap := range spans {
			ampAttrs := getMap(spanMap["ampAttributes"])
			if strings.ToLower(getString(ampAttrs["kind"])) != "tool" {
				continue
			}
			status := getMap(ampAttrs["status"])
			errVal, _ := status["error"].(bool)
			if errVal {
				return true
			}
		}
		return false

	case "excessive_steps":
		maxSpanCount := 40.0
		if input.MaxSpanCount != nil {
			maxSpanCount = float64(*input.MaxSpanCount)
		}
		spanCount, _ := trace["spanCount"].(float64)
		return spanCount > maxSpanCount

	default:
		return false
	}
}

// helper function to get details of a single trace
func extractTraceDetails(resp map[string]any, traceID string) map[string]any {
	reducedSpans := make([]map[string]any, 0)
	if rawSpans, ok := resp["spans"].([]any); ok {
		for _, span := range rawSpans {
			spanMap, ok := span.(map[string]any)
			if !ok {
				continue
			}
			parent := ""
			if v, ok := spanMap["parentSpanId"]; ok && v != nil {
				parent = getString(v)
			}
			reducedSpans = append(reducedSpans, map[string]any{
				"spanId":       getString(spanMap["spanId"]),
				"parentSpanId": parent,
				"spanName":     getString(spanMap["spanName"]),
				"startTime":    spanMap["startTime"],
				"endTime":      spanMap["endTime"],
				"durationNs":   spanMap["durationNs"],
			})
		}
	}
	return map[string]any{
		"traceId":   traceID,
		"spanCount": resp["totalCount"],
		"spans":     reducedSpans,
	}
}

func getMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return nil
}

func getSlice(value any) []any {
	if value == nil {
		return nil
	}
	if s, ok := value.([]any); ok {
		return s
	}
	return nil
}

func getString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

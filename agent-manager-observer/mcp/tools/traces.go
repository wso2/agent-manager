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

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-observer/controllers"
	"github.com/wso2/agent-manager/agent-manager-observer/rbac"
)

const (
	// defaultTraceListLimit/maxTraceListLimit mirror defaultLimit/maxLimit
	// used by GetTraceOverviews in handlers/handlers.go.
	defaultTraceListLimit = 10
	maxTraceListLimit     = 1000

	// defaultTraceExportLimit/maxTraceExportLimit mirror the (100, maxLimit)
	// pair ExportTraces uses in handlers/handlers.go.
	defaultTraceExportLimit = 100
	maxTraceExportLimit     = 1000

	// traceDetailsSpanLimit is the span-list size get_trace_details requests
	// from GetTraceSpans. The tool intentionally has no "limit" input (its
	// job is "show me this trace", not a paginated span list), so it always
	// asks for the controller's hard per-request cap rather than the REST
	// route's paginated default of 10.
	traceDetailsSpanLimit = controllers.MaxSpansPerRequest

	// traceDetailsSortOrder is the fixed sort order get_trace_details
	// requests, mirroring GetTraceSpans' REST default ("asc" — chronological).
	traceDetailsSortOrder = "asc"

	// traceFiltersDescription lists the filter inputs list_traces and get_traces share.
	traceFiltersDescription = "Filters (optional; all set filters must match, across the whole window): " +
		"status, min_duration_ms, min_tokens, min_span_count, model, conversation_id, missing, tool, tool_error, mcp_server."
)

// tracesInput backs get_traces (-> ExportTraces) and is the base of
// listTracesInput: both REST routes accept the same scope, time-window,
// paging and filter inputs. Set filters AND together.
type tracesInput struct {
	Organization   string   `json:"organization" jsonschema:"required"`
	Project        string   `json:"project" jsonschema:"required"`
	Agent          string   `json:"agent" jsonschema:"required"`
	Environment    string   `json:"environment" jsonschema:"required"`
	StartTime      string   `json:"start_time,omitempty"`
	EndTime        string   `json:"end_time,omitempty"`
	Limit          *int     `json:"limit,omitempty"`
	SortOrder      string   `json:"sort_order,omitempty"`
	Status         string   `json:"status,omitempty" jsonschema:"Root span status: 'ok' or 'error'."`
	MinDurationMs  *int64   `json:"min_duration_ms,omitempty" jsonschema:"Minimum trace duration, in milliseconds."`
	MinTokens      *int64   `json:"min_tokens,omitempty" jsonschema:"Minimum total tokens."`
	MinSpanCount   *int64   `json:"min_span_count,omitempty" jsonschema:"Minimum number of spans."`
	Model          string   `json:"model,omitempty" jsonschema:"Model name contains this text (case-insensitive)."`
	ConversationID string   `json:"conversation_id,omitempty" jsonschema:"Exact conversation ID."`
	Tool           string   `json:"tool,omitempty" jsonschema:"Tool name contains this text (case-insensitive)."`
	ToolError      bool     `json:"tool_error,omitempty" jsonschema:"Only traces where a tool failed; with tool set, that tool must have failed."`
	MCPServer      string   `json:"mcp_server,omitempty" jsonschema:"MCP server name contains this text (case-insensitive)."`
	Missing        []string `json:"missing,omitempty" jsonschema:"Only traces missing these fields. Accepts: conversation_id."`
}

// listTracesInput backs list_traces (-> GetTraceOverviews), which also takes
// the includes and a paging cursor.
type listTracesInput struct {
	tracesInput
	IncludeModels     bool   `json:"include_models,omitempty" jsonschema:"Add the models used to each trace; off by default."`
	IncludeTools      bool   `json:"include_tools,omitempty" jsonschema:"Add the tools called, and those that failed, to each trace; off by default."`
	IncludeMCPServers bool   `json:"include_mcp_servers,omitempty" jsonschema:"Add the MCP servers used to each trace; off by default."`
	Cursor            string `json:"cursor,omitempty" jsonschema:"nextCursor from the previous page. Keep the other inputs unchanged."`
}

// traceDetailsInput mirrors REST GetTraceSpans: only organization + trace_id
// are required. project/agent/environment are optional scoping filters (the
// REST route treats them as optionalStr), so a valid organization + trace_id
// lookup succeeds without them.
type traceDetailsInput struct {
	Organization string `json:"organization" jsonschema:"required"`
	TraceID      string `json:"trace_id" jsonschema:"required"`
	Project      string `json:"project,omitempty"`
	Agent        string `json:"agent,omitempty"`
	Environment  string `json:"environment,omitempty"`
	StartTime    string `json:"start_time,omitempty"`
	EndTime      string `json:"end_time,omitempty"`
}

// spanDetailsInput has no organization: GetSpanDetail looks a span up by
// trace/span ID alone and never consults organization/namespace scoping.
type spanDetailsInput struct {
	TraceID string `json:"trace_id" jsonschema:"required"`
	SpanID  string `json:"span_id" jsonschema:"required"`
}

// registerTraceTools registers the trace tools on server.
func (t *Toolsets) registerTraceTools(server *gomcp.Server) {
	authorize := t.guard(rbac.TraceRead)

	gomcp.AddTool(server, &gomcp.Tool{
		Name: "list_traces",
		Description: "Returns a summary view of recent traces for an agent within a time window. " +
			"A trace is a single end-to-end execution record for an agent request. " +
			"Returns lookedBackTo (how far back the search got) and truncated (the search stopped early); pass nextCursor back as cursor for the next page.\n\n" +
			traceFiltersDescription,
	}, withToolLogging("list_traces", listTraces(t.Tracing, authorize)))

	gomcp.AddTool(server, &gomcp.Tool{
		Name: "get_traces",
		Description: "Returns the traces for an agent including full span details within a time window. " +
			"A trace is a single end-to-end execution record for an agent which contains spans that record the internal steps of an execution. " +
			"Returns lookedBackTo (how far back a filtered search got) and truncated (the search stopped early or a trace's spans were cut).\n\n" +
			traceFiltersDescription,
	}, withToolLogging("get_traces", getTraces(t.Tracing, authorize)))

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "get_trace_details",
		Description: "Return the metadata plus its span list for one trace",
	}, withToolLogging("get_trace_details", getTraceDetails(t.Tracing, authorize)))

	gomcp.AddTool(server, &gomcp.Tool{
		Name: "get_span_details",
		Description: "Return the execution details for a single span. " +
			"A span is a single step within a trace execution, such as an LLM call, tool invocation, or retriever lookup, capturing its timing, inputs, outputs, and attributes",
	}, withToolLogging("get_span_details", getSpanDetails(t.Tracing, authorize)))
}

// scopedTraceInput is the common subset of listTracesInput/traceDetailsInput
// needed to validate and build the shared portion of TraceQueryParams.
type scopedTraceInput struct {
	Organization string
	Project      string
	Agent        string
	Environment  string
}

// requireTraceScope checks the organization, project, agent and environment are set.
func requireTraceScope(organization, project, agent, environment string) (scopedTraceInput, error) {
	org, err := requireField(organization, "organization")
	if err != nil {
		return scopedTraceInput{}, err
	}
	project, err = requireField(project, "project")
	if err != nil {
		return scopedTraceInput{}, err
	}
	agent, err = requireField(agent, "agent")
	if err != nil {
		return scopedTraceInput{}, err
	}
	environment, err = requireField(environment, "environment")
	if err != nil {
		return scopedTraceInput{}, err
	}
	return scopedTraceInput{Organization: org, Project: project, Agent: agent, Environment: environment}, nil
}

// queryParams validates the inputs list_traces and get_traces share.
func (in tracesInput) queryParams(defaultLimit, maxLimit int) (controllers.TraceQueryParams, error) {
	scope, err := requireTraceScope(in.Organization, in.Project, in.Agent, in.Environment)
	if err != nil {
		return controllers.TraceQueryParams{}, err
	}
	startTime, endTime, err := resolveTraceTimeWindow(in.StartTime, in.EndTime)
	if err != nil {
		return controllers.TraceQueryParams{}, err
	}
	sortOrder, err := validateSortOrder(in.SortOrder, "desc")
	if err != nil {
		return controllers.TraceQueryParams{}, err
	}
	limit, err := validateLimit(in.Limit, defaultLimit, maxLimit)
	if err != nil {
		return controllers.TraceQueryParams{}, err
	}
	filters, err := in.filters()
	if err != nil {
		return controllers.TraceQueryParams{}, err
	}
	return controllers.TraceQueryParams{
		Organization: scope.Organization,
		Project:      &scope.Project,
		Agent:        &scope.Agent,
		Environment:  &scope.Environment,
		StartTime:    startTime,
		EndTime:      endTime,
		Limit:        limit,
		SortOrder:    sortOrder,
		Filters:      filters,
	}, nil
}

// filters validates the filter inputs with the HTTP handler's rules.
func (in tracesInput) filters() (controllers.TraceFilters, error) {
	status, err := controllers.ParseTraceStatus(in.Status)
	if err != nil {
		return controllers.TraceFilters{}, err
	}
	for _, m := range []struct {
		name string
		val  *int64
	}{{"min_duration_ms", in.MinDurationMs}, {"min_tokens", in.MinTokens}, {"min_span_count", in.MinSpanCount}} {
		if m.val == nil {
			continue
		}
		if err := controllers.CheckMinThreshold(m.name, *m.val); err != nil {
			return controllers.TraceFilters{}, err
		}
	}
	if err := controllers.CheckFilterValue("model", in.Model); err != nil {
		return controllers.TraceFilters{}, err
	}
	if err := controllers.CheckFilterValue("conversation_id", in.ConversationID); err != nil {
		return controllers.TraceFilters{}, err
	}
	if err := controllers.CheckFilterValue("tool", in.Tool); err != nil {
		return controllers.TraceFilters{}, err
	}
	if err := controllers.CheckFilterValue("mcp_server", in.MCPServer); err != nil {
		return controllers.TraceFilters{}, err
	}
	var missing controllers.MissingFields
	for _, v := range in.Missing {
		if err := controllers.ParseMissingField(&missing, v, "conversation_id"); err != nil {
			return controllers.TraceFilters{}, err
		}
	}
	f := controllers.TraceFilters{
		Status:         status,
		MinDurationMs:  in.MinDurationMs,
		MinTokens:      in.MinTokens,
		MinSpanCount:   in.MinSpanCount,
		Model:          in.Model,
		ConversationID: in.ConversationID,
		Tool:           in.Tool,
		ToolError:      in.ToolError,
		MCPServer:      in.MCPServer,
		Missing:        missing,
	}
	if err := controllers.CheckMissingConflict(f, "conversation_id"); err != nil {
		return controllers.TraceFilters{}, err
	}
	return f, nil
}

// listQueryParams validates list_traces' inputs: the shared ones, the includes and the cursor.
func (in listTracesInput) listQueryParams() (controllers.TraceQueryParams, error) {
	params, err := in.queryParams(defaultTraceListLimit, maxTraceListLimit)
	if err != nil {
		return controllers.TraceQueryParams{}, err
	}
	params.Include = controllers.Include{Models: in.IncludeModels, Tools: in.IncludeTools, MCPServers: in.IncludeMCPServers}
	if in.Cursor != "" {
		if params.Cursor, err = controllers.DecodeTraceCursor(in.Cursor); err != nil {
			return controllers.TraceQueryParams{}, fmt.Errorf("invalid cursor")
		}
	}
	return params, nil
}

// listTraces handles list_traces: one page of trace overviews, filtered and paged by cursor.
func listTraces(tracing *controllers.TracingController, authorize func(*gomcp.CallToolRequest) error) func(context.Context, *gomcp.CallToolRequest, listTracesInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *gomcp.CallToolRequest, input listTracesInput) (*gomcp.CallToolResult, any, error) {
		if err := authorize(req); err != nil {
			return nil, nil, err
		}
		params, err := input.listQueryParams()
		if err != nil {
			return nil, nil, err
		}

		result, err := tracing.GetTraceOverviews(ctx, params)
		if err != nil {
			return nil, nil, wrapToolError("list_traces", err)
		}
		return handleToolResult(result, nil)
	}
}

// getTraces handles get_traces: full traces with their spans, filtered.
func getTraces(tracing *controllers.TracingController, authorize func(*gomcp.CallToolRequest) error) func(context.Context, *gomcp.CallToolRequest, tracesInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *gomcp.CallToolRequest, input tracesInput) (*gomcp.CallToolResult, any, error) {
		if err := authorize(req); err != nil {
			return nil, nil, err
		}
		params, err := input.queryParams(defaultTraceExportLimit, maxTraceExportLimit)
		if err != nil {
			return nil, nil, err
		}

		result, err := tracing.ExportTraces(ctx, params)
		if err != nil {
			return nil, nil, wrapToolError("get_traces", err)
		}
		return handleToolResult(result, nil)
	}
}

// getTraceDetails returns the handler for the trace details tool.
func getTraceDetails(tracing *controllers.TracingController, authorize func(*gomcp.CallToolRequest) error) func(context.Context, *gomcp.CallToolRequest, traceDetailsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *gomcp.CallToolRequest, input traceDetailsInput) (*gomcp.CallToolResult, any, error) {
		if err := authorize(req); err != nil {
			return nil, nil, err
		}
		organization, err := requireField(input.Organization, "organization")
		if err != nil {
			return nil, nil, err
		}
		traceID, err := requireField(input.TraceID, "trace_id")
		if err != nil {
			return nil, nil, err
		}

		startTime, endTime, err := resolveTraceTimeWindow(input.StartTime, input.EndTime)
		if err != nil {
			return nil, nil, err
		}

		params := controllers.TraceQueryParams{
			Organization: organization,
			Project:      optionalScope(input.Project),
			Agent:        optionalScope(input.Agent),
			Environment:  optionalScope(input.Environment),
			StartTime:    startTime,
			EndTime:      endTime,
			Limit:        traceDetailsSpanLimit,
			SortOrder:    traceDetailsSortOrder,
		}

		result, err := tracing.GetTraceSpans(ctx, traceID, params)
		if err != nil {
			return nil, nil, wrapToolError("get_trace_details", err)
		}
		return handleToolResult(result, nil)
	}
}

// getSpanDetails returns the handler for the span details tool.
func getSpanDetails(tracing *controllers.TracingController, authorize func(*gomcp.CallToolRequest) error) func(context.Context, *gomcp.CallToolRequest, spanDetailsInput) (*gomcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *gomcp.CallToolRequest, input spanDetailsInput) (*gomcp.CallToolResult, any, error) {
		if err := authorize(req); err != nil {
			return nil, nil, err
		}
		traceID, err := requireField(input.TraceID, "trace_id")
		if err != nil {
			return nil, nil, err
		}
		spanID, err := requireField(input.SpanID, "span_id")
		if err != nil {
			return nil, nil, err
		}

		result, err := tracing.GetSpanDetail(ctx, traceID, spanID)
		if err != nil {
			return nil, nil, wrapToolError("get_span_details", err)
		}
		return handleToolResult(result, nil)
	}
}

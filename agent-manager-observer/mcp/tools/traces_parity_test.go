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
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-observer/controllers"
)

// getParams validates a get_traces input in the test window.
func getParams(t *testing.T, in tracesInput) controllers.TraceQueryParams {
	t.Helper()
	in.Organization, in.Project, in.Agent, in.Environment = testOrgName, testProjectName, testAgentName, testEnvName
	in.StartTime, in.EndTime = testStart, testEnd
	params, err := in.queryParams(defaultTraceExportLimit, maxTraceExportLimit)
	if err != nil {
		t.Fatalf("queryParams: %v", err)
	}
	return params
}

// callTraceToolErr calls a trace tool that should fail and returns its error text.
func callTraceToolErr(t *testing.T, session *gomcp.ClientSession, tool string, extra map[string]any) string {
	t.Helper()
	args := map[string]any{
		"organization": testOrgName,
		"project":      testProjectName,
		"agent":        testAgentName,
		"environment":  testEnvName,
	}
	maps.Copy(args, extra)
	result, err := session.CallTool(context.Background(), &gomcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return err.Error()
	}
	if !result.IsError {
		t.Fatalf("expected an error, got success")
	}
	return result.Content[0].(*gomcp.TextContent).Text
}

// missing reaches TraceFilters on both tools, and unset leaves the filters empty.
func TestTraceMissingInputReachesFilters(t *testing.T) {
	want := controllers.TraceFilters{Missing: controllers.MissingFields{ConversationID: true}}
	in := tracesInput{Missing: []string{"conversation_id"}}
	if got := listParams(t, listTracesInput{tracesInput: in}).Filters; got != want {
		t.Errorf("list_traces filters = %+v, want %+v", got, want)
	}
	if got := getParams(t, in).Filters; got != want {
		t.Errorf("get_traces filters = %+v, want %+v", got, want)
	}
	if got := listParams(t, listTracesInput{}).Filters; !got.IsZero() {
		t.Errorf("unset missing gave filters %+v, want none", got)
	}
}

// missing keeps only the traces without a conversation ID, on both tools.
func TestTraceMissingFilter(t *testing.T) {
	for _, tool := range []string{"list_traces", "get_traces"} {
		t.Run(tool, func(t *testing.T) {
			session, fake := setupTestServer(t)
			withFilterTraces(fake)
			fake.spans["trace-0"][0].Attributes["gen_ai.conversation.id"] = "conv-1"

			out := callTraceTool(t, session, tool, map[string]any{"missing": []any{"conversation_id"}})

			if ids, _ := resultTraces(t, out); !slices.Equal(ids, []string{"trace-1", "trace-2"}) {
				t.Errorf("traces = %v, want [trace-1 trace-2]", ids)
			}
		})
	}
}

// A bad missing input is rejected with the handler's message, in the tools' spelling.
func TestTraceMissingInputRejected(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"unknown field", map[string]any{"missing": []any{"model"}}, `invalid missing value "model": must be 'conversation_id'`},
		{"API spelling", map[string]any{"missing": []any{"conversationId"}}, `invalid missing value "conversationId": must be 'conversation_id'`},
		{
			"with conversation_id",
			map[string]any{"missing": []any{"conversation_id"}, "conversation_id": "conv-1"},
			"conversation_id and missing=conversation_id can't be combined",
		},
	}
	for _, tc := range cases {
		for _, tool := range []string{"list_traces", "get_traces"} {
			t.Run(tc.name+"/"+tool, func(t *testing.T) {
				session, fake := setupTestServer(t)

				if got := callTraceToolErr(t, session, tool, tc.args); got != tc.want {
					t.Errorf("error = %q, want %q", got, tc.want)
				}
				if calls := fake.calls["QueryTraces"]; len(calls) != 0 {
					t.Errorf("QueryTraces calls = %d, want 0", len(calls))
				}
			})
		}
	}
}

// consoleParity maps each console trace-list param to the tools' input. Keep it in
// step with TRACE_FILTER_KEYS in console/workspaces/pages/traces/src/traceFilters.ts.
var consoleParity = []struct {
	param string
	// input and value are what an MCP client sends.
	input string
	value any
	// want is the TraceFilters the HTTP handler sets for the param, if it's a filter.
	want controllers.TraceFilters
	// listOnly marks inputs get_traces doesn't take, as the export route doesn't.
	listOnly bool
}{
	{param: "status", input: "status", value: "error", want: controllers.TraceFilters{Status: controllers.TraceStatusError}},
	{param: "minDurationMs", input: "min_duration_ms", value: 5000, want: controllers.TraceFilters{MinDurationMs: ptr(int64(5000))}},
	{param: "minTokens", input: "min_tokens", value: 0, want: controllers.TraceFilters{MinTokens: ptr(int64(0))}},
	{param: "minSpanCount", input: "min_span_count", value: 20, want: controllers.TraceFilters{MinSpanCount: ptr(int64(20))}},
	{param: "model", input: "model", value: "gpt-4o", want: controllers.TraceFilters{Model: "gpt-4o"}},
	{param: "conversationId", input: "conversation_id", value: "conv-1", want: controllers.TraceFilters{ConversationID: "conv-1"}},
	{param: "missing", input: "missing", value: []any{"conversation_id"}, want: controllers.TraceFilters{Missing: controllers.MissingFields{ConversationID: true}}},
	{param: "tool", input: "tool", value: "search", want: controllers.TraceFilters{Tool: "search"}},
	{param: "toolError", input: "tool_error", value: true, want: controllers.TraceFilters{ToolError: true}},
	{param: "mcpServer", input: "mcp_server", value: "github", want: controllers.TraceFilters{MCPServer: "github"}},
	{param: "startTime", input: "start_time", value: "2026-10-01T00:00:00Z"},
	{param: "endTime", input: "end_time", value: "2026-10-01T01:00:00Z"},
	{param: "sortOrder", input: "sort_order", value: "asc"},
	{param: "limit", input: "limit", value: 25},
	{param: "cursor", input: "cursor", listOnly: true},
}

// Every console param is an input on list_traces and get_traces.
func TestConsoleParamParity(t *testing.T) {
	session, _ := setupTestServer(t)
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	inputs := map[string]map[string]any{}
	for _, tool := range tools.Tools {
		if tool.Name == "list_traces" || tool.Name == "get_traces" {
			schema, ok := tool.InputSchema.(map[string]any)
			if !ok {
				t.Fatalf("%s input schema is %T", tool.Name, tool.InputSchema)
			}
			inputs[tool.Name], _ = schema["properties"].(map[string]any)
		}
	}
	for _, p := range consoleParity {
		t.Run(p.param, func(t *testing.T) {
			for _, tool := range []string{"list_traces", "get_traces"} {
				_, ok := inputs[tool][p.input]
				if want := tool == "list_traces" || !p.listOnly; ok != want {
					t.Errorf("%s has input %q = %v, want %v", tool, p.input, ok, want)
				}
			}
		})
	}
}

// Each console filter's input sets the TraceFilters field the HTTP handler sets for it.
func TestConsoleFilterParity(t *testing.T) {
	for _, p := range consoleParity {
		if p.want.IsZero() {
			continue
		}
		t.Run(p.param, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{p.input: p.value})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var in listTracesInput
			if err := json.Unmarshal(raw, &in); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := listParams(t, in).Filters; !reflect.DeepEqual(got, p.want) {
				t.Errorf("list_traces filters = %+v, want %+v", got, p.want)
			}
			if got := getParams(t, in.tracesInput).Filters; !reflect.DeepEqual(got, p.want) {
				t.Errorf("get_traces filters = %+v, want %+v", got, p.want)
			}
		})
	}
}

// ptr returns a pointer to v.
func ptr[T any](v T) *T { return &v }

// The window inputs reach the params as the handler reads startTime and endTime.
func TestConsoleWindowParity(t *testing.T) {
	limit := 25
	in := tracesInput{SortOrder: "asc", Limit: &limit}
	params := getParams(t, in)
	start, _ := time.Parse(time.RFC3339, testStart)
	end, _ := time.Parse(time.RFC3339, testEnd)
	if !params.StartTime.Equal(start) || !params.EndTime.Equal(end) || params.SortOrder != "asc" || params.Limit != 25 {
		t.Errorf("params = %+v, want the test window, asc and limit 25", params)
	}
}

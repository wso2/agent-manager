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
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-observer/controllers"
	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

const (
	testStart = "2026-10-01T00:00:00Z"
	testEnd   = "2026-10-01T01:00:00Z"
)

// listParams validates a list_traces input in the test window.
func listParams(t *testing.T, in listTracesInput) controllers.TraceQueryParams {
	t.Helper()
	in.Organization, in.Project, in.Agent, in.Environment = testOrgName, testProjectName, testAgentName, testEnvName
	in.StartTime, in.EndTime = testStart, testEnd
	params, err := in.listQueryParams()
	if err != nil {
		t.Fatalf("listQueryParams: %v", err)
	}
	return params
}

// Each new input reaches the controller params, and tool with tool_error arrives paired.
func TestListTracesToolInputsReachParams(t *testing.T) {
	cases := []struct {
		name        string
		in          listTracesInput
		wantFilters controllers.TraceFilters
		wantInclude controllers.Include
	}{
		{"tool", listTracesInput{tracesInput: tracesInput{Tool: "search_web"}}, controllers.TraceFilters{Tool: "search_web"}, controllers.Include{}},
		{"tool_error", listTracesInput{tracesInput: tracesInput{ToolError: true}}, controllers.TraceFilters{ToolError: true}, controllers.Include{}},
		{
			"tool with tool_error",
			listTracesInput{tracesInput: tracesInput{Tool: "search_web", ToolError: true}},
			controllers.TraceFilters{Tool: "search_web", ToolError: true},
			controllers.Include{},
		},
		{"mcp_server", listTracesInput{tracesInput: tracesInput{MCPServer: "github"}}, controllers.TraceFilters{MCPServer: "github"}, controllers.Include{}},
		{"include_tools", listTracesInput{IncludeTools: true}, controllers.TraceFilters{}, controllers.Include{Tools: true}},
		{"include_mcp_servers", listTracesInput{IncludeMCPServers: true}, controllers.TraceFilters{}, controllers.Include{MCPServers: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := listParams(t, tc.in)
			if params.Filters != tc.wantFilters {
				t.Errorf("filters = %+v, want %+v", params.Filters, tc.wantFilters)
			}
			if params.Include != tc.wantInclude {
				t.Errorf("include = %+v, want %+v", params.Include, tc.wantInclude)
			}
		})
	}
}

// Existing inputs give the params they gave before the new inputs.
func TestListTracesExistingInputsParamsUnchanged(t *testing.T) {
	start, _ := time.Parse(time.RFC3339, testStart)
	end, _ := time.Parse(time.RFC3339, testEnd)
	minTokens := int64(100)
	project, agent, env := testProjectName, testAgentName, testEnvName
	base := controllers.TraceQueryParams{
		Organization: testOrgName, Project: &project, Agent: &agent, Environment: &env,
		StartTime: start, EndTime: end,
		Limit: defaultTraceListLimit, SortOrder: "desc",
	}
	withFilters := base
	withFilters.Filters = controllers.TraceFilters{Status: controllers.TraceStatusError, MinTokens: &minTokens, Model: "gpt"}
	withFilters.Include = controllers.Include{Models: true}

	cases := []struct {
		name string
		in   listTracesInput
		want controllers.TraceQueryParams
	}{
		{"no inputs", listTracesInput{}, base},
		{
			"Milestone 1 filters and include_models",
			listTracesInput{tracesInput: tracesInput{Status: "error", MinTokens: &minTokens, Model: "gpt"}, IncludeModels: true},
			withFilters,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := listParams(t, tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("params = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// withToolTraces gives fake three traces a minute apart, newest first. trace-0
// calls search_web; trace-1 calls search_web and fails search_issues, whose MCP
// handshake names github-mcp-server; trace-2 fails search_web.
func withToolTraces(fake *fakeObserverClient) *fakeObserverClient {
	now := time.Now().UTC()
	fake.spans = map[string][]observer.SpanInfo{}
	for i, id := range []string{"trace-0", "trace-1", "trace-2"} {
		start := now.Add(-time.Duration(i+1) * time.Minute)
		var spans []observer.SpanInfo
		add := func(suffix, name, parent, status string) observer.SpanInfo {
			at := start.Add(time.Duration(len(spans)) * time.Millisecond)
			s := observer.SpanInfo{
				SpanID: id + "-" + suffix, SpanName: name, StartTime: at, EndTime: at.Add(time.Millisecond),
				Status: &observer.SpanStatus{Code: status}, Attributes: map[string]interface{}{},
			}
			if parent != "" {
				s.ParentSpanID = id + "-" + parent
			}
			spans = append(spans, s)
			return s
		}
		root := add("root", "invoke_agent", "", "ok")
		root.EndTime = start.Add(time.Second)
		spans[0] = root
		switch id {
		case "trace-0":
			add("web", "execute_tool search_web", "root", "ok")
		case "trace-1":
			add("web", "execute_tool search_web", "root", "ok")
			add("issues", "execute_tool search_issues", "root", "unset")
			add("issues-mcp", "search_issues.tool", "issues", "error")
			add("init", "initialize.mcp", "issues-mcp", "unset").Attributes["traceloop.entity.output"] =
				`{"serverInfo":{"name":"github-mcp-server","title":"GitHub MCP Server"}}`
		case "trace-2":
			add("web", "execute_tool search_web", "root", "error")
		}
		fake.spans[id] = spans
		fake.traces = append(fake.traces, observer.TraceInfo{
			TraceID: id, RootSpanID: root.SpanID, RootSpanName: root.SpanName, SpanCount: len(spans),
			StartTime: start, EndTime: root.EndTime, DurationNs: time.Second.Nanoseconds(),
		})
	}
	return fake
}

// traceField returns each result trace's value for key, by trace ID; traces without it are left out.
func traceField(t *testing.T, out map[string]any, key string) map[string]any {
	t.Helper()
	fields := map[string]any{}
	for _, tr := range out["traces"].([]any) {
		m := tr.(map[string]any)
		if v, ok := m[key]; ok {
			fields[m["traceId"].(string)] = v
		}
	}
	return fields
}

// handshakeFetches counts GetSpanDetails calls for MCP handshake spans.
func handshakeFetches(fake *fakeObserverClient) int {
	n := 0
	for _, call := range fake.calls["GetSpanDetails"] {
		if strings.HasSuffix(call.([]any)[1].(string), "-init") {
			n++
		}
	}
	return n
}

// The new filters narrow list_traces and get_traces alike.
func TestTraceToolFilters(t *testing.T) {
	cases := []struct {
		name    string
		args    map[string]any
		wantIDs []string
	}{
		{"tool, ignoring case", map[string]any{"tool": "Search_Web"}, []string{"trace-0", "trace-1", "trace-2"}},
		{"tool substring", map[string]any{"tool": "issues"}, []string{"trace-1"}},
		{"tool_error", map[string]any{"tool_error": true}, []string{"trace-1", "trace-2"}},
		{"tool with tool_error is paired", map[string]any{"tool": "search_web", "tool_error": true}, []string{"trace-2"}},
		{"mcp_server", map[string]any{"mcp_server": "GITHUB"}, []string{"trace-1"}},
		{"mcp_server with no match", map[string]any{"mcp_server": "slack"}, nil},
	}
	for _, tc := range cases {
		for _, tool := range []string{"list_traces", "get_traces"} {
			t.Run(tc.name+"/"+tool, func(t *testing.T) {
				session, fake := setupTestServer(t)
				withToolTraces(fake)

				out := callTraceTool(t, session, tool, tc.args)

				if ids, _ := resultTraces(t, out); !slices.Equal(ids, tc.wantIDs) {
					t.Errorf("traces = %v, want %v", ids, tc.wantIDs)
				}
			})
		}
	}
}

// list_traces fills tools, failedTools and mcpServers only when an input asks for them.
func TestListTracesToolFields(t *testing.T) {
	tools := map[string]any{
		"trace-0": []any{"search_web"},
		"trace-1": []any{"search_web", "search_issues"},
		"trace-2": []any{"search_web"},
	}
	failedTools := map[string]any{"trace-1": []any{"search_issues"}, "trace-2": []any{"search_web"}}
	mcpServers := map[string]any{"trace-1": []any{"github-mcp-server"}}
	none := map[string]any{}
	cases := []struct {
		name                  string
		args                  map[string]any
		wantTools, wantFailed map[string]any
		wantMCPServers        map[string]any
		wantHandshakes        int
	}{
		{"no inputs", nil, none, none, none, 0},
		{"Milestone 1 inputs", map[string]any{"status": "ok", "include_models": true}, none, none, none, 0},
		{"include_tools", map[string]any{"include_tools": true}, tools, failedTools, none, 0},
		{"tool_error implies tools", map[string]any{"tool_error": true}, none, failedTools, none, 0},
		{"include_mcp_servers", map[string]any{"include_mcp_servers": true}, none, none, mcpServers, 1},
		{"mcp_server implies mcpServers", map[string]any{"mcp_server": "github"}, none, none, mcpServers, 1},
	}
	// tool_error leaves only the traces with a failed tool, so its tools are those traces' tools.
	cases[3].wantTools = map[string]any{"trace-1": tools["trace-1"], "trace-2": tools["trace-2"]}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			session, fake := setupTestServer(t)
			withToolTraces(fake)

			out := callTraceTool(t, session, "list_traces", tc.args)

			for _, f := range []struct {
				key  string
				want map[string]any
			}{{"tools", tc.wantTools}, {"failedTools", tc.wantFailed}, {"mcpServers", tc.wantMCPServers}} {
				if got := traceField(t, out, f.key); !reflect.DeepEqual(got, f.want) {
					t.Errorf("%s = %v, want %v", f.key, got, f.want)
				}
			}
			if got := handshakeFetches(fake); got != tc.wantHandshakes {
				t.Errorf("handshake fetches = %d, want %d", got, tc.wantHandshakes)
			}
		})
	}
}

// Bad new inputs are rejected before any upstream call, and get_traces takes no new include.
func TestTraceToolInputsRejected(t *testing.T) {
	long := strings.Repeat("x", controllers.MaxFilterValueLen+1)
	cases := []struct {
		name  string
		tools []string
		args  map[string]any
	}{
		{"long tool", []string{"list_traces", "get_traces"}, map[string]any{"tool": long}},
		{"long mcp_server", []string{"list_traces", "get_traces"}, map[string]any{"mcp_server": long}},
		{"tool_error not a bool", []string{"list_traces", "get_traces"}, map[string]any{"tool_error": "yes"}},
		{"include_tools on get_traces", []string{"get_traces"}, map[string]any{"include_tools": true}},
		{"include_mcp_servers on get_traces", []string{"get_traces"}, map[string]any{"include_mcp_servers": true}},
	}
	for _, tc := range cases {
		for _, tool := range tc.tools {
			t.Run(tc.name+"/"+tool, func(t *testing.T) {
				session, fake := setupTestServer(t)
				args := map[string]any{
					"organization": testOrgName,
					"project":      testProjectName,
					"agent":        testAgentName,
					"environment":  testEnvName,
				}
				maps.Copy(args, tc.args)

				result, err := session.CallTool(context.Background(), &gomcp.CallToolParams{Name: tool, Arguments: args})
				if err == nil && (result == nil || !result.IsError) {
					t.Fatal("expected an error, got success")
				}
				if calls := fake.calls["QueryTraces"]; len(calls) != 0 {
					t.Errorf("QueryTraces calls = %d, want 0", len(calls))
				}
			})
		}
	}
}

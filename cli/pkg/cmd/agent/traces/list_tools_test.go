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

package traces

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clients/observersvc"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/config"
)

// toolFilterKeys are every filter and include param the list can send.
var toolFilterKeys = append(append([]string{}, filterKeys...), "tool", "toolError", "mcpServer")

// runTracesCmdOutput runs `traces my-agent` with args and returns the request, stdout and stderr.
func runTracesCmdOutput(t *testing.T, body any, args ...string) (*capturedRequest, *bytes.Buffer, *bytes.Buffer, error) {
	t.Helper()
	ios, out, errOut := newTraceTestIO(false)
	traceClient, captured, closeTraces := newCapturingTraceClient(t, body)
	defer closeTraces()
	am, closeAM := newAMTestClient(t, http.StatusOK)
	defer closeAM()

	f := &cmdutil.Factory{
		IOStreams:    ios,
		Observer:     traceClient,
		AgentManager: func(context.Context) (*amsvc.ClientWithResponses, error) { return am, nil },
		Config: func() (*config.Config, error) {
			return &config.Config{
				CurrentInstance: "default",
				Instances:       map[string]config.Instance{"default": {URL: "http://test", CurrentOrg: "acme"}},
			}, nil
		},
	}
	root := &cobra.Command{Use: "amctl", SilenceErrors: true, SilenceUsage: true}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return cmdutil.FlagErrorWrap(err) })
	cmdutil.EnableOrgOverride(root, f)
	cmdutil.EnableProjectOverride(root, f)
	root.AddCommand(NewTracesCmd(f))
	root.SetArgs(append([]string{"traces", "my-agent", "--project", "triage", "--env", "dev"}, args...))
	err := root.Execute()
	return captured, out, errOut, err
}

// assertToolFilterQuery checks every filter and include param is exactly want.
func assertToolFilterQuery(t *testing.T, q url.Values, want map[string]string) {
	t.Helper()
	for _, key := range toolFilterKeys {
		wantVal, set := want[key]
		if !set {
			if q.Has(key) {
				t.Errorf("%s = %q, want unset", key, q.Get(key))
			}
			continue
		}
		if q.Get(key) != wantVal {
			t.Errorf("%s = %q, want %q", key, q.Get(key), wantVal)
		}
	}
}

func TestTracesCmd_ToolAndMCPFlagsReachTheServer(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want map[string]string
	}{
		// A filter implies its include on the server, so the CLI sends none.
		{name: "tool", args: []string{"--tool", "search_web"}, want: map[string]string{"tool": "search_web"}},
		{name: "tool-error", args: []string{"--tool-error"}, want: map[string]string{"toolError": "true"}},
		{name: "tool-error=false sends nothing", args: []string{"--tool-error=false"}, want: map[string]string{}},
		{
			name: "tool and tool-error pair",
			args: []string{"--tool", "search_web", "--tool-error"},
			want: map[string]string{"tool": "search_web", "toolError": "true"},
		},
		{name: "mcp-server", args: []string{"--mcp-server", "github"}, want: map[string]string{"mcpServer": "github"}},
		{name: "show-tools", args: []string{"--show-tools"}, want: map[string]string{"include": "tools"}},
		{name: "show-mcp-servers", args: []string{"--show-mcp-servers"}, want: map[string]string{"include": "mcpServers"}},
		{
			name: "every include",
			args: []string{"--show-models", "--show-tools", "--show-mcp-servers"},
			want: map[string]string{"include": "models,tools,mcpServers"},
		},
		{
			name: "with the other filters and a condition",
			args: []string{"--condition", "error_status", "--model", "gpt", "--tool", "search", "--mcp-server", "git"},
			want: map[string]string{"status": "error", "model": "gpt", "tool": "search", "mcpServer": "git"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			captured, _, _, err := runTracesCmdOutput(t, observersvc.TraceOverviewResponse{}, tc.args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if captured.calls != 1 || captured.path != "/api/v1/traces" {
				t.Errorf("calls = %d, path = %q; want one list call", captured.calls, captured.path)
			}
			assertToolFilterQuery(t, captured.query, tc.want)
		})
	}
}

func TestTracesCmd_RejectsInvalidToolFlags(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		wantErr string
	}{
		{[]string{"--tool-error=maybe"}, "--tool-error"},
		{[]string{"--condition", "tool_call_fails", "--tool-error"}, "--tool-error can't be used with --condition tool_call_fails"},
		{[]string{"--condition", "tool_call_fails", "--show-tools"}, "--show-tools can't be used with --condition tool_call_fails"},
		{[]string{"--condition", "tool_call_fails", "--show-mcp-servers"}, "--show-mcp-servers can't be used with --condition tool_call_fails"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			captured, _, _, err := runTracesCmdOutput(t, observersvc.TraceOverviewResponse{}, tc.args...)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
			if captured.calls != 0 {
				t.Error("observer was called despite an invalid flag")
			}
		})
	}
}

func TestTracesCmd_ToolCallFailsSendsNewFiltersToExport(t *testing.T) {
	captured, _, _, err := runTracesCmdOutput(t, observersvc.TraceExportResponse{},
		"--condition", "tool_call_fails", "--tool", "search", "--mcp-server", "github")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.path != "/api/v1/traces/export" {
		t.Errorf("path = %q, want the export endpoint", captured.path)
	}
	assertToolFilterQuery(t, captured.query, map[string]string{"tool": "search", "mcpServer": "github"})
}

// tool_call_fails stays client-side because it sees failures the server's
// toolError can't: tool spans known only by attributes, and failures marked
// only by error.type.
func TestToolCallFails_MatchesFailuresOnlyAttributesShow(t *testing.T) {
	export := observersvc.TraceExportResponse{Traces: []observersvc.FullTrace{
		{
			// A function-named tool span: the server's name rule doesn't call it a tool.
			TraceOverview: observersvc.TraceOverview{TraceID: "named-after-function", RootSpanName: "agent"},
			Spans: []observersvc.Span{{
				Name: "search_web", Status: "error",
				AmpAttributes: &observersvc.AmpAttributes{Kind: "tool", Status: &observersvc.SpanStatus{Error: true}},
			}},
		},
		{
			// error.type only, with the OTel status unset.
			TraceOverview: observersvc.TraceOverview{TraceID: "error-type-only", RootSpanName: "agent"},
			Spans: []observersvc.Span{{
				Name: "search_issues.tool", Status: "unset",
				AmpAttributes: &observersvc.AmpAttributes{Kind: "tool", Status: &observersvc.SpanStatus{Error: true, ErrorType: "tool_error"}},
			}},
		},
		{
			TraceOverview: observersvc.TraceOverview{TraceID: "tool-ok", RootSpanName: "agent"},
			Spans: []observersvc.Span{{
				Name: "execute_tool search_issues", Status: "unset",
				AmpAttributes: &observersvc.AmpAttributes{Kind: "tool", Status: &observersvc.SpanStatus{}},
			}},
		},
	}}
	ios, out, _ := newTraceTestIO(false)
	client, captured, closeFn := newCapturingTraceClient(t, export)
	defer closeFn()

	err := runToolCallFails(context.Background(), &ListTracesOptions{
		IO: ios, TraceClient: client, Scope: traceBaseScope(), AgentName: "my-agent", Limit: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.query.Has("toolError") {
		t.Errorf("tool_call_fails should not send toolError, got %q", captured.query.Get("toolError"))
	}
	got := out.String()
	for _, id := range []string{"named-after-function", "error-type-only"} {
		if !strings.Contains(got, id) {
			t.Errorf("table should list %s, got %q", id, got)
		}
	}
	if strings.Contains(got, "tool-ok") {
		t.Errorf("table should leave out a trace whose tools succeeded, got %q", got)
	}
}

func TestListTraces_ToolsAndMCPServersColumns(t *testing.T) {
	resp := observersvc.TraceOverviewResponse{
		TotalCount: 1,
		Traces: []observersvc.TraceOverview{{
			TraceID:      "abc123",
			RootSpanName: "handle_request",
			Tools:        []string{"search_issues", "create_ticket"},
			FailedTools:  []string{"search_issues"},
			MCPServers:   []string{"github-mcp-server"},
		}},
	}
	for _, tc := range []struct {
		name        string
		opts        ListTracesOptions
		wantTools   bool
		wantMCP     bool
		wantInclude string
	}{
		{name: "hidden by default", opts: ListTracesOptions{}},
		{name: "show-tools", opts: ListTracesOptions{ShowTools: true}, wantTools: true, wantInclude: "tools"},
		{name: "tool filter implies it", opts: ListTracesOptions{Tool: "search"}, wantTools: true},
		{name: "tool-error implies it", opts: ListTracesOptions{ToolError: true}, wantTools: true},
		{name: "show-mcp-servers", opts: ListTracesOptions{ShowMCPServers: true}, wantMCP: true, wantInclude: "mcpServers"},
		{name: "mcp-server filter implies it", opts: ListTracesOptions{MCPServer: "github"}, wantMCP: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ios, out, _ := newTraceTestIO(false)
			client, captured, closeFn := newCapturingTraceClient(t, resp)
			defer closeFn()

			o := tc.opts
			o.IO, o.TraceClient, o.Scope = ios, client, traceBaseScope()
			o.AgentName, o.Limit = "my-agent", 10
			if err := runListTraces(context.Background(), &o); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := out.String()
			if strings.Contains(got, "TOOLS") != tc.wantTools {
				t.Errorf("Tools column shown = %v, want %v: %q", !tc.wantTools, tc.wantTools, got)
			}
			if tc.wantTools && !strings.Contains(got, "search_issues (failed), create_ticket") {
				t.Errorf("Tools column should list every tool and mark the failed one, got %q", got)
			}
			if strings.Contains(got, "MCP SERVERS") != tc.wantMCP {
				t.Errorf("MCP Servers column shown = %v, want %v: %q", !tc.wantMCP, tc.wantMCP, got)
			}
			if tc.wantMCP && !strings.Contains(got, "github-mcp-server") {
				t.Errorf("MCP Servers column should list the server, got %q", got)
			}
			if captured.query.Get("include") != tc.wantInclude {
				t.Errorf("include = %q, want %q", captured.query.Get("include"), tc.wantInclude)
			}
		})
	}
}

func TestToolList(t *testing.T) {
	for _, tc := range []struct {
		tools, failed []string
		want          string
	}{
		{nil, nil, "-"},
		{[]string{"search"}, nil, "search"},
		{[]string{"search", "fetch"}, []string{"fetch"}, "search, fetch (failed)"},
		{[]string{"a", "b"}, []string{"a", "b"}, "a (failed), b (failed)"},
	} {
		if got := toolList(tc.tools, tc.failed); got != tc.want {
			t.Errorf("toolList(%v, %v) = %q, want %q", tc.tools, tc.failed, got, tc.want)
		}
	}
}

// The default command sends today's request and prints today's table.
func TestTracesCmd_DefaultRequestAndTableUnchanged(t *testing.T) {
	resp := observersvc.TraceOverviewResponse{
		TotalCount: 1,
		Traces: []observersvc.TraceOverview{{
			TraceID:         "abc123",
			RootSpanName:    "handle_request",
			StartTime:       "2026-05-12",
			DurationInNanos: 1_500_000_000,
			SpanCount:       7,
			TokenUsage:      &observersvc.TokenUsage{TotalTokens: 42},
			ConversationID:  "conv-42",
			// Fields the server leaves out by default never reach the table.
			Tools:       []string{"search_issues"},
			FailedTools: []string{"search_issues"},
			MCPServers:  []string{"github-mcp-server"},
		}},
	}
	captured, out, errOut, err := runTracesCmdOutput(t, resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	keys := make([]string, 0, len(captured.query))
	for k := range captured.query {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	wantKeys := "agent,endTime,environment,limit,organization,project,sortOrder,startTime"
	if got := strings.Join(keys, ","); got != wantKeys {
		t.Errorf("query keys = %s, want %s", got, wantKeys)
	}
	if captured.query.Get("limit") != "10" || captured.query.Get("sortOrder") != "desc" {
		t.Errorf("limit = %q, sortOrder = %q; want 10 and desc", captured.query.Get("limit"), captured.query.Get("sortOrder"))
	}
	bold := func(s string) string { return "\x1b[1m" + s + "\x1b[0m" }
	wantTable := strings.Join([]string{
		bold("STATUS"), bold("TRACE ID"), bold("CONVERSATION"), bold("STARTED   "),
		bold("DURATION"), bold("TOKENS"), bold("SPANS"),
	}, "  ") + "\n" +
		"ok      abc123    conv-42       2026-05-12  1.5s      42      7\n"
	if out.String() != wantTable {
		t.Errorf("table =\n%s\nwant\n%s", out.String(), wantTable)
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want empty", errOut.String())
	}
}

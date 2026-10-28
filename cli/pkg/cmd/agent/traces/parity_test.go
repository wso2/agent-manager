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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clients/observersvc"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/config"
)

const (
	testStart = "2026-10-14T09:00:00Z"
	testEnd   = "2026-10-14T10:00:00Z"
)

// amctlRun is one amctl run's output and the observer requests it made.
type amctlRun struct {
	out, errOut string
	err         error
	requests    []url.Values
	paths       []string
}

// runAmctl runs amctl with args against handler as the observer.
func runAmctl(t *testing.T, handler http.HandlerFunc, jsonMode bool, args ...string) amctlRun {
	t.Helper()
	var run amctlRun
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		run.requests = append(run.requests, r.URL.Query())
		run.paths = append(run.paths, r.URL.Path)
		handler(w, r)
	}))
	defer server.Close()
	client, err := observersvc.NewClient(server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	am, closeAM := newAMTestClient(t, http.StatusOK)
	defer closeAM()

	ios, out, errOut := newTraceTestIO(jsonMode)
	f := &cmdutil.Factory{
		IOStreams:    ios,
		Observer:     func(context.Context) (*observersvc.Client, error) { return client, nil },
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
	root.AddCommand(NewTracesCmd(f), NewTraceCmd(f))
	root.SetArgs(args)
	run.err = root.Execute()
	run.out, run.errOut = out.String(), errOut.String()
	return run
}

// serve answers every request with body.
func serve(body any) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}
}

var (
	listArgs   = []string{"traces", "my-agent", "--project", "triage", "--env", "dev"}
	exportArgs = []string{"traces", "export", "my-agent", "--project", "triage", "--env", "dev"}
)

// consoleParamFlags maps each console trace-list param to the CLI flags that send
// it. Keep it in step with TRACE_FILTER_KEYS in
// console/workspaces/pages/traces/src/traceFilters.ts and the window, sortOrder,
// limit and cursor handling in Traces.Component.tsx.
var consoleParamFlags = []struct {
	param string
	args  []string
	// want is the query value the flags send; "" checks only that the param is sent.
	want string
	// listOnly marks params export doesn't take.
	listOnly bool
}{
	{param: "status", args: []string{"--status", "error"}, want: "error"},
	{param: "minDurationMs", args: []string{"--min-duration", "5s"}, want: "5000"},
	{param: "minTokens", args: []string{"--min-tokens", "5000"}, want: "5000"},
	{param: "minSpanCount", args: []string{"--min-spans", "20"}, want: "20"},
	{param: "model", args: []string{"--model", "gpt-4o"}, want: "gpt-4o"},
	{param: "conversationId", args: []string{"--conversation", "conv-1"}, want: "conv-1"},
	{param: "missing", args: []string{"--no-conversation"}, want: "conversationId"},
	{param: "tool", args: []string{"--tool", "search"}, want: "search"},
	{param: "toolError", args: []string{"--tool-error"}, want: "true"},
	{param: "mcpServer", args: []string{"--mcp-server", "github"}, want: "github"},
	// The console's timeRange is a relative window, which the request sends as startTime and endTime.
	{param: "timeRange", args: []string{"--since", "1h"}},
	{param: "startTime", args: []string{"--start", testStart, "--end", testEnd}, want: testStart},
	{param: "endTime", args: []string{"--start", testStart, "--end", testEnd}, want: testEnd},
	{param: "sortOrder", args: []string{"--sort", "asc"}, want: "asc"},
	{param: "limit", args: []string{"--limit", "25"}, want: "25"},
	{param: "cursor", args: []string{"--cursor", "abc"}, want: "abc", listOnly: true},
}

// Every console param has a flag on list and export that sends it.
func TestConsoleParamParity(t *testing.T) {
	for _, p := range consoleParamFlags {
		for _, cmd := range []struct {
			name string
			base []string
		}{{"list", listArgs}, {"export", exportArgs}} {
			if cmd.name == "export" && p.listOnly {
				continue
			}
			t.Run(p.param+"/"+cmd.name, func(t *testing.T) {
				args := append(slices.Clone(cmd.base), p.args...)
				if cmd.name == "export" && !slices.ContainsFunc(p.args, isWindowFlag) {
					args = append(args, "--since", "1h")
				}
				run := runAmctl(t, serve(map[string]any{"traces": []any{}}), false, args...)
				if run.err != nil {
					t.Fatalf("unexpected error: %v", run.err)
				}
				q := run.requests[0]
				if p.param == "timeRange" {
					start, _ := time.Parse(time.RFC3339, q.Get("startTime"))
					end, _ := time.Parse(time.RFC3339, q.Get("endTime"))
					if end.Sub(start) != time.Hour {
						t.Errorf("window = %s to %s, want one hour", q.Get("startTime"), q.Get("endTime"))
					}
					return
				}
				if got := q.Get(p.param); got != p.want {
					t.Errorf("%s = %q, want %q", p.param, got, p.want)
				}
			})
		}
	}
}

// isWindowFlag reports whether arg sets the window.
func isWindowFlag(arg string) bool {
	return arg == "--since" || arg == "--start" || arg == "--end"
}

// Export rejects a cursor, since the export route has none.
func TestExportCmd_RejectsCursor(t *testing.T) {
	run := runAmctl(t, serve(map[string]any{}), true, append(slices.Clone(exportArgs), "--since", "1h", "--cursor", "abc")...)
	if run.err == nil || len(run.requests) != 0 {
		t.Errorf("err = %v, requests = %d; want an error and no request", run.err, len(run.requests))
	}
}

// The default list and export send exactly the scope, window, limit and sort they sent before.
func TestTracesCmd_DefaultRequestsUnchanged(t *testing.T) {
	want := []string{"agent", "endTime", "environment", "limit", "organization", "project", "sortOrder", "startTime"}
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"list", listArgs},
		{"export", append(slices.Clone(exportArgs), "--since", "1h")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := runAmctl(t, serve(map[string]any{"traces": []any{}}), false, tc.args...)
			if run.err != nil {
				t.Fatalf("unexpected error: %v", run.err)
			}
			var keys []string
			for k := range run.requests[0] {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			if !slices.Equal(keys, want) {
				t.Errorf("query keys = %v, want %v", keys, want)
			}
		})
	}
}

// A full page that wasn't truncated prints no next-page command, as before.
func TestListTraces_NoNoticeForAFullPage(t *testing.T) {
	resp := observersvc.TraceOverviewResponse{
		Traces:     []observersvc.TraceOverview{{TraceID: "abc123"}},
		NextCursor: "c",
	}
	run := runAmctl(t, serve(resp), false, listArgs...)
	if run.err != nil || run.errOut != "" {
		t.Errorf("err = %v, stderr = %q; want neither", run.err, run.errOut)
	}
}

// Each new flag reaches the export request, with the list's filter flags.
func TestExportCmd_FilterFlagsReachTheServer(t *testing.T) {
	args := append(slices.Clone(exportArgs),
		"--start", testStart, "--end", testEnd,
		"--status", "error", "--min-duration", "2s", "--min-tokens", "0", "--min-spans", "3",
		"--model", "gpt", "--no-conversation", "--tool", "search", "--tool-error", "--mcp-server", "github")
	run := runAmctl(t, serve(map[string]any{"traces": []any{}}), true, args...)
	if run.err != nil {
		t.Fatalf("unexpected error: %v", run.err)
	}
	if run.paths[0] != "/api/v1/traces/export" {
		t.Errorf("path = %q, want /api/v1/traces/export", run.paths[0])
	}
	q := run.requests[0]
	for key, want := range map[string]string{
		"startTime": testStart, "endTime": testEnd,
		"status": "error", "minDurationMs": "2000", "minTokens": "0", "minSpanCount": "3",
		"model": "gpt", "missing": "conversationId", "tool": "search", "toolError": "true", "mcpServer": "github",
	} {
		if got := q.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if q.Has("include") || q.Has("cursor") {
		t.Errorf("export sent include or cursor: %v", q)
	}
}

// Conflicting or invalid window and filter flags are flag errors on list and export, before any request.
func TestTracesCmd_RejectsInvalidWindowAndFilterFlags(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		listOnly bool
	}{
		{name: "no-conversation with conversation", args: []string{"--no-conversation", "--conversation", "c-1"}},
		{name: "start without end", args: []string{"--start", testStart}},
		{name: "end without start", args: []string{"--end", testEnd}},
		{name: "start after end", args: []string{"--start", testEnd, "--end", testStart}},
		{name: "start equal to end", args: []string{"--start", testStart, "--end", testStart}},
		{name: "start with since", args: []string{"--start", testStart, "--end", testEnd, "--since", "1h"}},
		{name: "start not RFC 3339", args: []string{"--start", "yesterday", "--end", testEnd}},
		{name: "invalid status", args: []string{"--status", "failed"}},
		{name: "cursor with tool_call_fails", args: []string{"--cursor", "c", "--condition", "tool_call_fails"}, listOnly: true},
	} {
		for _, cmd := range []struct {
			name string
			base []string
		}{{"list", listArgs}, {"export", exportArgs}} {
			if cmd.name == "export" && tc.listOnly {
				continue
			}
			t.Run(tc.name+"/"+cmd.name, func(t *testing.T) {
				args := append(slices.Clone(cmd.base), tc.args...)
				if cmd.name == "export" && !slices.ContainsFunc(tc.args, isWindowFlag) {
					args = append(args, "--since", "1h")
				}
				run := runAmctl(t, serve(map[string]any{}), false, args...)
				var flagErr *cmdutil.FlagError
				if !errors.As(run.err, &flagErr) {
					t.Errorf("err = %v, want a flag error", run.err)
				}
				if len(run.requests) != 0 {
					t.Errorf("observer was called %d times despite an invalid flag", len(run.requests))
				}
			})
		}
	}
}

// Export without --since, --start or --end is a flag error.
func TestExportCmd_NeedsAWindow(t *testing.T) {
	run := runAmctl(t, serve(map[string]any{}), true, exportArgs...)
	var flagErr *cmdutil.FlagError
	if !errors.As(run.err, &flagErr) || len(run.requests) != 0 {
		t.Errorf("err = %v, requests = %d; want a flag error and no request", run.err, len(run.requests))
	}
}

// pagingServer serves ids in pages of limit; every page stops early with a cursor while more remain.
func pagingServer(ids []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		from := 0
		if c := q.Get("cursor"); c != "" {
			n, err := strconv.Atoi(strings.TrimPrefix(c, "c"))
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(observersvc.ErrorResponse{Error: "error", Message: "invalid cursor"})
				return
			}
			from = n
		}
		to := min(from+limit, len(ids))
		resp := observersvc.TraceOverviewResponse{Traces: []observersvc.TraceOverview{}}
		for _, id := range ids[from:to] {
			resp.Traces = append(resp.Traces, observersvc.TraceOverview{TraceID: id})
		}
		if to < len(ids) {
			resp.Truncated = true
			resp.NextCursor = "c" + strconv.Itoa(to)
		}
		serve(resp)(w, r)
	}
}

// shellSplit splits a command line the way a POSIX shell does for unquoted and single-quoted words.
func shellSplit(s string) []string {
	var words []string
	var word strings.Builder
	inWord, quoted, escaped := false, false, false
	for _, r := range s {
		switch {
		case escaped:
			word.WriteRune(r)
			escaped = false
		case quoted && r == '\'':
			quoted = false
		case quoted:
			word.WriteRune(r)
		case r == '\'':
			quoted, inWord = true, true
		case r == '\\':
			escaped, inWord = true, true
		case r == ' ':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		words = append(words, word.String())
	}
	return words
}

// Pasting the printed next-page command pages through every trace once, with the same window and filters.
func TestListTraces_CursorPagesWithThePrintedCommand(t *testing.T) {
	ids := []string{"trace-a", "trace-b", "trace-c", "trace-d", "trace-e"}
	args := append(slices.Clone(listArgs), "--limit", "2", "--tool-error", "--model", "gpt 4o's", "--since", "1h")
	var out strings.Builder
	var queries []url.Values
	for page := 0; ; page++ {
		if page > len(ids) {
			t.Fatal("paging didn't stop")
		}
		run := runAmctl(t, pagingServer(ids), false, args...)
		if run.err != nil {
			t.Fatalf("page %d: %v", page, run.err)
		}
		out.WriteString(run.out)
		queries = append(queries, run.requests[0])
		lines := strings.Split(strings.TrimSpace(run.errOut), "\n")
		if run.errOut == "" {
			break
		}
		cmd := strings.TrimSpace(lines[len(lines)-1])
		words := shellSplit(cmd)
		if len(words) < 2 || words[0] != "amctl" {
			t.Fatalf("page %d printed %q, want an amctl command", page, cmd)
		}
		args = words[1:]
	}
	for _, id := range ids {
		if n := strings.Count(out.String(), id); n != 1 {
			t.Errorf("%s listed %d times, want once", id, n)
		}
	}
	if len(queries) != 3 {
		t.Fatalf("pages = %d, want 3", len(queries))
	}
	for i, q := range queries[1:] {
		for _, key := range []string{"startTime", "endTime", "limit", "sortOrder", "toolError", "model", "include"} {
			if q.Get(key) != queries[0].Get(key) {
				t.Errorf("page %d %s = %q, first page sent %q", i+1, key, q.Get(key), queries[0].Get(key))
			}
		}
	}
	if got := queries[0].Get("model"); got != "gpt 4o's" {
		t.Errorf("model = %q, want the quoted value back", got)
	}
}

// JSON output carries nextCursor when the server sends one, and --cursor takes it.
func TestListTraces_JSONCursorPages(t *testing.T) {
	ids := []string{"trace-a", "trace-b", "trace-c"}
	args := append(slices.Clone(listArgs), "--limit", "2", "--start", testStart, "--end", testEnd)

	first := runAmctl(t, pagingServer(ids), true, args...)
	if first.err != nil {
		t.Fatalf("unexpected error: %v", first.err)
	}
	data, _ := decodeEnvelope(t, first.out)["data"].(map[string]any)
	cursor, _ := data["nextCursor"].(string)
	if cursor != "c2" {
		t.Fatalf("nextCursor = %v, want c2", data["nextCursor"])
	}
	if first.errOut != "" {
		t.Errorf("JSON mode printed a notice: %q", first.errOut)
	}

	second := runAmctl(t, pagingServer(ids), true, append(args, "--cursor", cursor)...)
	if second.err != nil {
		t.Fatalf("unexpected error: %v", second.err)
	}
	data, _ = decodeEnvelope(t, second.out)["data"].(map[string]any)
	traces, _ := data["traces"].([]any)
	if len(traces) != 1 || traces[0].(map[string]any)["traceId"] != "trace-c" {
		t.Errorf("second page = %v, want [trace-c]", traces)
	}
	if _, ok := data["nextCursor"]; ok {
		t.Errorf("last page carries nextCursor: %v", data)
	}
}

// A cursor the server rejects is a flag error naming --cursor.
func TestListTraces_MalformedCursor(t *testing.T) {
	run := runAmctl(t, pagingServer([]string{"trace-a"}), false, append(slices.Clone(listArgs), "--cursor", "not-a-cursor")...)
	var flagErr *cmdutil.FlagError
	if !errors.As(run.err, &flagErr) || !strings.Contains(run.err.Error(), "--cursor") {
		t.Errorf("err = %v, want a flag error naming --cursor", run.err)
	}
}

// shellQuote leaves plain values alone and quotes the rest so a shell reads them back.
func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"gpt-4o":          "gpt-4o",
		testStart:         testStart,
		"gpt 4o":          "'gpt 4o'",
		"it's":            `'it'\''s'`,
		"$(rm -rf /)":     "'$(rm -rf /)'",
		"search;whoami":   "'search;whoami'",
		"":                "''",
		"conv-1/session2": "conv-1/session2",
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
		if in != "" {
			if got := shellSplit(shellQuote(in)); len(got) != 1 || got[0] != in {
				t.Errorf("shellSplit(shellQuote(%q)) = %q", in, got)
			}
		}
	}
}

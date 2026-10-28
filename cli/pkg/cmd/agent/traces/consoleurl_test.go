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
	"errors"
	"net/url"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/wso2/agent-manager/cli/pkg/clients/observersvc"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
)

const (
	consolePath = "/org/acme/project/triage/agents/my-agent/environment/dev/observability/traces"
	testTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
)

var traceArgs = []string{"trace", "my-agent", testTraceID, "--project", "triage", "--env", "dev"}

// emptyList answers list and export with no traces.
var emptyList = serve(map[string]any{"traces": []any{}})

// consoleLinkOf returns the console path a run printed to stderr.
func consoleLinkOf(t *testing.T, run amctlRun) string {
	t.Helper()
	for _, line := range strings.Split(run.errOut, "\n") {
		if link, ok := strings.CutPrefix(line, "Console path: "); ok {
			return link
		}
	}
	t.Fatalf("no console path in stderr %q (err %v)", run.errOut, run.err)
	return ""
}

// linkQuery parses a printed link's query.
func linkQuery(t *testing.T, link string) url.Values {
	t.Helper()
	_, raw, _ := strings.Cut(link, "?")
	q, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", link, err)
	}
	return q
}

// mustRun runs amctl and fails on an error or a missing request.
func mustRun(t *testing.T, args ...string) amctlRun {
	t.Helper()
	run := runAmctl(t, emptyList, false, args...)
	if run.err != nil || len(run.requests) == 0 {
		t.Fatalf("amctl %v: err = %v, requests = %d", args, run.err, len(run.requests))
	}
	return run
}

// windowOf is a request's window length and end.
func windowOf(q url.Values) (time.Duration, time.Time) {
	start, _ := time.Parse(time.RFC3339, q.Get("startTime"))
	end, _ := time.Parse(time.RFC3339, q.Get("endTime"))
	return end.Sub(start), end
}

// sameRequest checks got sent want's query; windows ending now may end a few seconds apart.
func sameRequest(t *testing.T, want, got url.Values) {
	t.Helper()
	keys := func(q url.Values) []string {
		var k []string
		for key := range q {
			k = append(k, key)
		}
		sort.Strings(k)
		return k
	}
	if !slices.Equal(keys(want), keys(got)) {
		t.Errorf("query keys = %v, want %v", keys(got), keys(want))
	}
	for key := range want {
		if key != "startTime" && key != "endTime" && got.Get(key) != want.Get(key) {
			t.Errorf("%s = %q, want %q", key, got.Get(key), want.Get(key))
		}
	}
	wantLen, wantEnd := windowOf(want)
	gotLen, gotEnd := windowOf(got)
	if gotLen != wantLen || gotEnd.Sub(wantEnd).Abs() > 5*time.Second {
		t.Errorf("window = %s to %s, want %s to %s",
			got.Get("startTime"), got.Get("endTime"), want.Get("startTime"), want.Get("endTime"))
	}
}

// Every console param in the Task 16 parity table has a row in consoleParams, and the reverse.
func TestConsoleParams_MatchTheParityTable(t *testing.T) {
	var parity, table []string
	for _, p := range consoleParamFlags {
		if p.param != "cursor" {
			parity = append(parity, p.param)
		}
	}
	for _, p := range consoleParams {
		table = append(table, p.param)
	}
	sort.Strings(parity)
	sort.Strings(table)
	if !slices.Equal(parity, table) {
		t.Errorf("consoleParams = %v, parity table = %v", table, parity)
	}
}

// Each param's flag writes it to the link, and the link read back sends the same list request.
func TestConsoleURL_EachParamBothWays(t *testing.T) {
	for _, p := range consoleParamFlags {
		if p.param == "cursor" {
			continue
		}
		t.Run(p.param, func(t *testing.T) {
			flags := mustRun(t, append(append(slices.Clone(listArgs), p.args...), "--console-url")...)
			link := consoleLinkOf(t, flags)
			if !strings.HasPrefix(link, consolePath) {
				t.Errorf("link = %q, want it under %s", link, consolePath)
			}
			q := linkQuery(t, link)
			switch p.param {
			case "timeRange":
				// --since 1h is the console's default window, so the link leaves it out.
				if len(q) != 0 {
					t.Errorf("link query = %v, want none", q)
				}
			default:
				if got := q.Get(p.param); got != p.want {
					t.Errorf("link %s = %q, want %q", p.param, got, p.want)
				}
			}

			back := mustRun(t, "traces", "--from-url", link)
			sameRequest(t, flags.requests[0], back.requests[0])
		})
	}
}

// Filters, window, sort and limit together survive flags → --console-url → --from-url.
func TestConsoleURL_RoundTrip(t *testing.T) {
	for _, args := range [][]string{
		{"--status", "ok", "--min-duration", "2.5s", "--model", "gpt 4o", "--tool", "search", "--tool-error",
			"--sort", "asc", "--limit", "50", "--since", "3h"},
		{"--no-conversation", "--mcp-server", "github", "--min-tokens", "0", "--start", testStart, "--end", testEnd},
		{"--conversation", "conv/1&2", "--since", "45m"},
	} {
		flags := mustRun(t, append(append(slices.Clone(listArgs), args...), "--console-url")...)
		back := mustRun(t, "traces", "--from-url", consoleLinkOf(t, flags))
		sameRequest(t, flags.requests[0], back.requests[0])
	}
}

// The example path, a full URL, a route path and a URL under a base path read to the same request.
func TestFromURL_FormsReadTheSame(t *testing.T) {
	query := "?sortOrder=desc&minDurationMs=5000&minTokens=5000&minSpanCount=20&columns=traceId,status&foo=bar"
	want := mustRun(t, append(slices.Clone(listArgs), "--min-duration", "5s", "--min-tokens", "5000",
		"--min-spans", "20", "--since", "1h")...)
	for _, args := range [][]string{
		append(slices.Clone(listArgs), "--from-url", "/traces"+query),
		append(slices.Clone(listArgs), "--from-url", query),
		{"traces", "--from-url", "http://localhost:3000" + consolePath + query},
		{"traces", "--from-url", consolePath + query},
		{"traces", "--from-url", "https://console.example.com/amp/console" + consolePath + "/" + query + "#top"},
		{"traces", "--from-url", "https://console.example.com/org/console" + consolePath + query},
	} {
		got := mustRun(t, args...)
		sameRequest(t, want.requests[0], got.requests[0])
	}
}

// --since presets become timeRange, other windows become startTime and endTime.
func TestConsoleURL_Window(t *testing.T) {
	for _, tc := range []struct {
		args      []string
		timeRange string
		custom    time.Duration
	}{
		{args: []string{}, timeRange: "1d"},
		{args: []string{"--since", "3h"}, timeRange: "3h"},
		{args: []string{"--since", "24h"}, timeRange: "1d"},
		{args: []string{"--since", "7d"}, timeRange: "7d"},
		{args: []string{"--since", "60m"}},
		{args: []string{"--since", "45m"}, custom: 45 * time.Minute},
		{args: []string{"--start", testStart, "--end", testEnd}, custom: time.Hour},
	} {
		run := mustRun(t, append(append(slices.Clone(listArgs), tc.args...), "--console-url")...)
		q := linkQuery(t, consoleLinkOf(t, run))
		if q.Get("timeRange") != tc.timeRange {
			t.Errorf("%v: timeRange = %q, want %q", tc.args, q.Get("timeRange"), tc.timeRange)
		}
		if got, _ := windowOf(q); q.Has("startTime") != (tc.custom != 0) || got != tc.custom {
			t.Errorf("%v: startTime %q, endTime %q; want a custom window of %s", tc.args, q.Get("startTime"), q.Get("endTime"), tc.custom)
		}
	}
}

// With no window in the URL the list covers the console's 1h; a custom range beats timeRange.
func TestFromURL_Window(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  time.Duration
	}{
		{"?status=ok", time.Hour},
		{"?timeRange=30d", 30 * 24 * time.Hour},
		{"?timeRange=1d&startTime=2026-10-14T09:00:00.000Z&endTime=2026-10-14T09:30:00.000Z", 30 * time.Minute},
	} {
		run := mustRun(t, "traces", "--from-url", consolePath+tc.query)
		if got, _ := windowOf(run.requests[0]); got != tc.want {
			t.Errorf("%s: window = %s, want %s", tc.query, got, tc.want)
		}
	}
}

// Each --condition writes the server param it stands for; tool_call_fails has none, so it writes no toolError.
func TestConsoleURL_Conditions(t *testing.T) {
	for condition, want := range map[string][2]string{
		conditionErrorStatus:    {"status", "error"},
		conditionHighLatency:    {"minDurationMs", "30001"},
		conditionHighTokenUsage: {"minTokens", "10001"},
		conditionExcessiveSteps: {"minSpanCount", "41"},
		conditionToolCallFails:  {"toolError", ""},
	} {
		run := mustRun(t, append(slices.Clone(listArgs), "--condition", condition, "--console-url")...)
		if got := linkQuery(t, consoleLinkOf(t, run)).Get(want[0]); got != want[1] {
			t.Errorf("--condition %s: %s = %q, want %q", condition, want[0], got, want[1])
		}
	}
}

// A URL that disagrees with the command, sets a filter twice or holds an invalid value is a
// flag error naming it, before any request.
func TestFromURL_Rejects(t *testing.T) {
	full := consolePath + "?status=ok"
	for _, tc := range []struct {
		name    string
		args    []string
		mention string
	}{
		{"other agent", []string{"traces", "other-agent", "--from-url", full}, "agent"},
		{"other org", []string{"traces", "--org", "globex", "--from-url", full}, "--org"},
		{"other project", []string{"traces", "--project", "billing", "--from-url", full}, "--project"},
		{"other env", []string{"traces", "--env", "prod", "--from-url", full}, "--env"},
		{"filter set twice", []string{"traces", "--tool", "b", "--from-url", consolePath + "?tool=a"}, "--tool"},
		{"condition sets the URL's filter", []string{"traces", "--condition", "error_status", "--from-url", full}, "status"},
		{"tool_call_fails with toolError", []string{"traces", "--condition", "tool_call_fails", "--from-url", consolePath + "?toolError=true"}, "--tool-error"},
		{"not the trace list", []string{"traces", "--from-url", "/org/acme/project/triage/agents/my-agent/environment/dev/observability/logs"}, "trace list"},
		{"start without end", []string{"traces", "--from-url", consolePath + "?startTime=" + testStart}, "endTime"},
		{"start after end", []string{"traces", "--from-url", consolePath + "?startTime=" + testEnd + "&endTime=" + testStart}, "startTime"},
		{"status", []string{"traces", "--from-url", consolePath + "?status=failed"}, "status"},
		{"minDurationMs", []string{"traces", "--from-url", consolePath + "?minDurationMs=5s"}, "minDurationMs"},
		{"minTokens", []string{"traces", "--from-url", consolePath + "?minTokens=-1"}, "minTokens"},
		{"minSpanCount", []string{"traces", "--from-url", consolePath + "?minSpanCount=1e3"}, "minSpanCount"},
		{"missing", []string{"traces", "--from-url", consolePath + "?missing=model"}, "missing"},
		{"toolError", []string{"traces", "--from-url", consolePath + "?toolError=yes"}, "toolError"},
		{"timeRange", []string{"traces", "--from-url", consolePath + "?timeRange=2h"}, "timeRange"},
		{"startTime", []string{"traces", "--from-url", consolePath + "?startTime=yesterday&endTime=" + testEnd}, "startTime"},
		{"sortOrder", []string{"traces", "--from-url", consolePath + "?sortOrder=up"}, "sortOrder"},
		{"limit above 100", []string{"traces", "--from-url", consolePath + "?limit=101"}, "limit"},
		{"limit zero", []string{"traces", "--from-url", consolePath + "?limit=0"}, "limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := runAmctl(t, emptyList, false, tc.args...)
			var flagErr *cmdutil.FlagError
			if !errors.As(run.err, &flagErr) || !strings.Contains(run.err.Error(), tc.mention) {
				t.Errorf("err = %v, want a flag error naming %s", run.err, tc.mention)
			}
			if len(run.requests) != 0 {
				t.Errorf("observer was called %d times", len(run.requests))
			}
		})
	}
}

// The scope repeated as it is in the URL is fine, and --limit, --sort and --since override the URL.
func TestFromURL_FlagsOverride(t *testing.T) {
	link := consolePath + "?limit=50&sortOrder=asc&timeRange=7d&tool=search"
	run := mustRun(t, "traces", "my-agent", "--org", "acme", "--project", "triage", "--env", "dev",
		"--from-url", link, "--limit", "25", "--sort", "desc", "--since", "3h")
	q := run.requests[0]
	if q.Get("limit") != "25" || q.Get("sortOrder") != "desc" || q.Get("tool") != "search" {
		t.Errorf("limit = %q, sortOrder = %q, tool = %q; want 25, desc, search", q.Get("limit"), q.Get("sortOrder"), q.Get("tool"))
	}
	if got, _ := windowOf(q); got != 3*time.Hour {
		t.Errorf("window = %s, want 3h", got)
	}
	custom := mustRun(t, "traces", "--from-url", link, "--start", testStart, "--end", testEnd)
	if q := custom.requests[0]; q.Get("startTime") != testStart || q.Get("endTime") != testEnd {
		t.Errorf("window = %s to %s, want the flags'", q.Get("startTime"), q.Get("endTime"))
	}
}

// `agent traces --from-url` ignores selectedTrace.
func TestFromURL_ListIgnoresSelectedTrace(t *testing.T) {
	run := mustRun(t, "traces", "--from-url", consolePath+"?selectedTrace="+testTraceID)
	if run.paths[0] != "/api/v1/traces" || run.requests[0].Has("selectedTrace") {
		t.Errorf("path = %q, query = %v; want the plain list", run.paths[0], run.requests[0])
	}
}

// `agent trace --console-url` writes the trace and its window; --from-url reads them back.
func TestTraceConsoleURL_RoundTrip(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"--since", "3h"},
		{"--since", "45m"},
		{"--start", testStart, "--end", testEnd},
	} {
		flags := mustRun(t, append(append(slices.Clone(traceArgs), args...), "--console-url")...)
		link := consoleLinkOf(t, flags)
		q := linkQuery(t, link)
		if q.Get("selectedTrace") != testTraceID {
			t.Errorf("%v: selectedTrace = %q, want %s", args, q.Get("selectedTrace"), testTraceID)
		}
		back := mustRun(t, "trace", "--from-url", link)
		if back.paths[0] != flags.paths[0] {
			t.Errorf("%v: path = %q, want %q", args, back.paths[0], flags.paths[0])
		}
		sameRequest(t, flags.requests[0], back.requests[0])
	}
}

// `agent trace --from-url` opens selectedTrace over the URL's window, and an argument overrides it.
func TestTraceFromURL(t *testing.T) {
	link := consolePath + "?timeRange=3h&limit=25&status=error&selectedTrace=" + testTraceID
	for _, tc := range []struct {
		name  string
		args  []string
		trace string
	}{
		{"from the URL", []string{"trace", "--from-url", link}, testTraceID},
		{"the agent named again", []string{"trace", "my-agent", "--from-url", link}, testTraceID},
		{"trace ID argument", []string{"trace", "--from-url", link, "abc123"}, "abc123"},
		{"agent and trace ID", []string{"trace", "my-agent", "abc123", "--from-url", link}, "abc123"},
		{"query only", []string{"trace", "my-agent", "--project", "triage", "--env", "dev", "--from-url", "?selectedTrace=" + testTraceID}, testTraceID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := mustRun(t, tc.args...)
			if want := "/api/v1/traces/" + tc.trace + "/spans"; run.paths[0] != want {
				t.Errorf("path = %q, want %q", run.paths[0], want)
			}
			q := run.requests[0]
			if got, _ := windowOf(q); got != 3*time.Hour && tc.name != "query only" {
				t.Errorf("window = %s, want 3h", got)
			}
			if q.Get("limit") != "1000" || q.Has("status") {
				t.Errorf("limit = %q, status = %q; want the span limit and no filter", q.Get("limit"), q.Get("status"))
			}
		})
	}
}

// `agent trace --from-url` without selectedTrace, or for another agent, is a flag error.
func TestTraceFromURL_Rejects(t *testing.T) {
	for _, args := range [][]string{
		{"trace", "--from-url", consolePath + "?timeRange=3h"},
		{"trace", "other-agent", "abc123", "--from-url", consolePath + "?selectedTrace=" + testTraceID},
	} {
		run := runAmctl(t, emptyList, false, args...)
		var flagErr *cmdutil.FlagError
		if !errors.As(run.err, &flagErr) || len(run.requests) != 0 {
			t.Errorf("%v: err = %v, requests = %d; want a flag error and no request", args, run.err, len(run.requests))
		}
	}
}

// JSON output carries consoleUrl only with --console-url, on the list, tool_call_fails and trace paths.
func TestConsoleURL_JSON(t *testing.T) {
	for _, args := range [][]string{
		listArgs,
		append(slices.Clone(listArgs), "--condition", "tool_call_fails"),
		traceArgs,
	} {
		for _, withFlag := range []bool{false, true} {
			a := slices.Clone(args)
			if withFlag {
				a = append(a, "--console-url")
			}
			run := runAmctl(t, emptyList, true, a...)
			if run.err != nil {
				t.Fatalf("%v: %v", a, run.err)
			}
			data, _ := decodeEnvelope(t, run.out)["data"].(map[string]any)
			link, ok := data["consoleUrl"].(string)
			if ok != withFlag || (withFlag && !strings.HasPrefix(link, consolePath)) {
				t.Errorf("%v: consoleUrl = %v, want it only with --console-url", a, data["consoleUrl"])
			}
			if run.errOut != "" {
				t.Errorf("%v: JSON mode wrote stderr %q", a, run.errOut)
			}
		}
	}
}

// Without --console-url or --from-url, stderr is empty and `agent trace` sends today's request.
func TestConsoleURL_DefaultsUnchanged(t *testing.T) {
	list := runAmctl(t, serve(observersvc.TraceOverviewResponse{
		Traces: []observersvc.TraceOverview{{TraceID: "abc123"}},
	}), false, listArgs...)
	if list.err != nil || list.errOut != "" || !strings.Contains(list.out, "abc123") {
		t.Errorf("list: err = %v, stderr = %q, out = %q", list.err, list.errOut, list.out)
	}
	trace := mustRun(t, traceArgs...)
	if trace.errOut != "" {
		t.Errorf("trace: stderr = %q", trace.errOut)
	}
	var keys []string
	for k := range trace.requests[0] {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{"agent", "endTime", "environment", "limit", "organization", "project", "startTime"}
	if !slices.Equal(keys, want) {
		t.Errorf("trace query keys = %v, want %v", keys, want)
	}
	if got, _ := windowOf(trace.requests[0]); got != 24*time.Hour {
		t.Errorf("trace window = %s, want 24h", got)
	}
}

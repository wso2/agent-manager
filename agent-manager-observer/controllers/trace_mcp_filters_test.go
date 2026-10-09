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

package controllers

import (
	"fmt"
	"log/slog"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

// githubHandshakeOutput is the initialize.mcp output from the IT helpdesk export.
var githubHandshakeOutput = func() string {
	b, err := os.ReadFile("../opensearch/testdata/mcp_initialize_output.json")
	if err != nil {
		panic(err)
	}
	return string(b)
}()

// handshakeOutput is an initialize output naming server.
func handshakeOutput(server string) string {
	if server == "github-mcp-server" {
		return githubHandshakeOutput
	}
	return fmt.Sprintf(`{"protocolVersion":"2025-11-25","serverInfo":{"name":%q,"version":"1.0"}}`, server)
}

// mcpFake is costFake(true) with n initialize.mcp spans under each tool call's
// execute_tool span (every 3rd trace). Every 6th trace's handshakes name
// github-mcp-server, the others' atlassian-mcp; handshakes past
// maxMCPHandshakesPerTrace name late-server. They are listed latest first.
func mcpFake(n int) *fakeObserverClient { return withHandshakes(costFake(true), n, false) }

// mcpToolsFake is mcpFake(n) with the kth handshake after the first under its own execute_tool mcp_tool_k span.
func mcpToolsFake(n int) *fakeObserverClient { return withHandshakes(costFake(true), n, true) }

// withHandshakes adds mcpFake's handshakes to fake, each under its own tool call with ownTools.
func withHandshakes(fake *fakeObserverClient, n int, ownTools bool) *fakeObserverClient {
	for i := range fake.traces {
		if i%3 != 0 {
			continue
		}
		server := "github-mcp-server"
		if i%6 == 3 {
			server = "atlassian-mcp"
		}
		addHandshakes(fake, i, n, server, ownTools)
	}
	return fake
}

// addHandshakes puts n initialize.mcp spans naming server under trace i's
// execute_tool span, k ms after its start for the kth, or with ownTools under mcp_tool_k for k > 0.
func addHandshakes(fake *fakeObserverClient, i, n int, server string, ownTools bool) {
	info := &fake.traces[i]
	spans := fake.spansByTrace[info.TraceID]
	for k := n - 1; k >= 0; k-- {
		name := server
		if k >= maxMCPHandshakesPerTrace {
			name = "late-server"
		}
		start := info.StartTime.Add(time.Duration(k) * time.Millisecond)
		parent := fmt.Sprintf("tool-%04d", i)
		if ownTools && k > 0 {
			tool := observer.SpanInfo{
				SpanID: fmt.Sprintf("tool-%d-%04d", k, i), SpanName: fmt.Sprintf("execute_tool mcp_tool_%d", k),
				ParentSpanID: spans[0].SpanID, StartTime: start,
			}
			spans = slices.Insert(spans, len(spans)-1, tool)
			parent = tool.SpanID
		}
		s := observer.SpanInfo{
			SpanID: fmt.Sprintf("mcp-%d-%04d", k, i), SpanName: "initialize.mcp", ParentSpanID: parent,
			StartTime:  start,
			Attributes: map[string]interface{}{"traceloop.entity.output": handshakeOutput(name)},
		}
		spans = slices.Insert(spans, len(spans)-1, s)
		fake.spanDetails[s.SpanID] = &observer.SpanDetailsResponse{
			SpanID: s.SpanID, SpanName: s.SpanName, ParentSpanID: s.ParentSpanID, StartTime: s.StartTime, Attributes: s.Attributes,
		}
	}
	fake.spansByTrace[info.TraceID] = spans
	info.SpanCount = len(spans)
}

// mcpFake1 is mcpFake(1).
func mcpFake1() *fakeObserverClient { return mcpFake(1) }

// mcpRootCompleteFake is mcpFake(1) with roots that fill input, output and tokens.
func mcpRootCompleteFake() *fakeObserverClient {
	fake := mcpFake(1)
	for _, info := range fake.traces {
		maps.Copy(fake.spanDetails[info.RootSpanID].Attributes, completeRootAttrs())
	}
	return fake
}

// includeMCPServers turns on include=mcpServers.
func includeMCPServers(p *TraceQueryParams) { p.Include = Include{MCPServers: true} }

// mcpServersOf is what mcpFake(n) reports for trace i.
func mcpServersOf(i int) []string {
	switch i % 6 {
	case 0:
		return []string{"github-mcp-server"}
	case 3:
		return []string{"atlassian-mcp"}
	}
	return nil
}

// handshakeFetches splits fake's handshake GetSpanDetails calls by trace index, in call order.
func handshakeFetches(t *testing.T, fake *fakeObserverClient) map[int][]string {
	t.Helper()
	fake.mu.Lock()
	defer fake.mu.Unlock()
	got := map[int][]string{}
	for _, id := range fake.detailSpanIDs {
		if strings.HasPrefix(id, "mcp-") {
			i := traceIndex(t, id)
			got[i] = append(got[i], id)
		}
	}
	return got
}

// cascadeFetches lists the trace index of every GetSpanDetails call for neither a root nor a handshake.
func cascadeFetches(t *testing.T, fake *fakeObserverClient) []int {
	t.Helper()
	fake.mu.Lock()
	defer fake.mu.Unlock()
	var got []int
	for _, id := range fake.detailSpanIDs {
		if !strings.HasPrefix(id, "root-") && !strings.HasPrefix(id, "mcp-") {
			got = append(got, traceIndex(t, id))
		}
	}
	return got
}

// detailIndex is the position of spanID's first GetSpanDetails call, or -1.
func detailIndex(fake *fakeObserverClient, spanID string) int {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return slices.Index(fake.detailSpanIDs, spanID)
}

// matchesMCPServer judges mcpServer on the servers, as a case-insensitive substring.
func TestMatchesMCPServer(t *testing.T) {
	tests := []struct {
		name    string
		filter  string
		servers []string
		want    bool
	}{
		{name: "no filter", servers: []string{"a"}, want: true},
		{name: "no filter, no servers", want: true},
		{name: "exact", filter: "github-mcp-server", servers: []string{"github-mcp-server"}, want: true},
		{name: "substring", filter: "github", servers: []string{"github-mcp-server"}, want: true},
		{name: "ignores case", filter: "GitHub", servers: []string{"github-mcp-server"}, want: true},
		{name: "a later entry", filter: "jira", servers: []string{"github-mcp-server", "jira"}, want: true},
		{name: "absent", filter: "jira", servers: []string{"github-mcp-server"}},
		{name: "nil servers", filter: "jira"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesMCPServer(tt.servers, TraceFilters{MCPServer: tt.filter}); got != tt.want {
				t.Errorf("matchesMCPServer(%v, %q) = %t, want %t", tt.servers, tt.filter, got, tt.want)
			}
		})
	}
}

// mcpServer counts as a span-list filter, not a tool one, implies its include,
// is logged, fails over the span cap and ANDs in matchesFilters.
func TestTraceFilters_MCPServerFields(t *testing.T) {
	f := TraceFilters{MCPServer: "github"}
	if f.IsZero() || f.SummaryOnly() || !f.hasSpanListFilter() || f.hasToolFilter() {
		t.Errorf("IsZero = %t, SummaryOnly = %t, hasSpanListFilter = %t, hasToolFilter = %t",
			f.IsZero(), f.SummaryOnly(), f.hasSpanListFilter(), f.hasToolFilter())
	}
	if got, want := f.LogValue(), slog.GroupValue(slog.String("mcpServer", "github")); !got.Equal(want) {
		t.Errorf("LogValue = %v, want %v", got, want)
	}
	if got, want := impliedInclude(Include{}, f), (Include{MCPServers: true}); got != want {
		t.Errorf("impliedInclude = %+v, want %+v", got, want)
	}
	if got, want := impliedInclude(Include{MCPServers: true}, TraceFilters{}), (Include{MCPServers: true}); got != want {
		t.Errorf("impliedInclude kept = %+v, want %+v", got, want)
	}
	if !matchesSummary(0, maxToolListSpans, f) || matchesSummary(0, maxToolListSpans+1, f) {
		t.Error("mcpServer: want a pass at the span cap and a fail over it")
	}

	ov := filterOverview()
	ov.MCPServers = []string{"github-mcp-server"}
	if !matchesFilters(ov, f) || matchesFilters(ov, TraceFilters{MCPServer: "jira"}) ||
		matchesFilters(ov, TraceFilters{MCPServer: "github", Status: TraceStatusOK}) {
		t.Error("matchesFilters doesn't AND mcpServer with the other filters")
	}
}

// include=mcpServers reports each trace's servers, and nothing for traces without a handshake.
func TestGetTraceOverviews_IncludeMCPServers(t *testing.T) {
	fake := mcpFake(1)
	params := lookBackParams(12)
	params.Filters = TraceFilters{}
	params.Include = Include{MCPServers: true}

	resp, err := NewTracingController(fake).GetTraceOverviews(t.Context(), params)
	if err != nil {
		t.Fatalf("GetTraceOverviews: %v", err)
	}

	if len(resp.Traces) != 12 {
		t.Fatalf("got %d traces, want 12", len(resp.Traces))
	}
	for _, ov := range resp.Traces {
		if want := mcpServersOf(traceIndex(t, ov.TraceID)); !slices.Equal(ov.MCPServers, want) {
			t.Errorf("%s mcpServers = %v, want %v", ov.TraceID, ov.MCPServers, want)
		}
	}
	if got := handshakeFetches(t, fake); len(got) != 4 {
		t.Errorf("traces with handshake fetches = %d, want 4", len(got))
	}
}

// Without a handshake span a trace costs one attribute-free list and nothing else.
func TestGetTraceOverviews_MCPServerNoHandshakeCostsOneList(t *testing.T) {
	fake := costFake(true)
	params := lookBackParams(10)
	params.Filters = TraceFilters{MCPServer: "github"}

	ids, _, truncated := traceIDs(NewTracingController(fake), t, params)

	if len(ids) != 0 || !truncated {
		t.Fatalf("got %v, truncated %v; want none, true", ids, truncated)
	}
	lists, roots, others := fetchedTraces(t, fake)
	if len(lists) != maxExaminedTraces || duplicate(lists) >= 0 {
		t.Errorf("span lists = %d, a trace listed twice = %t; want %d, one per examined trace", len(lists), duplicate(lists) >= 0, maxExaminedTraces)
	}
	if got := atomic.LoadInt32(&fake.attrSpansCalls); got != 0 {
		t.Errorf("QueryTraceSpans calls with attributes = %d, want 0", got)
	}
	if len(roots)+len(others) != 0 {
		t.Errorf("GetSpanDetails calls = %d, want 0", len(roots)+len(others))
	}
}

// A trace with a handshake costs one more GetSpanDetails, after its list and
// before its root. Only matches fetch their root and the cascade.
func TestGetTraceOverviews_MCPServerOneHandshake(t *testing.T) {
	fake := mcpFake(1)
	params := lookBackParams(10)
	params.Filters = TraceFilters{MCPServer: "GitHub"}

	ids, _, _ := traceIDs(NewTracingController(fake), t, params)

	want := make([]string, 0, 10)
	for i := 0; i < 60; i += 6 {
		want = append(want, fmt.Sprintf("trace-%04d", i))
	}
	if !slices.Equal(ids, want) {
		t.Fatalf("traces = %v, want %v", ids, want)
	}
	lists, roots, _ := fetchedTraces(t, fake)
	if dup := duplicate(lists); dup >= 0 {
		t.Errorf("trace-%04d got more than one span list", dup)
	}
	for i, fetched := range handshakeFetches(t, fake) {
		if len(fetched) != 1 || i%3 != 0 {
			t.Errorf("trace-%04d handshake fetches = %v, want one for a trace with a handshake", i, fetched)
		}
	}
	for _, i := range append(roots, cascadeFetches(t, fake)...) {
		if i%6 != 0 {
			t.Errorf("trace-%04d is not a match but was fetched past its handshake", i)
		}
	}
	for _, id := range want {
		i := traceIndex(t, id)
		hs, root := detailIndex(fake, fmt.Sprintf("mcp-0-%04d", i)), detailIndex(fake, fmt.Sprintf("root-%04d", i))
		if hs < 0 || root < hs {
			t.Errorf("%s fetched its handshake at %d and its root at %d; want the handshake first", id, hs, root)
		}
	}
}

// With 8 handshake spans under 8 tools, only the first maxMCPHandshakesPerTrace in start order are fetched.
func TestGetTraceOverviews_MCPServerHandshakeCap(t *testing.T) {
	fake := mcpToolsFake(8)
	params := lookBackParams(5)
	params.Filters = TraceFilters{}
	params.Include = Include{MCPServers: true}

	resp, err := NewTracingController(fake).GetTraceOverviews(t.Context(), params)
	if err != nil {
		t.Fatalf("GetTraceOverviews: %v", err)
	}

	for _, ov := range resp.Traces {
		if want := mcpServersOf(traceIndex(t, ov.TraceID)); !slices.Equal(ov.MCPServers, want) {
			t.Errorf("%s mcpServers = %v, want %v, without late-server", ov.TraceID, ov.MCPServers, want)
		}
	}
	for i, fetched := range handshakeFetches(t, fake) {
		slices.Sort(fetched)
		var want []string
		for k := range maxMCPHandshakesPerTrace {
			want = append(want, fmt.Sprintf("mcp-%d-%04d", k, i))
		}
		if !slices.Equal(fetched, want) {
			t.Errorf("trace-%04d fetched handshakes %v, want %v", i, fetched, want)
		}
	}

	ids, _, _ := traceIDs(NewTracingController(mcpToolsFake(8)), t, exportParams(5, TraceFilters{MCPServer: "late-server"}))
	if len(ids) != 0 {
		t.Errorf("mcpServer=late-server matched %v, want none: those handshakes are past the cap", ids)
	}
}

// The handshake fetch comes after every cheaper check: tools, models and the root's status.
func TestGetTraceOverviews_MCPServerHandshakeAfterOtherChecks(t *testing.T) {
	tests := []struct {
		name    string
		filters TraceFilters
		// passes reports whether trace i passes the checks before the handshake.
		passes func(i int) bool
		want   []string
	}{
		{name: "toolError", filters: TraceFilters{MCPServer: "github", ToolError: true},
			passes: func(i int) bool { return i%30 == 0 },
			want:   []string{"trace-0000", "trace-0030", "trace-0060"}},
		{name: "model", filters: TraceFilters{MCPServer: "atlassian", Model: "claude"},
			passes: func(i int) bool { return i%2 == 1 },
			want:   []string{"trace-0003", "trace-0009", "trace-0015"}},
		{name: "status", filters: TraceFilters{MCPServer: "github", Status: TraceStatusError},
			passes: func(i int) bool { return i%20 == 0 },
			want:   []string{"trace-0000", "trace-0060", "trace-0120"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := mcpFake(1)
			params := lookBackParams(3)
			params.Filters = tt.filters

			ids, _, _ := traceIDs(NewTracingController(fake), t, params)

			if !slices.Equal(ids, tt.want) {
				t.Fatalf("traces = %v, want %v", ids, tt.want)
			}
			fetched := handshakeFetches(t, fake)
			if len(fetched) == 0 {
				t.Fatal("no handshake fetched")
			}
			for i := range fetched {
				if !tt.passes(i) {
					t.Errorf("trace-%04d fails an earlier check but fetched its handshake", i)
				}
			}
			lists, _, _ := fetchedTraces(t, fake)
			if dup := duplicate(lists); dup >= 0 {
				t.Errorf("trace-%04d got more than one span list", dup)
			}
		})
	}
}

// A trace over the span cap is rejected from the trace list at no calls.
func TestGetTraceOverviews_MCPServerOverCapCostsNothing(t *testing.T) {
	fake := costOverCapFake()
	params := lookBackParams(10)
	params.Filters = TraceFilters{MCPServer: "github"}

	ids, _, truncated := traceIDs(NewTracingController(fake), t, params)

	if len(ids) != 0 || !truncated {
		t.Errorf("got %d traces, truncated %v; want 0, true", len(ids), truncated)
	}
	if got := atomic.LoadInt32(&fake.queryTraceSpansCalls) + atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 0 {
		t.Errorf("per-trace calls = %d, want 0", got)
	}
}

// mcpFilterFake is mcpFake(1) with every 7th trace over the leaf threshold and
// every 11th over the span cap.
func mcpFilterFake() *fakeObserverClient {
	return overCap(overThreshold(mcpFake(1), 7), 11)
}

// An mcpServer filter returns what the filter keeps of the fully enriched
// traces: the same pages, cursors, lookedBackTo, truncated and rows, as if no
// trace had been rejected early. A handshake is fetched only for a trace that
// passed the checks before it, and the cascade only for one that passed it.
func TestGetTraceOverviews_MCPServerFilterMatchesFullEnrichment(t *testing.T) {
	tests := []struct {
		name    string
		filters TraceFilters
	}{
		{name: "mcpServer", filters: TraceFilters{MCPServer: "github-mcp-server"}},
		{name: "mcpServer substring", filters: TraceFilters{MCPServer: "MCP"}},
		{name: "mcpServer and tool", filters: TraceFilters{MCPServer: "atlassian", Tool: "search_issues"}},
		{name: "mcpServer and toolError", filters: TraceFilters{MCPServer: "github", ToolError: true}},
		{name: "mcpServer and status", filters: TraceFilters{MCPServer: "github", Status: TraceStatusError}},
		{name: "mcpServer and status ok", filters: TraceFilters{MCPServer: "mcp", Status: TraceStatusOK}},
		{name: "mcpServer and model", filters: TraceFilters{MCPServer: "atlassian", Model: "claude"}},
		{name: "mcpServer and minTokens", filters: TraceFilters{MCPServer: "github", MinTokens: ptr(40)}},
		{name: "mcpServer and conversationId", filters: TraceFilters{MCPServer: "github", ConversationID: "conv-01"}},
		{name: "mcpServer and minDurationMs", filters: TraceFilters{MCPServer: "mcp", MinDurationMs: ptr(300)}},
	}
	for _, tt := range tests {
		for _, order := range []string{"desc", "asc"} {
			t.Run(tt.name+"/"+order, func(t *testing.T) {
				ref := referenceOverviews(t, mcpFilterFake(), impliedInclude(Include{}, tt.filters))
				match := func(i int) bool {
					ov, ok := ref[i]
					return ok && matchesFilters(ov, tt.filters)
				}
				params := lookBackParams(10)
				params.Filters = tt.filters
				params.SortOrder = order
				fake := mcpFilterFake()

				pages := pagesListedOnce(t, NewTracingController(fake), fake, params, 3)

				assertPages(t, pages, wantPages(fake, params, 3, match))
				for _, page := range pages {
					for _, ov := range page.Traces {
						if want := ref[traceIndex(t, ov.TraceID)]; !reflect.DeepEqual(ov, want) {
							t.Errorf("%s row = %+v, want the fully enriched %+v", ov.TraceID, ov, want)
						}
					}
				}

				mcpPass := func(i int) bool { return matchesMCPServer(ref[i].MCPServers, tt.filters) }
				beforeMCP := func(i int) bool {
					ov := ref[i]
					return matchesTools(ov.Tools, ov.FailedTools, tt.filters) && matchesStatus(ov.Status, tt.filters) &&
						(tt.filters.Model == "" || matchesModel(ov.Models, tt.filters))
				}
				for i := range handshakeFetches(t, fake) {
					if !beforeMCP(i) {
						t.Errorf("trace-%04d fails an earlier check but fetched its handshake", i)
					}
				}
				for _, i := range cascadeFetches(t, fake) {
					if !mcpPass(i) {
						t.Errorf("trace-%04d failed the mcpServer filter but was fetched past its handshake", i)
					}
				}
				rootFirst := tt.filters.Status != TraceStatusAny || tt.filters.ConversationID != ""
				lists, roots, _ := fetchedTraces(t, fake)
				for _, i := range roots {
					// A model filter reads an over-threshold trace's root before its list.
					modelAtRoot := tt.filters.Model != "" && fake.traces[i].SpanCount > skipLeafAggregationSpanCountThreshold
					if !rootFirst && !mcpPass(i) && !modelAtRoot {
						t.Errorf("trace-%04d failed the mcpServer filter but fetched its root", i)
					}
				}
				for _, i := range lists {
					if fake.traces[i].SpanCount > maxToolListSpans {
						t.Errorf("trace-%04d is over the span cap but got a span list", i)
					}
				}
			})
		}
	}
}

// A handshake that can't be read can't be judged by mcpServer: the trace is
// listed as failed with the handshake warning, never the root-fetch one.
// Without the filter the row is kept.
func TestGetTraceOverviews_MCPServerHandshakeFails(t *testing.T) {
	tests := []struct {
		name    string
		filters TraceFilters
		want    []string
		failed  float64
	}{
		{name: "mcpServer", filters: TraceFilters{MCPServer: "github"},
			want: []string{"trace-0006", "trace-0012", "trace-0018"}, failed: 1},
		{name: "mcpServer and status", filters: TraceFilters{MCPServer: "github", Status: TraceStatusError},
			want: []string{"trace-0060", "trace-0120", "trace-0180"}, failed: 1},
		{name: "include only", want: []string{"trace-0000", "trace-0001", "trace-0002"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := mcpFake(1)
			fake.failCall = failTimes(map[string]int{"mcp-0-0000": -1})
			params := lookBackParams(3)
			params.Filters = tt.filters
			params.Include = Include{MCPServers: true}
			ctx, logs := logContext()

			resp, err := NewTracingController(fake).GetTraceOverviews(ctx, params)
			if err != nil {
				t.Fatalf("GetTraceOverviews: %v", err)
			}

			got := make([]string, len(resp.Traces))
			for i, ov := range resp.Traces {
				got[i] = ov.TraceID
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("traces = %v, want %v", got, tt.want)
			}
			if got := logField(t, logs, "Retrieved trace overviews", "failed"); got != tt.failed {
				t.Errorf("failed logged as %v, want %v", got, tt.failed)
			}
			warns := warnings(logs)
			if len(warns) == 0 {
				t.Error("no warning logged for the failed handshake")
			}
			for _, w := range warns {
				if w != "failed to fetch MCP handshake span details" {
					t.Errorf("unexpected warning %q", w)
				}
			}
			if tt.failed == 0 && resp.Traces[0].MCPServers != nil {
				t.Errorf("trace-0000 mcpServers = %v, want none", resp.Traces[0].MCPServers)
			}
		})
	}
}

// An export lists the trace whose handshake failed in failedTraceIds and leaves it out.
func TestExportTraces_MCPServerHandshakeFailsInSelection(t *testing.T) {
	fake := mcpFake(1)
	fake.failCall = failTimes(map[string]int{"mcp-0-0000": -1})

	resp := mustExport(t, NewTracingController(fake), exportParams(3, TraceFilters{MCPServer: "github"}))

	if got, want := exportedIDs(resp), []string{"trace-0006", "trace-0012", "trace-0018"}; !slices.Equal(got, want) {
		t.Fatalf("exported %v, want %v", got, want)
	}
	if want := []string{"trace-0000"}; !slices.Equal(resp.FailedTraceIDs, want) {
		t.Errorf("failedTraceIds = %v, want %v", resp.FailedTraceIDs, want)
	}
}

// An export with mcpServer selects as the list does and fetches full spans for the matches only.
func TestExportTraces_MCPServerFetchesSpansOnlyForMatches(t *testing.T) {
	fake := mcpFake(1)

	resp := mustExport(t, NewTracingController(fake), exportParams(5, TraceFilters{MCPServer: "atlassian"}))

	want := []string{"trace-0003", "trace-0009", "trace-0015", "trace-0021", "trace-0027"}
	if got := exportedIDs(resp); !slices.Equal(got, want) {
		t.Fatalf("exported %v, want %v", got, want)
	}
	fake.mu.Lock()
	full := slices.Sorted(slices.Values(fake.attrSpansTraceIDs))
	fake.mu.Unlock()
	if !slices.Equal(full, want) {
		t.Errorf("full span fetches for %v, want the matches %v", full, want)
	}
}

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
	"reflect"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

// mcpEveryTraceFake is costFake(true) with n handshakes naming server(i) in every trace i, each under its own tool call.
func mcpEveryTraceFake(n int, server func(i int) string) *fakeObserverClient {
	fake := costFake(true)
	for i := range fake.traces {
		if i%3 != 0 {
			addToolCall(fake, i, false)
		}
		addHandshakes(fake, i, n, server(i), true)
	}
	return fake
}

// atlassianOnly names atlassian-mcp in every trace.
func atlassianOnly(int) string { return "atlassian-mcp" }

// handshakeFetchCount is the number of handshake GetSpanDetails calls fake has answered.
func handshakeFetchCount(t *testing.T, fake *fakeObserverClient) int {
	t.Helper()
	n := 0
	for _, fetched := range handshakeFetches(t, fake) {
		n += len(fetched)
	}
	return n
}

// pageOrder is fake's trace indexes in params' page order.
func pageOrder(fake *fakeObserverClient, params TraceQueryParams) []int {
	order := make([]int, len(fake.traces))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return compareTraces(fake.traces[order[a]], fake.traces[order[b]], params.SortOrder == "asc") < 0
	})
	return order
}

// handshakesToFetch keeps the first handshake per tool in start order, up to maxMCPHandshakesPerTrace.
func TestHandshakesToFetch(t *testing.T) {
	at := func(ms int) time.Time { return lookBackWindowEnd.Add(time.Duration(ms) * time.Millisecond) }
	span := func(id, name, parent string, ms int) observer.SpanInfo {
		return observer.SpanInfo{SpanID: id, SpanName: name, ParentSpanID: parent, StartTime: at(ms)}
	}
	handshake := func(id, parent string, ms int) observer.SpanInfo { return span(id, "initialize.mcp", parent, ms) }
	chain := span("chain", "LangGraph.workflow", "root", 0)

	// Each list holds 8 handshakes, listed latest first.
	oneSpan := []observer.SpanInfo{chain, span("t", "execute_tool search_issues", "chain", 0)}
	var oneTool, eightTools, lone []observer.SpanInfo
	for k := 7; k >= 0; k-- {
		tool, h := fmt.Sprintf("t%d", k), fmt.Sprintf("h%d", k)
		oneSpan = append(oneSpan, handshake(h, "t", k))
		oneTool = append(oneTool, span(tool, "execute_tool search_issues", "chain", k), handshake(h, tool, k))
		eightTools = append(eightTools, span(tool, fmt.Sprintf("execute_tool tool_%d", k), "chain", k), handshake(h, tool, k))
		lone = append(lone, handshake(h, "gone", k))
	}

	tests := []struct {
		name  string
		spans []observer.SpanInfo
		want  []string
	}{
		{name: "no handshake", spans: []observer.SpanInfo{chain, span("t", "execute_tool search_issues", "chain", 0)}},
		{name: "8 under one tool span", spans: oneSpan, want: []string{"h0"}},
		{name: "8 calls of one tool", spans: oneTool, want: []string{"h0"}},
		{name: "8 tools", spans: eightTools, want: []string{"h0", "h1", "h2", "h3", "h4"}},
		{name: "one tool in both naming styles", spans: []observer.SpanInfo{
			span("a", "search_issues.tool", "chain", 2), handshake("h2", "a", 3),
			span("b", "execute_tool search_issues", "chain", 0), handshake("h1", "b", 1),
		}, want: []string{"h1"}},
		{name: "parent not in the list", spans: lone, want: []string{"h0", "h1", "h2", "h3", "h4"}},
		{name: "parent not a tool", spans: []observer.SpanInfo{chain, handshake("h2", "chain", 2), handshake("h1", "chain", 1)},
			want: []string{"h1", "h2"}},
		{name: "parent names no tool", spans: []observer.SpanInfo{
			span("t", "execute_tool", "chain", 0), handshake("h1", "t", 1), handshake("h2", "t", 2),
		}, want: []string{"h1", "h2"}},
		{name: "a lone handshake beside a tool's", spans: []observer.SpanInfo{
			span("t", "execute_tool search_issues", "chain", 0), handshake("h3", "t", 3), handshake("h2", "t", 2), handshake("h1", "gone", 1),
		}, want: []string{"h1", "h2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, s := range handshakesToFetch(tt.spans) {
				got = append(got, s.SpanID)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("handshakesToFetch = %v, want %v", got, tt.want)
			}
		})
	}
}

// Handshakes under one tool cost one fetch per trace and report the same servers.
func TestGetTraceOverviews_MCPServersOneFetchPerTool(t *testing.T) {
	for _, n := range []int{1, 8} {
		t.Run(fmt.Sprintf("%d handshakes", n), func(t *testing.T) {
			fake := mcpFake(n)
			params := lookBackParams(12)
			params.Filters = TraceFilters{}
			params.Include = Include{MCPServers: true}

			resp, err := NewTracingController(fake).GetTraceOverviews(t.Context(), params)
			if err != nil {
				t.Fatalf("GetTraceOverviews: %v", err)
			}

			for _, ov := range resp.Traces {
				if want := mcpServersOf(traceIndex(t, ov.TraceID)); !slices.Equal(ov.MCPServers, want) {
					t.Errorf("%s mcpServers = %v, want %v", ov.TraceID, ov.MCPServers, want)
				}
			}
			fetched := handshakeFetches(t, fake)
			if len(fetched) != 4 {
				t.Errorf("traces with handshake fetches = %d, want 4", len(fetched))
			}
			for i, ids := range fetched {
				if want := []string{fmt.Sprintf("mcp-0-%04d", i)}; !slices.Equal(ids, want) {
					t.Errorf("trace-%04d fetched handshakes %v, want only the first, %v", i, ids, want)
				}
			}
		})
	}
}

// The walk stops after the chunk that reached maxMCPHandshakesPerRequest, truncated with a cursor at its last trace.
func TestGetTraceOverviews_MCPHandshakeCapStopsWalk(t *testing.T) {
	tests := []struct {
		name     string
		n        int
		examined int
		fetches  int
	}{
		{name: "1 per trace", n: 1, examined: 2 * lookBackBatchSize, fetches: maxMCPHandshakesPerRequest},
		{name: "3 per trace", n: 3, examined: lookBackBatchSize, fetches: 3 * lookBackBatchSize},
		{name: "8 per trace", n: 8, examined: lookBackBatchSize, fetches: maxMCPHandshakesPerTrace * lookBackBatchSize},
	}
	for _, tt := range tests {
		for _, order := range []string{"desc", "asc"} {
			t.Run(tt.name+"/"+order, func(t *testing.T) {
				fake := mcpEveryTraceFake(tt.n, atlassianOnly)
				params := lookBackParams(10)
				params.Filters = TraceFilters{MCPServer: "github"}
				params.SortOrder = order
				ctx, logs := logContext()

				resp, err := NewTracingController(fake).GetTraceOverviews(ctx, params)
				if err != nil {
					t.Fatalf("GetTraceOverviews: %v", err)
				}

				last := fake.traces[pageOrder(fake, params)[tt.examined-1]]
				if len(resp.Traces) != 0 || !resp.Truncated {
					t.Errorf("got %d traces, truncated %v; want 0, true", len(resp.Traces), resp.Truncated)
				}
				cur, err := DecodeTraceCursor(resp.NextCursor)
				if err != nil {
					t.Fatalf("nextCursor %q: %v", resp.NextCursor, err)
				}
				if cur.ID != last.TraceID || !cur.Time.Equal(last.StartTime) || resp.LookedBackTo != formatCursor(last.StartTime) {
					t.Errorf("cursor at %s %s, lookedBackTo %s; want %s at %s", cur.ID, formatCursor(cur.Time), resp.LookedBackTo,
						last.TraceID, formatCursor(last.StartTime))
				}
				if got := handshakeFetchCount(t, fake); got != tt.fetches {
					t.Errorf("handshake fetches = %d, want %d", got, tt.fetches)
				}
				if lists, _, _ := fetchedTraces(t, fake); len(lists) != tt.examined {
					t.Errorf("span lists = %d, want one per examined trace, %d", len(lists), tt.examined)
				}
				if got := logField(t, logs, "Retrieved trace overviews", "handshakeCapReached"); got != true {
					t.Errorf("handshakeCapReached logged as %v, want true", got)
				}
				if got := logField(t, logs, "Retrieved trace overviews", "examined"); got != float64(tt.examined) {
					t.Errorf("examined logged as %v, want %d", got, tt.examined)
				}
			})
		}
	}
}

// Paging while the handshake cap stops walks returns each match once, in page order, with its fully enriched row.
func TestGetTraceOverviews_MCPHandshakeCapPagesEveryMatchOnce(t *testing.T) {
	// everyTrace has a handshake in every trace, naming github on every 25th.
	everyTrace := func() *fakeObserverClient {
		return overCap(overThreshold(mcpEveryTraceFake(1, func(i int) string {
			if i%25 == 0 {
				return "github-mcp-server"
			}
			return "atlassian-mcp"
		}), 7), 11)
	}
	tests := []struct {
		name    string
		fixture func() *fakeObserverClient
		filters TraceFilters
		include Include
	}{
		{name: "mcpServer nobody uses", fixture: mcpFilterFake, filters: TraceFilters{MCPServer: "jira"}},
		{name: "mcpServer", fixture: everyTrace, filters: TraceFilters{MCPServer: "github"}},
		{name: "mcpServer and model", fixture: everyTrace, filters: TraceFilters{MCPServer: "github", Model: "claude"}},
		{name: "include=mcpServers and minTokens", fixture: everyTrace, filters: TraceFilters{MinTokens: ptr(51)},
			include: Include{MCPServers: true}},
	}
	for _, tt := range tests {
		for _, order := range []string{"desc", "asc"} {
			t.Run(tt.name+"/"+order, func(t *testing.T) {
				ref := referenceOverviews(t, tt.fixture(), impliedInclude(tt.include, tt.filters))
				match := func(i int) bool {
					ov, ok := ref[i]
					return ok && matchesFilters(ov, tt.filters)
				}
				params := lookBackParams(10)
				params.Filters = tt.filters
				params.Include = tt.include
				params.SortOrder = order
				fake := tt.fixture()
				c := NewTracingController(fake)

				var want []string
				for _, p := range wantPages(fake, params, len(fake.traces), match) {
					want = append(want, p.ids...)
				}
				var got []string
				capped := 0
				for page := 0; ; page++ {
					if page == len(fake.traces) {
						t.Fatal("the cursor never ran out")
					}
					ctx, logs := logContext()
					before := handshakeFetchCount(t, fake)
					resp, err := c.GetTraceOverviews(ctx, params)
					if err != nil {
						t.Fatalf("page %d: %v", page, err)
					}
					for _, ov := range resp.Traces {
						got = append(got, ov.TraceID)
						if row := ref[traceIndex(t, ov.TraceID)]; !reflect.DeepEqual(ov, row) {
							t.Errorf("%s row = %+v, want the fully enriched %+v", ov.TraceID, ov, row)
						}
					}
					if logField(t, logs, "Retrieved trace overviews", "handshakeCapReached") == true {
						capped++
						if !resp.Truncated || resp.NextCursor == "" {
							t.Errorf("page %d stopped at the handshake cap without truncated and a cursor", page)
						}
					}
					if n := handshakeFetchCount(t, fake) - before; n >= maxMCPHandshakesPerRequest+lookBackBatchSize*maxMCPHandshakesPerTrace {
						t.Errorf("page %d fetched %d handshakes, more than one chunk past the cap", page, n)
					}
					if resp.NextCursor == "" {
						break
					}
					if params.Cursor, err = DecodeTraceCursor(resp.NextCursor); err != nil {
						t.Fatalf("page %d cursor: %v", page, err)
					}
				}
				if !slices.Equal(got, want) {
					t.Errorf("matches across pages = %v, want each once in page order, %v", got, want)
				}
				if capped == 0 {
					t.Error("the handshake cap never stopped a walk")
				}
			})
		}
	}
}

// The handshake cap ends a filtered export's selection as truncated.
func TestExportTraces_MCPHandshakeCapTruncates(t *testing.T) {
	fake := mcpEveryTraceFake(1, func(i int) string {
		if i%25 == 0 {
			return "github-mcp-server"
		}
		return "atlassian-mcp"
	})
	ctx, logs := logContext()

	resp, err := NewTracingController(fake).ExportTraces(ctx, exportParams(10, TraceFilters{MCPServer: "github"}))
	if err != nil {
		t.Fatalf("ExportTraces: %v", err)
	}

	if got, want := exportedIDs(resp), []string{"trace-0000", "trace-0025", "trace-0050", "trace-0075"}; !slices.Equal(got, want) {
		t.Errorf("exported %v, want %v", got, want)
	}
	if !resp.Truncated || resp.LookedBackTo != formatCursor(fake.traces[2*lookBackBatchSize-1].StartTime) {
		t.Errorf("truncated = %v, lookedBackTo = %s; want true, trace-%04d's start", resp.Truncated, resp.LookedBackTo, 2*lookBackBatchSize-1)
	}
	if got := logField(t, logs, "Selected traces for export", "handshakeCapReached"); got != true {
		t.Errorf("handshakeCapReached logged as %v, want true", got)
	}
}

// Only an MCP filter or a filtered include=mcpServers can stop on the handshake cap.
func TestGetTraceOverviews_MCPHandshakeCapOnlyWithMCP(t *testing.T) {
	t.Run("no MCP filter or include", func(t *testing.T) {
		fake := mcpEveryTraceFake(1, atlassianOnly)
		params := lookBackParams(10)
		params.Filters = TraceFilters{Tool: "send_email"}
		ctx, logs := logContext()

		resp, err := NewTracingController(fake).GetTraceOverviews(ctx, params)
		if err != nil {
			t.Fatalf("GetTraceOverviews: %v", err)
		}

		if len(resp.Traces) != 0 || !resp.Truncated {
			t.Errorf("got %d traces, truncated %v; want 0, true", len(resp.Traces), resp.Truncated)
		}
		if got := logField(t, logs, "Retrieved trace overviews", "examined"); got != float64(maxExaminedTraces) {
			t.Errorf("examined = %v, want the examine cap, %d", got, maxExaminedTraces)
		}
		if got := logField(t, logs, "Retrieved trace overviews", "handshakeCapReached"); got != false {
			t.Errorf("handshakeCapReached logged as %v, want false", got)
		}
		if got := handshakeFetchCount(t, fake); got != 0 {
			t.Errorf("handshake fetches = %d, want 0", got)
		}
	})
	t.Run("unfiltered include=mcpServers", func(t *testing.T) {
		fake := mcpEveryTraceFake(8, atlassianOnly)
		params := lookBackParams(50)
		params.Filters = TraceFilters{}
		params.Include = Include{MCPServers: true}

		resp, err := NewTracingController(fake).GetTraceOverviews(t.Context(), params)
		if err != nil {
			t.Fatalf("GetTraceOverviews: %v", err)
		}

		if len(resp.Traces) != 50 || resp.Truncated || resp.NextCursor == "" {
			t.Errorf("got %d traces, truncated %v, cursor %q; want 50, false, a cursor", len(resp.Traces), resp.Truncated, resp.NextCursor)
		}
		if got, want := handshakeFetchCount(t, fake), 50*maxMCPHandshakesPerTrace; got != want {
			t.Errorf("handshake fetches = %d, want %d", got, want)
		}
	})
}

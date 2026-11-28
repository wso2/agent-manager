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

package storetest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store"
)

// spanSpec is one span of a test trace, offset from the trace's start.
type spanSpec struct {
	id, parent, name string
	at, dur          time.Duration
	status           string
	attrs            map[string]any
}

// traceSeed builds a trace in sc from its spans, starting at start.
func traceSeed(sc store.Scope, traceID string, start time.Time, specs ...spanSpec) Seed {
	sd := Seed{Scope: sc}
	end := start
	for _, sp := range specs {
		d := store.SpanDetails{
			SpanID: traceID + "/" + sp.id, SpanName: sp.name,
			StartTime: start.Add(sp.at), EndTime: start.Add(sp.at + sp.dur), DurationNs: sp.dur.Nanoseconds(),
			Attributes: sp.attrs,
		}
		if sp.parent != "" {
			d.ParentSpanID = traceID + "/" + sp.parent
		}
		if sp.status != "" {
			d.Status = &store.SpanStatus{Code: sp.status}
		}
		if d.EndTime.After(end) {
			end = d.EndTime
		}
		sd.Spans = append(sd.Spans, d)
	}
	root := sd.Spans[0]
	sd.Trace = store.Trace{
		TraceID: traceID, TraceName: root.SpanName, SpanCount: len(specs),
		RootSpanID: root.SpanID, RootSpanName: root.SpanName,
		StartTime: start, EndTime: end, DurationNs: end.Sub(start).Nanoseconds(),
	}
	return sd
}

func usage(in, out int) map[string]any {
	return map[string]any{"gen_ai.usage.input_tokens": in, "gen_ai.usage.output_tokens": out}
}

func with(m map[string]any, kv ...any) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		out[k] = v
	}
	for i := 0; i+1 < len(kv); i += 2 {
		out[kv[i].(string)] = kv[i+1]
	}
	return out
}

// filterTraces are traces that each filter tells apart, in sc, with IDs prefixed by prefix.
func filterTraces(sc store.Scope, prefix string) []Seed {
	at := func(h int) time.Time { return windowStart.Add(time.Duration(h) * time.Hour) }
	sec := time.Second
	return []Seed{
		traceSeed(sc, prefix+"error", at(1),
			spanSpec{
				id: "r", name: "invoke_agent bot", dur: 5 * sec, status: "Error",
				attrs: map[string]any{"gen_ai.conversation.id": "conv-1"},
			},
			spanSpec{
				id: "c", parent: "r", name: "chat gpt-4o-mini", at: sec, dur: sec,
				attrs: with(usage(100, 50), "gen_ai.response.model", "gpt-4o-mini", "session.id", "conv-leaf"),
			}),
		traceSeed(sc, prefix+"ok", at(2),
			spanSpec{id: "r", name: "invoke_agent bot", dur: 2 * sec, status: "Ok"},
			spanSpec{
				id: "c", parent: "r", name: "openai.chat", at: sec, dur: sec,
				attrs: with(usage(10, 5), "gen_ai.request.model", "claude-3-haiku"),
			}),
		traceSeed(sc, prefix+"tool", at(3),
			spanSpec{id: "r", name: "invoke_agent bot", dur: 3 * sec},
			spanSpec{id: "s", parent: "r", name: "execute_tool search_web", at: sec, dur: sec, status: "Error"},
			spanSpec{id: "w", parent: "r", name: "execute_tool get_weather", at: 2 * sec, dur: sec, status: "Ok"},
			spanSpec{
				id: "c", parent: "r", name: "chat gpt-4o", at: 2 * sec, dur: sec,
				attrs: map[string]any{"traceloop.association.properties.thread_id": "conv-2"},
			}),
		traceSeed(sc, prefix+"mcp", at(4),
			spanSpec{id: "r", name: "invoke_agent bot", dur: 3 * sec},
			spanSpec{
				id: "k", parent: "r", name: "LangGraph.task", at: sec, dur: 2 * sec,
				attrs: map[string]any{"langfuse.session.id": "conv-3"},
			},
			spanSpec{
				id: "h", parent: "k", name: "initialize.mcp", at: sec, dur: sec,
				attrs: map[string]any{"traceloop.entity.output": `{"serverInfo":{"name":" github-mcp-server ","title":"GitHub"}}`},
			},
			spanSpec{id: "t", parent: "k", name: "get_issue.tool", at: 2 * sec, dur: sec}),
		// Strands: the agent and its chat wrapper both report the turn's usage, and the inner call repeats it.
		traceSeed(sc, prefix+"nested", at(5),
			spanSpec{id: "r", name: "invoke_agent strands", dur: 3 * sec, attrs: usage(600, 400)},
			spanSpec{
				id: "c", parent: "r", name: "chat", at: sec, dur: sec,
				attrs: with(usage(300, 200), "gen_ai.request.model", "gpt-4o"),
			},
			spanSpec{
				id: "o", parent: "c", name: "openai.chat", at: sec, dur: sec,
				attrs: with(usage(300, 200), "gen_ai.request.model", "gpt-4o"),
			}),
		traceSeed(sc, prefix+"root-tokens", at(6),
			spanSpec{
				id: "r", name: "invoke_agent bot", dur: sec,
				attrs: map[string]any{"gen_ai.usage.prompt_tokens": "40", "gen_ai.usage.completion_tokens": 2},
			}),
		traceSeed(sc, prefix+"long", at(7),
			spanSpec{id: "r", name: "invoke_agent bot", dur: 20 * sec},
			spanSpec{id: "a", parent: "r", name: "step.task", at: sec, dur: sec},
			spanSpec{id: "b", parent: "r", name: "step.task", at: 2 * sec, dur: sec},
			spanSpec{id: "c", parent: "r", name: "step.task", at: 3 * sec, dur: sec}),
		traceSeed(sc, prefix+"http", at(8),
			spanSpec{
				id: "r", name: "invoke_agent bot", dur: sec, status: "Unset",
				attrs: map[string]any{"http.status_code": 503},
			}),
	}
}

func i64(v int64) *int64 { return &v }

// testFilters checks every filter against the observer's rules. Another org
// holds the same traces, so a filter that leaked across orgs would list them.
func testFilters(t *testing.T, newStore NewStore) {
	ctx := context.Background()
	orgA := scopeFor("org-a", "acme")
	orgB := scopeFor("org-b", "globex")
	s := newStore(t, append(filterTraces(orgA, ""), filterTraces(orgB, "b-")...))

	for _, tc := range []struct {
		name string
		f    store.TraceFilters
		want []string
	}{
		{"none", store.TraceFilters{}, []string{"error", "ok", "tool", "mcp", "nested", "root-tokens", "long", "http"}},
		{"status error, from the root's status or attributes", store.TraceFilters{Status: "error"}, []string{"error", "http"}},
		{"status ok", store.TraceFilters{Status: "ok"}, []string{"ok", "tool", "mcp", "nested", "root-tokens", "long"}},
		{"min duration", store.TraceFilters{MinDurationMs: i64(10_000)}, []string{"long"}},
		{"min duration at the edge", store.TraceFilters{MinDurationMs: i64(5_000)}, []string{"error", "long"}},
		{"min span count", store.TraceFilters{MinSpanCount: i64(4)}, []string{"tool", "mcp", "long"}},
		{"min tokens 0 needs usage", store.TraceFilters{MinTokens: i64(0)}, []string{"error", "ok", "nested", "root-tokens"}},
		{"min tokens counts a nested report once", store.TraceFilters{MinTokens: i64(500)}, []string{"nested"}},
		{"min tokens over the nested total", store.TraceFilters{MinTokens: i64(501)}, nil},
		{"min tokens from the root without LLM usage", store.TraceFilters{MinTokens: i64(42)}, []string{"error", "nested", "root-tokens"}},
		{"model substring ignoring case", store.TraceFilters{Model: "GPT-4O"}, []string{"error", "nested"}},
		{"model from the request", store.TraceFilters{Model: "claude"}, []string{"ok"}},
		{"conversation on the root", store.TraceFilters{ConversationID: "conv-1"}, []string{"error"}},
		{"conversation on a leaf behind the root's", store.TraceFilters{ConversationID: "conv-leaf"}, nil},
		{"conversation on a leaf", store.TraceFilters{ConversationID: "conv-2"}, []string{"tool"}},
		{"conversation on the root's child", store.TraceFilters{ConversationID: "conv-3"}, []string{"mcp"}},
		{"conversation is exact", store.TraceFilters{ConversationID: "CONV-1"}, nil},
		{"missing conversation", store.TraceFilters{MissingConversationID: true}, []string{"ok", "nested", "root-tokens", "long", "http"}},
		{"tool substring ignoring case", store.TraceFilters{Tool: "SEARCH"}, []string{"tool"}},
		{"tool from a dotted name", store.TraceFilters{Tool: "get_"}, []string{"tool", "mcp"}},
		{"tool error", store.TraceFilters{ToolError: true}, []string{"tool"}},
		{"tool error on that tool", store.TraceFilters{Tool: "search", ToolError: true}, []string{"tool"}},
		{"tool error on a tool that didn't fail", store.TraceFilters{Tool: "weather", ToolError: true}, nil},
		{"MCP server", store.TraceFilters{MCPServer: "GitHub-MCP"}, []string{"mcp"}},
		{"MCP server title is a fallback only", store.TraceFilters{MCPServer: "GitHub "}, nil},
		{"filters combine", store.TraceFilters{Status: "ok", Model: "gpt"}, []string{"nested"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := s.QueryTraces(ctx, orgA, store.TraceQuery{
				StartTime: windowStart, EndTime: windowEnd, Limit: 100, SortOrder: "asc", Filters: tc.f,
			})
			if err != nil {
				t.Fatalf("QueryTraces: %v", err)
			}
			got := ids(page)
			if !slices.Equal(got, tc.want) {
				t.Errorf("traces = %v, want %v", got, tc.want)
			}
			if page.Total == nil || *page.Total != len(tc.want) || page.NextCursor != "" || page.Truncated {
				t.Errorf("page = total %v, cursor %q, truncated %v; want total %d, no cursor, not truncated",
					deref(page.Total), page.NextCursor, page.Truncated, len(tc.want))
			}
		})
	}
}

func ids(page *store.TracePage) []string {
	var out []string
	for _, tr := range page.Traces {
		out = append(out, tr.TraceID)
	}
	return out
}

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// pagingTraces are n one-span traces in sc, two at each start time.
func pagingTraces(sc store.Scope, n int) []Seed {
	seeds := make([]Seed, 0, n)
	for i := range n {
		start := windowStart.Add(time.Duration(i/2) * time.Minute)
		seeds = append(seeds, traceSeed(sc, fmt.Sprintf("t%02d", i), start,
			spanSpec{id: "r", name: "invoke_agent bot", dur: time.Second}))
	}
	return seeds
}

// pageAll follows the cursor from the first page and returns every page.
func pageAll(t *testing.T, s store.TraceStore, sc store.Scope, q store.TraceQuery) []*store.TracePage {
	t.Helper()
	var pages []*store.TracePage
	for {
		page, err := s.QueryTraces(context.Background(), sc, q)
		if err != nil {
			t.Fatalf("page %d: %v", len(pages)+1, err)
		}
		pages = append(pages, page)
		if page.NextCursor == "" {
			return pages
		}
		if len(pages) > 100 {
			t.Fatal("paging didn't stop")
		}
		q.Cursor = page.NextCursor
	}
}

// testPaging follows the cursor through a list whose start times tie across
// page edges, in both orders: every trace once, in order, with one total.
func testPaging(t *testing.T, newStore NewStore) {
	const n = 23
	orgA := scopeFor("org-a", "acme")
	s := newStore(t, append(pagingTraces(orgA, n), pagingTraces(scopeFor("org-b", "globex"), n)...))

	for _, order := range []string{"desc", "asc"} {
		t.Run(order, func(t *testing.T) {
			pages := pageAll(t, s, orgA, store.TraceQuery{StartTime: windowStart, EndTime: windowEnd, Limit: 5, SortOrder: order})
			var got []store.Trace
			for i, p := range pages {
				if p.Total == nil || *p.Total != n {
					t.Errorf("page %d total = %v, want %d on every page", i+1, deref(p.Total), n)
				}
				if i < len(pages)-1 && len(p.Traces) != 5 {
					t.Errorf("page %d has %d traces, want 5", i+1, len(p.Traces))
				}
				got = append(got, p.Traces...)
			}
			if len(pages) != 5 {
				t.Errorf("pages = %d, want 5", len(pages))
			}
			seen := map[string]int{}
			for _, tr := range got {
				seen[tr.TraceID]++
			}
			for i := range n {
				if id := fmt.Sprintf("t%02d", i); seen[id] != 1 {
					t.Errorf("%s listed %d times, want once", id, seen[id])
				}
			}
			if len(got) != n {
				t.Errorf("listed %d traces, want %d", len(got), n)
			}
			for i := 1; i < len(got); i++ {
				k := store.CursorKey{Order: order, StartNs: got[i-1].StartTime.UnixNano(), TraceID: got[i-1].TraceID}
				if !k.After(got[i].StartTime.UnixNano(), got[i].TraceID) {
					t.Errorf("%s comes after %s, out of %s order", got[i].TraceID, got[i-1].TraceID, order)
				}
			}
		})
	}
}

// testRareFilter finds all three matches among many traces on the first page.
func testRareFilter(t *testing.T, newStore NewStore) {
	orgA := scopeFor("org-a", "acme")
	seeds := pagingTraces(orgA, 40)
	for _, i := range []int{3, 21, 38} {
		seeds[i] = traceSeed(orgA, seeds[i].Trace.TraceID, seeds[i].Trace.StartTime,
			spanSpec{id: "r", name: "invoke_agent bot", dur: time.Second},
			spanSpec{id: "t", parent: "r", name: "execute_tool lookup", dur: time.Second, status: "Error"})
	}
	s := newStore(t, seeds)
	page, err := s.QueryTraces(context.Background(), orgA, store.TraceQuery{
		StartTime: windowStart, EndTime: windowEnd, Limit: 10, SortOrder: "desc",
		Filters: store.TraceFilters{ToolError: true},
	})
	if err != nil {
		t.Fatalf("QueryTraces: %v", err)
	}
	if got := ids(page); !slices.Equal(got, []string{"t38", "t21", "t03"}) {
		t.Errorf("traces = %v, want t38, t21, t03", got)
	}
	if page.Total == nil || *page.Total != 3 || page.NextCursor != "" {
		t.Errorf("page = total %v, cursor %q; want total 3 and no cursor", deref(page.Total), page.NextCursor)
	}
}

// testInvalidCursor refuses a cursor the store didn't issue for the query.
func testInvalidCursor(t *testing.T, newStore NewStore) {
	ctx := context.Background()
	orgA := scopeFor("org-a", "acme")
	s := newStore(t, pagingTraces(orgA, 4))
	q := store.TraceQuery{StartTime: windowStart, EndTime: windowEnd, Limit: 2, SortOrder: "desc"}
	first, err := s.QueryTraces(ctx, orgA, q)
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first page = %+v, %v; want a cursor", first, err)
	}
	key := store.CursorKey{Order: "desc", StartNs: windowStart.UnixNano(), TraceID: "t01"}
	for name, cursor := range map[string]string{
		"garbage":           "not-a-cursor",
		"bad payload":       first.NextCursor + "!",
		"another backend":   store.EncodeCursor("os1", key),
		"another version":   store.EncodeCursor("zz9", key),
		"another sort":      "", // set below
		"no trace ID":       "",
		"truncated payload": first.NextCursor[:len(first.NextCursor)-4],
	} {
		q := q
		q.Cursor = cursor
		switch name {
		case "another sort":
			q.SortOrder = "asc"
			q.Cursor = first.NextCursor
		case "no trace ID":
			tag, _, _ := strings.Cut(first.NextCursor, ".")
			q.Cursor = store.EncodeCursor(tag, store.CursorKey{Order: "desc", StartNs: key.StartNs})
		}
		if page, err := s.QueryTraces(ctx, orgA, q); !errors.Is(err, store.ErrInvalidCursor) {
			t.Errorf("%s: QueryTraces = %+v, %v; want ErrInvalidCursor", name, page, err)
		}
	}
}

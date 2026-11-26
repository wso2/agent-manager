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
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store"
)

// overview is what a listed trace should show.
type overview struct {
	errorCount   int
	tokens       *store.TokenUsage
	conversation string
	input        string
	output       string
}

func tokens(in, out int64) *store.TokenUsage {
	return &store.TokenUsage{InputTokens: in, OutputTokens: out, TotalTokens: in + out}
}

// overviewTraces are traces that tell each overview rule apart, with what each should show.
func overviewTraces(sc store.Scope) ([]Seed, map[string]overview) {
	at := func(m int) time.Time { return windowStart.Add(time.Duration(m) * time.Minute) }
	sec := time.Second
	seeds := []Seed{
		traceSeed(sc, "status-code", at(1),
			spanSpec{id: "r", name: "invoke_agent bot", dur: sec, status: "Error"}),
		traceSeed(sc, "http-status", at(2),
			spanSpec{id: "r", name: "invoke_agent bot", dur: sec, status: "Unset", attrs: map[string]any{"http.status_code": 500}}),
		traceSeed(sc, "error-type", at(3),
			spanSpec{id: "r", name: "invoke_agent bot", dur: sec, attrs: map[string]any{"error.type": "ValueError"}}),
		traceSeed(sc, "child-error", at(4),
			spanSpec{id: "r", name: "invoke_agent bot", dur: 2 * sec, status: "Ok"},
			spanSpec{id: "t", parent: "r", name: "execute_tool search", at: sec, dur: sec, status: "Error"}),
		// Strands: the agent and its chat wrapper both report the turn's usage, and the inner call repeats it.
		traceSeed(sc, "strands", at(5),
			spanSpec{id: "r", name: "invoke_agent strands", dur: 3 * sec, attrs: with(usage(600, 400), "session.id", "conv-root")},
			spanSpec{id: "c", parent: "r", name: "chat", at: sec, dur: sec, attrs: usage(300, 200)},
			spanSpec{id: "o", parent: "c", name: "openai.chat", at: sec, dur: sec, attrs: usage(300, 200)}),
		traceSeed(sc, "root-tokens", at(6),
			spanSpec{
				id: "r", name: "invoke_agent bot", dur: sec,
				attrs: map[string]any{"gen_ai.usage.prompt_tokens": "40", "gen_ai.usage.completion_tokens": 2},
			}),
		traceSeed(sc, "conv-child", at(7),
			spanSpec{id: "r", name: "invoke_agent bot", dur: 3 * sec},
			spanSpec{id: "w", parent: "r", name: "LangGraph.workflow", at: sec, dur: sec, attrs: map[string]any{"langfuse.session.id": "conv-child"}},
			spanSpec{id: "c", parent: "w", name: "openai.chat", at: 2 * sec, dur: sec, attrs: map[string]any{"gen_ai.conversation.id": "conv-other"}}),
		traceSeed(sc, "conv-leaf", at(8),
			spanSpec{id: "r", name: "invoke_agent bot", dur: 3 * sec},
			spanSpec{
				id: "c", parent: "r", name: "chat gpt-4o", at: sec, dur: sec,
				attrs: map[string]any{"traceloop.association.properties.thread_id": "conv-leaf"},
			}),
		traceSeed(sc, "root-io", at(9),
			spanSpec{
				id: "r", name: "LangGraph.workflow", dur: sec,
				attrs: map[string]any{
					"traceloop.entity.input":  `{"inputs":"hi there","metadata":{"user":"u1"}}`,
					"traceloop.entity.output": `{"outputs":{"messages":[{"kwargs":{"content":"first"}},{"kwargs":{"content":"hello back"}}]}}`,
				},
			}),
		traceSeed(sc, "child-io", at(10),
			spanSpec{id: "r", name: "invoke_agent bot", dur: 3 * sec},
			spanSpec{
				id: "w", parent: "r", name: "LangGraph.workflow", at: sec, dur: sec,
				attrs: map[string]any{"traceloop.entity.input": `{"inputs":"question"}`, "traceloop.entity.output": "plain answer"},
			}),
		// The wrapping chat leaf carries no messages; the output is the last leaf's answer.
		traceSeed(sc, "leaf-io", at(11),
			spanSpec{id: "r", name: "invoke_agent bot", dur: 4 * sec},
			spanSpec{id: "c", parent: "r", name: "chat", at: sec, dur: sec},
			spanSpec{
				id: "o", parent: "c", name: "openai.chat", at: sec + time.Millisecond, dur: sec,
				attrs: map[string]any{
					"gen_ai.input.messages":  `[{"role":"system","content":"be brief"},{"role":"user","content":"what is 2+2"}]`,
					"gen_ai.output.messages": `[{"role":"assistant","parts":[{"type":"text","content":"4"}]}]`,
				},
			},
			spanSpec{
				id: "l", parent: "r", name: "chat gpt-4o", at: 3 * sec, dur: sec,
				attrs: map[string]any{"gen_ai.output.messages": `[{"role":"assistant","content":"final answer"},{"role":"tool","content":"tool output"}]`},
			}),
		traceSeed(sc, "crewai", at(12),
			spanSpec{
				id: "r", name: "Crew_research.workflow", dur: sec,
				attrs: map[string]any{
					"crewai.crew.tasks_output": "research tasks", "crewai.crew.result": "report",
					"traceloop.entity.input": "not read",
				},
			}),
		traceSeed(sc, "none", at(13),
			spanSpec{id: "r", name: "invoke_agent bot", dur: sec}),
		manySpans(sc, "many-spans", at(14)),
	}
	want := map[string]overview{
		"status-code": {errorCount: 1},
		"http-status": {errorCount: 1},
		"error-type":  {errorCount: 1},
		"child-error": {},
		"strands":     {tokens: tokens(300, 200), conversation: "conv-root"},
		"root-tokens": {tokens: tokens(40, 2)},
		"conv-child":  {conversation: "conv-child"},
		"conv-leaf":   {conversation: "conv-leaf"},
		"root-io":     {input: `{"inputs":"hi there","metadata":{"user":"u1"}}`, output: "hello back"},
		"child-io":    {input: `"question"`, output: "plain answer"},
		"leaf-io":     {input: "what is 2+2", output: "final answer"},
		"crewai":      {input: "research tasks", output: "report"},
		"none":        {},
		"many-spans":  {},
	}
	return seeds, want
}

// manySpans is a trace past the span count whose LLM leaves are read for input and output.
func manySpans(sc store.Scope, traceID string, start time.Time) Seed {
	specs := []spanSpec{{id: "r", name: "invoke_agent bot", dur: time.Minute}}
	for i := range store.MaxIOSpanCount - 1 {
		specs = append(specs, spanSpec{id: fmt.Sprint("s", i), parent: "r", name: "step.task", at: time.Second, dur: time.Second})
	}
	specs = append(specs, spanSpec{
		id: "c", parent: "r", name: "chat gpt-4o", at: 2 * time.Second, dur: time.Second,
		attrs: map[string]any{"gen_ai.input.messages": `[{"role":"user","content":"unread"}]`},
	})
	return traceSeed(sc, traceID, start, specs...)
}

// testOverview lists each trace's status, tokens, conversation, input and
// output, and checks that the filters agree with what the list shows.
func testOverview(t *testing.T, newStore NewStore) {
	ctx := context.Background()
	orgA := scopeFor("org-a", "acme")
	seeds, want := overviewTraces(orgA)
	// Another org's trace with a colliding ID has its own overview.
	orgB := scopeFor("org-b", "globex")
	seeds = append(seeds, traceSeed(orgB, "none", windowStart.Add(13*time.Minute),
		spanSpec{
			id: "r", name: "invoke_agent bot", dur: time.Second, status: "Error",
			attrs: with(usage(1, 1), "session.id", "conv-b", "traceloop.entity.input", "b"),
		}))
	s := newStore(t, seeds)

	list := func(f store.TraceFilters) []store.Trace {
		t.Helper()
		page, err := s.QueryTraces(ctx, orgA, store.TraceQuery{
			StartTime: windowStart, EndTime: windowEnd, Limit: 100, SortOrder: "asc", Filters: f,
		})
		if err != nil {
			t.Fatalf("QueryTraces(%+v): %v", f, err)
		}
		return page.Traces
	}

	listed := list(store.TraceFilters{})
	if len(listed) != len(want) {
		t.Fatalf("listed %d traces, want %d", len(listed), len(want))
	}
	for _, tr := range listed {
		w := want[tr.TraceID]
		if tr.Status == nil || tr.Status.ErrorCount != w.errorCount {
			t.Errorf("%s: status = %+v, want errorCount %d", tr.TraceID, tr.Status, w.errorCount)
		}
		if !equalTokens(tr.Tokens, w.tokens) {
			t.Errorf("%s: tokens = %+v, want %+v", tr.TraceID, tr.Tokens, w.tokens)
		}
		if tr.ConversationID != w.conversation {
			t.Errorf("%s: conversation = %q, want %q", tr.TraceID, tr.ConversationID, w.conversation)
		}
		if tr.Input != w.input || tr.Output != w.output {
			t.Errorf("%s: input, output = %q, %q; want %q, %q", tr.TraceID, tr.Input, tr.Output, w.input, w.output)
		}
	}

	// The filters keep exactly the traces whose overview matches them.
	where := func(keep func(store.Trace) bool) []string {
		var out []string
		for _, tr := range listed {
			if keep(tr) {
				out = append(out, tr.TraceID)
			}
		}
		return out
	}
	for name, tc := range map[string]struct {
		f    store.TraceFilters
		want []string
	}{
		"status error": {store.TraceFilters{Status: "error"}, where(func(tr store.Trace) bool { return tr.Status.ErrorCount > 0 })},
		"status ok":    {store.TraceFilters{Status: "ok"}, where(func(tr store.Trace) bool { return tr.Status.ErrorCount == 0 })},
		"min tokens 0": {store.TraceFilters{MinTokens: i64(0)}, where(func(tr store.Trace) bool { return tr.Tokens != nil })},
		"missing conversation": {
			store.TraceFilters{MissingConversationID: true}, where(func(tr store.Trace) bool { return tr.ConversationID == "" }),
		},
	} {
		if got := ids(&store.TracePage{Traces: list(tc.f)}); !slices.Equal(got, tc.want) {
			t.Errorf("%s: traces = %v, want %v", name, got, tc.want)
		}
	}
	for _, tr := range listed {
		if tr.Tokens != nil {
			if got := ids(&store.TracePage{Traces: list(store.TraceFilters{MinTokens: i64(tr.Tokens.TotalTokens)})}); !slices.Contains(got, tr.TraceID) {
				t.Errorf("%s: minTokens=%d leaves it out", tr.TraceID, tr.Tokens.TotalTokens)
			}
			if got := ids(&store.TracePage{Traces: list(store.TraceFilters{MinTokens: i64(tr.Tokens.TotalTokens + 1)})}); slices.Contains(got, tr.TraceID) {
				t.Errorf("%s: minTokens=%d keeps it", tr.TraceID, tr.Tokens.TotalTokens+1)
			}
		}
		if tr.ConversationID != "" {
			if got := ids(&store.TracePage{Traces: list(store.TraceFilters{ConversationID: tr.ConversationID})}); !slices.Contains(got, tr.TraceID) {
				t.Errorf("%s: conversationId=%s leaves it out", tr.TraceID, tr.ConversationID)
			}
		}
	}

	// A later page has its overview too.
	page, err := s.QueryTraces(ctx, orgA, store.TraceQuery{StartTime: windowStart, EndTime: windowEnd, Limit: 1, SortOrder: "desc"})
	if err != nil {
		t.Fatal(err)
	}
	next, err := s.QueryTraces(ctx, orgA, store.TraceQuery{
		StartTime: windowStart, EndTime: windowEnd, Limit: 1, SortOrder: "desc", Cursor: page.NextCursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Traces) != 1 || next.Traces[0].TraceID != "none" || next.Traces[0].Status == nil ||
		next.Traces[0].Status.ErrorCount != 0 || next.Traces[0].ConversationID != "" {
		t.Errorf("second page = %+v, want org-a's trace none with its own overview", next.Traces)
	}
}

func equalTokens(a, b *store.TokenUsage) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

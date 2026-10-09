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
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

// costRootCompleteFake is costFake(true) with roots that fill input, output
// and tokens, so the cascade reads only the root.
func costRootCompleteFake() *fakeObserverClient {
	fake := costFake(true)
	for _, info := range fake.traces {
		// The span list and span details share the root's attribute map.
		maps.Copy(fake.spanDetails[info.RootSpanID].Attributes, completeRootAttrs())
	}
	return fake
}

// costOverCapFake is costRootCompleteFake with every trace over maxToolListSpans.
func costOverCapFake() *fakeObserverClient {
	fake := costRootCompleteFake()
	for i := range fake.traces {
		fake.traces[i].SpanCount = maxToolListSpans + 1
	}
	return fake
}

// includeTools turns on include=tools, and include=models with models set.
func includeTools(models bool) func(p *TraceQueryParams) {
	return func(p *TraceQueryParams) { p.Include = Include{Models: models, Tools: true} }
}

// costRowNamed runs the baseline scenario with this name.
func costRowNamed(t *testing.T, name string) costRow {
	t.Helper()
	for _, sc := range costScenarios() {
		if sc.name == name {
			return runCostScenario(t, sc)
		}
	}
	t.Fatalf("no baseline scenario %q", name)
	return costRow{}
}

// assertSameCost checks that two rows made the same calls and read the same bytes.
func assertSameCost(t *testing.T, got, want costRow) {
	t.Helper()
	if got.Calls != want.Calls {
		t.Errorf("calls = %s, want %s", got.Calls.literal(), want.Calls.literal())
	}
	if got.Bytes != want.Bytes {
		t.Errorf("bytes = %s, want %s", got.Bytes.literal(), want.Bytes.literal())
	}
}

// The export's failed MCP call: the outer execute_tool span is unset, the
// inner .tool span errored. Both name search_issues.
func TestToolsFromSpanList_ExportMCPTrace(t *testing.T) {
	at := func(ms int) time.Time { return lookBackWindowEnd.Add(time.Duration(ms) * time.Millisecond) }
	unset := &observer.SpanStatus{Code: "unset"}
	spans := []observer.SpanInfo{
		{SpanID: "s1", SpanName: "execute_task tools", StartTime: at(4), Status: unset},
		{SpanID: "s2", SpanName: "execute_tool search_issues", StartTime: at(11), Status: unset},
		{SpanID: "s3", SpanName: "execute_tool check_system_status", StartTime: at(23), Status: unset},
		{SpanID: "s4", SpanName: "initialize.mcp", StartTime: at(500), Status: &observer.SpanStatus{Code: "ok"}},
		{SpanID: "s5", SpanName: "search_issues.tool", StartTime: at(1710), Status: &observer.SpanStatus{Code: "error", Message: "failed to search issues"}},
		{SpanID: "s6", SpanName: "ChatOpenAI.chat", StartTime: at(5212), Status: unset},
		// Upstream lists the root last.
		{SpanID: "root", SpanName: "invoke_agent LangGraph", StartTime: at(0), Status: unset},
	}
	tools, failed := toolsFromSpanList(spans)
	assertModels(t, tools, []string{"search_issues", "check_system_status"})
	assertModels(t, failed, []string{"search_issues"})
}

// Only names and OTel statuses count: tool attributes and error attributes don't.
func TestToolsFromSpanList_IgnoresAttributes(t *testing.T) {
	spans := []observer.SpanInfo{
		{SpanID: "a", SpanName: "lookup.tool", StartTime: lookBackWindowEnd,
			Attributes: map[string]interface{}{"error.type": "tool_error", "gen_ai.tool.status": "error"}},
		{SpanID: "b", SpanName: "search_issues", StartTime: lookBackWindowEnd,
			Attributes: map[string]interface{}{"gen_ai.tool.name": "search_issues", "openinference.span.kind": "TOOL"}},
	}
	tools, failed := toolsFromSpanList(spans)
	assertModels(t, tools, []string{"lookup"})
	if failed != nil {
		t.Errorf("failedTools = %v, want none", failed)
	}
}

// Names are distinct, in first-call order, and skip unnamed tool spans; a
// later failed call fails the tool.
func TestToolsFromSpanList_DistinctInStartOrder(t *testing.T) {
	at := func(s int) time.Time { return lookBackWindowEnd.Add(time.Duration(s) * time.Second) }
	errored := &observer.SpanStatus{Code: "Error"}
	spans := []observer.SpanInfo{
		{SpanID: "a", SpanName: "execute_tool b_tool", StartTime: at(3), Status: errored},
		{SpanID: "b", SpanName: "execute_tool a_tool", StartTime: at(1)},
		{SpanID: "c", SpanName: "execute_tool", StartTime: at(0), Status: errored},
		{SpanID: "d", SpanName: "b_tool.tool", StartTime: at(2)},
		{SpanID: "e", SpanName: "execute_tool a_tool", StartTime: at(4)},
	}
	tools, failed := toolsFromSpanList(spans)
	assertModels(t, tools, []string{"a_tool", "b_tool"})
	assertModels(t, failed, []string{"b_tool"})

	if tools, failed := toolsFromSpanList(spans[2:3]); tools != nil || failed != nil {
		t.Errorf("unnamed tool only: tools = %v, failedTools = %v, want none", tools, failed)
	}
}

// A LangGraph root lacks input and output, so the cascade lists the spans
// anyway: include=tools costs nothing.
func TestIncludeTools_LangGraphAddsNoCall(t *testing.T) {
	assertSameCost(t, costRowNamed(t, "list include=tools"), costRowNamed(t, "list default limit=10"))
}

// A root-complete trace gets one attribute-free span list and no span details.
func TestIncludeTools_RootCompleteAddsOneList(t *testing.T) {
	base := costRowNamed(t, "list root-complete")
	got := costRowNamed(t, "list root-complete include=tools")
	if base.Calls.Spans != 0 || base.Calls.AttrSpans != 0 {
		t.Fatalf("root-complete base lists spans: %s", base.Calls.literal())
	}
	want := base.Calls
	want.Spans += 10
	if got.Calls != want {
		t.Errorf("calls = %s, want %s", got.Calls.literal(), want.literal())
	}
	if got.Order != "trace-0000: root, spans" {
		t.Errorf("order = %q, want one root and one attribute-free list", got.Order)
	}
}

// An over-cap trace costs nothing extra.
func TestIncludeTools_OverCapAddsNoCall(t *testing.T) {
	if n := costOverCapFake().traces[0].SpanCount; n <= maxToolListSpans {
		t.Fatalf("SpanCount = %d, want over %d", n, maxToolListSpans)
	}
	got := runCostScenario(t, costScenario{name: "over cap include=tools", fixture: costOverCapFake, params: includeTools(false)})
	base := runCostScenario(t, costScenario{name: "over cap", fixture: costOverCapFake})
	assertSameCost(t, got, base)
}

// include=models,tools reads one span list per trace, the same one include=models reads.
func TestIncludeTools_WithModelsOneListPerTrace(t *testing.T) {
	for _, fixture := range []func() *fakeObserverClient{nil, costRootCompleteFake} {
		got := runCostScenario(t, costScenario{name: "models,tools", fixture: fixture, params: includeTools(true)})
		base := runCostScenario(t, costScenario{name: "models", fixture: fixture, params: func(p *TraceQueryParams) { p.Include.Models = true }})
		assertSameCost(t, got, base)
		if lists := got.Calls.Spans + got.Calls.AttrSpans; lists != 10 {
			t.Errorf("span lists = %d, want 10, one per trace", lists)
		}
	}
}

// Every 3rd trace calls search_issues and every 30th fails it. Rows report
// them with include=tools, whichever list the cascade read, and not over the cap.
func TestIncludeTools_Rows(t *testing.T) {
	tests := []struct {
		name      string
		fixture   func() *fakeObserverClient
		include   Include
		wantTools bool
	}{
		{name: "langgraph", fixture: func() *fakeObserverClient { return costFake(true) }, include: Include{Tools: true}, wantTools: true},
		{name: "langgraph with models", fixture: func() *fakeObserverClient { return costFake(true) }, include: Include{Models: true, Tools: true}, wantTools: true},
		{name: "root-complete", fixture: costRootCompleteFake, include: Include{Tools: true}, wantTools: true},
		{name: "root-complete with models", fixture: costRootCompleteFake, include: Include{Models: true, Tools: true}, wantTools: true},
		{name: "over cap", fixture: costOverCapFake, include: Include{Tools: true}},
		{name: "not asked", fixture: func() *fakeObserverClient { return costFake(true) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := lookBackParams(31)
			params.Filters = TraceFilters{}
			params.Include = tt.include
			resp, err := NewTracingController(tt.fixture()).GetTraceOverviews(t.Context(), params)
			if err != nil {
				t.Fatalf("GetTraceOverviews: %v", err)
			}
			if len(resp.Traces) != 31 {
				t.Fatalf("got %d traces, want 31", len(resp.Traces))
			}
			for _, ov := range resp.Traces {
				i := traceIndex(t, ov.TraceID)
				var wantTools, wantFailed []string
				if tt.wantTools && i%3 == 0 {
					wantTools = []string{"search_issues"}
					if i%30 == 0 {
						wantFailed = wantTools
					}
				}
				if !slices.Equal(ov.Tools, wantTools) || !slices.Equal(ov.FailedTools, wantFailed) {
					t.Errorf("%s: tools = %v, failedTools = %v, want %v, %v", ov.TraceID, ov.Tools, ov.FailedTools, wantTools, wantFailed)
				}
			}
		})
	}
}

// A root-complete trace whose list fetch fails keeps its row without tools,
// logs the list warning and isn't counted as failed.
func TestIncludeTools_ListFetchFailureKeepsRow(t *testing.T) {
	fake := costRootCompleteFake()
	fake.failCall = func(id string) error {
		if id == "trace-0000" {
			return failEvery(id)
		}
		return nil
	}
	params := lookBackParams(3)
	params.Filters = TraceFilters{}
	params.Include.Tools = true
	ctx, logs := logContext()
	resp, err := NewTracingController(fake).GetTraceOverviews(ctx, params)
	if err != nil {
		t.Fatalf("GetTraceOverviews: %v", err)
	}
	if len(resp.Traces) != 3 || resp.Traces[0].TraceID != "trace-0000" {
		t.Fatalf("traces = %v, want trace-0000 first of 3", resp.Traces)
	}
	if got := resp.Traces[0]; got.Tools != nil || got.Input == nil {
		t.Errorf("trace-0000: tools = %v, input = %v; want no tools and the root's input", got.Tools, got.Input)
	}
	if got := resp.Traces[1]; got.Tools != nil {
		t.Errorf("trace-0001 calls no tool, got %v", got.Tools)
	}
	want := []string{"enrichTraceOverview: QueryTraceSpans failed, skipping enrichment"}
	if got := warnings(logs); !slices.Equal(got, want) {
		t.Errorf("warnings = %q, want %q", got, want)
	}
	if n := len(fake.spansTraceIDs); n != 3 {
		t.Errorf("span list calls = %d, want 3, one per trace and no retry", n)
	}
}

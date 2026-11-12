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
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/opensearch"
	"github.com/wso2/agent-manager/agent-manager-observer/store/openchoreo"
)

// threadIDKey is where Traceloop's LangChain instrumentation puts LangGraph's thread_id.
const threadIDKey = "traceloop.association.properties.thread_id"

// setThreads puts thread(i) on trace i's chain span (with onChain) and leaves,
// as Traceloop does. "" sets nothing.
func setThreads(fake *fakeObserverClient, onChain bool, thread func(i int) string) {
	for i, info := range fake.traces {
		id := thread(i)
		if id == "" {
			continue
		}
		spans := fake.spansByTrace[info.TraceID]
		for _, s := range spans[:len(spans)-1] {
			if s.SpanName == "LangGraph.workflow" && !onChain {
				continue
			}
			s.Attributes[threadIDKey] = id
		}
	}
}

// threadOf names trace i's thread.
func threadOf(i int) string { return fmt.Sprintf("thread-%04d", i) }

// A root without a conversation ID takes it from the spans the cascade reads
// anyway, so the row costs the same 5 calls.
func TestGetTraceOverviews_ConversationIDFromChildSpans(t *testing.T) {
	tests := []struct {
		name string
		// onChain puts the thread on the chain span; without it only the
		// leaves carry it, as under a FastAPI root whose earliest child is
		// its http receive span.
		onChain   bool
		rootAttrs func(i int) map[string]interface{}
		want      func(i int) string
	}{
		{name: "chain span", onChain: true, rootAttrs: noRootAttrs, want: threadOf},
		{name: "leaves only", rootAttrs: noRootAttrs, want: threadOf},
		{name: "root wins", onChain: true,
			rootAttrs: func(int) map[string]interface{} {
				return map[string]interface{}{"gen_ai.conversation.id": "conv-root"}
			},
			want: func(int) string { return "conv-root" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := langGraphFake(10, tt.rootAttrs)
			setThreads(fake, tt.onChain, threadOf)
			params := lookBackParams(10)
			params.Filters = TraceFilters{}

			pages := pagesOf(t, NewTracingController(openchoreo.New(fake)), params, 1)

			if len(pages[0].Traces) != 10 {
				t.Fatalf("got %d traces, want 10", len(pages[0].Traces))
			}
			for _, ov := range pages[0].Traces {
				if want := tt.want(traceIndex(t, ov.TraceID)); ov.ConversationID != want {
					t.Errorf("%s conversationId = %q, want %q", ov.TraceID, ov.ConversationID, want)
				}
			}
			if got := upstreamCalls(fake); got != 1+10*5 {
				t.Errorf("upstream calls = %d, want %d (one trace query, 5 per trace)", got, 1+10*5)
			}
		})
	}
}

// conversationId on traces whose root has none: the chain span rules out
// another thread before the leaf fetches, and a trace with no thread at all
// is read in full and left out.
func TestGetTraceOverviews_ConversationIDFilterReadsChildSpans(t *testing.T) {
	fake := langGraphFake(100, noRootAttrs)
	setThreads(fake, true, func(i int) string {
		switch i % 10 {
		case 0:
			return "thread-match"
		case 5:
			return ""
		}
		return "thread-other"
	})
	params := lookBackParams(5)
	params.Filters.ConversationID = "thread-match"

	ids, _, truncated := traceIDs(NewTracingController(openchoreo.New(fake)), t, params)

	want := []string{"trace-0000", "trace-0010", "trace-0020", "trace-0030", "trace-0040"}
	if !slices.Equal(ids, want) || truncated {
		t.Fatalf("traces = %v, truncated %v; want %v, false", ids, truncated, want)
	}
	// The first chunk of 50 fills the page: a root, span list and chain per
	// trace, and two leaves for each of the 5 matches and 5 threadless traces.
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 50+50+10*2 {
		t.Errorf("GetSpanDetails calls = %d, want %d", got, 50+50+10*2)
	}
	if got := atomic.LoadInt32(&fake.queryTraceSpansCalls); got != 50 {
		t.Errorf("QueryTraceSpans calls = %d, want 50", got)
	}
	for _, id := range fake.detailSpanIDs {
		if i := traceIndex(t, id); strings.HasPrefix(id, "leaf-") && i%5 != 0 {
			t.Errorf("fetched %s, whose chain span names another thread", id)
		}
	}
}

// A root without a conversation ID whose span list can't be read is listed as
// failed under a conversationId filter, since its thread is unknown. A complete
// root is no exception.
func TestGetTraceOverviews_ConversationIDFilterSpanListFails(t *testing.T) {
	for name, rootAttrs := range map[string]func(int) map[string]interface{}{
		"incomplete root": noRootAttrs,
		"complete root":   completeRootNoID,
	} {
		t.Run(name, func(t *testing.T) {
			fake := langGraphFake(20, rootAttrs)
			setThreads(fake, true, func(int) string { return "thread-match" })
			fake.failCall = func(id string) error {
				if id == "trace-0003" {
					return failEvery(id)
				}
				return nil
			}
			params := lookBackParams(20)
			params.Filters.ConversationID = "thread-match"
			ctx, logs := logContext()

			resp, err := NewTracingController(openchoreo.New(fake)).GetTraceOverviews(ctx, params)
			if err != nil {
				t.Fatalf("GetTraceOverviews returned error: %v", err)
			}

			if len(resp.Traces) != 19 || slices.ContainsFunc(resp.Traces, func(ov opensearch.TraceOverview) bool {
				return ov.TraceID == "trace-0003"
			}) {
				t.Errorf("got %d traces, want the 19 besides trace-0003", len(resp.Traces))
			}
			if got := logField(t, logs, "Retrieved trace overviews", "failed"); got != float64(1) {
				t.Errorf("failed logged as %v, want 1", got)
			}
		})
	}
}

// completeRootNoID is a root that fills input, output and tokens by itself and
// names no conversation.
func completeRootNoID(int) map[string]interface{} {
	attrs := completeRootAttrs()
	delete(attrs, "gen_ai.conversation.id")
	return attrs
}

// A complete root without a conversation ID doesn't end the cascade under a
// conversationId filter: the chain span names the thread, and one naming
// another thread rules the trace out before the leaf fetches.
func TestGetTraceOverviews_ConversationIDFilterCompleteRootReadsChild(t *testing.T) {
	fake := langGraphFake(30, completeRootNoID)
	setThreads(fake, true, func(i int) string {
		if i%10 == 0 {
			return "thread-match"
		}
		return "thread-other"
	})
	params := lookBackParams(10)
	params.Filters.ConversationID = "thread-match"

	ids, _, truncated := traceIDs(NewTracingController(openchoreo.New(fake)), t, params)

	want := []string{"trace-0000", "trace-0010", "trace-0020"}
	if !slices.Equal(ids, want) || truncated {
		t.Fatalf("traces = %v, truncated %v; want %v, false", ids, truncated, want)
	}
	// A root and chain per trace, and a span list.
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 30*2 {
		t.Errorf("GetSpanDetails calls = %d, want %d", got, 30*2)
	}
	if got := atomic.LoadInt32(&fake.queryTraceSpansCalls); got != 30 {
		t.Errorf("QueryTraceSpans calls = %d, want 30", got)
	}
	for _, id := range fake.detailSpanIDs {
		if strings.HasPrefix(id, "leaf-") {
			t.Errorf("fetched %s, though the chain span settled its trace", id)
		}
	}
}

// A complete root with no ID on any span is read in full and left out. It isn't
// listed as failed, since every read succeeded.
func TestGetTraceOverviews_ConversationIDFilterCompleteRootNoIDAnywhere(t *testing.T) {
	fake := langGraphFake(10, completeRootNoID)
	params := lookBackParams(10)
	ctx, logs := logContext()

	resp, err := NewTracingController(openchoreo.New(fake)).GetTraceOverviews(ctx, params)
	if err != nil {
		t.Fatalf("GetTraceOverviews returned error: %v", err)
	}

	if len(resp.Traces) != 0 || resp.Truncated {
		t.Fatalf("got %d traces, truncated %v; want 0, false", len(resp.Traces), resp.Truncated)
	}
	// A root, chain and two leaves per trace.
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 10*4 {
		t.Errorf("GetSpanDetails calls = %d, want %d", got, 10*4)
	}
	if got := logField(t, logs, "Retrieved trace overviews", "failed"); got != float64(0) {
		t.Errorf("failed logged as %v, want 0", got)
	}
}

// A span the ID may sit on that can't be read lists the trace as failed rather
// than absent or matched, unless an ID already found ahead of it settles it.
func TestGetTraceOverviews_ConversationIDFilterUnreadSpans(t *testing.T) {
	tests := []struct {
		name string
		// onChain puts the thread on the chain span as well as the leaves.
		onChain bool
		fail    string
		failed  bool
	}{
		{name: "chain span", onChain: true, fail: "chain-0003", failed: true},
		{name: "leaf ahead of the ID", fail: "leaf-a-0003", failed: true},
		{name: "leaf after the ID", fail: "leaf-b-0003"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := langGraphFake(20, noRootAttrs)
			setThreads(fake, tt.onChain, func(int) string { return "thread-match" })
			// leaf-b starts after leaf-a, so the leaf order is fixed.
			spans := fake.spansByTrace["trace-0003"]
			spans[2].StartTime = spans[1].StartTime.Add(time.Second)
			fake.failCall = failTimes(map[string]int{tt.fail: -1})
			params := lookBackParams(20)
			params.Filters.ConversationID = "thread-match"
			ctx, logs := logContext()

			resp, err := NewTracingController(openchoreo.New(fake)).GetTraceOverviews(ctx, params)
			if err != nil {
				t.Fatalf("GetTraceOverviews returned error: %v", err)
			}

			matched := slices.ContainsFunc(resp.Traces, func(ov opensearch.TraceOverview) bool {
				return ov.TraceID == "trace-0003"
			})
			wantTraces, wantFailed := 20, float64(0)
			if tt.failed {
				wantTraces, wantFailed = 19, 1
			}
			if len(resp.Traces) != wantTraces || matched == tt.failed {
				t.Errorf("got %d traces, trace-0003 matched %v; want %d, %v", len(resp.Traces), matched, wantTraces, !tt.failed)
			}
			if got := logField(t, logs, "Retrieved trace overviews", "failed"); got != wantFailed {
				t.Errorf("failed logged as %v, want %v", got, wantFailed)
			}
		})
	}
}

// Under a conversationId filter, a trace whose ID can only sit on leaves it
// didn't read is listed as failed: leaves skipped over the span threshold, or
// cut off by the cap.
func TestGetTraceOverviews_ConversationIDFilterUnreadLeaves(t *testing.T) {
	tests := []struct {
		name  string
		setup func(fake *fakeObserverClient)
	}{
		{name: "over the span threshold", setup: func(fake *fakeObserverClient) {
			fake.traces[3].SpanCount = skipLeafAggregationSpanCountThreshold + 1
		}},
		{name: "over the leaf cap", setup: fillLeafCap},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := langGraphFake(20, noRootAttrs)
			setThreads(fake, false, func(int) string { return "thread-match" })
			tt.setup(fake)
			params := lookBackParams(20)
			params.Filters.ConversationID = "thread-match"
			c := NewTracingController(openchoreo.New(fake))
			ctx, logs := logContext()

			resp, err := c.GetTraceOverviews(ctx, params)
			if err != nil {
				t.Fatalf("GetTraceOverviews returned error: %v", err)
			}

			if len(resp.Traces) != 19 || slices.ContainsFunc(resp.Traces, func(ov opensearch.TraceOverview) bool {
				return ov.TraceID == "trace-0003"
			}) {
				t.Errorf("got %d traces, want the 19 besides trace-0003", len(resp.Traces))
			}
			if got := logField(t, logs, "Retrieved trace overviews", "failed"); got != float64(1) {
				t.Errorf("failed logged as %v, want 1", got)
			}
			export := mustExport(t, c, params)
			if want := []string{"trace-0003"}; !slices.Equal(export.FailedTraceIDs, want) {
				t.Errorf("failedTraceIds = %v, want %v", export.FailedTraceIDs, want)
			}
		})
	}
}

// fillLeafCap gives trace 3 enough leaves without an ID, starting first, to
// fill the leaf cap.
func fillLeafCap(fake *fakeObserverClient) {
	info := &fake.traces[3]
	extra := make([]observer.SpanInfo, maxLLMLeavesPerTrace)
	for j := range extra {
		extra[j] = observer.SpanInfo{SpanID: fmt.Sprintf("leaf-x%02d-0003", j), SpanName: "ChatOpenAI.chat",
			ParentSpanID: "chain-0003", StartTime: info.StartTime.Add(-time.Duration(len(extra)-j) * time.Millisecond),
			Attributes: map[string]interface{}{}}
		fake.spanDetails[extra[j].SpanID] = &observer.SpanDetailsResponse{
			SpanID: extra[j].SpanID, SpanName: extra[j].SpanName, ParentSpanID: extra[j].ParentSpanID, Attributes: extra[j].Attributes,
		}
	}
	spans := append(extra, fake.spansByTrace[info.TraceID]...)
	fake.spansByTrace[info.TraceID] = spans
	info.SpanCount = len(spans)
}

// missingConversationParams lists traces without a conversation ID.
func missingConversationParams(limit int) TraceQueryParams {
	params := lookBackParams(limit)
	params.Filters = TraceFilters{Missing: MissingFields{ConversationID: true}}
	return params
}

// missing=conversationId keeps the traces with no ID on any span. An ID on the
// root rules a trace out before any other fetch, and one on the chain span
// before the leaf fetches.
func TestGetTraceOverviews_MissingConversationID(t *testing.T) {
	fake := langGraphFake(40, func(i int) map[string]interface{} {
		if i%10 == 1 {
			return map[string]interface{}{"gen_ai.conversation.id": "conv-root"}
		}
		return nil
	})
	setThreads(fake, true, func(i int) string {
		if i%10 == 2 {
			return "thread-chain"
		}
		return ""
	})
	setThreads(fake, false, func(i int) string {
		if i%10 == 3 {
			return "thread-leaf"
		}
		return ""
	})

	ids, _, truncated := traceIDs(NewTracingController(openchoreo.New(fake)), t, missingConversationParams(10))

	want := []string{"trace-0000", "trace-0004", "trace-0005", "trace-0006", "trace-0007",
		"trace-0008", "trace-0009", "trace-0010", "trace-0014", "trace-0015"}
	if !slices.Equal(ids, want) || truncated {
		t.Fatalf("traces = %v, truncated %v; want %v, false", ids, truncated, want)
	}
	for _, id := range fake.detailSpanIDs {
		switch i := traceIndex(t, id) % 10; {
		case i == 1 && !strings.HasPrefix(id, "root-"):
			t.Errorf("fetched %s, though its root names a conversation", id)
		case i == 2 && strings.HasPrefix(id, "leaf-"):
			t.Errorf("fetched %s, though its chain span names a conversation", id)
		}
	}
}

// Under missing=conversationId, a trace whose ID may sit on a span it didn't
// read is listed as failed: a failed span list, chain span or leaf, leaves
// skipped over the span threshold, or leaves cut off by the cap.
func TestGetTraceOverviews_MissingConversationIDUnreadSpans(t *testing.T) {
	tests := []struct {
		name  string
		setup func(fake *fakeObserverClient)
	}{
		{name: "span list", setup: func(fake *fakeObserverClient) {
			fake.failCall = func(id string) error {
				if id == "trace-0003" {
					return failEvery(id)
				}
				return nil
			}
		}},
		{name: "chain span", setup: func(fake *fakeObserverClient) {
			fake.failCall = failTimes(map[string]int{"chain-0003": -1})
		}},
		{name: "leaf", setup: func(fake *fakeObserverClient) {
			fake.failCall = failTimes(map[string]int{"leaf-b-0003": -1})
		}},
		{name: "over the span threshold", setup: func(fake *fakeObserverClient) {
			fake.traces[3].SpanCount = skipLeafAggregationSpanCountThreshold + 1
		}},
		{name: "over the leaf cap", setup: fillLeafCap},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := langGraphFake(20, completeRootNoID)
			tt.setup(fake)
			params := missingConversationParams(20)
			c := NewTracingController(openchoreo.New(fake))
			ctx, logs := logContext()

			resp, err := c.GetTraceOverviews(ctx, params)
			if err != nil {
				t.Fatalf("GetTraceOverviews returned error: %v", err)
			}

			if len(resp.Traces) != 19 || slices.ContainsFunc(resp.Traces, func(ov opensearch.TraceOverview) bool {
				return ov.TraceID == "trace-0003"
			}) {
				t.Errorf("got %d traces, want the 19 besides trace-0003", len(resp.Traces))
			}
			if got := logField(t, logs, "Retrieved trace overviews", "failed"); got != float64(1) {
				t.Errorf("failed logged as %v, want 1", got)
			}
			export := mustExport(t, c, params)
			if want := []string{"trace-0003"}; !slices.Equal(export.FailedTraceIDs, want) {
				t.Errorf("failedTraceIds = %v, want %v", export.FailedTraceIDs, want)
			}
		})
	}
}

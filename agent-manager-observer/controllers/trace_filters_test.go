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
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/opensearch"
)

func ptr(v int64) *int64 { return &v }

// filterOverview: errored, 5s, 12 spans, 1000 tokens, two models, conv-1.
func filterOverview() opensearch.TraceOverview {
	return opensearch.TraceOverview{
		TraceID:         "trace-1",
		DurationInNanos: int64(5 * time.Second),
		SpanCount:       12,
		TokenUsage:      &opensearch.TokenUsage{InputTokens: 800, OutputTokens: 200, TotalTokens: 1000},
		Status:          &opensearch.TraceStatus{ErrorCount: 2},
		Models:          []string{"gpt-4o", "gpt-4o-mini"},
		ConversationID:  "conv-1",
	}
}

func TestMatchesFilters(t *testing.T) {
	ok := filterOverview()
	ok.Status = &opensearch.TraceStatus{ErrorCount: 0}

	noTokens := filterOverview()
	noTokens.TokenUsage = nil

	noStatus := filterOverview()
	noStatus.Status = nil

	noModels := filterOverview()
	noModels.Models = nil

	tests := []struct {
		name     string
		overview opensearch.TraceOverview
		filters  TraceFilters
		want     bool
	}{
		{name: "empty filter matches", overview: filterOverview(), filters: TraceFilters{}, want: true},
		{name: "empty filter matches a bare overview", overview: opensearch.TraceOverview{}, filters: TraceFilters{}, want: true},

		{name: "status error matches errored trace", overview: filterOverview(), filters: TraceFilters{Status: TraceStatusError}, want: true},
		{name: "status error rejects zero errors", overview: ok, filters: TraceFilters{Status: TraceStatusError}, want: false},
		{name: "status ok matches zero errors", overview: ok, filters: TraceFilters{Status: TraceStatusOK}, want: true},
		{name: "status ok rejects errored trace", overview: filterOverview(), filters: TraceFilters{Status: TraceStatusOK}, want: false},
		{name: "status error rejects nil status", overview: noStatus, filters: TraceFilters{Status: TraceStatusError}, want: false},
		{name: "status ok matches nil status", overview: noStatus, filters: TraceFilters{Status: TraceStatusOK}, want: true},

		{name: "minDurationMs at duration matches", overview: filterOverview(), filters: TraceFilters{MinDurationMs: ptr(5000)}, want: true},
		{name: "minDurationMs above duration rejects", overview: filterOverview(), filters: TraceFilters{MinDurationMs: ptr(5001)}, want: false},
		{name: "minDurationMs zero matches", overview: opensearch.TraceOverview{}, filters: TraceFilters{MinDurationMs: ptr(0)}, want: true},
		{name: "huge minDurationMs rejects without overflow", overview: filterOverview(), filters: TraceFilters{MinDurationMs: ptr(1 << 62)}, want: false},

		{name: "minTokens at total matches", overview: filterOverview(), filters: TraceFilters{MinTokens: ptr(1000)}, want: true},
		{name: "minTokens above total rejects", overview: filterOverview(), filters: TraceFilters{MinTokens: ptr(1001)}, want: false},
		{name: "minTokens rejects nil token usage", overview: noTokens, filters: TraceFilters{MinTokens: ptr(1)}, want: false},
		{name: "minTokens zero rejects nil token usage", overview: noTokens, filters: TraceFilters{MinTokens: ptr(0)}, want: false},

		{name: "minSpanCount at count matches", overview: filterOverview(), filters: TraceFilters{MinSpanCount: ptr(12)}, want: true},
		{name: "minSpanCount above count rejects", overview: filterOverview(), filters: TraceFilters{MinSpanCount: ptr(13)}, want: false},

		{name: "model matches any entry", overview: filterOverview(), filters: TraceFilters{Model: "gpt-4o-mini"}, want: true},
		{name: "model matches a substring", overview: filterOverview(), filters: TraceFilters{Model: "4o-mi"}, want: true},
		{name: "model ignores case", overview: filterOverview(), filters: TraceFilters{Model: "GPT-4O"}, want: true},
		{name: "model rejects a non-substring", overview: filterOverview(), filters: TraceFilters{Model: "claude"}, want: false},
		{name: "model rejects nil models", overview: noModels, filters: TraceFilters{Model: "gpt-4o"}, want: false},

		{name: "conversationId exact match", overview: filterOverview(), filters: TraceFilters{ConversationID: "conv-1"}, want: true},
		{name: "conversationId mismatch rejects", overview: filterOverview(), filters: TraceFilters{ConversationID: "conv-10"}, want: false},
		{name: "conversationId rejects empty", overview: opensearch.TraceOverview{}, filters: TraceFilters{ConversationID: "conv-1"}, want: false},

		{name: "two filters both match", overview: filterOverview(), filters: TraceFilters{Status: TraceStatusError, Model: "gpt-4o"}, want: true},
		{name: "two filters, one fails", overview: filterOverview(), filters: TraceFilters{Status: TraceStatusError, MinTokens: ptr(2000)}, want: false},
		{name: "two filters, other fails", overview: filterOverview(), filters: TraceFilters{MinSpanCount: ptr(50), ConversationID: "conv-1"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesFilters(tt.overview, tt.filters); got != tt.want {
				t.Errorf("matchesFilters(%+v) = %t, want %t", tt.filters, got, tt.want)
			}
		})
	}
}

func TestTraceFiltersIsZero(t *testing.T) {
	if !(TraceFilters{}).IsZero() {
		t.Error("zero TraceFilters should report IsZero")
	}
	if (TraceFilters{MinTokens: ptr(0)}).IsZero() {
		t.Error("an explicit minTokens=0 is a filter, not the zero value")
	}
}

// rootCompleteFake has a complete root and one gpt-4o leaf.
func rootCompleteFake() *fakeObserverClient {
	return overviewFake(
		completeRootAttrs(),
		[]observer.SpanInfo{
			{SpanID: "leaf-1", SpanName: "openai.chat", ParentSpanID: "root", StartTime: time.Now(),
				Attributes: map[string]interface{}{"gen_ai.response.model": "gpt-4o"}},
		},
		nil,
	)
}

// A model filter turns on Include.Models.
func TestGetTraceOverviews_ModelFilterFetchesModels(t *testing.T) {
	tests := []struct {
		model     string
		wantMatch bool
	}{
		{model: "gpt-4o", wantMatch: true},
		{model: "claude-sonnet-4-5", wantMatch: false},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			fake := rootCompleteFake()
			c := NewTracingController(fake)
			params := baseParams()
			params.Filters.Model = tt.model

			resp, err := c.GetTraceOverviews(context.Background(), params)
			if err != nil {
				t.Fatalf("GetTraceOverviews returned error: %v", err)
			}

			if got := atomic.LoadInt32(&fake.queryTraceSpansCalls); got != 1 {
				t.Errorf("expected 1 QueryTraceSpans call, got %d", got)
			}
			if !fake.lastSpansReq.IncludeAttributes {
				t.Error("expected QueryTraceSpans to request IncludeAttributes=true")
			}
			// The root comes from the attribute list.
			if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 0 {
				t.Errorf("expected 0 GetSpanDetails calls, got %d", got)
			}
			wantCount := 0
			if tt.wantMatch {
				wantCount = 1
			}
			if len(resp.Traces) != wantCount || resp.TotalCount != wantCount {
				t.Fatalf("got %d traces, totalCount %d; want %d of each", len(resp.Traces), resp.TotalCount, wantCount)
			}
			if tt.wantMatch {
				assertModels(t, resp.Traces[0].Models, []string{"gpt-4o"})
			}
		})
	}
}

// A status-only filter costs no span-list call.
func TestGetTraceOverviews_StatusFilterFetchesNoModels(t *testing.T) {
	fake := rootCompleteFake()
	c := NewTracingController(fake)
	params := baseParams()
	params.Filters.Status = TraceStatusOK

	ov := singleOverview(t, c, params)

	if ov.Models != nil {
		t.Errorf("models = %v, want nil for a status-only filter", ov.Models)
	}
	if got := atomic.LoadInt32(&fake.queryTraceSpansCalls); got != 0 {
		t.Errorf("expected 0 QueryTraceSpans calls, got %d", got)
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 1 {
		t.Errorf("expected exactly 1 GetSpanDetails call (root only), got %d", got)
	}
}

// TotalCount is the matched count when filtered, the upstream total otherwise.
// Summary filters drop traces before their root span is fetched.
func TestGetTraceOverviews_FiltersDropNonMatching(t *testing.T) {
	// trace-a: 2 spans, 1s; trace-b: 12 spans, 2s; trace-c: 22 spans, 3s.
	newFake := func() *fakeObserverClient {
		fake := rootCompleteFake()
		fake.traces = nil
		for i, id := range []string{"trace-a", "trace-b", "trace-c"} {
			info := baseTraceInfo(2 + i*10)
			info.TraceID = id
			info.DurationNs = int64(time.Duration(i+1) * time.Second)
			fake.traces = append(fake.traces, info)
		}
		return fake
	}

	tests := []struct {
		name    string
		filters TraceFilters
		wantIDs []string
	}{
		{name: "minSpanCount", filters: TraceFilters{MinSpanCount: ptr(12)}, wantIDs: []string{"trace-b", "trace-c"}},
		{name: "minDurationMs", filters: TraceFilters{MinDurationMs: ptr(3000)}, wantIDs: []string{"trace-c"}},
		{name: "none match", filters: TraceFilters{MinSpanCount: ptr(100)}, wantIDs: nil},
		{name: "summary and enriched filters", filters: TraceFilters{MinSpanCount: ptr(12), Status: TraceStatusError}, wantIDs: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newFake()
			c := NewTracingController(fake)
			params := baseParams()
			params.Filters = tt.filters

			resp, err := c.GetTraceOverviews(context.Background(), params)
			if err != nil {
				t.Fatalf("GetTraceOverviews returned error: %v", err)
			}
			if resp.TotalCount != len(tt.wantIDs) || len(resp.Traces) != len(tt.wantIDs) {
				t.Fatalf("got %d traces, totalCount %d; want %d of each", len(resp.Traces), resp.TotalCount, len(tt.wantIDs))
			}
			for i, id := range tt.wantIDs {
				if resp.Traces[i].TraceID != id {
					t.Errorf("traces[%d] = %s, want %s", i, resp.Traces[i].TraceID, id)
				}
			}
			// Only traces passing the summary filters get a root fetch.
			wantRootFetches := int32(len(filterTraceInfos(fake.traces, tt.filters)))
			if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != wantRootFetches {
				t.Errorf("GetSpanDetails calls = %d, want %d", got, wantRootFetches)
			}
		})
	}

	t.Run("unfiltered", func(t *testing.T) {
		c := NewTracingController(newFake())

		resp, err := c.GetTraceOverviews(context.Background(), baseParams())
		if err != nil {
			t.Fatalf("GetTraceOverviews returned error: %v", err)
		}
		if resp.TotalCount != 3 || len(resp.Traces) != 3 {
			t.Fatalf("got %d traces, totalCount %d; want 3 of each", len(resp.Traces), resp.TotalCount)
		}
	})
}

// lookedBackTo keeps sub-second precision so a client can page from it losslessly.
func TestGetTraceOverviews_LookedBackToIsWindowStart(t *testing.T) {
	start := time.Date(2026, 9, 1, 8, 14, 0, int(700*time.Millisecond), time.FixedZone("IST", 5*3600+1800))
	for name, fake := range map[string]*fakeObserverClient{
		"with traces": rootCompleteFake(),
		"empty":       {},
	} {
		t.Run(name, func(t *testing.T) {
			c := NewTracingController(fake)
			params := baseParams()
			params.StartTime = start

			resp, err := c.GetTraceOverviews(context.Background(), params)
			if err != nil {
				t.Fatalf("GetTraceOverviews returned error: %v", err)
			}
			if resp.LookedBackTo != "2026-09-01T02:44:00.7Z" {
				t.Errorf("lookedBackTo = %q, want 2026-09-01T02:44:00.7Z", resp.LookedBackTo)
			}
			if resp.Truncated {
				t.Error("truncated = true, want false")
			}
		})
	}
}

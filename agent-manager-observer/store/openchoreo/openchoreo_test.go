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

package openchoreo

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/store"
)

// fakeReader records each observer call and answers from its fields.
type fakeReader struct {
	calls      []string
	namespaces []string
	req        observer.TracesQueryRequest
	traces     *observer.TracesQueryResponse
	spans      *observer.TraceSpansQueryResponse
	details    *observer.SpanDetailsResponse
	err        error
}

func (f *fakeReader) QueryTraces(_ context.Context, req observer.TracesQueryRequest) (*observer.TracesQueryResponse, error) {
	f.calls, f.req = append(f.calls, "QueryTraces"), req
	return f.traces, f.err
}

func (f *fakeReader) QueryTraceSpans(_ context.Context, traceID string, req observer.TracesQueryRequest) (*observer.TraceSpansQueryResponse, error) {
	f.calls, f.req = append(f.calls, "QueryTraceSpans "+traceID), req
	return f.spans, f.err
}

func (f *fakeReader) GetSpanDetails(_ context.Context, traceID, spanID string) (*observer.SpanDetailsResponse, error) {
	f.calls = append(f.calls, "GetSpanDetails "+traceID+" "+spanID)
	return f.details, f.err
}

func (f *fakeReader) NamespaceFor(org string) string {
	f.namespaces = append(f.namespaces, org)
	return "default"
}

var (
	start = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	end   = start.Add(time.Hour)
	scope = store.Scope{Names: store.Names{Org: "acme", Project: "shop", Agent: "bot", Environment: "dev"}}
)

func wireJSON(t *testing.T, req observer.TracesQueryRequest) string {
	t.Helper()
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func TestQueryTraces_Request(t *testing.T) {
	f := &fakeReader{traces: &observer.TracesQueryResponse{}}
	if _, err := New(f).QueryTraces(context.Background(), scope, store.TraceQuery{StartTime: start, EndTime: end, Limit: 10, SortOrder: "desc"}); err != nil {
		t.Fatalf("QueryTraces: %v", err)
	}
	want := `{"startTime":"2026-11-01T00:00:00Z","endTime":"2026-11-01T01:00:00Z","limit":10,"sortOrder":"desc",` +
		`"searchScope":{"namespace":"default","project":"shop","component":"bot","environment":"dev"}}`
	if got := wireJSON(t, f.req); got != want {
		t.Errorf("request\n got %s\nwant %s", got, want)
	}
	if len(f.namespaces) != 1 || f.namespaces[0] != "acme" {
		t.Errorf("NamespaceFor called with %v, want [acme]", f.namespaces)
	}
}

func TestQueryTraceSpans_Request(t *testing.T) {
	tests := []struct {
		name  string
		scope store.Scope
		q     store.SpanQuery
		want  string
	}{
		{
			name:  "sorted, org only",
			scope: store.Scope{Names: store.Names{Org: "acme"}},
			q:     store.SpanQuery{StartTime: start, EndTime: end, Limit: 1000, SortOrder: "asc"},
			want: `{"startTime":"2026-11-01T00:00:00Z","endTime":"2026-11-01T01:00:00Z","limit":1000,"sortOrder":"asc",` +
				`"searchScope":{"namespace":"default"}}`,
		},
		{
			name:  "unsorted, with attributes",
			scope: scope,
			q:     store.SpanQuery{StartTime: start, EndTime: end, Limit: 5, IncludeAttributes: true},
			want: `{"startTime":"2026-11-01T00:00:00Z","endTime":"2026-11-01T01:00:00Z","limit":5,` +
				`"searchScope":{"namespace":"default","project":"shop","component":"bot","environment":"dev"},"includeAttributes":true}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeReader{spans: &observer.TraceSpansQueryResponse{}}
			if _, err := New(f).QueryTraceSpans(context.Background(), tt.scope, "trace-1", tt.q); err != nil {
				t.Fatalf("QueryTraceSpans: %v", err)
			}
			if got := wireJSON(t, f.req); got != tt.want {
				t.Errorf("request\n got %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestResponsesAreMapped(t *testing.T) {
	attrs := map[string]interface{}{"gen_ai.system": "openai"}
	res := map[string]interface{}{"openchoreo.dev/component-uid": "uid-1"}
	f := &fakeReader{
		traces: &observer.TracesQueryResponse{Total: 7, Traces: []observer.TraceInfo{{
			TraceID: "t1", TraceName: "n", SpanCount: 3, RootSpanID: "r", RootSpanName: "rn", RootSpanKind: "SERVER",
			StartTime: start, EndTime: end, DurationNs: 9,
		}}},
		spans: &observer.TraceSpansQueryResponse{Total: 4, Spans: []observer.SpanInfo{{
			SpanID: "s1", SpanName: "sn", ParentSpanID: "r", StartTime: start, EndTime: end, DurationNs: 5,
			Kind: "CLIENT", Status: &observer.SpanStatus{Code: "error", Message: "boom"}, Attributes: attrs, ResourceAttributes: res,
		}}},
		details: &observer.SpanDetailsResponse{
			SpanID: "s1", SpanName: "sn", ParentSpanID: "r", StartTime: start, EndTime: end, DurationNs: 5,
			Kind: "CLIENT", Status: &observer.SpanStatus{Code: "ok"}, Attributes: attrs, ResourceAttributes: res,
		},
	}
	s := New(f)
	ctx := context.Background()

	page, err := s.QueryTraces(ctx, scope, store.TraceQuery{})
	if err != nil {
		t.Fatalf("QueryTraces: %v", err)
	}
	wantTrace := store.Trace{TraceID: "t1", TraceName: "n", SpanCount: 3, RootSpanID: "r", RootSpanName: "rn", RootSpanKind: "SERVER", StartTime: start, EndTime: end, DurationNs: 9}
	if page.Total != 7 || len(page.Traces) != 1 || page.Traces[0] != wantTrace {
		t.Errorf("QueryTraces = %+v", page)
	}

	spans, err := s.QueryTraceSpans(ctx, scope, "t1", store.SpanQuery{})
	if err != nil {
		t.Fatalf("QueryTraceSpans: %v", err)
	}
	sp := spans.Spans[0]
	if spans.Total != 4 || sp.SpanID != "s1" || sp.SpanName != "sn" || sp.ParentSpanID != "r" || sp.Kind != "CLIENT" ||
		sp.DurationNs != 5 || !sp.StartTime.Equal(start) || !sp.EndTime.Equal(end) ||
		*sp.Status != (store.SpanStatus{Code: "error", Message: "boom"}) ||
		sp.Attributes["gen_ai.system"] != "openai" || sp.ResourceAttributes["openchoreo.dev/component-uid"] != "uid-1" {
		t.Errorf("QueryTraceSpans = %+v", spans)
	}

	d, err := s.GetSpanDetails(ctx, scope, "t1", "s1")
	if err != nil {
		t.Fatalf("GetSpanDetails: %v", err)
	}
	if d.SpanID != "s1" || d.Kind != "CLIENT" || d.Status.Code != "ok" ||
		d.Attributes["gen_ai.system"] != "openai" || d.ResourceAttributes["openchoreo.dev/component-uid"] != "uid-1" {
		t.Errorf("GetSpanDetails = %+v", d)
	}
}

func TestEmptyOrgRunsNoQuery(t *testing.T) {
	f := &fakeReader{}
	s := New(f)
	noOrg := store.Scope{Names: store.Names{Project: "shop", Agent: "bot", Environment: "dev"}}

	if _, err := s.QueryTraces(context.Background(), noOrg, store.TraceQuery{}); !errors.Is(err, store.ErrMissingOrg) {
		t.Errorf("QueryTraces error = %v, want ErrMissingOrg", err)
	}
	if _, err := s.QueryTraceSpans(context.Background(), noOrg, "trace-1", store.SpanQuery{}); !errors.Is(err, store.ErrMissingOrg) {
		t.Errorf("QueryTraceSpans error = %v, want ErrMissingOrg", err)
	}
	if len(f.calls) != 0 || len(f.namespaces) != 0 {
		t.Errorf("observer was called: %v, NamespaceFor %v", f.calls, f.namespaces)
	}
}

func TestGetSpanDetails_NoOrgStillReads(t *testing.T) {
	f := &fakeReader{details: &observer.SpanDetailsResponse{SpanID: "s1"}}
	if _, err := New(f).GetSpanDetails(context.Background(), store.Scope{}, "t1", "s1"); err != nil {
		t.Fatalf("GetSpanDetails: %v", err)
	}
	if len(f.calls) != 1 || f.calls[0] != "GetSpanDetails t1 s1" {
		t.Errorf("calls = %v", f.calls)
	}
}

func TestErrorsPassThrough(t *testing.T) {
	upstream := errors.New("unexpected status 502")
	s := New(&fakeReader{err: upstream})
	ctx := context.Background()
	if _, err := s.QueryTraces(ctx, scope, store.TraceQuery{}); !errors.Is(err, upstream) {
		t.Errorf("QueryTraces error = %v", err)
	}
	if _, err := s.QueryTraceSpans(ctx, scope, "t1", store.SpanQuery{}); !errors.Is(err, upstream) {
		t.Errorf("QueryTraceSpans error = %v", err)
	}
	if _, err := s.GetSpanDetails(ctx, scope, "t1", "s1"); !errors.Is(err, upstream) {
		t.Errorf("GetSpanDetails error = %v", err)
	}
}

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
package opensearch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	ospkg "github.com/wso2/agent-manager/agent-manager-observer/opensearch"
	"github.com/wso2/agent-manager/agent-manager-observer/store"
	"github.com/wso2/agent-manager/agent-manager-observer/store/storetest"
)

// fakeClient records each client call and answers from its fields.
type fakeClient struct {
	calls   []string
	req     observer.TracesQueryRequest
	traces  *observer.TracesQueryResponse
	spans   *observer.TraceSpansQueryResponse
	details *observer.SpanDetailsResponse
	err     error
}

func (f *fakeClient) QueryTraces(_ context.Context, req observer.TracesQueryRequest) (*observer.TracesQueryResponse, error) {
	f.calls, f.req = append(f.calls, "QueryTraces"), req
	return f.traces, f.err
}

func (f *fakeClient) QueryTraceSpans(_ context.Context, traceID string, req observer.TracesQueryRequest) (*observer.TraceSpansQueryResponse, error) {
	f.calls, f.req = append(f.calls, "QueryTraceSpans "+traceID), req
	return f.spans, f.err
}

func (f *fakeClient) GetSpanDetails(_ context.Context, traceID, spanID string) (*observer.SpanDetailsResponse, error) {
	f.calls = append(f.calls, "GetSpanDetails "+traceID+" "+spanID)
	return f.details, f.err
}

// seededClient answers like the span index: it filters seeds on the request's organization, project and agent names.
type seededClient struct {
	seeds []storetest.Seed
}

func (c *seededClient) matches(seed storetest.Seed, sc observer.TraceScope) bool {
	n := seed.Scope.Names
	return n.Org == sc.Organization &&
		(sc.Project == nil || *sc.Project == n.Project) &&
		(sc.Agent == nil || *sc.Agent == n.Agent)
}

func (c *seededClient) QueryTraces(_ context.Context, req observer.TracesQueryRequest) (*observer.TracesQueryResponse, error) {
	resp := &observer.TracesQueryResponse{}
	for _, seed := range c.seeds {
		if c.matches(seed, req.SearchScope) {
			resp.Traces = append(resp.Traces, observer.TraceInfo(seed.Trace))
		}
	}
	resp.Total = len(resp.Traces)
	return resp, nil
}

func (c *seededClient) QueryTraceSpans(_ context.Context, traceID string, req observer.TracesQueryRequest) (*observer.TraceSpansQueryResponse, error) {
	resp := &observer.TraceSpansQueryResponse{}
	for _, seed := range c.seeds {
		if seed.Trace.TraceID != traceID || !c.matches(seed, req.SearchScope) {
			continue
		}
		for _, sp := range seed.Spans {
			resp.Spans = append(resp.Spans, observer.SpanInfo{SpanID: sp.SpanID, SpanName: sp.SpanName, StartTime: sp.StartTime, EndTime: sp.EndTime})
		}
	}
	resp.Total = len(resp.Spans)
	return resp, nil
}

func (c *seededClient) GetSpanDetails(_ context.Context, traceID, spanID string) (*observer.SpanDetailsResponse, error) {
	for _, seed := range c.seeds {
		for _, sp := range seed.Spans {
			if seed.Trace.TraceID == traceID && sp.SpanID == spanID {
				return &observer.SpanDetailsResponse{
					SpanID: sp.SpanID, SpanName: sp.SpanName, StartTime: sp.StartTime, EndTime: sp.EndTime,
					ResourceAttributes: map[string]interface{}{ospkg.ResourceOrganization: seed.Scope.Names.Org},
				}, nil
			}
		}
	}
	return nil, observer.ErrSpanNotFound
}

func TestSuite(t *testing.T) {
	storetest.Run(t, func(_ *testing.T, seeds []storetest.Seed) store.TraceStore {
		return New(&seededClient{seeds: seeds})
	})
}

var (
	start = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	end   = start.Add(time.Hour)
	scope = store.Scope{Names: store.Names{Org: "acme", Project: "shop", Agent: "bot", Environment: "dev"}}
)

func TestQueryTraces_Request(t *testing.T) {
	f := &fakeClient{traces: &observer.TracesQueryResponse{}}
	if _, err := New(f).QueryTraces(context.Background(), scope, store.TraceQuery{StartTime: start, EndTime: end, Limit: 10, SortOrder: "desc"}); err != nil {
		t.Fatalf("QueryTraces: %v", err)
	}
	sc := f.req.SearchScope
	if sc.Organization != "acme" || sc.Project == nil || *sc.Project != "shop" || sc.Agent == nil || *sc.Agent != "bot" {
		t.Errorf("search scope = %+v, want acme/shop/bot", sc)
	}
	if !f.req.StartTime.Equal(start) || !f.req.EndTime.Equal(end) || *f.req.Limit != 10 || *f.req.SortOrder != "desc" {
		t.Errorf("request = %+v", f.req)
	}
}

func TestQueryTraceSpans_Request(t *testing.T) {
	f := &fakeClient{spans: &observer.TraceSpansQueryResponse{}}
	orgOnly := store.Scope{Names: store.Names{Org: "acme"}}
	if _, err := New(f).QueryTraceSpans(context.Background(), orgOnly, "trace-1", store.SpanQuery{StartTime: start, EndTime: end, Limit: 5, IncludeAttributes: true}); err != nil {
		t.Fatalf("QueryTraceSpans: %v", err)
	}
	sc := f.req.SearchScope
	if sc.Organization != "acme" || sc.Project != nil || sc.Agent != nil {
		t.Errorf("search scope = %+v, want acme only", sc)
	}
	if *f.req.Limit != 5 || f.req.SortOrder != nil || !f.req.IncludeAttributes {
		t.Errorf("request = %+v, want limit 5, no sort order, attributes", f.req)
	}
}

func TestResponsesAreMapped(t *testing.T) {
	attrs := map[string]interface{}{"gen_ai.system": "openai"}
	res := map[string]interface{}{ospkg.ResourceOrganization: "acme"}
	f := &fakeClient{
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
		sp.Attributes["gen_ai.system"] != "openai" || sp.ResourceAttributes[ospkg.ResourceOrganization] != "acme" {
		t.Errorf("QueryTraceSpans = %+v", spans)
	}

	d, err := s.GetSpanDetails(ctx, scope, "t1", "s1")
	if err != nil {
		t.Fatalf("GetSpanDetails: %v", err)
	}
	if d.SpanID != "s1" || d.Kind != "CLIENT" || d.Status.Code != "ok" || d.Attributes["gen_ai.system"] != "openai" {
		t.Errorf("GetSpanDetails = %+v", d)
	}
}

func TestEmptyOrgRunsNoQuery(t *testing.T) {
	f := &fakeClient{}
	s := New(f)
	noOrg := store.Scope{Names: store.Names{Project: "shop", Agent: "bot"}}

	if _, err := s.QueryTraces(context.Background(), noOrg, store.TraceQuery{}); !errors.Is(err, store.ErrMissingOrg) {
		t.Errorf("QueryTraces error = %v, want ErrMissingOrg", err)
	}
	if _, err := s.QueryTraceSpans(context.Background(), noOrg, "trace-1", store.SpanQuery{}); !errors.Is(err, store.ErrMissingOrg) {
		t.Errorf("QueryTraceSpans error = %v, want ErrMissingOrg", err)
	}
	if len(f.calls) != 0 {
		t.Errorf("client was called: %v", f.calls)
	}
}

func TestGetSpanDetails_OrgScope(t *testing.T) {
	details := &observer.SpanDetailsResponse{SpanID: "s1", ResourceAttributes: map[string]interface{}{ospkg.ResourceOrganization: "acme"}}
	for org, wantErr := range map[string]error{"": nil, "acme": nil, "globex": store.ErrNotFound} {
		_, err := New(&fakeClient{details: details}).GetSpanDetails(context.Background(), store.Scope{Names: store.Names{Org: org}}, "t1", "s1")
		if !errors.Is(err, wantErr) {
			t.Errorf("org %q: error = %v, want %v", org, err, wantErr)
		}
	}
}

func TestGetSpanDetails_MissingSpanIsNotFound(t *testing.T) {
	_, err := New(&fakeClient{err: observer.ErrSpanNotFound}).GetSpanDetails(context.Background(), scope, "t1", "s1")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestErrorsPassThrough(t *testing.T) {
	upstream := errors.New("search returned status 502")
	s := New(&fakeClient{err: upstream})
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

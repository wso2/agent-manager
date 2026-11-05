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

package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/deployments/dev/observer-lite/store"
)

type staticToken struct{}

func (staticToken) GetToken(context.Context) (string, error) { return "t", nil }
func (staticToken) InvalidateToken()                         {}

type fakeClient struct {
	tracesReq  observer.TracesQueryRequest
	traces     *observer.TracesQueryResponse
	spansTrace string
	spansReq   observer.TracesQueryRequest
	spans      *observer.TraceSpansQueryResponse
	detail     *observer.SpanDetailsResponse
	detailErr  error
	logsReq    observer.LogsQueryRequest
	metrics    *observer.ResourceMetricsTimeSeries
}

func (f *fakeClient) QueryTraces(_ context.Context, req observer.TracesQueryRequest) (*observer.TracesQueryResponse, error) {
	f.tracesReq = req
	return f.traces, nil
}

func (f *fakeClient) QueryTraceSpans(_ context.Context, traceID string, req observer.TracesQueryRequest) (*observer.TraceSpansQueryResponse, error) {
	f.spansTrace, f.spansReq = traceID, req
	return f.spans, nil
}

func (f *fakeClient) GetSpanDetails(context.Context, string, string) (*observer.SpanDetailsResponse, error) {
	return f.detail, f.detailErr
}

func (f *fakeClient) QueryLogs(_ context.Context, req observer.LogsQueryRequest) (*observer.LogsQueryResponse, error) {
	f.logsReq = req
	return &observer.LogsQueryResponse{Logs: []observer.LogEntry{}}, nil
}

func (f *fakeClient) QueryMetrics(context.Context, observer.MetricsQueryRequest) (*observer.ResourceMetricsTimeSeries, error) {
	return f.metrics, nil
}

func (f *fakeClient) NamespaceFor(string) string { return "default" }

func start(t *testing.T, f *fakeClient) (*httptest.Server, observer.Client) {
	t.Helper()
	srv := httptest.NewServer(NewHandler(f))
	t.Cleanup(srv.Close)
	return srv, observer.NewClient(srv.URL, staticToken{}, "default")
}

func TestQueryTracesRoundTrip(t *testing.T) {
	start1 := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	want := &observer.TracesQueryResponse{
		Traces: []observer.TraceInfo{{
			TraceID: "t1", TraceName: "n", SpanCount: 2, RootSpanID: "s1", RootSpanName: "root",
			RootSpanKind: "Server", StartTime: start1, EndTime: start1.Add(time.Second), DurationNs: 1e9,
		}},
		Total: 3,
	}
	f := &fakeClient{traces: want}
	_, c := start(t, f)
	comp := "my-comp"
	got, err := c.QueryTraces(context.Background(), observer.TracesQueryRequest{
		StartTime: start1, EndTime: start1.Add(time.Hour),
		SearchScope: observer.ComponentSearchScope{Namespace: "ns", Component: &comp},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if f.tracesReq.SearchScope.Component == nil || *f.tracesReq.SearchScope.Component != comp {
		t.Fatalf("component not received: %+v", f.tracesReq.SearchScope)
	}
}

func TestQueryTraceSpansRoundTrip(t *testing.T) {
	f := &fakeClient{spans: &observer.TraceSpansQueryResponse{
		Spans: []observer.SpanInfo{{
			SpanID: "s1", Kind: "Server",
			Status:     &observer.SpanStatus{Code: "error", Message: "boom"},
			Attributes: map[string]interface{}{"k": "v"},
		}},
		Total: 1,
	}}
	_, c := start(t, f)
	got, err := c.QueryTraceSpans(context.Background(), "trace-9", observer.TracesQueryRequest{
		SearchScope:       observer.ComponentSearchScope{Namespace: "ns"},
		IncludeAttributes: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if f.spansTrace != "trace-9" || !f.spansReq.IncludeAttributes {
		t.Fatalf("fake saw trace %q req %+v", f.spansTrace, f.spansReq)
	}
	span := got.Spans[0]
	if span.Kind != "Server" || span.Status.Code != "error" || span.Status.Message != "boom" || span.Attributes["k"] != "v" {
		t.Fatalf("unexpected span %+v", span)
	}
}

func TestSpanDetailsRoundTrip(t *testing.T) {
	f := &fakeClient{detail: &observer.SpanDetailsResponse{
		SpanID: "s1", Kind: "Internal",
		Status:             &observer.SpanStatus{Code: "ok"},
		ResourceAttributes: map[string]interface{}{"openchoreo.dev/component-uid": "c-uid"},
	}}
	_, c := start(t, f)
	got, err := c.GetSpanDetails(context.Background(), "t1", "s1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != "Internal" || got.Status.Code != "ok" || got.ResourceAttributes["openchoreo.dev/component-uid"] != "c-uid" {
		t.Fatalf("unexpected details %+v", got)
	}
}

func TestSpanDetailsNotFound(t *testing.T) {
	f := &fakeClient{detailErr: fmt.Errorf("x: %w", store.ErrSpanNotFound)}
	_, c := start(t, f)
	_, err := c.GetSpanDetails(context.Background(), "t1", "missing")
	if err == nil || !strings.Contains(err.Error(), "unexpected status 404") {
		t.Fatalf("want 404 error, got %v", err)
	}
}

func TestSpanDetailsInternalError(t *testing.T) {
	f := &fakeClient{detailErr: fmt.Errorf("disk on fire")}
	_, c := start(t, f)
	_, err := c.GetSpanDetails(context.Background(), "t1", "s1")
	if err == nil || !strings.Contains(err.Error(), "unexpected status 500") || !strings.Contains(err.Error(), "disk on fire") {
		t.Fatalf("want 500 error with text, got %v", err)
	}
}

func TestLogsScopeDecoding(t *testing.T) {
	run, comp := "run-1", "my-comp"
	tests := []struct {
		name  string
		scope any
		check func(t *testing.T, got any)
	}{
		{
			name:  "workflow",
			scope: observer.WorkflowSearchScope{Namespace: "ns", WorkflowRunName: &run},
			check: func(t *testing.T, got any) {
				s, ok := got.(observer.WorkflowSearchScope)
				if !ok || s.WorkflowRunName == nil || *s.WorkflowRunName != run {
					t.Fatalf("got %T %+v", got, got)
				}
			},
		},
		{
			name:  "component",
			scope: observer.ComponentSearchScope{Namespace: "ns", Component: &comp},
			check: func(t *testing.T, got any) {
				s, ok := got.(observer.ComponentSearchScope)
				if !ok || s.Component == nil || *s.Component != comp {
					t.Fatalf("got %T %+v", got, got)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeClient{}
			_, c := start(t, f)
			if _, err := c.QueryLogs(context.Background(), observer.LogsQueryRequest{SearchScope: tc.scope}); err != nil {
				t.Fatal(err)
			}
			tc.check(t, f.logsReq.SearchScope)
		})
	}
}

func TestQueryMetricsRoundTrip(t *testing.T) {
	f := &fakeClient{metrics: &observer.ResourceMetricsTimeSeries{}}
	_, c := start(t, f)
	if _, err := c.QueryMetrics(context.Background(), observer.MetricsQueryRequest{Metric: "resource"}); err != nil {
		t.Fatal(err)
	}
}

func TestRoutes(t *testing.T) {
	srv, _ := start(t, &fakeClient{})
	oversize := strings.Repeat("a", 2<<20)
	tests := []struct {
		name, method, path, body string
		want                     int
	}{
		{"wrong method", http.MethodGet, "/api/v1/logs/query", "", http.StatusMethodNotAllowed},
		{"unknown path", http.MethodGet, "/nope", "", http.StatusNotFound},
		{"bad json", http.MethodPost, "/api/v1alpha1/traces/query", "{", http.StatusBadRequest},
		{"oversize body", http.MethodPost, "/api/v1alpha1/traces/query", `{"x":"` + oversize + `"}`, http.StatusBadRequest},
		{"logs scope missing", http.MethodPost, "/api/v1/logs/query", `{}`, http.StatusBadRequest},
		{"logs scope not object", http.MethodPost, "/api/v1/logs/query", `{"searchScope":"x"}`, http.StatusBadRequest},
		{"healthz", http.MethodGet, "/healthz", "", http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, srv.URL+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Fatalf("got %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

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

package traceobssvc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func intPtr(i int) *int       { return &i }
func strPtr(s string) *string { return &s }

func TestListTraces_BuildsQueryAndDecodes(t *testing.T) {
	start := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/traces" {
			t.Errorf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("organization") != "acme" || q.Get("project") != "triage" ||
			q.Get("agent") != "my-agent" || q.Get("environment") != "dev" {
			t.Errorf("missing scope query params: %v", q)
		}
		if q.Get("startTime") != start.Format(time.RFC3339) {
			t.Errorf("startTime = %q", q.Get("startTime"))
		}
		if q.Get("endTime") != end.Format(time.RFC3339) {
			t.Errorf("endTime = %q", q.Get("endTime"))
		}
		if q.Get("limit") != "10" || q.Get("sortOrder") != "desc" {
			t.Errorf("missing limit/sortOrder: %v", q)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TraceOverviewResponse{
			Traces:     []TraceOverview{{TraceID: "abc"}},
			TotalCount: 1,
		})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	sort := "desc"
	resp, err := c.ListTraces(context.Background(), &ListTracesParams{
		Organization: "acme",
		Project:      "triage",
		Agent:        "my-agent",
		Environment:  "dev",
		StartTime:    start,
		EndTime:      end,
		Limit:        intPtr(10),
		SortOrder:    &sort,
	})
	if err != nil {
		t.Fatalf("ListTraces: %v", err)
	}
	if resp.TotalCount != 1 || len(resp.Traces) != 1 || resp.Traces[0].TraceID != "abc" {
		t.Fatalf("unexpected resp: %+v", resp)
	}
}

func TestExportTraces_DecodesFullTrace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/traces/export" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TraceExportResponse{
			Traces: []FullTrace{
				{TraceOverview: TraceOverview{TraceID: "t1"}, Spans: []Span{{SpanID: "s1"}}},
			},
			TotalCount: 1,
		})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	resp, err := c.ExportTraces(context.Background(), &ExportTracesParams{
		Organization: "acme", Project: "p", Agent: "a", Environment: "e",
		StartTime: time.Now().Add(-time.Hour), EndTime: time.Now(),
	})
	if err != nil {
		t.Fatalf("ExportTraces: %v", err)
	}
	if len(resp.Traces) != 1 || resp.Traces[0].TraceID != "t1" {
		t.Fatalf("unexpected resp: %+v", resp)
	}
	if len(resp.Traces[0].Spans) != 1 || resp.Traces[0].Spans[0].SpanID != "s1" {
		t.Fatalf("spans missing: %+v", resp.Traces[0].Spans)
	}
}

func TestGetTraceSpans_PathAndOptionalParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/traces/trace-xyz/spans" {
			t.Errorf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("organization") != "acme" {
			t.Errorf("organization missing")
		}
		if q.Get("project") != "triage" {
			t.Errorf("project missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(SpanListResponse{
			Spans: []SpanSummary{
				{SpanID: "s1", SpanName: "root"},
				{SpanID: "s2", ParentSpanID: "s1", SpanName: "child"},
			},
			TotalCount: 2,
		})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	resp, err := c.GetTraceSpans(context.Background(), "trace-xyz", &GetTraceSpansParams{
		Organization: "acme",
		Project:      strPtr("triage"),
		StartTime:    time.Now().Add(-time.Hour),
		EndTime:      time.Now(),
	})
	if err != nil {
		t.Fatalf("GetTraceSpans: %v", err)
	}
	if len(resp.Spans) != 2 || resp.Spans[0].SpanID != "s1" {
		t.Fatalf("unexpected resp: %+v", resp)
	}
}

func TestGetSpanDetail_404ReturnsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/traces/t/spans/s" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "not_found", Message: "no such span"})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	_, err := c.GetSpanDetail(context.Background(), "t", "s")
	if err == nil {
		t.Fatal("expected error")
	}
	var herr *HTTPError
	if !errors.As(err, &herr) {
		t.Fatalf("expected *HTTPError, got %T: %v", err, err)
	}
	if herr.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d", herr.StatusCode)
	}
	if herr.Body == nil || herr.Body.Error != "not_found" {
		t.Errorf("body = %+v", herr.Body)
	}
}

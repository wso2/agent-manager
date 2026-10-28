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

package observersvc

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

func int64Ptr(i int64) *int64 { return &i }

func TestListTraces_SendsFiltersAndDecodesSearchFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for key, want := range map[string]string{
			"status": "error", "minDurationMs": "1500", "minTokens": "0", "minSpanCount": "4",
			"model": "gpt-4o", "conversationId": "c-1", "include": "models",
		} {
			if q.Get(key) != want {
				t.Errorf("%s = %q, want %q", key, q.Get(key), want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"traces":[],"totalCount":0,"lookedBackTo":"2026-05-12T03:00:00.5Z","truncated":true,"nextCursor":"abc"}`))
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	resp, err := c.ListTraces(context.Background(), &ListTracesParams{
		Organization: "acme", Project: "p", Agent: "a", Environment: "e",
		Status: "error", MinDurationMs: int64Ptr(1500), MinTokens: int64Ptr(0), MinSpanCount: int64Ptr(4),
		Model: "gpt-4o", ConversationID: "c-1", IncludeModels: true,
	})
	if err != nil {
		t.Fatalf("ListTraces: %v", err)
	}
	if resp.LookedBackTo != "2026-05-12T03:00:00.5Z" || !resp.Truncated || resp.NextCursor != "abc" {
		t.Errorf("unexpected resp: %+v", resp)
	}
}

func TestListTraces_OmitsUnsetFilters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for _, key := range []string{"status", "minDurationMs", "minTokens", "minSpanCount", "model", "conversationId", "include"} {
			if q.Has(key) {
				t.Errorf("%s = %q, want unset", key, q.Get(key))
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TraceOverviewResponse{})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	if _, err := c.ListTraces(context.Background(), &ListTracesParams{Organization: "acme"}); err != nil {
		t.Fatalf("ListTraces: %v", err)
	}
}

func TestExportTraces_SendsFiltersButNotInclude(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("status") != "ok" || q.Get("model") != "claude" {
			t.Errorf("filters missing: %v", q)
		}
		if q.Has("include") {
			t.Errorf("export takes no include, got %q", q.Get("include"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TraceExportResponse{})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	if _, err := c.ExportTraces(context.Background(), &ExportTracesParams{
		Organization: "acme", Status: "ok", Model: "claude", IncludeModels: true,
	}); err != nil {
		t.Fatalf("ExportTraces: %v", err)
	}
}

func TestListTraces_SendsToolAndMCPFilters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for key, want := range map[string]string{
			"tool": "search_web", "toolError": "true", "mcpServer": "github",
			"include": "models,tools,mcpServers",
		} {
			if q.Get(key) != want {
				t.Errorf("%s = %q, want %q", key, q.Get(key), want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"traces":[{"traceId":"t1","tools":["search_web"],"failedTools":["search_web"],"mcpServers":["github-mcp-server"]}],"totalCount":1}`))
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	resp, err := c.ListTraces(context.Background(), &ListTracesParams{
		Organization: "acme", Tool: "search_web", ToolError: true, MCPServer: "github",
		IncludeModels: true, IncludeTools: true, IncludeMCPServers: true,
	})
	if err != nil {
		t.Fatalf("ListTraces: %v", err)
	}
	tr := resp.Traces[0]
	if len(tr.Tools) != 1 || len(tr.FailedTools) != 1 || len(tr.MCPServers) != 1 {
		t.Errorf("tools, failedTools and mcpServers should decode, got %+v", tr)
	}
}

func TestListTraces_OmitsUnsetToolAndMCPFilters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for _, key := range []string{"tool", "toolError", "mcpServer", "include"} {
			if q.Has(key) {
				t.Errorf("%s = %q, want unset", key, q.Get(key))
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TraceOverviewResponse{})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	if _, err := c.ListTraces(context.Background(), &ListTracesParams{Organization: "acme"}); err != nil {
		t.Fatalf("ListTraces: %v", err)
	}
}

func TestExportTraces_SendsToolFiltersButNoInclude(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("tool") != "search" || q.Get("toolError") != "true" || q.Get("mcpServer") != "github" {
			t.Errorf("filters missing: %v", q)
		}
		if q.Has("include") {
			t.Errorf("export takes no include, got %q", q.Get("include"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TraceExportResponse{})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	if _, err := c.ExportTraces(context.Background(), &ExportTracesParams{
		Organization: "acme", Tool: "search", ToolError: true, MCPServer: "github",
		IncludeTools: true, IncludeMCPServers: true,
	}); err != nil {
		t.Fatalf("ExportTraces: %v", err)
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

func TestGetRuntimeLogs_BuildsQueryAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/logs" {
			t.Errorf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("organization") != "acme" || q.Get("project") != "triage" ||
			q.Get("agent") != "my-agent" || q.Get("environment") != "dev" {
			t.Errorf("missing scope query params: %v", q)
		}
		if q.Get("startTime") != "2026-05-12T00:00:00Z" || q.Get("endTime") != "2026-05-13T00:00:00Z" {
			t.Errorf("missing time range: %v", q)
		}
		if q.Get("searchPhrase") != "boom" {
			t.Errorf("searchPhrase = %q", q.Get("searchPhrase"))
		}
		if q.Get("logLevels") != "ERROR,WARN" {
			t.Errorf("logLevels = %q, want comma-joined", q.Get("logLevels"))
		}
		if q.Get("limit") != "50" {
			t.Errorf("limit = %q", q.Get("limit"))
		}
		if q.Get("sortOrder") != "desc" {
			t.Errorf("sortOrder = %q", q.Get("sortOrder"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(LogsResponse{
			Logs:       []LogEntry{{Log: "boom", LogLevel: "ERROR"}},
			TotalCount: 1,
			TookMs:     3.5,
		})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	resp, err := c.GetRuntimeLogs(context.Background(), RuntimeLogsParams{
		Organization: "acme",
		Project:      "triage",
		Agent:        "my-agent",
		Environment:  "dev",
		StartTime:    "2026-05-12T00:00:00Z",
		EndTime:      "2026-05-13T00:00:00Z",
		SearchPhrase: "boom",
		SortOrder:    "desc",
		LogLevels:    []string{"ERROR", "WARN"},
		Limit:        intPtr(50),
	})
	if err != nil {
		t.Fatalf("GetRuntimeLogs: %v", err)
	}
	if resp.TotalCount != 1 || len(resp.Logs) != 1 || resp.Logs[0].Log != "boom" {
		t.Fatalf("unexpected resp: %+v", resp)
	}
}

func TestGetRuntimeLogs_OmitsUnsetOptionalParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for _, key := range []string{"searchPhrase", "logLevels", "limit", "sortOrder"} {
			if q.Has(key) {
				t.Errorf("unexpected query param %q = %q", key, q.Get(key))
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(LogsResponse{})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	_, err := c.GetRuntimeLogs(context.Background(), RuntimeLogsParams{
		Organization: "acme", Project: "triage", Agent: "my-agent", Environment: "dev",
	})
	if err != nil {
		t.Fatalf("GetRuntimeLogs: %v", err)
	}
}

func TestGetRuntimeLogs_NonOKReturnsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(ErrorResponse{Error: "bad_gateway", Message: "upstream unavailable"})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	_, err := c.GetRuntimeLogs(context.Background(), RuntimeLogsParams{Organization: "acme"})
	if err == nil {
		t.Fatal("expected error")
	}
	var herr *HTTPError
	if !errors.As(err, &herr) {
		t.Fatalf("expected *HTTPError, got %T: %v", err, err)
	}
	if herr.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d", herr.StatusCode)
	}
}

func TestGetBuildLogs_SendsExactlyOrgAndBuildName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/build-logs" {
			t.Errorf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if len(q) != 2 {
			t.Errorf("expected exactly 2 query params, got %v", q)
		}
		if q.Get("organization") != "acme" || q.Get("buildName") != "build-123" {
			t.Errorf("query = %v", q)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(LogsResponse{
			Logs:       []LogEntry{{Log: "starting build"}},
			TotalCount: 1,
		})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	resp, err := c.GetBuildLogs(context.Background(), BuildLogsParams{
		Organization: "acme",
		BuildName:    "build-123",
	})
	if err != nil {
		t.Fatalf("GetBuildLogs: %v", err)
	}
	if len(resp.Logs) != 1 || resp.Logs[0].Log != "starting build" {
		t.Fatalf("unexpected resp: %+v", resp)
	}
}

func TestGetMetrics_BuildsQueryAndDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/metrics" {
			t.Errorf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("organization") != "acme" || q.Get("project") != "triage" ||
			q.Get("agent") != "my-agent" || q.Get("environment") != "dev" {
			t.Errorf("missing scope query params: %v", q)
		}
		if q.Get("startTime") != "2026-05-12T00:00:00Z" || q.Get("endTime") != "2026-05-13T00:00:00Z" {
			t.Errorf("missing time range: %v", q)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(MetricsResponse{
			CpuUsage: []MetricDataPoint{{Time: "2026-05-13T10:00:00Z", Value: 0.5}},
			Memory:   []MetricDataPoint{{Time: "2026-05-13T10:00:00Z", Value: 1024}},
		})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	resp, err := c.GetMetrics(context.Background(), MetricsParams{
		Organization: "acme",
		Project:      "triage",
		Agent:        "my-agent",
		Environment:  "dev",
		StartTime:    "2026-05-12T00:00:00Z",
		EndTime:      "2026-05-13T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	if len(resp.CpuUsage) != 1 || resp.CpuUsage[0].Value != 0.5 {
		t.Fatalf("unexpected CpuUsage: %+v", resp.CpuUsage)
	}
	if len(resp.Memory) != 1 || resp.Memory[0].Value != 1024 {
		t.Fatalf("unexpected Memory: %+v", resp.Memory)
	}
}

func TestGetMetrics_NonOKReturnsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	_, err := c.GetMetrics(context.Background(), MetricsParams{Organization: "acme"})
	if err == nil {
		t.Fatal("expected error")
	}
	var herr *HTTPError
	if !errors.As(err, &herr) {
		t.Fatalf("expected *HTTPError, got %T: %v", err, err)
	}
}

func TestListTraces_SendsMissingAndCursor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("missing") != "conversationId" || q.Get("cursor") != "abc" {
			t.Errorf("missing = %q, cursor = %q; want conversationId and abc", q.Get("missing"), q.Get("cursor"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TraceOverviewResponse{})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	if _, err := c.ListTraces(context.Background(), &ListTracesParams{
		Organization: "acme", Missing: []string{"conversationId"}, Cursor: "abc",
	}); err != nil {
		t.Fatalf("ListTraces: %v", err)
	}
}

func TestExportTraces_SendsMissingButNoCursor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("missing") != "conversationId" {
			t.Errorf("missing = %q, want conversationId", q.Get("missing"))
		}
		if q.Has("cursor") {
			t.Errorf("export sent cursor %q", q.Get("cursor"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TraceExportResponse{})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	if _, err := c.ExportTraces(context.Background(), &ExportTracesParams{
		Organization: "acme", Missing: []string{"conversationId"}, Cursor: "abc",
	}); err != nil {
		t.Fatalf("ExportTraces: %v", err)
	}
}

func TestListTraces_OmitsUnsetMissingAndCursor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if q := r.URL.Query(); q.Has("missing") || q.Has("cursor") {
			t.Errorf("unset params sent: %v", q)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TraceOverviewResponse{})
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL)
	if _, err := c.ListTraces(context.Background(), &ListTracesParams{Organization: "acme"}); err != nil {
		t.Fatalf("ListTraces: %v", err)
	}
}

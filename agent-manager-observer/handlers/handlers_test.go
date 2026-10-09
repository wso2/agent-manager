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

package handlers

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/controllers"
	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

// newHandler creates a Handler with nil controllers — safe for validation-only
// tests because neither controller is ever called when a 400/405 is returned first.
func newHandler() *Handler {
	return NewHandler(nil, nil)
}

// baseParams returns a query string with all required parameters present,
// using the field names the handlers actually read (organization, agent).
func baseParams() string {
	return "organization=default&project=myproject&agent=myagent&environment=dev" +
		"&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z"
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Errorf("expected %d, got %d (body: %s)", want, rec.Code, rec.Body.String())
	}
}

func assertBadRequest(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// ── GetTraceOverviews ────────────────────────────────────────────────────────

func TestGetTraceOverviews_MissingNamespace(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces?project=p&component=c&environment=e&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetTraceOverviews(rec, r)
	assertBadRequest(t, rec)
}

func TestGetTraceOverviews_MissingProject(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces?namespace=default&component=c&environment=e&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetTraceOverviews(rec, r)
	assertBadRequest(t, rec)
}

func TestGetTraceOverviews_MissingComponent(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces?namespace=default&project=p&environment=e&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetTraceOverviews(rec, r)
	assertBadRequest(t, rec)
}

func TestGetTraceOverviews_MissingEnvironment(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces?namespace=default&project=p&component=c&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetTraceOverviews(rec, r)
	assertBadRequest(t, rec)
}

func TestGetTraceOverviews_MissingStartTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces?namespace=default&project=p&component=c&environment=e&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetTraceOverviews(rec, r)
	assertBadRequest(t, rec)
}

func TestGetTraceOverviews_MissingEndTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces?namespace=default&project=p&component=c&environment=e&startTime=2026-04-01T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	h.GetTraceOverviews(rec, r)
	assertBadRequest(t, rec)
}

func TestGetTraceOverviews_InvalidSortOrder(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces?"+baseParams()+"&sortOrder=invalid", nil)
	rec := httptest.NewRecorder()
	h.GetTraceOverviews(rec, r)
	assertBadRequest(t, rec)
}

// ── GetTraceSpans ────────────────────────────────────────────────────────────

func TestGetTraceSpans_MissingNamespace(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces/abc123/spans?startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetTraceSpans(rec, r)
	assertBadRequest(t, rec)
}

func TestGetTraceSpans_MissingStartTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces/abc123/spans?namespace=default&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetTraceSpans(rec, r)
	assertBadRequest(t, rec)
}

func TestGetTraceSpans_MissingEndTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces/abc123/spans?namespace=default&startTime=2026-04-01T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	h.GetTraceSpans(rec, r)
	assertBadRequest(t, rec)
}

// ── ExportTraces ─────────────────────────────────────────────────────────────

func TestExportTraces_MissingNamespace(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces/export?project=p&component=c&environment=e&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.ExportTraces(rec, r)
	assertBadRequest(t, rec)
}

func TestExportTraces_MissingProject(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces/export?namespace=default&component=c&environment=e&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.ExportTraces(rec, r)
	assertBadRequest(t, rec)
}

func TestExportTraces_MissingComponent(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces/export?namespace=default&project=p&environment=e&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.ExportTraces(rec, r)
	assertBadRequest(t, rec)
}

func TestExportTraces_MissingEnvironment(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces/export?namespace=default&project=p&component=c&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.ExportTraces(rec, r)
	assertBadRequest(t, rec)
}

func TestExportTraces_MissingStartTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces/export?namespace=default&project=p&component=c&environment=e&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.ExportTraces(rec, r)
	assertBadRequest(t, rec)
}

func TestExportTraces_MissingEndTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces/export?namespace=default&project=p&component=c&environment=e&startTime=2026-04-01T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	h.ExportTraces(rec, r)
	assertBadRequest(t, rec)
}

// ── GetLogs ──────────────────────────────────────────────────────────────────

func TestGetLogs_MissingOrganization(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?project=p&agent=a&environment=e&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)
	assertBadRequest(t, rec)
}

func TestGetLogs_MissingProject(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?organization=default&agent=a&environment=e&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)
	assertBadRequest(t, rec)
}

func TestGetLogs_MissingAgent(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?organization=default&project=p&environment=e&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)
	assertBadRequest(t, rec)
}

func TestGetLogs_MissingEnvironment(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?organization=default&project=p&agent=a&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)
	assertBadRequest(t, rec)
}

func TestGetLogs_MissingStartTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?organization=default&project=p&agent=a&environment=e&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)
	assertBadRequest(t, rec)
}

func TestGetLogs_MissingEndTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?organization=default&project=p&agent=a&environment=e&startTime=2026-04-01T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)
	assertBadRequest(t, rec)
}

func TestGetLogs_InvalidStartTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?organization=default&project=p&agent=a&environment=e&startTime=not-a-time&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)
	assertBadRequest(t, rec)
}

func TestGetLogs_InvalidEndTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?organization=default&project=p&agent=a&environment=e&startTime=2026-04-01T00:00:00Z&endTime=not-a-time", nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)
	assertBadRequest(t, rec)
}

func TestGetLogs_EndTimeBeforeStartTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?organization=default&project=p&agent=a&environment=e&startTime=2026-04-06T23:59:59Z&endTime=2026-04-01T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)
	assertBadRequest(t, rec)
}

func TestGetLogs_StartTimeInFuture(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?organization=default&project=p&agent=a&environment=e&startTime=2099-01-01T00:00:00Z&endTime=2099-01-02T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)
	assertBadRequest(t, rec)
}

func TestGetLogs_TimeRangeExceeds14Days(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?organization=default&project=p&agent=a&environment=e&startTime=2026-01-01T00:00:00Z&endTime=2026-01-20T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)
	assertBadRequest(t, rec)
}

// An unknown log level is ignored (filtering by the valid ones) rather than
// failing the whole request, matching typical filter semantics.
func TestGetLogs_UnknownLogLevelIgnored(t *testing.T) {
	fake := &fakeObserverClient{}
	h := NewHandler(nil, controllers.NewObservabilityController(fake))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?"+baseParams()+"&logLevels=INFO,BOGUS", nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)

	assertStatus(t, rec, http.StatusOK)
	if got := fake.lastLogsReq.LogLevels; len(got) != 1 || got[0] != "INFO" {
		t.Errorf("expected only valid levels forwarded, got %v", got)
	}
}

// A trailing or duplicate comma yields empty tokens that must be skipped, not
// rejected as an invalid ("") level.
func TestGetLogs_EmptyLogLevelTokensSkipped(t *testing.T) {
	fake := &fakeObserverClient{}
	h := NewHandler(nil, controllers.NewObservabilityController(fake))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?"+baseParams()+"&logLevels=INFO,,ERROR,", nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)

	assertStatus(t, rec, http.StatusOK)
	got := fake.lastLogsReq.LogLevels
	if len(got) != 2 || got[0] != "INFO" || got[1] != "ERROR" {
		t.Errorf("expected [INFO ERROR], got %v", got)
	}
}

// When every supplied level is unknown, no filter is applied (nil levels)
// rather than failing the request.
func TestGetLogs_AllUnknownLogLevelsYieldNoFilter(t *testing.T) {
	fake := &fakeObserverClient{}
	h := NewHandler(nil, controllers.NewObservabilityController(fake))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?"+baseParams()+"&logLevels=BOGUS,NOPE", nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)

	assertStatus(t, rec, http.StatusOK)
	if got := fake.lastLogsReq.LogLevels; len(got) != 0 {
		t.Errorf("expected no log-level filter, got %v", got)
	}
}

func TestGetLogs_LimitTooHigh(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?"+baseParams()+"&limit=10001", nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)
	assertBadRequest(t, rec)
}

func TestGetLogs_LimitZero(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?"+baseParams()+"&limit=0", nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)
	assertBadRequest(t, rec)
}

func TestGetLogs_InvalidSortOrder(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?"+baseParams()+"&sortOrder=invalid", nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)
	assertBadRequest(t, rec)
}

// fakeObserverClient is a minimal observer.Client that captures requests so
// handler tests can assert on pass-through parameters.
type fakeObserverClient struct {
	traces       []observer.TraceInfo
	lastLogsReq  observer.LogsQueryRequest
	lastSpansReq observer.TracesQueryRequest
}

// QueryTraces returns the configured traces.
func (f *fakeObserverClient) QueryTraces(_ context.Context, _ observer.TracesQueryRequest) (*observer.TracesQueryResponse, error) {
	return &observer.TracesQueryResponse{Traces: f.traces, Total: len(f.traces)}, nil
}

// QueryTraceSpans records the request and returns no spans.
func (f *fakeObserverClient) QueryTraceSpans(_ context.Context, _ string, req observer.TracesQueryRequest) (*observer.TraceSpansQueryResponse, error) {
	f.lastSpansReq = req
	return &observer.TraceSpansQueryResponse{}, nil
}

func (f *fakeObserverClient) GetSpanDetails(_ context.Context, _, _ string) (*observer.SpanDetailsResponse, error) {
	return &observer.SpanDetailsResponse{}, nil
}

func (f *fakeObserverClient) QueryLogs(_ context.Context, req observer.LogsQueryRequest) (*observer.LogsQueryResponse, error) {
	f.lastLogsReq = req
	return &observer.LogsQueryResponse{}, nil
}

func (f *fakeObserverClient) QueryMetrics(_ context.Context, _ observer.MetricsQueryRequest) (*observer.ResourceMetricsTimeSeries, error) {
	return &observer.ResourceMetricsTimeSeries{}, nil
}

func (f *fakeObserverClient) NamespaceFor(_ string) string {
	return "default"
}

// include=models maps to IncludeAttributes on the span-list request; unknown values are rejected.
func TestGetTraceOverviews_IncludeQueryParam(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantModels bool
		wantStatus int
	}{
		{name: "models sets Models", query: "&include=models", wantModels: true, wantStatus: http.StatusOK},
		{name: "models,models sets Models once", query: "&include=models,models", wantModels: true, wantStatus: http.StatusOK},
		{name: "repeated parameter sets Models", query: "&include=models&include=models", wantModels: true, wantStatus: http.StatusOK},
		{name: "spaces around items are trimmed", query: "&include=%20models%20,", wantModels: true, wantStatus: http.StatusOK},
		{name: "absent leaves it off", query: "", wantStatus: http.StatusOK},
		{name: "empty leaves it off", query: "&include=", wantStatus: http.StatusOK},
		{name: "spans is rejected", query: "&include=spans", wantStatus: http.StatusBadRequest},
		{name: "Models is rejected", query: "&include=Models", wantStatus: http.StatusBadRequest},
		{name: "one bad item rejects the list", query: "&include=models,spans", wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeObserverClient{traces: []observer.TraceInfo{{TraceID: "trace-1", RootSpanID: "root", SpanCount: 2}}}
			h := NewHandler(controllers.NewTracingController(fake), nil)

			r := httptest.NewRequest(http.MethodGet, "/api/v1/traces?"+baseParams()+tt.query, nil)
			rec := httptest.NewRecorder()
			h.GetTraceOverviews(rec, r)

			assertStatus(t, rec, tt.wantStatus)
			if tt.wantStatus != http.StatusOK {
				if fake.lastSpansReq.Limit != nil {
					t.Error("expected no upstream call for a rejected include value")
				}
				return
			}
			if fake.lastSpansReq.Limit == nil {
				t.Fatal("expected the trace's spans to be listed upstream, got no QueryTraceSpans call")
			}
			if fake.lastSpansReq.IncludeAttributes != tt.wantModels {
				t.Errorf("IncludeAttributes = %t, want %t for %q", fake.lastSpansReq.IncludeAttributes, tt.wantModels, tt.query)
			}
		})
	}
}

// Rejected include values name the offending value in the error body.
func TestParseInclude_ErrorNamesValue(t *testing.T) {
	_, err := parseInclude([]string{"models,spans"})
	if err == nil {
		t.Fatal("expected an error for unknown include value")
	}
	if !strings.Contains(err.Error(), `"spans"`) {
		t.Errorf("error %q does not name the bad value", err)
	}
}

// The list and export endpoints reject the same invalid filters.
func TestTraceEndpoints_InvalidFilters(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "unknown status", query: "&status=failed"},
		{name: "status is case-sensitive", query: "&status=ERROR"},
		{name: "negative minTokens", query: "&minTokens=-1"},
		{name: "non-numeric minTokens", query: "&minTokens=lots"},
		{name: "negative minDurationMs", query: "&minDurationMs=-5"},
		{name: "fractional minDurationMs", query: "&minDurationMs=1.5"},
		{name: "negative minSpanCount", query: "&minSpanCount=-2"},
	}
	endpoints := map[string]func(*Handler) http.HandlerFunc{
		"/api/v1/traces":        func(h *Handler) http.HandlerFunc { return h.GetTraceOverviews },
		"/api/v1/traces/export": func(h *Handler) http.HandlerFunc { return h.ExportTraces },
	}
	for path, handler := range endpoints {
		for _, tt := range tests {
			t.Run(path+" "+tt.name, func(t *testing.T) {
				r := httptest.NewRequest(http.MethodGet, path+"?"+baseParams()+tt.query, nil)
				rec := httptest.NewRecorder()
				handler(newHandler())(rec, r)
				assertBadRequest(t, rec)
			})
		}
	}
}

// model and conversationId accept up to MaxFilterValueLen characters on both endpoints.
func TestTraceEndpoints_FilterValueLength(t *testing.T) {
	endpoints := map[string]func(*Handler) http.HandlerFunc{
		"/api/v1/traces":        func(h *Handler) http.HandlerFunc { return h.GetTraceOverviews },
		"/api/v1/traces/export": func(h *Handler) http.HandlerFunc { return h.ExportTraces },
	}
	values := map[string]struct {
		value string
		want  int
	}{
		"256 characters":            {strings.Repeat("a", 256), http.StatusOK},
		"256 multi-byte characters": {strings.Repeat("é", 256), http.StatusOK},
		"257 characters":            {strings.Repeat("a", 257), http.StatusBadRequest},
	}
	for path, handler := range endpoints {
		for _, param := range []string{"model", "conversationId"} {
			for name, v := range values {
				t.Run(path+" "+param+" "+name, func(t *testing.T) {
					h := NewHandler(controllers.NewTracingController(&fakeObserverClient{}), nil)
					r := httptest.NewRequest(http.MethodGet, path+"?"+baseParams()+"&"+param+"="+url.QueryEscape(v.value), nil)
					rec := httptest.NewRecorder()
					handler(h)(rec, r)

					assertStatus(t, rec, v.want)
					if v.want != http.StatusBadRequest {
						return
					}
					if want := param + " must be at most 256 characters"; !strings.Contains(rec.Body.String(), want) {
						t.Errorf("body = %s, want it to contain %q", rec.Body.String(), want)
					}
					if strings.Contains(rec.Body.String(), v.value) {
						t.Error("body echoes the rejected value")
					}
				})
			}
		}
	}
}

// Malformed cursors return 400.
func TestGetTraceOverviews_InvalidCursor(t *testing.T) {
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	tests := map[string]string{
		"not base64":        "not base64!",
		"not JSON":          enc("nope"),
		"negative rank":     enc(`{"r":-1,"t":"2026-04-02T00:00:00Z"}`),
		"rank near max int": enc(`{"r":9223372036854775807,"t":"2026-04-02T00:00:00Z"}`),
		"missing time":      enc(`{"r":1}`),
	}
	for name, cursor := range tests {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/traces?"+baseParams()+"&cursor="+url.QueryEscape(cursor), nil)
			rec := httptest.NewRecorder()
			newHandler().GetTraceOverviews(rec, r)
			assertBadRequest(t, rec)
		})
	}
}

// A valid cursor returns 200.
func TestGetTraceOverviews_ValidCursorAccepted(t *testing.T) {
	fake := &fakeObserverClient{traces: []observer.TraceInfo{{TraceID: "trace-1", RootSpanID: "root", SpanCount: 2}}}
	h := NewHandler(controllers.NewTracingController(fake), nil)
	cursor := controllers.TraceCursor{Rank: 5, Time: time.Date(2026, 4, 2, 0, 0, 0, 500, time.UTC)}.Encode()

	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces?"+baseParams()+"&cursor="+cursor, nil)
	rec := httptest.NewRecorder()
	h.GetTraceOverviews(rec, r)

	assertStatus(t, rec, http.StatusOK)
}

// parseTraceFilters parses every filter and rejects invalid values.
func TestParseTraceFilters(t *testing.T) {
	t.Run("absent leaves every filter unset", func(t *testing.T) {
		f, err := parseTraceFilters(url.Values{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !f.IsZero() {
			t.Errorf("filters = %+v, want zero", f)
		}
	})

	t.Run("all six parse", func(t *testing.T) {
		q, _ := url.ParseQuery("status=error&minDurationMs=5000&minTokens=0&minSpanCount=20&model=gpt-4o&conversationId=conv-42")
		f, err := parseTraceFilters(q)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f.Status != controllers.TraceStatusError {
			t.Errorf("status = %q, want error", f.Status)
		}
		if f.MinDurationMs == nil || *f.MinDurationMs != 5000 {
			t.Errorf("minDurationMs = %v, want 5000", f.MinDurationMs)
		}
		if f.MinTokens == nil || *f.MinTokens != 0 {
			t.Errorf("minTokens = %v, want an explicit 0", f.MinTokens)
		}
		if f.MinSpanCount == nil || *f.MinSpanCount != 20 {
			t.Errorf("minSpanCount = %v, want 20", f.MinSpanCount)
		}
		if f.Model != "gpt-4o" || f.ConversationID != "conv-42" {
			t.Errorf("model/conversationId = %q/%q, want gpt-4o/conv-42", f.Model, f.ConversationID)
		}
	})

	t.Run("error names the parameter", func(t *testing.T) {
		_, err := parseTraceFilters(url.Values{"minTokens": {"-1"}})
		if err == nil || !strings.Contains(err.Error(), "minTokens") {
			t.Errorf("error = %v, want one naming minTokens", err)
		}
	})
}

// A model filter requests inline attributes without include=models.
func TestGetTraceOverviews_ModelFilterPassedThrough(t *testing.T) {
	fake := &fakeObserverClient{traces: []observer.TraceInfo{{TraceID: "trace-1", RootSpanID: "root", SpanCount: 2}}}
	h := NewHandler(controllers.NewTracingController(fake), nil)

	r := httptest.NewRequest(http.MethodGet, "/api/v1/traces?"+baseParams()+"&model=gpt-4o", nil)
	rec := httptest.NewRecorder()
	h.GetTraceOverviews(rec, r)

	assertStatus(t, rec, http.StatusOK)
	if !fake.lastSpansReq.IncludeAttributes {
		t.Error("expected the span list to be requested with inline attributes")
	}
}

func TestGetLogs_SearchPhrasePassedThrough(t *testing.T) {
	fake := &fakeObserverClient{}
	h := NewHandler(nil, controllers.NewObservabilityController(fake))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?"+baseParams()+"&searchPhrase=connection+refused", nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)

	assertStatus(t, rec, http.StatusOK)
	if fake.lastLogsReq.SearchPhrase == nil {
		t.Fatal("expected searchPhrase to be forwarded to the upstream request, got nil")
	}
	if got := *fake.lastLogsReq.SearchPhrase; got != "connection refused" {
		t.Errorf("expected searchPhrase %q, got %q", "connection refused", got)
	}
}

func TestGetLogs_EmptySearchPhraseOmittedUpstream(t *testing.T) {
	fake := &fakeObserverClient{}
	h := NewHandler(nil, controllers.NewObservabilityController(fake))

	r := httptest.NewRequest(http.MethodGet, "/api/v1/logs?"+baseParams(), nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)

	assertStatus(t, rec, http.StatusOK)
	if fake.lastLogsReq.SearchPhrase != nil {
		t.Errorf("expected no searchPhrase upstream when absent, got %q", *fake.lastLogsReq.SearchPhrase)
	}
}

func TestGetLogs_MethodNotAllowed(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/logs?"+baseParams(), nil)
	rec := httptest.NewRecorder()
	h.GetLogs(rec, r)
	assertStatus(t, rec, http.StatusMethodNotAllowed)
}

// ── GetBuildLogs ─────────────────────────────────────────────────────────────

func TestGetBuildLogs_MissingOrganization(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/build-logs?buildName=build-1", nil)
	rec := httptest.NewRecorder()
	h.GetBuildLogs(rec, r)
	assertBadRequest(t, rec)
}

func TestGetBuildLogs_MissingBuildName(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/build-logs?organization=default", nil)
	rec := httptest.NewRecorder()
	h.GetBuildLogs(rec, r)
	assertBadRequest(t, rec)
}

func TestGetBuildLogs_MethodNotAllowed(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/build-logs?organization=default&buildName=build-1", nil)
	rec := httptest.NewRecorder()
	h.GetBuildLogs(rec, r)
	assertStatus(t, rec, http.StatusMethodNotAllowed)
}

// ── GetMetrics ───────────────────────────────────────────────────────────────

func TestGetMetrics_MissingOrganization(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/metrics?project=p&agent=a&environment=e&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetMetrics(rec, r)
	assertBadRequest(t, rec)
}

func TestGetMetrics_MissingProject(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/metrics?organization=default&agent=a&environment=e&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetMetrics(rec, r)
	assertBadRequest(t, rec)
}

func TestGetMetrics_MissingAgent(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/metrics?organization=default&project=p&environment=e&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetMetrics(rec, r)
	assertBadRequest(t, rec)
}

func TestGetMetrics_MissingEnvironment(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/metrics?organization=default&project=p&agent=a&startTime=2026-04-01T00:00:00Z&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetMetrics(rec, r)
	assertBadRequest(t, rec)
}

func TestGetMetrics_MissingStartTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/metrics?organization=default&project=p&agent=a&environment=e&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetMetrics(rec, r)
	assertBadRequest(t, rec)
}

func TestGetMetrics_MissingEndTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/metrics?organization=default&project=p&agent=a&environment=e&startTime=2026-04-01T00:00:00Z", nil)
	rec := httptest.NewRecorder()
	h.GetMetrics(rec, r)
	assertBadRequest(t, rec)
}

func TestGetMetrics_InvalidStartTime(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/metrics?organization=default&project=p&agent=a&environment=e&startTime=not-a-time&endTime=2026-04-06T23:59:59Z", nil)
	rec := httptest.NewRecorder()
	h.GetMetrics(rec, r)
	assertBadRequest(t, rec)
}

func TestGetMetrics_MethodNotAllowed(t *testing.T) {
	h := newHandler()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/metrics?"+baseParams(), nil)
	rec := httptest.NewRecorder()
	h.GetMetrics(rec, r)
	assertStatus(t, rec, http.StatusMethodNotAllowed)
}

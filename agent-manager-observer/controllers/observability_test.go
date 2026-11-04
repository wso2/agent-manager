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
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

// fakeObservabilityClient is a minimal observer.Client implementation scoped
// to the observability controller tests. It captures the requests passed to
// QueryLogs/QueryMetrics so tests can assert on search scopes, limits, sort
// order, and time windows, and returns scripted responses.
type fakeObservabilityClient struct {
	logsResp    *observer.LogsQueryResponse
	metricsResp *observer.ResourceMetricsTimeSeries
	// tipMetricsResp and tipMetricsErr answer the single-instant tip query
	// GetMetrics issues alongside the grid query.
	tipMetricsResp *observer.ResourceMetricsTimeSeries
	tipMetricsErr  error
	// nilMetrics makes every QueryMetrics call return (nil, nil).
	nilMetrics bool

	lastLogsReq observer.LogsQueryRequest

	mu              sync.Mutex
	gridMetricsReqs []observer.MetricsQueryRequest
	tipMetricsReqs  []observer.MetricsQueryRequest

	defaultNamespace string
}

func (f *fakeObservabilityClient) QueryTraces(_ context.Context, _ observer.TracesQueryRequest) (*observer.TracesQueryResponse, error) {
	return &observer.TracesQueryResponse{}, nil
}

func (f *fakeObservabilityClient) QueryTraceSpans(_ context.Context, _ string, _ observer.TracesQueryRequest) (*observer.TraceSpansQueryResponse, error) {
	return &observer.TraceSpansQueryResponse{}, nil
}

func (f *fakeObservabilityClient) GetSpanDetails(_ context.Context, _, _ string) (*observer.SpanDetailsResponse, error) {
	return &observer.SpanDetailsResponse{}, nil
}

func (f *fakeObservabilityClient) QueryLogs(_ context.Context, req observer.LogsQueryRequest) (*observer.LogsQueryResponse, error) {
	f.lastLogsReq = req
	if f.logsResp != nil {
		return f.logsResp, nil
	}
	return &observer.LogsQueryResponse{}, nil
}

func (f *fakeObservabilityClient) QueryMetrics(_ context.Context, req observer.MetricsQueryRequest) (*observer.ResourceMetricsTimeSeries, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.nilMetrics {
		return nil, nil
	}
	if req.StartTime.Equal(req.EndTime) {
		f.tipMetricsReqs = append(f.tipMetricsReqs, req)
		if f.tipMetricsErr != nil {
			return nil, f.tipMetricsErr
		}
		if f.tipMetricsResp != nil {
			return f.tipMetricsResp, nil
		}
		return &observer.ResourceMetricsTimeSeries{}, nil
	}
	f.gridMetricsReqs = append(f.gridMetricsReqs, req)
	if f.metricsResp != nil {
		return f.metricsResp, nil
	}
	return &observer.ResourceMetricsTimeSeries{}, nil
}

func (f *fakeObservabilityClient) NamespaceFor(_ string) string {
	return f.defaultNamespace
}

func strPtr(s string) *string { return &s }

func timePtr(t time.Time) *time.Time { return &t }

func floatPtr(v float64) *float64 { return &v }

func intPtr(v int) *int { return &v }

// (a) GetLogs builds a ComponentSearchScope from the params and converts
// entries.
func TestGetLogs_BuildsComponentSearchScope(t *testing.T) {
	fake := &fakeObservabilityClient{defaultNamespace: "ns-1"}
	c := NewObservabilityController(fake)

	start := time.Now().Add(-1 * time.Hour)
	end := time.Now()
	params := LogsQueryParams{
		Organization: "org-1",
		Project:      "proj-1",
		Agent:        "agent-1",
		Environment:  "env-1",
		StartTime:    start,
		EndTime:      end,
		SearchPhrase: "boom",
		LogLevels:    []string{"ERROR"},
		Limit:        intPtr(50),
		SortOrder:    "desc",
	}

	if _, err := c.GetLogs(context.Background(), params); err != nil {
		t.Fatalf("GetLogs returned error: %v", err)
	}

	scope, ok := fake.lastLogsReq.SearchScope.(observer.ComponentSearchScope)
	if !ok {
		t.Fatalf("expected ComponentSearchScope, got %T", fake.lastLogsReq.SearchScope)
	}
	if scope.Namespace != "ns-1" {
		t.Errorf("scope.Namespace = %q, want ns-1", scope.Namespace)
	}
	if scope.Project == nil || *scope.Project != "proj-1" {
		t.Errorf("scope.Project = %v, want proj-1", scope.Project)
	}
	if scope.Component == nil || *scope.Component != "agent-1" {
		t.Errorf("scope.Component = %v, want agent-1", scope.Component)
	}
	if scope.Environment == nil || *scope.Environment != "env-1" {
		t.Errorf("scope.Environment = %v, want env-1", scope.Environment)
	}
	if !fake.lastLogsReq.StartTime.Equal(start) || !fake.lastLogsReq.EndTime.Equal(end) {
		t.Errorf("time window not passed through: got start=%v end=%v", fake.lastLogsReq.StartTime, fake.lastLogsReq.EndTime)
	}
	if fake.lastLogsReq.Limit == nil || *fake.lastLogsReq.Limit != 50 {
		t.Errorf("Limit = %v, want 50", fake.lastLogsReq.Limit)
	}
	if len(fake.lastLogsReq.LogLevels) != 1 || fake.lastLogsReq.LogLevels[0] != "ERROR" {
		t.Errorf("LogLevels = %v, want [ERROR]", fake.lastLogsReq.LogLevels)
	}
	if fake.lastLogsReq.SearchPhrase == nil || *fake.lastLogsReq.SearchPhrase != "boom" {
		t.Errorf("SearchPhrase = %v, want boom", fake.lastLogsReq.SearchPhrase)
	}
	if fake.lastLogsReq.SortOrder == nil || *fake.lastLogsReq.SortOrder != "desc" {
		t.Errorf("SortOrder = %v, want desc", fake.lastLogsReq.SortOrder)
	}
}

// (b) JSON-log msg extraction: an entry whose Log is a JSON object with a
// non-empty "msg" field replaces Log; an empty upstream Level falls back to
// the parsed (uppercased) "level".
func TestGetLogs_ExtractsMsgFromJSONLogLine(t *testing.T) {
	rawLog := `{"time":"2026-01-01T00:00:00Z","level":"info","msg":"hello"}`
	fake := &fakeObservabilityClient{
		logsResp: &observer.LogsQueryResponse{
			Logs: []observer.LogEntry{
				{
					Timestamp: timePtr(time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)),
					Log:       strPtr(rawLog),
					Level:     strPtr(""),
				},
			},
		},
	}
	c := NewObservabilityController(fake)

	resp, err := c.GetLogs(context.Background(), LogsQueryParams{
		Organization: "org-1",
		Project:      "proj-1",
		Agent:        "agent-1",
		Environment:  "env-1",
		StartTime:    time.Now().Add(-time.Hour),
		EndTime:      time.Now(),
	})
	if err != nil {
		t.Fatalf("GetLogs returned error: %v", err)
	}
	if len(resp.Logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(resp.Logs))
	}
	entry := resp.Logs[0]
	if entry.Log != "hello" {
		t.Errorf("Log = %q, want hello", entry.Log)
	}
	if entry.LogLevel != "INFO" {
		t.Errorf("LogLevel = %q, want INFO", entry.LogLevel)
	}
}

// The timestamp fallback only applies when the upstream Timestamp is zero;
// here the upstream Timestamp is already set, so it must win over the
// parsed "time" field embedded in the JSON log line.
func TestGetLogs_TimestampFallbackOnlyWhenZero(t *testing.T) {
	upstreamTime := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	rawLog := `{"time":"2026-01-01T00:00:00Z","level":"info","msg":"hello"}`
	fake := &fakeObservabilityClient{
		logsResp: &observer.LogsQueryResponse{
			Logs: []observer.LogEntry{
				{Timestamp: timePtr(upstreamTime), Log: strPtr(rawLog), Level: strPtr("")},
			},
		},
	}
	c := NewObservabilityController(fake)

	resp, err := c.GetLogs(context.Background(), LogsQueryParams{
		Organization: "org-1",
		StartTime:    time.Now().Add(-time.Hour),
		EndTime:      time.Now(),
	})
	if err != nil {
		t.Fatalf("GetLogs returned error: %v", err)
	}
	if !resp.Logs[0].Timestamp.Equal(upstreamTime) {
		t.Errorf("Timestamp = %v, want unchanged upstream %v", resp.Logs[0].Timestamp, upstreamTime)
	}
}

// A non-empty upstream Level must not be overwritten by the JSON-parsed
// level, and a non-JSON log line must pass through unchanged.
func TestGetLogs_PlainLogLinePassesThroughUnchanged(t *testing.T) {
	fake := &fakeObservabilityClient{
		logsResp: &observer.LogsQueryResponse{
			Logs: []observer.LogEntry{
				{
					Timestamp: timePtr(time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)),
					Log:       strPtr("plain text log line"),
					Level:     strPtr("WARN"),
				},
			},
		},
	}
	c := NewObservabilityController(fake)

	resp, err := c.GetLogs(context.Background(), LogsQueryParams{
		Organization: "org-1",
		StartTime:    time.Now().Add(-time.Hour),
		EndTime:      time.Now(),
	})
	if err != nil {
		t.Fatalf("GetLogs returned error: %v", err)
	}
	entry := resp.Logs[0]
	if entry.Log != "plain text log line" {
		t.Errorf("Log = %q, want unchanged", entry.Log)
	}
	if entry.LogLevel != "WARN" {
		t.Errorf("LogLevel = %q, want unchanged WARN", entry.LogLevel)
	}

	// Unset SearchPhrase/SortOrder/Limit in the params must translate to nil
	// pointers on the upstream request (so the omitempty JSON tags drop them),
	// not pointers to zero values.
	if fake.lastLogsReq.SearchPhrase != nil {
		t.Errorf("SearchPhrase = %v, want nil for unset params.SearchPhrase", *fake.lastLogsReq.SearchPhrase)
	}
	if fake.lastLogsReq.SortOrder != nil {
		t.Errorf("SortOrder = %v, want nil for unset params.SortOrder", *fake.lastLogsReq.SortOrder)
	}
	if fake.lastLogsReq.Limit != nil {
		t.Errorf("Limit = %v, want nil for unset params.Limit", *fake.lastLogsReq.Limit)
	}
}

// TotalCount falls back to len(logs) when upstream Total is nil; TookMs
// falls back to 0 when upstream TookMs is nil.
func TestGetLogs_TotalAndTookMsFallbacks(t *testing.T) {
	fake := &fakeObservabilityClient{
		logsResp: &observer.LogsQueryResponse{
			Logs: []observer.LogEntry{
				{Timestamp: timePtr(time.Now()), Log: strPtr("a"), Level: strPtr("INFO")},
				{Timestamp: timePtr(time.Now()), Log: strPtr("b"), Level: strPtr("INFO")},
			},
			Total:  nil,
			TookMs: nil,
		},
	}
	c := NewObservabilityController(fake)

	resp, err := c.GetLogs(context.Background(), LogsQueryParams{
		Organization: "org-1",
		StartTime:    time.Now().Add(-time.Hour),
		EndTime:      time.Now(),
	})
	if err != nil {
		t.Fatalf("GetLogs returned error: %v", err)
	}
	if resp.TotalCount != 2 {
		t.Errorf("TotalCount = %d, want 2 (len(logs))", resp.TotalCount)
	}
	if resp.TookMs != 0 {
		t.Errorf("TookMs = %v, want 0", resp.TookMs)
	}

	// When upstream values are present, they should be used verbatim.
	fake.logsResp.Total = intPtr(42)
	tookMs := 7
	fake.logsResp.TookMs = &tookMs
	resp, err = c.GetLogs(context.Background(), LogsQueryParams{
		Organization: "org-1",
		StartTime:    time.Now().Add(-time.Hour),
		EndTime:      time.Now(),
	})
	if err != nil {
		t.Fatalf("GetLogs returned error: %v", err)
	}
	if resp.TotalCount != 42 {
		t.Errorf("TotalCount = %d, want 42 (from upstream Total)", resp.TotalCount)
	}
	if resp.TookMs != 7 {
		t.Errorf("TookMs = %v, want 7 (from upstream TookMs)", resp.TookMs)
	}
}

// (c) GetBuildLogs uses a WorkflowSearchScope keyed on the build/workflow
// run name, a limit of 1000, ascending sort order, and a ~30-day window.
func TestGetBuildLogs_UsesWorkflowSearchScopeAndFixedWindow(t *testing.T) {
	fake := &fakeObservabilityClient{defaultNamespace: "ns-1"}
	c := NewObservabilityController(fake)

	before := time.Now()
	if _, err := c.GetBuildLogs(context.Background(), "org-1", "build-123"); err != nil {
		t.Fatalf("GetBuildLogs returned error: %v", err)
	}
	after := time.Now()

	scope, ok := fake.lastLogsReq.SearchScope.(observer.WorkflowSearchScope)
	if !ok {
		t.Fatalf("expected WorkflowSearchScope, got %T", fake.lastLogsReq.SearchScope)
	}
	if scope.Namespace != "ns-1" {
		t.Errorf("scope.Namespace = %q, want ns-1", scope.Namespace)
	}
	if scope.WorkflowRunName == nil || *scope.WorkflowRunName != "build-123" {
		t.Errorf("scope.WorkflowRunName = %v, want build-123", scope.WorkflowRunName)
	}
	if fake.lastLogsReq.Limit == nil || *fake.lastLogsReq.Limit != 1000 {
		t.Errorf("Limit = %v, want 1000", fake.lastLogsReq.Limit)
	}
	if fake.lastLogsReq.SortOrder == nil || *fake.lastLogsReq.SortOrder != "asc" {
		t.Errorf("SortOrder = %v, want asc", fake.lastLogsReq.SortOrder)
	}

	wantStart := before.Add(-30 * 24 * time.Hour)
	wantEnd := before
	if fake.lastLogsReq.StartTime.Before(wantStart.Add(-time.Minute)) || fake.lastLogsReq.StartTime.After(after.Add(-30*24*time.Hour+time.Minute)) {
		t.Errorf("StartTime = %v, want ~%v", fake.lastLogsReq.StartTime, wantStart)
	}
	if fake.lastLogsReq.EndTime.Before(wantEnd.Add(-time.Minute)) || fake.lastLogsReq.EndTime.After(after.Add(time.Minute)) {
		t.Errorf("EndTime = %v, want ~now", fake.lastLogsReq.EndTime)
	}
}

// (d) GetMetrics maps all six series (MemoryUsage -> Memory) and nil
// upstream series convert to empty (never nil) slices.
func TestGetMetrics_MapsAllSixSeriesAndNilBecomesEmpty(t *testing.T) {
	fake := &fakeObservabilityClient{
		defaultNamespace: "ns-1",
		metricsResp: &observer.ResourceMetricsTimeSeries{
			CpuUsage: &[]observer.MetricsTimeSeriesItem{
				{Timestamp: timePtr(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), Value: floatPtr(1.5)},
			},
			MemoryUsage: &[]observer.MetricsTimeSeriesItem{
				{Timestamp: timePtr(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), Value: floatPtr(2048)},
			},
			// CpuRequests, CpuLimits, MemoryRequests, MemoryLimits left nil.
		},
	}
	c := NewObservabilityController(fake)

	resp, err := c.GetMetrics(context.Background(), MetricsQueryParams{
		Organization: "org-1",
		Project:      "proj-1",
		Agent:        "agent-1",
		Environment:  "env-1",
		StartTime:    time.Now().Add(-time.Hour),
		EndTime:      time.Now(),
	})
	if err != nil {
		t.Fatalf("GetMetrics returned error: %v", err)
	}

	if len(resp.CpuUsage) != 1 || resp.CpuUsage[0].Value != 1.5 || resp.CpuUsage[0].Time != "2026-01-01T00:00:00Z" {
		t.Errorf("CpuUsage = %+v, want one point (1.5, 2026-01-01T00:00:00Z)", resp.CpuUsage)
	}
	if len(resp.Memory) != 1 || resp.Memory[0].Value != 2048 {
		t.Errorf("Memory = %+v, want MemoryUsage mapped to Memory", resp.Memory)
	}
	if resp.CpuRequests == nil || len(resp.CpuRequests) != 0 {
		t.Errorf("CpuRequests = %#v, want non-nil empty slice", resp.CpuRequests)
	}
	if resp.CpuLimits == nil || len(resp.CpuLimits) != 0 {
		t.Errorf("CpuLimits = %#v, want non-nil empty slice", resp.CpuLimits)
	}
	if resp.MemoryRequests == nil || len(resp.MemoryRequests) != 0 {
		t.Errorf("MemoryRequests = %#v, want non-nil empty slice", resp.MemoryRequests)
	}
	if resp.MemoryLimits == nil || len(resp.MemoryLimits) != 0 {
		t.Errorf("MemoryLimits = %#v, want non-nil empty slice", resp.MemoryLimits)
	}

	scope := fake.gridMetricsReqs[0].SearchScope
	if scope.Namespace != "ns-1" {
		t.Errorf("scope.Namespace = %q, want ns-1", scope.Namespace)
	}
	if scope.Project == nil || *scope.Project != "proj-1" {
		t.Errorf("scope.Project = %v, want proj-1", scope.Project)
	}
	if fake.gridMetricsReqs[0].Metric != "resource" {
		t.Errorf("Metric = %q, want resource", fake.gridMetricsReqs[0].Metric)
	}
}

// (e) GetMetrics picks a step that keeps each console time range within
// maxMetricsPoints points.
func TestMetricsStep_ScalesWithWindow(t *testing.T) {
	cases := []struct {
		window time.Duration
		want   time.Duration
	}{
		{10 * time.Minute, 30 * time.Second},
		{time.Hour, 30 * time.Second},
		{6 * time.Hour, 2 * time.Minute},
		{12 * time.Hour, 5 * time.Minute},
		{24 * time.Hour, 10 * time.Minute},
		{7 * 24 * time.Hour, time.Hour},
		{365 * 24 * time.Hour, 24 * time.Hour},
	}
	for _, tc := range cases {
		if got := metricsStep(tc.window); got != tc.want {
			t.Errorf("metricsStep(%v) = %v, want %v", tc.window, got, tc.want)
		}
	}
}

// (f) Two refreshes of the same range a few seconds apart query the same
// step-aligned window, so the returned points land on the same timestamps.
func TestGetMetrics_AlignsWindowToStep(t *testing.T) {
	first := time.Date(2026, 9, 23, 5, 59, 39, 908_000_000, time.UTC)
	refreshes := []time.Time{first, first.Add(15 * time.Second)}

	var got []observer.MetricsQueryRequest
	for _, end := range refreshes {
		fake := &fakeObservabilityClient{}
		c := NewObservabilityController(fake)
		if _, err := c.GetMetrics(context.Background(), MetricsQueryParams{
			StartTime: end.Add(-12 * time.Hour),
			EndTime:   end,
		}); err != nil {
			t.Fatalf("GetMetrics returned error: %v", err)
		}
		got = append(got, fake.gridMetricsReqs[0])
	}

	wantStart := time.Date(2026, 9, 22, 17, 55, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 9, 23, 5, 55, 0, 0, time.UTC)
	for i, req := range got {
		if !req.StartTime.Equal(wantStart) || !req.EndTime.Equal(wantEnd) {
			t.Errorf("refresh %d window = [%v, %v], want [%v, %v]", i, req.StartTime, req.EndTime, wantStart, wantEnd)
		}
		if req.Step != "5m0s" {
			t.Errorf("refresh %d Step = %q, want 5m0s", i, req.Step)
		}
	}
}

// (g) A window ending between grid points gets a live tip point at the
// requested end, so an agent younger than one step still shows its newest data.
func TestGetMetrics_AppendsLiveTipAfterGrid(t *testing.T) {
	end := time.Date(2026, 10, 27, 12, 29, 0, 0, time.UTC)
	gridPoint := time.Date(2026, 10, 27, 12, 20, 0, 0, time.UTC)
	fake := &fakeObservabilityClient{
		metricsResp: &observer.ResourceMetricsTimeSeries{
			MemoryUsage: &[]observer.MetricsTimeSeriesItem{{Timestamp: timePtr(gridPoint), Value: floatPtr(1)}},
		},
		tipMetricsResp: &observer.ResourceMetricsTimeSeries{
			CpuUsage:    &[]observer.MetricsTimeSeriesItem{{Timestamp: timePtr(end), Value: floatPtr(0.5)}},
			MemoryUsage: &[]observer.MetricsTimeSeriesItem{{Timestamp: timePtr(end), Value: floatPtr(2)}},
		},
	}
	c := NewObservabilityController(fake)

	resp, err := c.GetMetrics(context.Background(), MetricsQueryParams{
		StartTime: end.Add(-24 * time.Hour),
		EndTime:   end,
	})
	if err != nil {
		t.Fatalf("GetMetrics returned error: %v", err)
	}

	if len(fake.gridMetricsReqs) != 1 || len(fake.tipMetricsReqs) != 1 {
		t.Fatalf("got %d grid and %d tip queries, want one of each", len(fake.gridMetricsReqs), len(fake.tipMetricsReqs))
	}
	if grid := fake.gridMetricsReqs[0]; !grid.EndTime.Equal(gridPoint) || grid.Step != "10m0s" {
		t.Errorf("grid query end = %v step = %q, want %v, 10m0s", grid.EndTime, grid.Step, gridPoint)
	}
	if tip := fake.tipMetricsReqs[0]; !tip.StartTime.Equal(end) || !tip.EndTime.Equal(end) {
		t.Errorf("tip query window = [%v, %v], want the instant %v", tip.StartTime, tip.EndTime, end)
	}
	if len(resp.CpuUsage) != 1 || resp.CpuUsage[0].Value != 0.5 {
		t.Errorf("CpuUsage = %+v, want the tip point alone", resp.CpuUsage)
	}
	if len(resp.Memory) != 2 || resp.Memory[0].Value != 1 || resp.Memory[1].Value != 2 {
		t.Errorf("Memory = %+v, want the grid point then the tip point", resp.Memory)
	}
}

// (h) A window already ending on a grid point needs no tip query.
func TestGetMetrics_SkipsTipWhenEndIsOnGrid(t *testing.T) {
	end := time.Date(2026, 10, 27, 12, 30, 0, 0, time.UTC)
	fake := &fakeObservabilityClient{}
	c := NewObservabilityController(fake)

	if _, err := c.GetMetrics(context.Background(), MetricsQueryParams{
		StartTime: end.Add(-24 * time.Hour),
		EndTime:   end,
	}); err != nil {
		t.Fatalf("GetMetrics returned error: %v", err)
	}
	if len(fake.tipMetricsReqs) != 0 {
		t.Errorf("got %d tip queries, want none", len(fake.tipMetricsReqs))
	}
}

// (i) A failed tip query still returns the grid, which holds nearly all the
// data; only the newest point is lost.
func TestGetMetrics_TipQueryErrorReturnsGrid(t *testing.T) {
	end := time.Date(2026, 10, 27, 12, 29, 0, 0, time.UTC)
	gridPoint := time.Date(2026, 10, 27, 12, 20, 0, 0, time.UTC)
	fake := &fakeObservabilityClient{
		metricsResp: &observer.ResourceMetricsTimeSeries{
			MemoryUsage: &[]observer.MetricsTimeSeriesItem{{Timestamp: timePtr(gridPoint), Value: floatPtr(1)}},
		},
		tipMetricsErr: errors.New("upstream unavailable"),
	}
	c := NewObservabilityController(fake)

	resp, err := c.GetMetrics(context.Background(), MetricsQueryParams{
		StartTime: end.Add(-24 * time.Hour),
		EndTime:   end,
	})
	if err != nil {
		t.Fatalf("GetMetrics returned error: %v", err)
	}
	if len(resp.Memory) != 1 || resp.Memory[0].Value != 1 {
		t.Errorf("Memory = %+v, want the grid point alone", resp.Memory)
	}
}

// (j) An upstream that answers without a series or an error yields empty
// series rather than a nil dereference when the grid and tip are merged.
func TestGetMetrics_NilUpstreamResponseYieldsEmptySeries(t *testing.T) {
	end := time.Date(2026, 10, 27, 12, 29, 0, 0, time.UTC)
	c := NewObservabilityController(&fakeObservabilityClient{nilMetrics: true})

	resp, err := c.GetMetrics(context.Background(), MetricsQueryParams{
		StartTime: end.Add(-24 * time.Hour),
		EndTime:   end,
	})
	if err != nil {
		t.Fatalf("GetMetrics returned error: %v", err)
	}
	if resp.CpuUsage == nil || len(resp.CpuUsage) != 0 {
		t.Errorf("CpuUsage = %#v, want non-nil empty slice", resp.CpuUsage)
	}
}

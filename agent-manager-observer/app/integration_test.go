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

package app

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/clickhouse"
	"github.com/wso2/agent-manager/agent-manager-observer/config"
	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/opensearchreader"
	"github.com/wso2/agent-manager/agent-manager-observer/scope"
)

const staticTestScopeJSON = `{
	"projects": {"default/default": "55f125d0-35fc-4b66-af37-50b9fc97e0a0"},
	"components": {"default/it-support": "f41255dd-7647-4401-895e-a55d629dbf5e"},
	"environments": {"default/default": "c7acefa5-b717-40f6-b056-2c05a739420c"}
}`

// setupBackendConfig inspects environment variables and selects a configured TraceReader backend.
func setupBackendConfig(t *testing.T) (*config.Config, string) {
	t.Helper()

	backend := os.Getenv("TRACE_BACKEND")
	chAddr := os.Getenv("CLICKHOUSE_TEST_ADDR")
	osAddr := os.Getenv("OPENSEARCH_TEST_ADDR")

	if backend == "" {
		if chAddr != "" {
			backend = config.TraceBackendClickHouse
		} else if osAddr != "" {
			backend = config.TraceBackendOpenSearch
		} else {
			t.Skip("Neither TRACE_BACKEND, CLICKHOUSE_TEST_ADDR, nor OPENSEARCH_TEST_ADDR is set; skipping integration test")
		}
	}

	cfg := &config.Config{
		Observer: config.ObserverConfig{
			DefaultNamespace: "default",
			BaseURL:          "http://localhost:8085",
		},
		Traces: config.TraceReaderConfig{
			Backend:   backend,
			ScopeUIDs: staticTestScopeJSON,
			Batching:  true,
		},
	}

	switch backend {
	case config.TraceBackendClickHouse:
		if chAddr == "" {
			t.Skip("TRACE_BACKEND is clickhouse but CLICKHOUSE_TEST_ADDR is not set")
		}
		cfg.Traces.Addr = chAddr
		cfg.Traces.Database = "otel"
		cfg.Traces.User = os.Getenv("CLICKHOUSE_TEST_USER")
		cfg.Traces.Password = os.Getenv("CLICKHOUSE_TEST_PASSWORD")
		cfg.Traces.MaxOpenConns = 2
		cfg.Traces.MaxIdleConns = 1
		cfg.Traces.ConnMaxLifetime = time.Minute
		cfg.Traces.DialTimeout = 5 * time.Second
		cfg.Traces.Table = "otel_traces"
		cfg.Traces.Layout = config.TraceLayoutExporter

	case config.TraceBackendOpenSearch:
		if osAddr == "" {
			t.Skip("TRACE_BACKEND is opensearch but OPENSEARCH_TEST_ADDR is not set")
		}
		cfg.Traces.OSAddr = osAddr
		cfg.Traces.OSUser = os.Getenv("OPENSEARCH_TEST_USER")
		cfg.Traces.OSPassword = os.Getenv("OPENSEARCH_TEST_PASSWORD")
		cfg.Traces.OSIndex = "otel-traces-*"
		cfg.Traces.OSInsecureTLS = true
		cfg.Traces.DialTimeout = 10 * time.Second

	case config.TraceBackendOpenChoreo:
		// Upstream observer HTTP client
		if os.Getenv("OPENCHOREO_TEST_URL") != "" {
			cfg.Observer.BaseURL = os.Getenv("OPENCHOREO_TEST_URL")
		}

	default:
		t.Fatalf("unsupported backend: %s", backend)
	}

	return cfg, backend
}

func unifiedIntegrationRequest(limit int) observer.TracesQueryRequest {
	str := func(s string) *string { return &s }
	order := "desc"
	return observer.TracesQueryRequest{
		StartTime: time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC),
		Limit:     &limit,
		SortOrder: &order,
		SearchScope: observer.ComponentSearchScope{
			Namespace:   "default",
			Project:     str("default"),
			Component:   str("it-support"),
			Environment: str("default"),
		},
	}
}

// TestIntegration_SelectedTraceReader selects a trace reader backend via newTraceReader
// and performs end-to-end trace queries, span queries, detail lookups, and batch fetches.
func TestIntegration_SelectedTraceReader(t *testing.T) {
	cfg, backendName := setupBackendConfig(t)
	t.Logf("Running integration test against selected backend: %s", backendName)

	obsClient := observer.NewClient(cfg.Observer.BaseURL, requestTokenProvider{}, cfg.Observer.DefaultNamespace)

	// Dynamically select and construct the TraceReader using the application factory
	reader, err := newTraceReader(cfg, obsClient)
	if err != nil {
		t.Fatalf("failed to select/initialize trace reader for backend %q: %v", backendName, err)
	}
	if closer, ok := reader.(io.Closer); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}

	ctx := context.Background()

	// 1. QueryTraces: List traces matching scope and window
	listReq := unifiedIntegrationRequest(10)
	tracesResp, err := reader.QueryTraces(ctx, listReq)
	if err != nil {
		t.Fatalf("[%s] QueryTraces failed: %v", backendName, err)
	}
	if len(tracesResp.Traces) == 0 {
		t.Fatalf("[%s] expected traces, got none", backendName)
	}
	if tracesResp.Total < len(tracesResp.Traces) {
		t.Errorf("[%s] total %d less than returned traces %d", backendName, tracesResp.Total, len(tracesResp.Traces))
	}

	for i, tr := range tracesResp.Traces {
		if tr.SpanCount <= 0 {
			t.Errorf("[%s] trace %d has invalid SpanCount: %d", backendName, i, tr.SpanCount)
		}
		if tr.StartTime.Location() != time.UTC {
			t.Errorf("[%s] trace %d StartTime is not UTC", backendName, i)
		}
		if i > 0 && tr.StartTime.After(tracesResp.Traces[i-1].StartTime) {
			t.Errorf("[%s] traces not returned in descending order at index %d", backendName, i)
		}
	}

	targetTrace := tracesResp.Traces[0]

	// 2. QueryTraceSpans: Spans with attributes
	spansReq := unifiedIntegrationRequest(1000)
	spansReq.SortOrder = nil // default: oldest first
	spansReq.IncludeAttributes = true

	spansResp, err := reader.QueryTraceSpans(ctx, targetTrace.TraceID, spansReq)
	if err != nil {
		t.Fatalf("[%s] QueryTraceSpans failed: %v", backendName, err)
	}
	if len(spansResp.Spans) != targetTrace.SpanCount {
		t.Fatalf("[%s] expected %d spans, got %d", backendName, targetTrace.SpanCount, len(spansResp.Spans))
	}

	foundRoot := false
	for _, sp := range spansResp.Spans {
		if sp.ParentSpanID == "" {
			foundRoot = true
			if targetTrace.RootSpanID != "" && sp.SpanID != targetTrace.RootSpanID {
				t.Errorf("[%s] root span mismatch: got %s, want %s", backendName, sp.SpanID, targetTrace.RootSpanID)
			}
		}
		if sp.Attributes == nil {
			t.Errorf("[%s] expected span %s to include attributes", backendName, sp.SpanID)
		}
	}
	if !foundRoot {
		t.Errorf("[%s] root span not found in spans list", backendName)
	}

	// 3. QueryTraceSpans: Spans with attributes stripped
	spansReq.IncludeAttributes = false
	plainSpans, err := reader.QueryTraceSpans(ctx, targetTrace.TraceID, spansReq)
	if err != nil {
		t.Fatalf("[%s] QueryTraceSpans without attributes failed: %v", backendName, err)
	}
	if len(plainSpans.Spans) > 0 && plainSpans.Spans[0].Attributes != nil {
		t.Errorf("[%s] attributes must be excluded when IncludeAttributes is false", backendName)
	}

	// 4. GetSpanDetails: Fetch full span details
	rootID := targetTrace.RootSpanID
	if rootID == "" && len(spansResp.Spans) > 0 {
		rootID = spansResp.Spans[0].SpanID
	}
	detail, err := reader.GetSpanDetails(ctx, targetTrace.TraceID, rootID)
	if err != nil {
		t.Fatalf("[%s] GetSpanDetails failed: %v", backendName, err)
	}
	if detail.SpanID != rootID {
		t.Errorf("[%s] expected span ID %s, got %s", backendName, rootID, detail.SpanID)
	}

	// Verify not found errors
	_, err = reader.GetSpanDetails(ctx, targetTrace.TraceID, "0000000000000000")
	if !errors.Is(err, clickhouse.ErrSpanNotFound) && !errors.Is(err, opensearchreader.ErrSpanNotFound) {
		t.Errorf("[%s] expected span not found error for invalid span ID, got %v", backendName, err)
	}

	// 5. QueryTraceSpansBatch: If the selected reader supports batching, test it
	if batchReader, ok := reader.(observer.BatchTraceReader); ok {
		traceIDs := make([]string, 0, len(tracesResp.Traces))
		for _, tr := range tracesResp.Traces {
			traceIDs = append(traceIDs, tr.TraceID)
		}
		if len(traceIDs) > 5 {
			traceIDs = traceIDs[:5]
		}

		batchReq := unifiedIntegrationRequest(500)
		batchResult, err := batchReader.QueryTraceSpansBatch(ctx, traceIDs, batchReq)
		if err != nil {
			t.Fatalf("[%s] QueryTraceSpansBatch failed: %v", backendName, err)
		}
		if len(batchResult) == 0 {
			t.Errorf("[%s] expected batched spans for trace IDs %v, got none", backendName, traceIDs)
		}
		for _, id := range traceIDs {
			spans, ok := batchResult[id]
			if !ok || len(spans) == 0 {
				t.Errorf("[%s] missing or empty spans in batch result for trace %s", backendName, id)
			}
		}
	} else {
		t.Logf("[%s] selected reader does not implement BatchTraceReader, skipping batch query test", backendName)
	}

	// 6. Scope validation: querying with an unknown component must fail
	str := func(s string) *string { return &s }
	badReq := unifiedIntegrationRequest(5)
	badReq.SearchScope.Component = str("non-existent-agent-xyz")
	if _, err := reader.QueryTraces(ctx, badReq); !errors.Is(err, scope.ErrUnknownScope) {
		t.Errorf("[%s] expected ErrUnknownScope for unknown component, got %v", backendName, err)
	}
}

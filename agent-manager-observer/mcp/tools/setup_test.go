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

package tools

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-observer/controllers"
	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/rbac"
)

// constants for testing
const (
	testOrgName     = "default-org"
	testProjectName = "default-project"
	testAgentName   = "default-agent"
	testBuildName   = "default-build"
	testRunName     = "default-monitor-run"
	testEnvName     = "default-env"
	testTraceID     = "test-trace-id"
	testSpanID      = "test-span-id"
)

// fakeObserverClient is a minimal observer.Client stub: it records every
// call it receives and returns canned, always-well-formed responses, so the
// real controllers (and, through them, the tool handlers) can be exercised
// end-to-end without a live upstream Observer.
type fakeObserverClient struct {
	mu    sync.Mutex
	calls map[string][]any
	// traces and spans (keyed by trace ID) are what the trace queries
	// return; both are empty unless a test sets them.
	traces []observer.TraceInfo
	spans  map[string][]observer.SpanInfo
}

func newFakeObserverClient() *fakeObserverClient {
	return &fakeObserverClient{calls: make(map[string][]any)}
}

func (f *fakeObserverClient) recordCall(method string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[method] = append(f.calls[method], args)
}

func (f *fakeObserverClient) NamespaceFor(organization string) string {
	f.recordCall("NamespaceFor", organization)
	return "ns-" + organization
}

// QueryTraces returns the first limit traces in page order, ignoring the window.
func (f *fakeObserverClient) QueryTraces(_ context.Context, req observer.TracesQueryRequest) (*observer.TracesQueryResponse, error) {
	f.recordCall("QueryTraces", req)
	traces := slices.SortedFunc(slices.Values(f.traces), func(a, b observer.TraceInfo) int {
		c := a.StartTime.Compare(b.StartTime)
		if req.SortOrder == nil || *req.SortOrder != "asc" {
			c = -c
		}
		return cmp.Or(c, cmp.Compare(a.TraceID, b.TraceID))
	})
	if req.Limit != nil && *req.Limit < len(traces) {
		traces = traces[:*req.Limit]
	}
	return &observer.TracesQueryResponse{Traces: append([]observer.TraceInfo{}, traces...), Total: len(f.traces)}, nil
}

// QueryTraceSpans returns the trace's spans, with attributes only when asked.
func (f *fakeObserverClient) QueryTraceSpans(_ context.Context, traceID string, req observer.TracesQueryRequest) (*observer.TraceSpansQueryResponse, error) {
	f.recordCall("QueryTraceSpans", traceID, req)
	spans := make([]observer.SpanInfo, 0, len(f.spans[traceID]))
	for _, s := range f.spans[traceID] {
		if !req.IncludeAttributes {
			s.Attributes = nil
		}
		spans = append(spans, s)
	}
	return &observer.TraceSpansQueryResponse{Spans: spans, Total: len(spans)}, nil
}

func (f *fakeObserverClient) GetSpanDetails(_ context.Context, traceID, spanID string) (*observer.SpanDetailsResponse, error) {
	f.recordCall("GetSpanDetails", traceID, spanID)
	resp := &observer.SpanDetailsResponse{
		SpanID:             spanID,
		Attributes:         map[string]interface{}{},
		ResourceAttributes: map[string]interface{}{},
	}
	for _, s := range f.spans[traceID] {
		if s.SpanID == spanID {
			resp.SpanName, resp.ParentSpanID, resp.Status = s.SpanName, s.ParentSpanID, s.Status
			resp.StartTime, resp.EndTime = s.StartTime, s.EndTime
			if s.Attributes != nil {
				resp.Attributes = s.Attributes
			}
		}
	}
	return resp, nil
}

func (f *fakeObserverClient) QueryLogs(_ context.Context, req observer.LogsQueryRequest) (*observer.LogsQueryResponse, error) {
	f.recordCall("QueryLogs", req)
	return &observer.LogsQueryResponse{Logs: []observer.LogEntry{}}, nil
}

func (f *fakeObserverClient) QueryMetrics(_ context.Context, req observer.MetricsQueryRequest) (*observer.ResourceMetricsTimeSeries, error) {
	f.recordCall("QueryMetrics", req)
	return &observer.ResourceMetricsTimeSeries{}, nil
}

// Creates an MCP server with both toolsets backed by real controllers wired
// to the same fakeObserverClient, connects an in-memory client, and returns
// both for assertions.
func setupTestServer(t *testing.T) (*gomcp.ClientSession, *fakeObserverClient) {
	t.Helper()

	fake := newFakeObserverClient()
	toolsets := &Toolsets{
		Tracing:       controllers.NewTracingController(fake),
		Observability: controllers.NewObservabilityController(fake),
		// The in-memory transport carries no Authorization header, so the real
		// guard would refuse every call. Authorization has its own tests
		// (authorization_test.go, observability_guard_test.go); these specs are
		// about what the tools do with their inputs.
		authorize: allowEveryCall,
	}
	return setupTestServerWithToolsets(t, toolsets), fake
}

func setupTestServerWithToolsets(t *testing.T, toolsets *Toolsets) *gomcp.ClientSession {
	t.Helper()

	server := gomcp.NewServer(&gomcp.Implementation{
		Name:    "test-am-obs-mcp",
		Version: "0.0.1",
	}, nil)

	toolsets.Register(server)

	ctx := context.Background()
	clientTransport, serverTransport := gomcp.NewInMemoryTransports()

	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("failed to connect server: %v", err)
	}

	client := gomcp.NewClient(&gomcp.Implementation{
		Name:    "test-mcp-client",
		Version: "0.0.1",
	}, nil)

	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("failed to connect client: %v", err)
	}

	t.Cleanup(func() { _ = clientSession.Close() })
	return clientSession
}

// describes everything the registration tests need to know about a single
// MCP tool.
type toolTestSpec struct {
	name string

	// Description validation.
	descriptionKeywords []string
	descriptionMinLen   int

	// Schema validation.
	requiredParams []string
	optionalParams []string

	// A minimal valid argument set for a smoke-test call.
	testArgs map[string]any
}

// aggregates specs from every per-toolset spec file.
var allToolSpecs = func() []toolTestSpec {
	specs := make([]toolTestSpec, 0)
	specs = append(specs, observabilityToolSpecs()...)
	specs = append(specs, tracesToolSpecs()...)
	return specs
}()

// allowEveryCall is the spec tests' stand-in for requireToolPermission: it
// admits every call so a test over the in-memory transport, which has no
// Authorization header to offer, reaches the handler under test.
func allowEveryCall(rbac.Permission) func(*gomcp.CallToolRequest) error {
	return func(*gomcp.CallToolRequest) error { return nil }
}

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
	"context"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-observer/controllers"
	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

// constants for testing
const (
	testOrgName     = "default-org"
	testProjectName = "default-project"
	testAgentName   = "default-agent"
	testBuildName   = "default-build"
	testEnvName     = "default-env"
	testTraceID     = "test-trace-id"
	testSpanID      = "test-span-id"
)

// fakeObserverClient is a minimal observer.Client stub: it records every
// call it receives and returns canned, always-well-formed responses, so the
// real controllers (and, through them, the tool handlers) can be exercised
// end-to-end without a live upstream Observer.
type fakeObserverClient struct {
	calls map[string][]any
}

func newFakeObserverClient() *fakeObserverClient {
	return &fakeObserverClient{calls: make(map[string][]any)}
}

func (f *fakeObserverClient) recordCall(method string, args ...any) {
	f.calls[method] = append(f.calls[method], args)
}

func (f *fakeObserverClient) NamespaceFor(organization string) string {
	f.recordCall("NamespaceFor", organization)
	return "ns-" + organization
}

func (f *fakeObserverClient) QueryTraces(_ context.Context, req observer.TracesQueryRequest) (*observer.TracesQueryResponse, error) {
	f.recordCall("QueryTraces", req)
	return &observer.TracesQueryResponse{Traces: []observer.TraceInfo{}, Total: 0}, nil
}

func (f *fakeObserverClient) QueryTraceSpans(_ context.Context, traceID string, req observer.TracesQueryRequest) (*observer.TraceSpansQueryResponse, error) {
	f.recordCall("QueryTraceSpans", traceID, req)
	return &observer.TraceSpansQueryResponse{Spans: []observer.SpanInfo{}, Total: 0}, nil
}

func (f *fakeObserverClient) GetSpanDetails(_ context.Context, traceID, spanID string) (*observer.SpanDetailsResponse, error) {
	f.recordCall("GetSpanDetails", traceID, spanID)
	return &observer.SpanDetailsResponse{
		SpanID:             spanID,
		Attributes:         map[string]interface{}{},
		ResourceAttributes: map[string]interface{}{},
	}, nil
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

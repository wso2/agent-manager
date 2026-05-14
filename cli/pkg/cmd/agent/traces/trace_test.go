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

package traces

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/clients/traceobssvc"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func TestTrace_TextOutput(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ios.JSON = false
	client, closeFn := newTraceTestClient(t, http.StatusOK, traceobssvc.SpanListResponse{
		TotalCount: 2,
		Spans: []traceobssvc.SpanSummary{
			{SpanID: "s1", SpanName: "handle_request", DurationNs: 1200000000},
			{SpanID: "s2", ParentSpanID: "s1", SpanName: "llm_call", DurationNs: 800000000},
		},
	})
	defer closeFn()

	err := runTrace(context.Background(), &TraceOptions{
		IO: ios, TraceClient: client, Scope: traceBaseScope(),
		Org: "acme", Proj: "triage", AgentName: "my-agent", Env: "dev",
		TraceID:   "abc123",
		StartTime: "2026-05-12T00:00:00Z", EndTime: "2026-05-13T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "handle_request") {
		t.Errorf("output should contain span name, got %q", got)
	}
	if !strings.Contains(got, "llm_call") {
		t.Errorf("output should contain child span, got %q", got)
	}
}

func TestTrace_JSONOutput(t *testing.T) {
	ios, out, _ := newTraceTestIO(true)
	client, closeFn := newTraceTestClient(t, http.StatusOK, traceobssvc.SpanListResponse{
		TotalCount: 1,
		Spans: []traceobssvc.SpanSummary{
			{SpanID: "s1", SpanName: "root"},
		},
	})
	defer closeFn()

	err := runTrace(context.Background(), &TraceOptions{
		IO: ios, TraceClient: client, Scope: traceBaseScope(),
		Org: "acme", Proj: "triage", AgentName: "my-agent", Env: "dev",
		TraceID:   "abc123",
		StartTime: "2026-05-12T00:00:00Z", EndTime: "2026-05-13T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env := decodeEnvelope(t, out.String())
	if _, ok := env["data"]; !ok {
		t.Fatal("expected data key in JSON envelope")
	}
}

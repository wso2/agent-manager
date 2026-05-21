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
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestPrintSpanDetail_PrefersResourceServiceAndAmpKind(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ios.JSON = false
	span := &traceobssvc.Span{
		SpanID:  "s1",
		Name:    "openai.chat",
		Service: "40240e31-2d59-4892-a3cf-550e05d1f7df",
		Kind:    "",
		Resource: map[string]any{
			"service.name": "otel-agent",
		},
		AmpAttributes: &traceobssvc.AmpAttributes{Kind: "llm"},
	}

	if err := printSpanDetail(ios, span); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Service:        otel-agent") {
		t.Errorf("Service should read from resource.service.name, got %q", got)
	}
	if strings.Contains(got, "40240e31") {
		t.Errorf("Service should not show component UID when resource.service.name is set, got %q", got)
	}
	if !strings.Contains(got, "Kind:           llm") {
		t.Errorf("Kind should read from ampAttributes.kind, got %q", got)
	}
}

func TestRunTrace_LowercasesTraceID(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(traceobssvc.SpanListResponse{})
	}))
	defer server.Close()

	client, err := traceobssvc.NewClient(server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ios, _, _, _ := iostreams.Test()
	upperID := "F367180F0717ABCDEF1234567890ABCD"
	err = runTrace(context.Background(), &TraceOptions{
		IO: ios,
		TraceClient: func(context.Context) (*traceobssvc.Client, error) {
			return client, nil
		},
		Scope:     traceBaseScope(),
		Org:       "acme",
		Proj:      "triage",
		AgentName: "my-agent",
		Env:       "dev",
		TraceID:   upperID,
		StartTime: "2026-05-12T00:00:00Z",
		EndTime:   "2026-05-13T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotPath, strings.ToLower(upperID)) {
		t.Errorf("request path should contain lowercase trace ID, got %q", gotPath)
	}
	if strings.Contains(gotPath, upperID) {
		t.Errorf("request path should not contain upper-case trace ID, got %q", gotPath)
	}
}

func TestRunSpanDetail_LowercasesTraceAndSpanID(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(traceobssvc.Span{})
	}))
	defer server.Close()

	client, err := traceobssvc.NewClient(server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ios, _, _, _ := iostreams.Test()
	upperTrace := "F367180F0717ABCDEF1234567890ABCD"
	upperSpan := "ABCDEF1234567890"
	err = runSpanDetail(context.Background(), &TraceOptions{
		IO: ios,
		TraceClient: func(context.Context) (*traceobssvc.Client, error) {
			return client, nil
		},
		Scope:     traceBaseScope(),
		Org:       "acme",
		Proj:      "triage",
		AgentName: "my-agent",
		Env:       "dev",
		TraceID:   upperTrace,
		SpanID:    upperSpan,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotPath, strings.ToLower(upperTrace)) {
		t.Errorf("request path should contain lowercase trace ID, got %q", gotPath)
	}
	if !strings.Contains(gotPath, strings.ToLower(upperSpan)) {
		t.Errorf("request path should contain lowercase span ID, got %q", gotPath)
	}
}

func TestPrintSpanDetail_FallbackWhenNoResourceOrAmpKind(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ios.JSON = false
	span := &traceobssvc.Span{
		SpanID:  "s1",
		Name:    "openai.chat",
		Service: "otel-agent",
		Kind:    "SPAN_KIND_INTERNAL",
	}
	if err := printSpanDetail(ios, span); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Service:        otel-agent") {
		t.Errorf("should fall back to span.Service, got %q", got)
	}
	if !strings.Contains(got, "Kind:           SPAN_KIND_INTERNAL") {
		t.Errorf("should fall back to span.Kind, got %q", got)
	}
}

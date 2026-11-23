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

package observability

import (
	"strings"
	"time"
)

// The response bodies match the Agent Manager Observer's trace endpoints, so
// the console reads either.

// ErrorResponse is the error body of every route.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// TraceOverviewResponse is the body of the trace list.
type TraceOverviewResponse struct {
	Traces       []TraceOverview `json:"traces"`
	TotalCount   int             `json:"totalCount"`
	LookedBackTo string          `json:"lookedBackTo"`
	Truncated    bool            `json:"truncated"`
	NextCursor   string          `json:"nextCursor,omitempty"`
}

// TraceOverview is one trace in the list.
type TraceOverview struct {
	TraceID         string       `json:"traceId"`
	RootSpanID      string       `json:"rootSpanId"`
	RootSpanName    string       `json:"rootSpanName"`
	RootSpanKind    string       `json:"rootSpanKind"`
	StartTime       string       `json:"startTime"`
	EndTime         string       `json:"endTime"`
	DurationInNanos int64        `json:"durationInNanos"`
	SpanCount       int          `json:"spanCount"`
	TokenUsage      *TokenUsage  `json:"tokenUsage,omitempty"`
	Status          *TraceStatus `json:"status,omitempty"`
	Input           any          `json:"input,omitempty"`
	Output          any          `json:"output,omitempty"`
}

// TraceExportResponse is the body of the trace export.
type TraceExportResponse struct {
	Traces         []FullTrace `json:"traces"`
	TotalCount     int         `json:"totalCount"`
	Truncated      bool        `json:"truncated"`
	SpansTruncated bool        `json:"spansTruncated"`
	FailedTraceIDs []string    `json:"failedTraceIds,omitempty"`
}

// FullTrace is one exported trace with its spans.
type FullTrace struct {
	TraceOverview
	Spans []Span `json:"spans"`
}

// TokenUsage is a trace's token usage.
type TokenUsage struct {
	InputTokens  int  `json:"inputTokens"`
	OutputTokens int  `json:"outputTokens"`
	TotalTokens  int  `json:"totalTokens"`
	Partial      bool `json:"partial,omitempty"`
}

// TraceStatus is a trace's error count.
type TraceStatus struct {
	ErrorCount int `json:"errorCount"`
}

// SpanListResponse is the body of a trace's span list.
type SpanListResponse struct {
	Spans      []SpanSummary `json:"spans"`
	TotalCount int           `json:"totalCount"`
}

// SpanSummary is one span in the span list, without attributes.
type SpanSummary struct {
	SpanID        string    `json:"spanId"`
	SpanName      string    `json:"spanName"`
	SpanKind      string    `json:"spanKind,omitempty"`
	ParentSpanID  string    `json:"parentSpanId,omitempty"`
	StartTime     time.Time `json:"startTime"`
	EndTime       time.Time `json:"endTime"`
	DurationNs    int64     `json:"durationNs"`
	Error         bool      `json:"error,omitempty"`
	StatusMessage string    `json:"statusMessage,omitempty"`
}

// Span is one span with its attributes.
type Span struct {
	TraceID         string         `json:"traceId"`
	SpanID          string         `json:"spanId"`
	ParentSpanID    string         `json:"parentSpanId,omitempty"`
	Name            string         `json:"name"`
	Service         string         `json:"service"`
	StartTime       time.Time      `json:"startTime"`
	EndTime         time.Time      `json:"endTime"`
	DurationInNanos int64          `json:"durationInNanos"`
	Kind            string         `json:"kind,omitempty"`
	Status          string         `json:"status,omitempty"`
	StatusMessage   string         `json:"statusMessage,omitempty"`
	Attributes      map[string]any `json:"attributes,omitempty"`
	Resource        map[string]any `json:"resource,omitempty"`
	AmpAttributes   *AmpAttributes `json:"ampAttributes,omitempty"`
}

// AmpAttributes are the attributes Agent Manager derives for a span.
type AmpAttributes struct {
	Kind   string      `json:"kind"`
	Input  any         `json:"input,omitempty"`
	Output any         `json:"output,omitempty"`
	Status *SpanStatus `json:"status,omitempty"`
	Data   any         `json:"data,omitempty"`
}

// SpanStatus is a span's error state.
type SpanStatus struct {
	Error     bool   `json:"error"`
	ErrorType string `json:"errorType,omitempty"`
	Message   string `json:"message,omitempty"`
}

// Semantic span kinds.
const (
	kindLLM       = "llm"
	kindEmbedding = "embedding"
	kindTool      = "tool"
	kindRetriever = "retriever"
	kindRerank    = "rerank"
	kindAgent     = "agent"
	kindChain     = "chain"
	kindUnknown   = "unknown"
)

var openClawSpanKinds = map[string]string{
	"openclaw.harness.run":       kindAgent,
	"openclaw.run":               kindAgent,
	"openclaw.model.usage":       kindLLM,
	"openclaw.model.call":        kindLLM,
	"openclaw.tool.execution":    kindTool,
	"openclaw.context.assembled": kindChain,
}

// spanKind classifies a span by its name, as the observer does.
func spanKind(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return kindUnknown
	}
	if kind, ok := openClawSpanKinds[lower]; ok {
		return kind
	}
	switch {
	case strings.HasPrefix(lower, "invoke_agent"), strings.HasPrefix(lower, "execute_agent"),
		strings.HasPrefix(lower, "create_agent"):
		return kindAgent
	case strings.HasPrefix(lower, "execute_tool"):
		return kindTool
	case strings.HasPrefix(lower, "execute_task"), strings.HasPrefix(lower, "execute_event_loop_cycle"):
		return kindChain
	}
	if i := strings.IndexByte(lower, ' '); i > 0 {
		switch lower[:i] {
		case "chat", "text_completion", "completion", "generate_content":
			return kindLLM
		case "embeddings", "embedding":
			return kindEmbedding
		}
	}
	parts := strings.Split(lower, ".")
	switch parts[len(parts)-1] {
	case "llm", "chat", "chats", "completion", "completions":
		return kindLLM
	case "embedding", "embeddings", "embed":
		return kindEmbedding
	case "tool", "tools", "function", "functions":
		return kindTool
	case "retriever", "retrievers", "retrieve", "retrieval":
		return kindRetriever
	case "rerank", "reranker", "rerankers":
		return kindRerank
	case "agent", "agents":
		return kindAgent
	case "task", "tasks", "workflow", "workflows":
		return kindChain
	}
	return kindUnknown
}

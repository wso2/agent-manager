// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package traceobserversvc

import (
	"fmt"
	"time"
)

// HTTPError represents an HTTP error response from the trace observer service
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("trace observer returned status %d: %s", e.StatusCode, e.Message)
}

// ListTracesParams holds parameters for listing trace overviews
type ListTracesParams struct {
	ServiceName    string
	ComponentUid   string
	EnvironmentUid string
	StartTime      string
	EndTime        string
	Limit          int
	Offset         int
	SortOrder      string
}

// TraceDetailsByIdParams holds parameters for getting trace details by ID
type TraceDetailsByIdParams struct {
	TraceID        string
	ServiceName    string
	ComponentUid   string
	EnvironmentUid string
}

// TraceOverview represents a single trace overview with root span info
type TraceOverview struct {
	TraceID         string       `json:"traceId"`
	RootSpanID      string       `json:"rootSpanId"`
	RootSpanName    string       `json:"rootSpanName"`
	RootSpanKind    string       `json:"rootSpanKind"` // Semantic kind of the root span (llm, tool, embedding, etc.)
	StartTime       string       `json:"startTime"`
	EndTime         string       `json:"endTime"`
	DurationInNanos int64        `json:"durationInNanos"`
	SpanCount       int          `json:"spanCount"`
	TokenUsage      *TokenUsage  `json:"tokenUsage,omitempty"` // Aggregated token usage from GenAI spans
	Status          *TraceStatus `json:"status,omitempty"`     // Trace status including error information
	Input           interface{}  `json:"input,omitempty"`      // Input from root span (nil if not found)
	Output          interface{}  `json:"output,omitempty"`     // Output from root span (nil if not found)
}

// TokenUsage represents aggregated token usage from GenAI spans
type TokenUsage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	TotalTokens  int `json:"totalTokens"`
}

// TraceStatus represents the status of a trace
type TraceStatus struct {
	ErrorCount int `json:"errorCount"` // Number of spans with errors (0 means no errors)
}

// TraceOverviewResponse represents the response for trace overview queries
type TraceOverviewResponse struct {
	Traces     []TraceOverview `json:"traces"`
	TotalCount int             `json:"totalCount"`
}

// Span represents a single trace span
type Span struct {
	TraceID         string                 `json:"traceId"`
	SpanID          string                 `json:"spanId"`
	ParentSpanID    string                 `json:"parentSpanId,omitempty"`
	Name            string                 `json:"name"`
	Service         string                 `json:"service"`
	StartTime       time.Time              `json:"startTime"`
	EndTime         time.Time              `json:"endTime,omitempty"`
	DurationInNanos int64                  `json:"durationInNanos"`
	Kind            string                 `json:"kind,omitempty"`
	Status          string                 `json:"status,omitempty"`
	Attributes      map[string]interface{} `json:"attributes,omitempty"`
	Resource        map[string]interface{} `json:"resource,omitempty"`
	AmpAttributes   *AmpAttributes         `json:"ampAttributes,omitempty"` // AMP-specific enriched attributes
}

// AmpAttributes contains AMP-specific enriched attributes
// The Data field contains kind-specific information defined by traces-observer-service.
// This service passes it through without unpacking to avoid tight coupling.
type AmpAttributes struct {
	Kind   string      `json:"kind"`             // Semantic span type (llm, tool, embedding, etc.)
	Input  interface{} `json:"input,omitempty"`  // Input: []PromptMessage for LLM spans, string for tool spans, etc.
	Output interface{} `json:"output,omitempty"` // Output: []PromptMessage for LLM spans, string for tool spans, etc.
	Status *SpanStatus `json:"status,omitempty"` // Execution status with error information
	Data   interface{} `json:"data,omitempty"`   // Kind-specific data from traces-observer-service
}

// SpanStatus represents the execution status of a span
type SpanStatus struct {
	Error     bool   `json:"error"`               // Whether the span has an error
	ErrorType string `json:"errorType,omitempty"` // Error type from error.type attribute (only if error is true)
}

// LLMTokenUsage represents token usage for a single LLM span
type LLMTokenUsage struct {
	InputTokens          int `json:"inputTokens"`
	OutputTokens         int `json:"outputTokens"`
	CacheReadInputTokens int `json:"cacheReadInputTokens,omitempty"`
	TotalTokens          int `json:"totalTokens"`
}

// PromptMessage represents a single message in a conversation
type PromptMessage struct {
	Role      string     `json:"role"`                // system, user, assistant, tool
	Content   string     `json:"content,omitempty"`   // The message content (text)
	ToolCalls []ToolCall `json:"toolCalls,omitempty"` // Tool calls made by assistant (for assistant role with tool calls)
}

// ToolCall represents a tool/function call made by the assistant
type ToolCall struct {
	ID        string `json:"id"`        // Tool call ID
	Name      string `json:"name"`      // Function/tool name
	Arguments string `json:"arguments"` // JSON arguments for the tool
}

// ToolDefinition represents a tool/function available to the LLM
type ToolDefinition struct {
	Name        string `json:"name"`                  // Function name
	Description string `json:"description,omitempty"` // Function description
	Parameters  string `json:"parameters,omitempty"`  // JSON schema of parameters
}

// TraceResponse represents the response for trace queries
type TraceResponse struct {
	Spans      []Span       `json:"spans"`
	TotalCount int          `json:"totalCount"`
	TokenUsage *TokenUsage  `json:"tokenUsage,omitempty"` // Aggregated token usage from GenAI spans
	Status     *TraceStatus `json:"status,omitempty"`     // Trace status including error information
}

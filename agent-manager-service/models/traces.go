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

package models

import (
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/traceobserversvc"
)

// TraceOverview represents a summary of a trace
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

// TraceOverviewResponse represents the response for listing traces
type TraceOverviewResponse struct {
	Traces     []TraceOverview `json:"traces"`
	TotalCount int             `json:"totalCount"`
}

// Span represents a single span in a trace
type Span struct {
	TraceID         string                 `json:"traceId"`
	SpanID          string                 `json:"spanId"`
	ParentSpanID    string                 `json:"parentSpanId,omitempty"`
	Name            string                 `json:"name"`
	Service         string                 `json:"service"`
	Kind            string                 `json:"kind,omitempty"`
	StartTime       time.Time              `json:"startTime"`
	EndTime         time.Time              `json:"endTime,omitempty"`
	DurationInNanos int64                  `json:"durationInNanos"`
	Status          string                 `json:"status,omitempty"`
	Attributes      map[string]interface{} `json:"attributes,omitempty"`
	Resource        map[string]interface{} `json:"resource,omitempty"`
	AmpAttributes   *AmpAttributes         `json:"ampAttributes,omitempty"` // AMP-specific enriched attributes
}

// AmpAttributes contains AMP-specific enriched attributes
// The Data field is passed through as-is from traces-observer-service
// to avoid duplicating type definitions and tight coupling.
// The frontend (console) handles type-specific rendering based on the Kind field.
type AmpAttributes struct {
	Kind   string                       `json:"kind"`             // Semantic span type (llm, tool, embedding, retriever, etc.)
	Input  interface{}                  `json:"input,omitempty"`  // Input data (type varies by kind)
	Output interface{}                  `json:"output,omitempty"` // Output data (type varies by kind)
	Status *traceobserversvc.SpanStatus `json:"status,omitempty"` // Execution status with error information
	Data   interface{}                  `json:"data,omitempty"`   // Kind-specific data passed through from traces-observer-service
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

// SpanEvent represents an event within a span (for future use)
type SpanEvent struct {
	Name       string            `json:"name"`
	Timestamp  time.Time         `json:"timestamp"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// SpanStatus represents the status of a span (for future use)
type SpanStatus struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

// TraceResponse represents the response for trace details
type TraceResponse struct {
	Spans      []Span       `json:"spans"`
	TotalCount int          `json:"totalCount"`
	TokenUsage *TokenUsage  `json:"tokenUsage,omitempty"` // Aggregated token usage from GenAI spans
	Status     *TraceStatus `json:"status,omitempty"`     // Trace status including error information
}

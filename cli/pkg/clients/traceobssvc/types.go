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

package traceobssvc

import (
	"fmt"
	"time"
)

// ErrorResponse mirrors the {"error","message"} body returned by the
// traces-observer handlers on non-2xx responses.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// HTTPError is returned by all client methods for non-2xx responses.
// Body is nil if the response was not parseable JSON.
type HTTPError struct {
	StatusCode int
	Body       *ErrorResponse
	RawBody    []byte
}

func (e *HTTPError) Error() string {
	if e.Body != nil && e.Body.Message != "" {
		return fmt.Sprintf("traces-observer: %d %s: %s", e.StatusCode, e.Body.Error, e.Body.Message)
	}
	return fmt.Sprintf("traces-observer: %d", e.StatusCode)
}

// TokenUsage mirrors opensearch.TokenUsage.
type TokenUsage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	TotalTokens  int `json:"totalTokens"`
}

// TraceStatus mirrors opensearch.TraceStatus.
type TraceStatus struct {
	ErrorCount int `json:"errorCount"`
}

// SpanStatus mirrors opensearch.SpanStatus.
type SpanStatus struct {
	Error     bool   `json:"error"`
	ErrorType string `json:"errorType,omitempty"`
}

// AmpAttributes mirrors opensearch.AmpAttributes.
type AmpAttributes struct {
	Kind   string      `json:"kind"`
	Input  any         `json:"input,omitempty"`
	Output any         `json:"output,omitempty"`
	Data   any         `json:"data,omitempty"`
	Status *SpanStatus `json:"status,omitempty"`
}

// TraceOverview mirrors opensearch.TraceOverview.
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

// TraceOverviewResponse mirrors opensearch.TraceOverviewResponse.
type TraceOverviewResponse struct {
	Traces     []TraceOverview `json:"traces"`
	TotalCount int             `json:"totalCount"`
}

// Span mirrors opensearch.Span.
type Span struct {
	TraceID         string         `json:"traceId"`
	SpanID          string         `json:"spanId"`
	ParentSpanID    string         `json:"parentSpanId,omitempty"`
	Name            string         `json:"name"`
	Service         string         `json:"service"`
	StartTime       time.Time      `json:"startTime"`
	EndTime         time.Time      `json:"endTime"`
	DurationInNanos int64          `json:"durationInNanos"`
	Kind            string         `json:"kind"`
	Status          string         `json:"status"`
	Attributes      map[string]any `json:"attributes,omitempty"`
	Resource        map[string]any `json:"resource,omitempty"`
	AmpAttributes   *AmpAttributes `json:"ampAttributes,omitempty"`
}

// FullTrace mirrors opensearch.FullTrace: TraceOverview + task/trial ids + spans.
type FullTrace struct {
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
	TaskId          string       `json:"taskId,omitempty"`
	TrialId         string       `json:"trialId,omitempty"`
	Spans           []Span       `json:"spans"`
}

// TraceExportResponse mirrors opensearch.TraceExportResponse.
type TraceExportResponse struct {
	Traces     []FullTrace `json:"traces"`
	TotalCount int         `json:"totalCount"`
	Truncated  bool        `json:"truncated"`
}

// SpanSummary mirrors controllers.SpanSummary.
type SpanSummary struct {
	SpanID       string    `json:"spanId"`
	SpanName     string    `json:"spanName"`
	SpanKind     string    `json:"spanKind,omitempty"`
	ParentSpanID string    `json:"parentSpanId,omitempty"`
	StartTime    time.Time `json:"startTime"`
	EndTime      time.Time `json:"endTime"`
	DurationNs   int64     `json:"durationNs"`
}

// SpanListResponse mirrors controllers.SpanListResponse.
type SpanListResponse struct {
	Spans      []SpanSummary `json:"spans"`
	TotalCount int           `json:"totalCount"`
}

// ListTracesParams collects query params for GET /api/v1/traces.
type ListTracesParams struct {
	Organization string
	Project      string
	Agent        string
	Environment  string
	StartTime    time.Time
	EndTime      time.Time
	Limit        *int
	SortOrder    *string
}

// ExportTracesParams is identical to ListTracesParams in shape.
type ExportTracesParams = ListTracesParams

// GetTraceSpansParams collects query params for GET /api/v1/traces/{traceId}/spans.
// Only Organization, StartTime and EndTime are required on the server side.
type GetTraceSpansParams struct {
	Organization string
	Project      *string
	Agent        *string
	Environment  *string
	StartTime    time.Time
	EndTime      time.Time
	Limit        *int
	SortOrder    *string
}

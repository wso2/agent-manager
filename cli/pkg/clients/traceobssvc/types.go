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

// Package traceobssvc is a handwritten client for the traces-observer-service.
// Types mirror the opensearch/controllers shapes used by the upstream service.
package traceobssvc

import (
	"fmt"
	"time"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

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

type TokenUsage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	TotalTokens  int `json:"totalTokens"`
}

type TraceStatus struct {
	ErrorCount int `json:"errorCount"`
}

type SpanStatus struct {
	Error     bool   `json:"error"`
	ErrorType string `json:"errorType,omitempty"`
}

type AmpAttributes struct {
	Kind   string      `json:"kind"`
	Input  any         `json:"input,omitempty"`
	Output any         `json:"output,omitempty"`
	Data   any         `json:"data,omitempty"`
	Status *SpanStatus `json:"status,omitempty"`
}

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

type TraceOverviewResponse struct {
	Traces     []TraceOverview `json:"traces"`
	TotalCount int             `json:"totalCount"`
}

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

type FullTrace struct {
	TraceOverview
	TaskId  string `json:"taskId,omitempty"`
	TrialId string `json:"trialId,omitempty"`
	Spans   []Span `json:"spans"`
}

type TraceExportResponse struct {
	Traces     []FullTrace `json:"traces"`
	TotalCount int         `json:"totalCount"`
	Truncated  bool        `json:"truncated"`
}

type SpanSummary struct {
	SpanID       string    `json:"spanId"`
	SpanName     string    `json:"spanName"`
	SpanKind     string    `json:"spanKind,omitempty"`
	ParentSpanID string    `json:"parentSpanId,omitempty"`
	StartTime    time.Time `json:"startTime"`
	EndTime      time.Time `json:"endTime"`
	DurationNs   int64     `json:"durationNs"`
}

type SpanListResponse struct {
	Spans      []SpanSummary `json:"spans"`
	TotalCount int           `json:"totalCount"`
}

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

type ExportTracesParams = ListTracesParams

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

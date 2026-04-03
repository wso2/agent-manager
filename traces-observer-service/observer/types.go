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

package observer

import "time"

// ComponentSearchScope identifies a component in the observer service.
// Namespace is required; Project, Component, and Environment are optional filters.
type ComponentSearchScope struct {
	Namespace   string  `json:"namespace"`
	Project     *string `json:"project,omitempty"`
	Component   *string `json:"component,omitempty"`
	Environment *string `json:"environment,omitempty"`
}

// TracesQueryRequest is the POST body for both
// POST /api/v1alpha1/traces/query and
// POST /api/v1alpha1/traces/{traceId}/spans/query.
type TracesQueryRequest struct {
	StartTime   time.Time            `json:"startTime"`
	EndTime     time.Time            `json:"endTime"`
	Limit       *int                 `json:"limit,omitempty"`
	SortOrder   *string              `json:"sortOrder,omitempty"`
	SearchScope ComponentSearchScope `json:"searchScope"`
}

// TraceInfo represents a single trace entry in TracesQueryResponse.
type TraceInfo struct {
	TraceID      string    `json:"traceId"`
	TraceName    string    `json:"traceName"`
	SpanCount    int       `json:"spanCount"`
	RootSpanID   string    `json:"rootSpanId"`
	RootSpanName string    `json:"rootSpanName"`
	RootSpanKind string    `json:"rootSpanKind"`
	StartTime    time.Time `json:"startTime"`
	EndTime      time.Time `json:"endTime"`
	DurationNs   int64     `json:"durationNs"`
}

// TracesQueryResponse is the response from POST /api/v1alpha1/traces/query.
type TracesQueryResponse struct {
	Traces []TraceInfo `json:"traces"`
	Total  int         `json:"total"`
	TookMs int         `json:"tookMs"`
}

// SpanInfo represents a single span summary in TraceSpansQueryResponse.
// Attributes are not included; use GetSpanDetails for the full span.
type SpanInfo struct {
	SpanID       string    `json:"spanId"`
	SpanName     string    `json:"spanName"`
	ParentSpanID string    `json:"parentSpanId"`
	StartTime    time.Time `json:"startTime"`
	EndTime      time.Time `json:"endTime"`
	DurationNs   int64     `json:"durationNs"`
}

// TraceSpansQueryResponse is the response from
// POST /api/v1alpha1/traces/{traceId}/spans/query.
type TraceSpansQueryResponse struct {
	Spans  []SpanInfo `json:"spans"`
	Total  int        `json:"total"`
	TookMs int        `json:"tookMs"`
}

// SpanDetailsResponse is the response from
// GET /api/v1alpha1/traces/{traceId}/spans/{spanId}.
//
// Attributes is already a map[string]interface{} (JSON object), with values
// that may be strings, float64, bool, or nested objects. This matches the
// map[string]interface{} format expected by opensearch/process.go directly.
//
// ResourceAttributes is returned separately and contains OpenChoreo-specific
// metadata such as openchoreo.dev/component-uid.
type SpanDetailsResponse struct {
	SpanID             string                 `json:"spanId"`
	SpanName           string                 `json:"spanName"`
	ParentSpanID       string                 `json:"parentSpanId"`
	StartTime          time.Time              `json:"startTime"`
	EndTime            time.Time              `json:"endTime"`
	DurationNs         int64                  `json:"durationNs"`
	Attributes         map[string]interface{} `json:"attributes"`
	ResourceAttributes map[string]interface{} `json:"resourceAttributes"`
}

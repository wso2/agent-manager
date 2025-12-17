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

import "time"

// TraceOverview represents a summary of a trace
type TraceOverview struct {
	TraceID         string `json:"traceId"`
	RootSpanID      string `json:"rootSpanId"`
	RootSpanName    string `json:"rootSpanName"`
	StartTime       string `json:"startTime"`
	EndTime         string `json:"endTime"`
	DurationInNanos int64  `json:"durationInNanos"`
	SpanCount       int    `json:"spanCount"`
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
	Spans      []Span `json:"spans"`
	TotalCount int    `json:"totalCount"`
}

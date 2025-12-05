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

package opensearch

import "time"

// TraceQueryParams holds parameters for trace queries
type TraceQueryParams struct {
	ServiceName string
	StartTime   string
	EndTime     string
	Limit       int
	Offset      int
	SortOrder   string
}

// TraceByIdAndServiceParams holds parameters for querying by both traceId and serviceName
type TraceByIdAndServiceParams struct {
	TraceID     string
	ServiceName string
	SortOrder   string
	Limit       int
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
	DurationInNanos int64                  `json:"durationInNanos"` // in nanoseconds
	Kind            string                 `json:"kind,omitempty"`
	Status          string                 `json:"status,omitempty"`
	Attributes      map[string]interface{} `json:"attributes,omitempty"`
	Resource        map[string]interface{} `json:"resource,omitempty"`
}

// TraceResponse represents the response for trace queries
type TraceResponse struct {
	Spans      []Span `json:"spans"`
	TotalCount int    `json:"totalCount"`
}

// TraceDetailResponse represents detailed information for a single trace
type TraceDetailResponse struct {
	TraceID    string   `json:"traceId"`
	Spans      []Span   `json:"spans"`
	TotalSpans int      `json:"totalSpans"`
	Duration   int64    `json:"duration"` // Total trace duration in microseconds
	Services   []string `json:"services"` // List of services involved
}

// TraceOverview represents a single trace overview with root span info
type TraceOverview struct {
	TraceID         string `json:"traceId"`
	RootSpanID      string `json:"rootSpanId"`
	RootSpanName    string `json:"rootSpanName"`
	StartTime       string `json:"startTime"`
	EndTime         string `json:"endTime"`
	DurationInNanos int64  `json:"durationInNanos"` // Total trace duration in nanoseconds
	SpanCount       int    `json:"spanCount"`
}

// TraceOverviewResponse represents the response for trace overview queries
type TraceOverviewResponse struct {
	Traces     []TraceOverview `json:"traces"`
	TotalCount int             `json:"totalCount"`
}

// SearchResponse represents OpenSearch search response
type SearchResponse struct {
	Hits struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		Hits []struct {
			Source map[string]interface{} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

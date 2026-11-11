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

import (
	"bytes"
	"encoding/json"
	"time"
)

// TraceScope scopes a trace query to an organization and, optionally, a
// project and agent, matched by name against the span resource attributes.
type TraceScope struct {
	Organization string
	Project      *string
	Agent        *string
}

// TracesQueryRequest is a trace list or trace spans query.
type TracesQueryRequest struct {
	StartTime   time.Time
	EndTime     time.Time
	Limit       *int
	SortOrder   *string
	SearchScope TraceScope
	// IncludeAttributes returns span attributes with a trace spans query.
	IncludeAttributes bool
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

// TracesQueryResponse is a page of traces; Total counts every trace in the window.
type TracesQueryResponse struct {
	Traces []TraceInfo `json:"traces"`
	Total  int         `json:"total"`
	TookMs int         `json:"tookMs"`
}

type SpanStatus struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// UnmarshalJSON accepts a span status as either a bare code string or an
// object with code and message.
func (s *SpanStatus) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	// Legacy string form: the value is the status code.
	if trimmed[0] == '"' {
		var code string
		if err := json.Unmarshal(trimmed, &code); err != nil {
			return err
		}
		s.Code = code
		s.Message = ""
		return nil
	}
	// Object form. Alias avoids recursing back into this method.
	type spanStatusAlias SpanStatus
	var alias spanStatusAlias
	if err := json.Unmarshal(trimmed, &alias); err != nil {
		return err
	}
	*s = SpanStatus(alias)
	return nil
}

// SpanInfo represents a single span in TraceSpansQueryResponse.
// Attributes and ResourceAttributes are populated only when the request set
// IncludeAttributes.
type SpanInfo struct {
	SpanID             string                 `json:"spanId"`
	SpanName           string                 `json:"spanName"`
	ParentSpanID       string                 `json:"parentSpanId"`
	StartTime          time.Time              `json:"startTime"`
	EndTime            time.Time              `json:"endTime"`
	DurationNs         int64                  `json:"durationNs"`
	Kind               string                 `json:"spanKind,omitempty"`
	Status             *SpanStatus            `json:"status,omitempty"`
	Attributes         map[string]interface{} `json:"attributes,omitempty"`
	ResourceAttributes map[string]interface{} `json:"resourceAttributes,omitempty"`
}

// TraceSpansQueryResponse is the spans of one trace.
type TraceSpansQueryResponse struct {
	Spans  []SpanInfo `json:"spans"`
	Total  int        `json:"total"`
	TookMs int        `json:"tookMs"`
}

// SpanDetailsResponse is a single span with its attributes.
type SpanDetailsResponse struct {
	SpanID             string                 `json:"spanId"`
	SpanName           string                 `json:"spanName"`
	ParentSpanID       string                 `json:"parentSpanId"`
	StartTime          time.Time              `json:"startTime"`
	EndTime            time.Time              `json:"endTime"`
	DurationNs         int64                  `json:"durationNs"`
	Kind               string                 `json:"kind,omitempty"`
	Status             *SpanStatus            `json:"status,omitempty"`
	Attributes         map[string]interface{} `json:"attributes"`
	ResourceAttributes map[string]interface{} `json:"resourceAttributes"`
}

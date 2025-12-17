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

import (
	"fmt"
	"time"
)

// ParseSpans converts OpenSearch response to Span structs
func ParseSpans(response *SearchResponse) []Span {
	spans := make([]Span, 0, len(response.Hits.Hits))

	for _, hit := range response.Hits.Hits {
		span := parseSpan(hit.Source)
		spans = append(spans, span)
	}

	return spans
}

// parseSpan extracts span information from a source document
func parseSpan(source map[string]interface{}) Span {
	span := Span{}

	// Try standard OTEL fields first
	if traceID, ok := source["traceId"].(string); ok {
		span.TraceID = traceID
	}
	if spanID, ok := source["spanId"].(string); ok {
		span.SpanID = spanID
	}
	if parentSpanID, ok := source["parentSpanId"].(string); ok {
		span.ParentSpanID = parentSpanID
	}
	if name, ok := source["name"].(string); ok {
		span.Name = name
	}
	if kind, ok := source["kind"].(string); ok {
		span.Kind = kind
	}

	// Extract component UID from resource
	if resource, ok := source["resource"].(map[string]interface{}); ok {
		if componentUid, ok := resource["openchoreo.dev/component-uid"].(string); ok {
			span.Service = componentUid
		}

		// Store the complete resource object
		span.Resource = resource
	}

	// Parse timestamps
	if startTime, ok := source["startTime"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, startTime); err == nil {
			span.StartTime = t
		}
	}
	if endTime, ok := source["endTime"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, endTime); err == nil {
			span.EndTime = t
		}
	}

	// Parse duration - try durationInNanos field first
	if duration, ok := source["durationInNanos"].(float64); ok {
		span.DurationInNanos = int64(duration)
	} else if !span.StartTime.IsZero() && !span.EndTime.IsZero() {
		// Fallback: calculate duration from timestamps if durationInNanos not present
		span.DurationInNanos = span.EndTime.Sub(span.StartTime).Nanoseconds()
	}

	// Parse status
	if status, ok := source["status"].(map[string]interface{}); ok {
		if code, ok := status["code"].(string); ok {
			span.Status = code
		} else if code, ok := status["code"].(float64); ok {
			span.Status = fmt.Sprintf("%d", int(code))
		}
	}

	// Parse attributes
	if attributes, ok := source["attributes"].(map[string]interface{}); ok {
		span.Attributes = attributes
	}

	return span
}

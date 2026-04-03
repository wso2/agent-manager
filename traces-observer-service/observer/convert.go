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
	"strconv"

	"github.com/wso2/ai-agent-management-platform/traces-observer-service/opensearch"
)

// ConvertAttributeKVToMap converts a slice of AttributeKV into the
// map[string]interface{} format expected by opensearch.ProcessSpan and
// all existing attribute extraction functions in opensearch/process.go.
//
// Type inference is applied in order: float64 → bool → string.
// This ensures existing type assertions such as .(float64) and .(bool)
// in process.go continue to work correctly on observer-sourced attributes.
func ConvertAttributeKVToMap(attrs []AttributeKV) map[string]interface{} {
	if len(attrs) == 0 {
		return nil
	}
	result := make(map[string]interface{}, len(attrs))
	for _, kv := range attrs {
		if f, err := strconv.ParseFloat(kv.Value, 64); err == nil {
			result[kv.Key] = f
		} else if b, err := strconv.ParseBool(kv.Value); err == nil {
			result[kv.Key] = b
		} else {
			result[kv.Key] = kv.Value
		}
	}
	return result
}

// ConvertSpanDetailsToSpan builds an opensearch.Span from observer service
// span detail data, ready to be passed to opensearch.ProcessSpan.
//
// componentUid populates span.Service, which is normally extracted from
// OpenSearch resource attributes (openchoreo.dev/component-uid). When the
// value is empty, span.Service is left blank.
func ConvertSpanDetailsToSpan(traceID, componentUid string, d *SpanDetailsResponse) opensearch.Span {
	return opensearch.Span{
		TraceID:         traceID,
		SpanID:          d.SpanID,
		ParentSpanID:    d.ParentSpanID,
		Name:            d.SpanName,
		Service:         componentUid,
		StartTime:       d.StartTime,
		EndTime:         d.EndTime,
		DurationInNanos: d.DurationNs,
		Attributes:      ConvertAttributeKVToMap(d.Attributes),
	}
}

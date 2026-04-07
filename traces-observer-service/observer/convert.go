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
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/opensearch"
)

// componentUIDResourceKey is the resource attribute key used by OpenChoreo to
// identify the component. It matches what opensearch/process.go reads when
// building span.Service from resource attributes.
const componentUIDResourceKey = "openchoreo.dev/component-uid"

// ConvertSpanDetailsToSpan builds an opensearch.Span from a SpanDetailsResponse.
// The result is ready to be passed directly to opensearch.ProcessSpan.
//
// Attributes: the observer service returns attributes as a map[string]interface{}
// (JSON object) whose values are already native Go types (string, float64, bool,
// nested maps). This matches the format expected by process.go without any
// additional conversion.
//
// Service: extracted from resourceAttributes["openchoreo.dev/component-uid"].
// Resource: set to resourceAttributes so that any existing resource-based lookups
// in process.go continue to work.
func ConvertSpanDetailsToSpan(traceID string, d *SpanDetailsResponse) opensearch.Span {
	service := ""
	if uid, ok := d.ResourceAttributes[componentUIDResourceKey].(string); ok {
		service = uid
	}

	return opensearch.Span{
		TraceID:         traceID,
		SpanID:          d.SpanID,
		ParentSpanID:    d.ParentSpanID,
		Name:            d.SpanName,
		Service:         service,
		StartTime:       d.StartTime,
		EndTime:         d.EndTime,
		DurationInNanos: d.DurationNs,
		Status:          d.Status,
		Attributes:      d.Attributes,
		Resource:        d.ResourceAttributes,
	}
}

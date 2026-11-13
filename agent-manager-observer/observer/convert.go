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
	"github.com/wso2/agent-manager/agent-manager-observer/opensearch"
)

func statusCodeAndMessage(s *SpanStatus) (code, message string) {
	if s == nil {
		return "", ""
	}
	return s.Code, s.Message
}

// ConvertSpanDetailsToSpan builds an opensearch.Span from a SpanDetailsResponse,
// ready to be passed to opensearch.ProcessSpan.
func ConvertSpanDetailsToSpan(traceID string, d *SpanDetailsResponse) opensearch.Span {
	service := opensearch.ServiceFromResource(d.ResourceAttributes)
	code, message := statusCodeAndMessage(d.Status)
	return opensearch.Span{
		TraceID:         traceID,
		SpanID:          d.SpanID,
		ParentSpanID:    d.ParentSpanID,
		Name:            d.SpanName,
		Service:         service,
		StartTime:       d.StartTime,
		EndTime:         d.EndTime,
		DurationInNanos: d.DurationNs,
		Kind:            d.Kind,
		Status:          code,
		StatusMessage:   message,
		Attributes:      d.Attributes,
		Resource:        d.ResourceAttributes,
	}
}

// ConvertSpanInfoToSpan builds an opensearch.Span from a SpanInfo queried with
// IncludeAttributes, so the export path needs no GetSpanDetails round-trip.
func ConvertSpanInfoToSpan(traceID string, s SpanInfo) opensearch.Span {
	service := opensearch.ServiceFromResource(s.ResourceAttributes)
	code, message := statusCodeAndMessage(s.Status)
	return opensearch.Span{
		TraceID:         traceID,
		SpanID:          s.SpanID,
		ParentSpanID:    s.ParentSpanID,
		Name:            s.SpanName,
		Service:         service,
		StartTime:       s.StartTime,
		EndTime:         s.EndTime,
		DurationInNanos: s.DurationNs,
		Kind:            s.Kind,
		Status:          code,
		StatusMessage:   message,
		Attributes:      s.Attributes,
		Resource:        s.ResourceAttributes,
	}
}

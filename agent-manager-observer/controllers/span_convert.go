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

package controllers

import (
	"github.com/wso2/agent-manager/agent-manager-observer/opensearch"
	"github.com/wso2/agent-manager/agent-manager-observer/store"
)

// componentUIDResourceKey is the resource attribute key used by OpenChoreo to
// identify the component. It matches what opensearch/process.go reads when
// building span.Service from resource attributes.
const componentUIDResourceKey = "openchoreo.dev/component-uid"

func statusCodeAndMessage(s *store.SpanStatus) (code, message string) {
	if s == nil {
		return "", ""
	}
	return s.Code, s.Message
}

// spanFromDetails builds an opensearch.Span, ready for opensearch.ProcessSpan, from a span's details.
func spanFromDetails(traceID string, d *store.SpanDetails) opensearch.Span {
	service := ""
	if uid, ok := d.ResourceAttributes[componentUIDResourceKey].(string); ok {
		service = uid
	}

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

// spanFromSummary builds an opensearch.Span from a span-list entry fetched with attributes.
func spanFromSummary(traceID string, s store.Span) opensearch.Span {
	service := ""
	if uid, ok := s.ResourceAttributes[componentUIDResourceKey].(string); ok {
		service = uid
	}

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

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
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/opensearch"
)

func TestConvertSpanInfoToSpan(t *testing.T) {
	start := time.Now().Add(-time.Minute)
	end := time.Now()
	info := SpanInfo{
		SpanID:       "span-1",
		SpanName:     "llm call",
		ParentSpanID: "root",
		StartTime:    start,
		EndTime:      end,
		DurationNs:   42,
		Kind:         "INTERNAL",
		Status:       &SpanStatus{Code: "error", Message: "boom: connection refused"},
		Attributes:   map[string]interface{}{"gen_ai.system": "openai"},
		ResourceAttributes: map[string]interface{}{
			opensearch.ResourceAgent: "comp-abc",
		},
	}

	span := ConvertSpanInfoToSpan("trace-1", info)

	if span.TraceID != "trace-1" || span.SpanID != "span-1" || span.ParentSpanID != "root" {
		t.Errorf("identity fields not mapped: %+v", span)
	}
	if span.Name != "llm call" {
		t.Errorf("expected Name from SpanName, got %q", span.Name)
	}
	if span.Kind != "INTERNAL" || span.Status != "error" {
		t.Errorf("expected Kind/Status mapped, got Kind=%q Status=%q", span.Kind, span.Status)
	}
	if span.StatusMessage != "boom: connection refused" {
		t.Errorf("expected StatusMessage mapped from status.message, got %q", span.StatusMessage)
	}
	if span.DurationInNanos != 42 {
		t.Errorf("expected DurationInNanos 42, got %d", span.DurationInNanos)
	}
	if span.Service != "comp-abc" {
		t.Errorf("expected Service from agent resource attribute, got %q", span.Service)
	}
	if span.Attributes["gen_ai.system"] != "openai" {
		t.Errorf("expected attributes preserved, got %+v", span.Attributes)
	}
	if span.Resource[opensearch.ResourceAgent] != "comp-abc" {
		t.Errorf("expected resource preserved, got %+v", span.Resource)
	}
}

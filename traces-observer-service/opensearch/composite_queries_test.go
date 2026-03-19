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
	"encoding/json"
	"testing"
)

func TestBuildCompositeTraceAggregationQuery(t *testing.T) {
	tests := []struct {
		name      string
		params    TraceQueryParams
		afterKey  *CompositeAfterKey
		batchSize int
		check     func(t *testing.T, query map[string]interface{})
	}{
		{
			name: "basic query with required fields",
			params: TraceQueryParams{
				ComponentUid:   "comp-1",
				EnvironmentUid: "env-1",
			},
			batchSize: 100,
			check: func(t *testing.T, query map[string]interface{}) {
				if size, ok := query["size"].(int); !ok || size != 0 {
					t.Errorf("expected size=0, got %v", query["size"])
				}

				aggs, ok := query["aggs"].(map[string]interface{})
				if !ok {
					t.Fatal("expected aggs in query")
				}
				if _, ok := aggs["trace_composite"]; !ok {
					t.Error("expected trace_composite aggregation")
				}
			},
		},
		{
			name: "includes component and environment filters",
			params: TraceQueryParams{
				ComponentUid:   "comp-1",
				EnvironmentUid: "env-1",
			},
			batchSize: 100,
			check: func(t *testing.T, query map[string]interface{}) {
				q := query["query"].(map[string]interface{})
				boolQ := q["bool"].(map[string]interface{})
				mustConds := boolQ["must"].([]map[string]interface{})
				if len(mustConds) != 2 {
					t.Errorf("expected 2 must conditions, got %d", len(mustConds))
				}
			},
		},
		{
			name: "includes time range filter",
			params: TraceQueryParams{
				ComponentUid:   "comp-1",
				EnvironmentUid: "env-1",
				StartTime:      "2025-01-15T00:00:00Z",
				EndTime:        "2025-01-15T23:59:59Z",
			},
			batchSize: 100,
			check: func(t *testing.T, query map[string]interface{}) {
				q := query["query"].(map[string]interface{})
				boolQ := q["bool"].(map[string]interface{})
				mustConds := boolQ["must"].([]map[string]interface{})
				if len(mustConds) != 3 {
					t.Errorf("expected 3 must conditions (component, env, time range), got %d", len(mustConds))
				}
			},
		},
		{
			name: "default batch size when zero",
			params: TraceQueryParams{
				ComponentUid:   "comp-1",
				EnvironmentUid: "env-1",
			},
			batchSize: 0,
			check: func(t *testing.T, query map[string]interface{}) {
				aggs := query["aggs"].(map[string]interface{})
				tc := aggs["trace_composite"].(map[string]interface{})
				composite := tc["composite"].(map[string]interface{})
				if composite["size"] != 1000 {
					t.Errorf("expected default batch size=1000, got %v", composite["size"])
				}
			},
		},
		{
			name: "custom batch size",
			params: TraceQueryParams{
				ComponentUid:   "comp-1",
				EnvironmentUid: "env-1",
			},
			batchSize: 500,
			check: func(t *testing.T, query map[string]interface{}) {
				aggs := query["aggs"].(map[string]interface{})
				tc := aggs["trace_composite"].(map[string]interface{})
				composite := tc["composite"].(map[string]interface{})
				if composite["size"] != 500 {
					t.Errorf("expected batch size=500, got %v", composite["size"])
				}
			},
		},
		{
			name: "no after key on first request",
			params: TraceQueryParams{
				ComponentUid:   "comp-1",
				EnvironmentUid: "env-1",
			},
			afterKey:  nil,
			batchSize: 100,
			check: func(t *testing.T, query map[string]interface{}) {
				aggs := query["aggs"].(map[string]interface{})
				tc := aggs["trace_composite"].(map[string]interface{})
				composite := tc["composite"].(map[string]interface{})
				if _, ok := composite["after"]; ok {
					t.Error("expected no 'after' key on first request")
				}
			},
		},
		{
			name: "after key set for pagination",
			params: TraceQueryParams{
				ComponentUid:   "comp-1",
				EnvironmentUid: "env-1",
			},
			afterKey:  &CompositeAfterKey{TraceID: "trace-abc"},
			batchSize: 100,
			check: func(t *testing.T, query map[string]interface{}) {
				aggs := query["aggs"].(map[string]interface{})
				tc := aggs["trace_composite"].(map[string]interface{})
				composite := tc["composite"].(map[string]interface{})
				after, ok := composite["after"].(map[string]interface{})
				if !ok {
					t.Fatal("expected 'after' key for pagination")
				}
				if after["trace_id"] != "trace-abc" {
					t.Errorf("expected after trace_id='trace-abc', got %v", after["trace_id"])
				}
			},
		},
		{
			name: "has sub-aggregations for earliest_start, span_count, and root_span_count",
			params: TraceQueryParams{
				ComponentUid:   "comp-1",
				EnvironmentUid: "env-1",
			},
			batchSize: 100,
			check: func(t *testing.T, query map[string]interface{}) {
				aggs := query["aggs"].(map[string]interface{})
				tc := aggs["trace_composite"].(map[string]interface{})
				subAggs, ok := tc["aggs"].(map[string]interface{})
				if !ok {
					t.Fatal("expected sub-aggregations")
				}
				if _, ok := subAggs["earliest_start"]; !ok {
					t.Error("expected earliest_start sub-aggregation")
				}
				if _, ok := subAggs["span_count"]; !ok {
					t.Error("expected span_count sub-aggregation")
				}
				if _, ok := subAggs["root_span_count"]; !ok {
					t.Error("expected root_span_count sub-aggregation")
				}
			},
		},
		{
			name: "composite source uses traceId field",
			params: TraceQueryParams{
				ComponentUid:   "comp-1",
				EnvironmentUid: "env-1",
			},
			batchSize: 100,
			check: func(t *testing.T, query map[string]interface{}) {
				aggs := query["aggs"].(map[string]interface{})
				tc := aggs["trace_composite"].(map[string]interface{})
				composite := tc["composite"].(map[string]interface{})
				sources := composite["sources"].([]map[string]interface{})
				if len(sources) != 1 {
					t.Fatalf("expected 1 source, got %d", len(sources))
				}
				traceIdSource, ok := sources[0]["trace_id"].(map[string]interface{})
				if !ok {
					t.Fatal("expected trace_id source")
				}
				terms, ok := traceIdSource["terms"].(map[string]interface{})
				if !ok {
					t.Fatal("expected terms in trace_id source")
				}
				if terms["field"] != "traceId" {
					t.Errorf("expected field='traceId', got %v", terms["field"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := BuildCompositeTraceAggregationQuery(tt.params, tt.afterKey, tt.batchSize)
			tt.check(t, query)
		})
	}
}

func TestCompositeAggregationResponseUnmarshal(t *testing.T) {
	jsonData := `{
		"aggregations": {
			"trace_composite": {
				"after_key": {"trace_id": "trace-xyz"},
				"buckets": [
					{
						"key": {"trace_id": "trace-abc"},
						"doc_count": 10,
						"earliest_start": {"value": 1705276800000},
						"span_count": {"value": 10},
						"root_span_count": {"doc_count": 1}
					},
					{
						"key": {"trace_id": "trace-def"},
						"doc_count": 5,
						"earliest_start": {"value": 1705363200000},
						"span_count": {"value": 5},
						"root_span_count": {"doc_count": 1}
					}
				]
			}
		}
	}`

	var response CompositeAggregationResponse
	if err := json.Unmarshal([]byte(jsonData), &response); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	buckets := response.Aggregations.TraceComposite.Buckets
	if len(buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %d", len(buckets))
	}

	if buckets[0].Key.TraceID != "trace-abc" {
		t.Errorf("expected trace-abc, got %s", buckets[0].Key.TraceID)
	}
	if buckets[0].DocCount != 10 {
		t.Errorf("expected doc_count=10, got %d", buckets[0].DocCount)
	}
	if buckets[0].EarliestStart.Value != 1705276800000 {
		t.Errorf("expected earliest_start=1705276800000, got %f", buckets[0].EarliestStart.Value)
	}
	if buckets[0].SpanCount.Value != 10 {
		t.Errorf("expected span_count=10, got %d", buckets[0].SpanCount.Value)
	}
	if buckets[0].RootSpanCount.DocCount != 1 {
		t.Errorf("expected root_span_count=1, got %d", buckets[0].RootSpanCount.DocCount)
	}

	if buckets[1].Key.TraceID != "trace-def" {
		t.Errorf("expected trace-def, got %s", buckets[1].Key.TraceID)
	}

	afterKey := response.Aggregations.TraceComposite.AfterKey
	if afterKey == nil {
		t.Fatal("expected after_key")
	}
	if afterKey.TraceID != "trace-xyz" {
		t.Errorf("expected after_key trace_id=trace-xyz, got %s", afterKey.TraceID)
	}
}

func TestCompositeAggregationResponseUnmarshal_NoAfterKey(t *testing.T) {
	jsonData := `{
		"aggregations": {
			"trace_composite": {
				"buckets": [
					{
						"key": {"trace_id": "trace-abc"},
						"doc_count": 3,
						"earliest_start": {"value": 1705276800000},
						"span_count": {"value": 3}
					}
				]
			}
		}
	}`

	var response CompositeAggregationResponse
	if err := json.Unmarshal([]byte(jsonData), &response); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if response.Aggregations.TraceComposite.AfterKey != nil {
		t.Error("expected nil after_key when not present")
	}
	if len(response.Aggregations.TraceComposite.Buckets) != 1 {
		t.Errorf("expected 1 bucket, got %d", len(response.Aggregations.TraceComposite.Buckets))
	}
}

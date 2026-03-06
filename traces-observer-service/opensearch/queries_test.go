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
	"testing"
)

func TestGetAllTraceIndices(t *testing.T) {
	indices := GetAllTraceIndices()
	if len(indices) != 1 {
		t.Fatalf("expected 1 index, got %d", len(indices))
	}
	if indices[0] != "otel-traces-*" {
		t.Errorf("expected 'otel-traces-*', got %q", indices[0])
	}
}

func TestGetIndicesForTimeRange(t *testing.T) {
	tests := []struct {
		name      string
		startTime string
		endTime   string
		want      []string
		wantErr   bool
	}{
		{
			name:      "valid single day range",
			startTime: "2025-01-15T00:00:00Z",
			endTime:   "2025-01-15T23:59:59Z",
			want:      []string{"otel-traces-2025-01-15"},
		},
		{
			name:      "valid multi-day range",
			startTime: "2025-01-14T10:00:00Z",
			endTime:   "2025-01-16T10:00:00Z",
			want:      []string{"otel-traces-2025-01-14", "otel-traces-2025-01-15", "otel-traces-2025-01-16"},
		},
		{
			name:      "empty start time",
			startTime: "",
			endTime:   "2025-01-15T00:00:00Z",
			wantErr:   true,
		},
		{
			name:      "empty end time",
			startTime: "2025-01-15T00:00:00Z",
			endTime:   "",
			wantErr:   true,
		},
		{
			name:      "invalid start time format",
			startTime: "not-a-time",
			endTime:   "2025-01-15T00:00:00Z",
			wantErr:   true,
		},
		{
			name:      "invalid end time format",
			startTime: "2025-01-15T00:00:00Z",
			endTime:   "not-a-time",
			wantErr:   true,
		},
		{
			name:      "start after end",
			startTime: "2025-01-16T00:00:00Z",
			endTime:   "2025-01-14T00:00:00Z",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetIndicesForTimeRange(tt.startTime, tt.endTime)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetIndicesForTimeRange() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("expected %d indices, got %d: %v", len(tt.want), len(got), got)
			}
			for i, idx := range got {
				if idx != tt.want[i] {
					t.Errorf("index[%d] = %q, want %q", i, idx, tt.want[i])
				}
			}
		})
	}
}

func TestSetAndGetDefaultSpanQueryLimit(t *testing.T) {
	// Save original and restore after test
	original := GetDefaultSpanQueryLimit()
	defer SetDefaultSpanQueryLimit(original)

	SetDefaultSpanQueryLimit(500)
	if got := GetDefaultSpanQueryLimit(); got != 500 {
		t.Errorf("expected 500, got %d", got)
	}

	// Non-positive values should be ignored
	SetDefaultSpanQueryLimit(0)
	if got := GetDefaultSpanQueryLimit(); got != 500 {
		t.Errorf("expected 500 (unchanged), got %d", got)
	}

	SetDefaultSpanQueryLimit(-1)
	if got := GetDefaultSpanQueryLimit(); got != 500 {
		t.Errorf("expected 500 (unchanged), got %d", got)
	}
}

func TestBuildTraceByIdsQuery_ParentSpanAddsCollapseAndSort(t *testing.T) {
	query := BuildTraceByIdsQuery(TraceByIdParams{
		TraceIDs:       []string{"trace-a", "trace-b"},
		ComponentUid:   "comp-1",
		EnvironmentUid: "env-1",
		ParentSpan:     true,
		Limit:          5,
	})

	if query["size"] != 5 {
		t.Fatalf("expected size=5, got %v", query["size"])
	}

	collapse, ok := query["collapse"].(map[string]interface{})
	if !ok {
		t.Fatal("expected collapse in query for parentSpan=true")
	}
	if collapse["field"] != "traceId" {
		t.Fatalf("expected collapse.field=traceId, got %v", collapse["field"])
	}

	sortFields, ok := query["sort"].([]map[string]interface{})
	if !ok || len(sortFields) != 1 {
		t.Fatalf("expected one sort field, got %v", query["sort"])
	}
	startSort, ok := sortFields[0]["startTime"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected startTime sort, got %v", sortFields[0])
	}
	if startSort["order"] != "asc" {
		t.Fatalf("expected startTime sort asc, got %v", startSort["order"])
	}
}

func TestBuildTraceByIdsQuery_NoParentSpanNoCollapse(t *testing.T) {
	query := BuildTraceByIdsQuery(TraceByIdParams{
		TraceIDs:       []string{"trace-a", "trace-b"},
		ComponentUid:   "comp-1",
		EnvironmentUid: "env-1",
		ParentSpan:     false,
		Limit:          5,
	})

	if _, ok := query["collapse"]; ok {
		t.Fatalf("did not expect collapse when parentSpan=false: %v", query["collapse"])
	}
	if _, ok := query["sort"]; ok {
		t.Fatalf("did not expect sort when parentSpan=false: %v", query["sort"])
	}
}

func TestBuildTraceByIdsQuery_EmptyTraceIDs(t *testing.T) {
	query := BuildTraceByIdsQuery(TraceByIdParams{})

	if query["size"] != 0 {
		t.Fatalf("expected size=0 for empty trace IDs, got %v", query["size"])
	}
	if _, ok := query["query"].(map[string]interface{})["match_none"]; !ok {
		t.Fatalf("expected match_none query, got %v", query["query"])
	}
}

func TestBuildTraceByIdsQuery(t *testing.T) {
	// Save and restore default limit
	original := GetDefaultSpanQueryLimit()
	defer SetDefaultSpanQueryLimit(original)
	SetDefaultSpanQueryLimit(1000)

	tests := []struct {
		name   string
		params TraceByIdParams
		check  func(t *testing.T, query map[string]interface{})
	}{
		{
			name: "empty trace IDs returns match_none",
			params: TraceByIdParams{
				TraceIDs: []string{},
			},
			check: func(t *testing.T, query map[string]interface{}) {
				q := query["query"].(map[string]interface{})
				if _, ok := q["match_none"]; !ok {
					t.Error("expected match_none query for empty trace IDs")
				}
				if query["size"] != 0 {
					t.Errorf("expected size=0, got %v", query["size"])
				}
			},
		},
		{
			name: "single trace ID uses term query",
			params: TraceByIdParams{
				TraceIDs:       []string{"trace-123"},
				ComponentUid:   "comp-1",
				EnvironmentUid: "env-1",
			},
			check: func(t *testing.T, query map[string]interface{}) {
				q := query["query"].(map[string]interface{})
				boolQ := q["bool"].(map[string]interface{})
				mustConds := boolQ["must"].([]map[string]interface{})
				// First condition should be a "term" (not "terms")
				firstCond := mustConds[0]
				if _, ok := firstCond["term"]; !ok {
					t.Error("expected 'term' query for single trace ID")
				}
			},
		},
		{
			name: "multiple trace IDs uses terms query",
			params: TraceByIdParams{
				TraceIDs:       []string{"trace-1", "trace-2", "trace-3"},
				ComponentUid:   "comp-1",
				EnvironmentUid: "env-1",
			},
			check: func(t *testing.T, query map[string]interface{}) {
				q := query["query"].(map[string]interface{})
				boolQ := q["bool"].(map[string]interface{})
				mustConds := boolQ["must"].([]map[string]interface{})
				firstCond := mustConds[0]
				if _, ok := firstCond["terms"]; !ok {
					t.Error("expected 'terms' query for multiple trace IDs")
				}
			},
		},
		{
			name: "parent span filter adds parentSpanId condition",
			params: TraceByIdParams{
				TraceIDs:       []string{"trace-1"},
				ComponentUid:   "comp-1",
				EnvironmentUid: "env-1",
				ParentSpan:     true,
			},
			check: func(t *testing.T, query map[string]interface{}) {
				q := query["query"].(map[string]interface{})
				boolQ := q["bool"].(map[string]interface{})
				mustConds := boolQ["must"].([]map[string]interface{})
				found := false
				for _, cond := range mustConds {
					if term, ok := cond["term"].(map[string]interface{}); ok {
						if _, ok := term["parentSpanId"]; ok {
							found = true
						}
					}
				}
				if !found {
					t.Error("expected parentSpanId condition when ParentSpan=true")
				}
			},
		},
		{
			name: "uses default limit when limit is 0",
			params: TraceByIdParams{
				TraceIDs:       []string{"trace-1"},
				ComponentUid:   "comp-1",
				EnvironmentUid: "env-1",
				Limit:          0,
			},
			check: func(t *testing.T, query map[string]interface{}) {
				if query["size"] != 1000 {
					t.Errorf("expected default limit 1000, got %v", query["size"])
				}
			},
		},
		{
			name: "uses provided limit",
			params: TraceByIdParams{
				TraceIDs:       []string{"trace-1"},
				ComponentUid:   "comp-1",
				EnvironmentUid: "env-1",
				Limit:          50,
			},
			check: func(t *testing.T, query map[string]interface{}) {
				if query["size"] != 50 {
					t.Errorf("expected limit 50, got %v", query["size"])
				}
			},
		},
		{
			name: "schema field names for component and environment filters",
			params: TraceByIdParams{
				TraceIDs:       []string{"trace-1"},
				ComponentUid:   "comp-1",
				EnvironmentUid: "env-1",
			},
			check: func(t *testing.T, query map[string]interface{}) {
				q := query["query"].(map[string]interface{})
				boolQ := q["bool"].(map[string]interface{})
				mustConds := boolQ["must"].([]map[string]interface{})
				foundComp, foundEnv := false, false
				for _, cond := range mustConds {
					if term, ok := cond["term"].(map[string]interface{}); ok {
						if _, ok := term["resource.openchoreo.dev/component-uid"]; ok {
							foundComp = true
						}
						if _, ok := term["resource.openchoreo.dev/environment-uid"]; ok {
							foundEnv = true
						}
					}
				}
				if !foundComp {
					t.Error("expected field 'resource.openchoreo.dev/component-uid' in component filter")
				}
				if !foundEnv {
					t.Error("expected field 'resource.openchoreo.dev/environment-uid' in environment filter")
				}
			},
		},
		{
			name: "empty componentUid and environmentUid omits those filters",
			params: TraceByIdParams{
				TraceIDs:       []string{"trace-1"},
				ComponentUid:   "",
				EnvironmentUid: "",
			},
			check: func(t *testing.T, query map[string]interface{}) {
				q := query["query"].(map[string]interface{})
				boolQ := q["bool"].(map[string]interface{})
				mustConds := boolQ["must"].([]map[string]interface{})
				// Only the traceId term condition should remain
				if len(mustConds) != 1 {
					t.Fatalf("expected 1 must condition (traceId only), got %d", len(mustConds))
				}
				termCond := mustConds[0]["term"].(map[string]interface{})
				if _, ok := termCond["traceId"]; !ok {
					t.Error("expected only traceId filter when component/env are empty")
				}
			},
		},
		{
			name: "parentSpan with multiple trace IDs combines both filters",
			params: TraceByIdParams{
				TraceIDs:       []string{"trace-1", "trace-2"},
				ComponentUid:   "comp-1",
				EnvironmentUid: "env-1",
				ParentSpan:     true,
			},
			check: func(t *testing.T, query map[string]interface{}) {
				q := query["query"].(map[string]interface{})
				boolQ := q["bool"].(map[string]interface{})
				mustConds := boolQ["must"].([]map[string]interface{})
				// Expect: terms(traceId) + term(parentSpanId) + term(component) + term(env) = 4
				if len(mustConds) != 4 {
					t.Fatalf("expected 4 must conditions, got %d", len(mustConds))
				}
				// First should be "terms" (multiple IDs)
				if _, ok := mustConds[0]["terms"]; !ok {
					t.Error("expected 'terms' query for multiple trace IDs")
				}
				// Second should be parentSpanId
				parentTerm := mustConds[1]["term"].(map[string]interface{})
				if val, ok := parentTerm["parentSpanId"]; !ok || val != "" {
					t.Error("expected parentSpanId='' filter")
				}
			},
		},
		{
			name: "parentSpan false does not add parentSpanId condition",
			params: TraceByIdParams{
				TraceIDs:       []string{"trace-1"},
				ComponentUid:   "comp-1",
				EnvironmentUid: "env-1",
				ParentSpan:     false,
			},
			check: func(t *testing.T, query map[string]interface{}) {
				q := query["query"].(map[string]interface{})
				boolQ := q["bool"].(map[string]interface{})
				mustConds := boolQ["must"].([]map[string]interface{})
				for _, cond := range mustConds {
					if term, ok := cond["term"].(map[string]interface{}); ok {
						if _, ok := term["parentSpanId"]; ok {
							t.Error("did not expect parentSpanId condition when ParentSpan=false")
						}
					}
				}
			},
		},
		{
			name: "single trace ID value propagated correctly",
			params: TraceByIdParams{
				TraceIDs:       []string{"abc-def-123"},
				ComponentUid:   "",
				EnvironmentUid: "",
			},
			check: func(t *testing.T, query map[string]interface{}) {
				q := query["query"].(map[string]interface{})
				boolQ := q["bool"].(map[string]interface{})
				mustConds := boolQ["must"].([]map[string]interface{})
				termCond := mustConds[0]["term"].(map[string]interface{})
				if termCond["traceId"] != "abc-def-123" {
					t.Errorf("expected traceId 'abc-def-123', got %v", termCond["traceId"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := BuildTraceByIdsQuery(tt.params)
			tt.check(t, query)
		})
	}
}

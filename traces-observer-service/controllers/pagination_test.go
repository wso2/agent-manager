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

package controllers

import (
	"testing"
)

func TestSortAndPaginateTraceBuckets(t *testing.T) {
	buckets := []traceBucketWithMetadata{
		{TraceID: "trace-c", EarliestStart: 3000, DocCount: 5, SpanCount: 5},
		{TraceID: "trace-a", EarliestStart: 1000, DocCount: 10, SpanCount: 10},
		{TraceID: "trace-e", EarliestStart: 5000, DocCount: 2, SpanCount: 2},
		{TraceID: "trace-b", EarliestStart: 2000, DocCount: 8, SpanCount: 8},
		{TraceID: "trace-d", EarliestStart: 4000, DocCount: 3, SpanCount: 3},
	}

	tests := []struct {
		name      string
		sortOrder string
		offset    int
		limit     int
		wantIDs   []string
	}{
		{
			name:      "desc sort, first page",
			sortOrder: "desc",
			offset:    0,
			limit:     3,
			wantIDs:   []string{"trace-e", "trace-d", "trace-c"},
		},
		{
			name:      "desc sort, second page",
			sortOrder: "desc",
			offset:    3,
			limit:     3,
			wantIDs:   []string{"trace-b", "trace-a"},
		},
		{
			name:      "asc sort, first page",
			sortOrder: "asc",
			offset:    0,
			limit:     3,
			wantIDs:   []string{"trace-a", "trace-b", "trace-c"},
		},
		{
			name:      "asc sort, second page",
			sortOrder: "asc",
			offset:    3,
			limit:     3,
			wantIDs:   []string{"trace-d", "trace-e"},
		},
		{
			name:      "offset beyond total",
			sortOrder: "desc",
			offset:    10,
			limit:     5,
			wantIDs:   []string{},
		},
		{
			name:      "limit exceeds remaining",
			sortOrder: "desc",
			offset:    4,
			limit:     5,
			wantIDs:   []string{"trace-a"},
		},
		{
			name:      "single item page",
			sortOrder: "desc",
			offset:    2,
			limit:     1,
			wantIDs:   []string{"trace-c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Copy buckets to avoid mutation between tests
			input := make([]traceBucketWithMetadata, len(buckets))
			copy(input, buckets)

			result := sortAndPaginateTraceBuckets(input, tt.sortOrder, tt.offset, tt.limit)

			if len(result) != len(tt.wantIDs) {
				t.Fatalf("expected %d results, got %d", len(tt.wantIDs), len(result))
			}

			for i, want := range tt.wantIDs {
				if result[i].TraceID != want {
					t.Errorf("result[%d].TraceID = %s, want %s", i, result[i].TraceID, want)
				}
			}
		})
	}
}

func TestSortAndPaginateTraceBuckets_EmptyInput(t *testing.T) {
	result := sortAndPaginateTraceBuckets([]traceBucketWithMetadata{}, "desc", 0, 10)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d items", len(result))
	}
}

func TestSortAndPaginateTraceBuckets_PreservesMetadata(t *testing.T) {
	buckets := []traceBucketWithMetadata{
		{TraceID: "trace-b", EarliestStart: 2000, DocCount: 8, SpanCount: 15},
		{TraceID: "trace-a", EarliestStart: 1000, DocCount: 3, SpanCount: 5},
	}

	result := sortAndPaginateTraceBuckets(buckets, "asc", 0, 10)

	if result[0].TraceID != "trace-a" {
		t.Errorf("expected trace-a first, got %s", result[0].TraceID)
	}
	if result[0].DocCount != 3 {
		t.Errorf("expected DocCount=3, got %d", result[0].DocCount)
	}
	if result[0].SpanCount != 5 {
		t.Errorf("expected SpanCount=5, got %d", result[0].SpanCount)
	}
}

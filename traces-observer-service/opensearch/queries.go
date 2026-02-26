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

const traceIndexPrefix = "otel-traces-"

// GetAllTraceIndices returns a wildcard index pattern that matches all trace indices.
func GetAllTraceIndices() []string {
	return []string{traceIndexPrefix + "*"}
}

// GetIndicesForTimeRange generates index names for the given time range
// Returns indices in format: otel-traces-YYYY-MM-DD
func GetIndicesForTimeRange(startTime, endTime string) ([]string, error) {
	if startTime == "" || endTime == "" {
		return nil, fmt.Errorf("start time and end time are required")
	}

	// Parse the time strings (expecting RFC3339 format)
	start, err := time.Parse(time.RFC3339, startTime)
	if err != nil {
		return nil, fmt.Errorf("invalid start time format: %w", err)
	}

	end, err := time.Parse(time.RFC3339, endTime)
	if err != nil {
		return nil, fmt.Errorf("invalid end time format: %w", err)
	}

	// Ensure start is before end
	if start.After(end) {
		return nil, fmt.Errorf("start time must be before end time")
	}

	// Generate indices for each day in the range
	indices := []string{}
	indexMap := make(map[string]bool) // To avoid duplicates

	// Iterate through each day from start to end
	currentDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	endDay := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, end.Location())

	for !currentDay.After(endDay) {
		indexName := fmt.Sprintf("%s%04d-%02d-%02d", traceIndexPrefix, currentDay.Year(), currentDay.Month(), currentDay.Day())
		if !indexMap[indexName] {
			indices = append(indices, indexName)
			indexMap[indexName] = true
		}
		currentDay = currentDay.AddDate(0, 0, 1) // Add one day
	}

	return indices, nil
}

// BuildTraceAggregationQuery builds an OpenSearch aggregation query that groups spans by traceId.
// Returns unique trace IDs sorted by earliest start time, with span counts per trace.
func BuildTraceAggregationQuery(params TraceQueryParams) map[string]interface{} {
	mustConditions := []map[string]interface{}{}

	if params.ComponentUid != "" {
		mustConditions = append(mustConditions, map[string]interface{}{
			"term": map[string]interface{}{
				"resource.openchoreo.dev/component-uid": params.ComponentUid,
			},
		})
	}
	if params.EnvironmentUid != "" {
		mustConditions = append(mustConditions, map[string]interface{}{
			"term": map[string]interface{}{
				"resource.openchoreo.dev/environment-uid": params.EnvironmentUid,
			},
		})
	}
	if params.StartTime != "" && params.EndTime != "" {
		mustConditions = append(mustConditions, map[string]interface{}{
			"range": map[string]interface{}{
				"startTime": map[string]interface{}{
					"gte": params.StartTime,
					"lte": params.EndTime,
				},
			},
		})
	}

	sortOrder := params.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	aggSize := params.Offset + params.Limit
	if aggSize <= 0 {
		aggSize = 10
	}

	return map[string]interface{}{
		"size": 0,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": mustConditions,
			},
		},
		"aggs": map[string]interface{}{
			"total_traces": map[string]interface{}{
				"cardinality": map[string]interface{}{
					"field": "traceId",
				},
			},
			"traces": map[string]interface{}{
				"terms": map[string]interface{}{
					"field": "traceId",
					"size":  aggSize,
					"order": map[string]interface{}{
						"earliest_start": sortOrder,
					},
				},
				"aggs": map[string]interface{}{
					"earliest_start": map[string]interface{}{
						"min": map[string]interface{}{
							"field": "startTime",
						},
					},
					"span_count": map[string]interface{}{
						"value_count": map[string]interface{}{
							"field": "spanId",
						},
					},
				},
			},
		},
	}
}

// BuildTraceByIdsQuery builds a query to fetch spans for one or more trace IDs.
// When parentSpan is true, adds a filter for parentSpanId == "" to return only root spans.
func BuildTraceByIdsQuery(params TraceByIdParams) map[string]interface{} {
	mustConditions := []map[string]interface{}{}

	// Support single or multiple trace IDs
	if len(params.TraceIDs) == 1 {
		mustConditions = append(mustConditions, map[string]interface{}{
			"term": map[string]interface{}{
				"traceId": params.TraceIDs[0],
			},
		})
	} else {
		mustConditions = append(mustConditions, map[string]interface{}{
			"terms": map[string]interface{}{
				"traceId": params.TraceIDs,
			},
		})
	}

	// Parent span filter
	if params.ParentSpan {
		mustConditions = append(mustConditions, map[string]interface{}{
			"term": map[string]interface{}{
				"parentSpanId": "",
			},
		})
	}

	if params.ComponentUid != "" {
		mustConditions = append(mustConditions, map[string]interface{}{
			"term": map[string]interface{}{
				"resource.openchoreo.dev/component-uid": params.ComponentUid,
			},
		})
	}
	if params.EnvironmentUid != "" {
		mustConditions = append(mustConditions, map[string]interface{}{
			"term": map[string]interface{}{
				"resource.openchoreo.dev/environment-uid": params.EnvironmentUid,
			},
		})
	}

	limit := params.Limit
	if limit == 0 {
		limit = 1000
	}

	return map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": mustConditions,
			},
		},
		"size": limit,
	}
}

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
		indexName := fmt.Sprintf("otel-traces-%04d-%02d-%02d", currentDay.Year(), currentDay.Month(), currentDay.Day())
		if !indexMap[indexName] {
			indices = append(indices, indexName)
			indexMap[indexName] = true
		}
		currentDay = currentDay.AddDate(0, 0, 1) // Add one day
	}

	return indices, nil
}

// BuildTraceQuery builds an OpenSearch query for traces
func BuildTraceQuery(params TraceQueryParams) map[string]interface{} {
	// Build the must conditions
	mustConditions := []map[string]interface{}{}

	// Add component UID filter
	if params.ComponentUid != "" {
		mustConditions = append(mustConditions, map[string]interface{}{
			"term": map[string]interface{}{
				"resource.openchoreo.dev/component-uid": params.ComponentUid,
			},
		})
	}

	// Add project UID filter
	if params.ProjectUid != "" {
		mustConditions = append(mustConditions, map[string]interface{}{
			"term": map[string]interface{}{
				"resource.openchoreo.dev/project-uid": params.ProjectUid,
			},
		})
	}

	// Add environment UID filter
	if params.EnvironmentUid != "" {
		mustConditions = append(mustConditions, map[string]interface{}{
			"term": map[string]interface{}{
				"resource.openchoreo.dev/environment-uid": params.EnvironmentUid,
			},
		})
	}

	// Add organization UID filter (optional)
	if params.OrganizationUid != "" {
		mustConditions = append(mustConditions, map[string]interface{}{
			"term": map[string]interface{}{
				"resource.openchoreo.dev/organization-uid": params.OrganizationUid,
			},
		})
	}

	// Add time range filter
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

	// Set default limit if not provided
	limit := params.Limit
	if limit == 0 {
		limit = 100
	}

	// Set default offset
	offset := params.Offset
	if offset < 0 {
		offset = 0
	}

	// Set default sort order
	sortOrder := params.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	// Build the complete query
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": mustConditions,
			},
		},
		"size": limit,
		"from": offset,
		"sort": []map[string]interface{}{
			{
				"startTime": map[string]string{
					"order": sortOrder,
				},
			},
		},
	}

	return query
}

// BuildTraceByIdAndServiceQuery builds a query to get spans by both traceId and componentUid
func BuildTraceByIdAndServiceQuery(params TraceByIdAndServiceParams) map[string]interface{} {
	// Build the must conditions - traceId and resource filters must match
	mustConditions := []map[string]interface{}{
		{
			"term": map[string]interface{}{
				"traceId": params.TraceID,
			},
		},
	}

	// Add component UID filter
	if params.ComponentUid != "" {
		mustConditions = append(mustConditions, map[string]interface{}{
			"term": map[string]interface{}{
				"resource.openchoreo.dev/component-uid": params.ComponentUid,
			},
		})
	}

	// Add project UID filter
	if params.ProjectUid != "" {
		mustConditions = append(mustConditions, map[string]interface{}{
			"term": map[string]interface{}{
				"resource.openchoreo.dev/project-uid": params.ProjectUid,
			},
		})
	}

	// Add environment UID filter
	if params.EnvironmentUid != "" {
		mustConditions = append(mustConditions, map[string]interface{}{
			"term": map[string]interface{}{
				"resource.openchoreo.dev/environment-uid": params.EnvironmentUid,
			},
		})
	}

	// Add organization UID filter (optional)
	if params.OrganizationUid != "" {
		mustConditions = append(mustConditions, map[string]interface{}{
			"term": map[string]interface{}{
				"resource.openchoreo.dev/organization-uid": params.OrganizationUid,
			},
		})
	}

	// Set default limit if not provided
	limit := params.Limit
	if limit == 0 {
		limit = 10000 // Get all spans for the trace by default
	}

	// Set default sort order
	sortOrder := params.SortOrder
	if sortOrder == "" {
		sortOrder = "asc"
	}

	// Build the complete query
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": mustConditions,
			},
		},
		"size": limit,
		"sort": []map[string]interface{}{
			{
				"startTime": map[string]string{
					"order": sortOrder,
				},
			},
		},
	}

	return query
}

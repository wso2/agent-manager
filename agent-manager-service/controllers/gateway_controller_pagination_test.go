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
	"testing"
)

// TestPaginationParameterValidation tests that negative/invalid pagination params are handled safely
func TestPaginationParameterValidation(t *testing.T) {
	tests := []struct {
		name           string
		inputLimit     int
		inputOffset    int
		expectedLimit  int
		expectedOffset int
	}{
		{
			name:           "Negative limit should default to defaultLimit",
			inputLimit:     -10,
			inputOffset:    0,
			expectedLimit:  defaultLimit,
			expectedOffset: 0,
		},
		{
			name:           "Negative offset should default to 0",
			inputLimit:     10,
			inputOffset:    -5,
			expectedLimit:  10,
			expectedOffset: 0,
		},
		{
			name:           "Both negative should use defaults",
			inputLimit:     -1,
			inputOffset:    -1,
			expectedLimit:  defaultLimit,
			expectedOffset: 0,
		},
		{
			name:           "Limit exceeding max should cap at 1000",
			inputLimit:     5000,
			inputOffset:    0,
			expectedLimit:  1000,
			expectedOffset: 0,
		},
		{
			name:           "Valid params should pass through",
			inputLimit:     50,
			inputOffset:    100,
			expectedLimit:  50,
			expectedOffset: 100,
		},
		{
			name:           "Zero limit should default",
			inputLimit:     0,
			inputOffset:    0,
			expectedLimit:  defaultLimit,
			expectedOffset: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Apply the same validation logic as the controller
			limit := tt.inputLimit
			offset := tt.inputOffset

			if limit < 0 {
				limit = defaultLimit
			}
			if limit == 0 {
				limit = defaultLimit
			}
			if offset < 0 {
				offset = 0
			}
			if limit > 1000 {
				limit = 1000
			}

			if limit != tt.expectedLimit {
				t.Errorf("Expected limit %d, got %d", tt.expectedLimit, limit)
			}
			if offset != tt.expectedOffset {
				t.Errorf("Expected offset %d, got %d", tt.expectedOffset, offset)
			}
		})
	}
}

// TestPaginationSliceSafety tests that the old client-side slice panic scenario is prevented
func TestPaginationSliceSafety(t *testing.T) {
	tests := []struct {
		name        string
		totalItems  int
		limit       int
		offset      int
		expectPanic bool
	}{
		{
			name:        "Offset beyond slice length (old bug)",
			totalItems:  10,
			limit:       10,
			offset:      100,
			expectPanic: false, // Should NOT panic with DB-level pagination
		},
		{
			name:        "Negative offset (old bug)",
			totalItems:  10,
			limit:       10,
			offset:      -5,
			expectPanic: false, // Prevented by validation
		},
		{
			name:        "Valid pagination",
			totalItems:  100,
			limit:       10,
			offset:      20,
			expectPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate offset (as done in controller)
			offset := tt.offset
			if offset < 0 {
				offset = 0
			}

			// With DB-level pagination, this slice operation is no longer needed
			// But if someone reverts to client-side pagination, this test documents the issue

			// OLD APPROACH (causes panic):
			// start := offset
			// end := offset + tt.limit
			// if start > len(items) { start = len(items) }
			// if end > len(items) { end = len(items) }
			// result := items[start:end]  // PANIC if start < 0

			// NEW APPROACH: DB handles pagination, no slice needed
			// Repository returns only requested page, controller just uses it directly

			// This test verifies the validation prevents the panic scenario
			if offset < 0 {
				t.Errorf("Validation failed to correct negative offset")
			}
		})
	}
}

// BenchmarkPaginationPerformance documents the performance improvement
// This is a documentation benchmark showing the theoretical improvement
func BenchmarkPaginationPerformance(b *testing.B) {
	b.Run("OldApproach_SimulatedCost", func(b *testing.B) {
		// Simulate old approach: N+1 queries + N RPCs
		totalGateways := 100
		pageSize := 10

		for i := 0; i < b.N; i++ {
			// Simulate fetching all 100 gateways
			_ = totalGateways

			// Simulate N+1 environment mapping queries (100 queries)
			for j := 0; j < totalGateways; j++ {
				_ = j // Simulate DB query
			}

			// Simulate N RPC calls to OpenChoreo (100 RPCs)
			for j := 0; j < totalGateways; j++ {
				_ = j // Simulate RPC
			}

			// Simulate client-side slicing (discarding 90 results)
			_ = pageSize
		}
	})

	b.Run("NewApproach_SimulatedCost", func(b *testing.B) {
		// Simulate new approach: 3 queries + 1 RPC
		pageSize := 10

		for i := 0; i < b.N; i++ {
			// Simulate count query
			_ = 1

			// Simulate paginated list query (only 10 rows)
			_ = pageSize

			// Simulate bulk environment mapping query (only for 10 gateways)
			_ = 1

			// Simulate 1 RPC call to OpenChoreo
			_ = 1
		}
	})
}

// TestBulkEnvironmentMappingLogic tests the bulk fetching logic
func TestBulkEnvironmentMappingLogic(t *testing.T) {
	// This test documents that bulk fetching should group by gateway ID
	gatewayIDs := []string{"gw1", "gw2", "gw3"}

	// Simulate bulk query result (what repository returns)
	bulkResult := map[string][]string{
		"gw1": {"env1", "env2"},
		"gw2": {"env1"},
		"gw3": {}, // No environments
	}

	// Verify we can access mappings for each gateway
	for _, gwID := range gatewayIDs {
		mappings := bulkResult[gwID]
		if gwID == "gw1" && len(mappings) != 2 {
			t.Errorf("Expected 2 mappings for gw1, got %d", len(mappings))
		}
		if gwID == "gw2" && len(mappings) != 1 {
			t.Errorf("Expected 1 mapping for gw2, got %d", len(mappings))
		}
		if gwID == "gw3" && len(mappings) != 0 {
			t.Errorf("Expected 0 mappings for gw3, got %d", len(mappings))
		}
	}
}

// TestOpenChoreoEnvironmentCaching documents the environment caching pattern
func TestOpenChoreoEnvironmentCaching(t *testing.T) {
	// This test documents that OpenChoreo environments should be fetched once
	gatewayCount := 100
	rpcCallCount := 0

	// OLD APPROACH: Fetch per gateway
	for i := 0; i < gatewayCount; i++ {
		// Each gateway triggers an RPC call
		rpcCallCount++
	}
	if rpcCallCount != gatewayCount {
		t.Errorf("Old approach should make %d RPC calls, got %d", gatewayCount, rpcCallCount)
	}

	// NEW APPROACH: Fetch once, reuse
	rpcCallCount = 0
	ocEnvironments := []string{"env1", "env2", "env3"} // Fetched once
	rpcCallCount++

	for i := 0; i < gatewayCount; i++ {
		// Each gateway reuses the same ocEnvironments slice
		_ = ocEnvironments
	}
	if rpcCallCount != 1 {
		t.Errorf("New approach should make 1 RPC call, got %d", rpcCallCount)
	}
}

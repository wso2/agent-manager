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

package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/tests/apitestutils"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/wiring"
)

func TestListEvaluators(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("List all evaluators should return 200 with builtin evaluators", func(t *testing.T) {
		testClients := wiring.TestClients{}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Make request
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators", nil)
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)

		// Assert
		require.Equal(t, http.StatusOK, resp.Code)

		var result spec.EvaluatorListResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Should have 18 builtin evaluators from the migration (12 standard + 6 deepeval)
		assert.Equal(t, int32(18), result.Total)
		assert.Len(t, result.Evaluators, 18)

		// Verify response structure
		for _, evaluator := range result.Evaluators {
			assert.NotEmpty(t, evaluator.Id)
			assert.NotEmpty(t, evaluator.Identifier)
			assert.NotEmpty(t, evaluator.DisplayName)
			assert.NotEmpty(t, evaluator.Description)
			assert.NotEmpty(t, evaluator.Version)
			assert.NotEmpty(t, evaluator.Provider)
			assert.True(t, evaluator.IsBuiltin)
			assert.NotNil(t, evaluator.Tags)
			assert.NotNil(t, evaluator.ConfigSchema)
		}
	})

	t.Run("Filter by provider=deepeval should return only deepeval evaluators", func(t *testing.T) {
		testClients := wiring.TestClients{}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Make request with provider filter
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators?provider=deepeval", nil)
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)

		// Assert
		require.Equal(t, http.StatusOK, resp.Code)

		var result spec.EvaluatorListResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Should have 6 deepeval evaluators
		assert.Equal(t, int32(6), result.Total)
		assert.Len(t, result.Evaluators, 6)

		// All should be deepeval provider
		for _, evaluator := range result.Evaluators {
			assert.Equal(t, "deepeval", evaluator.Provider)
			assert.True(t, evaluator.IsBuiltin)
		}
	})

	t.Run("Filter by provider=standard should return only standard evaluators", func(t *testing.T) {
		testClients := wiring.TestClients{}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Make request with provider filter
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators?provider=standard", nil)
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)

		// Assert
		require.Equal(t, http.StatusOK, resp.Code)

		var result spec.EvaluatorListResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Should have 12 standard evaluators
		assert.Equal(t, int32(12), result.Total)
		assert.Len(t, result.Evaluators, 12)

		// All should be standard provider
		for _, evaluator := range result.Evaluators {
			assert.Equal(t, "standard", evaluator.Provider)
			assert.True(t, evaluator.IsBuiltin)
		}
	})

	t.Run("Filter by tags should return evaluators with matching tags", func(t *testing.T) {
		testClients := wiring.TestClients{}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Make request with tags filter
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators?tags=deepeval,tool-use", nil)
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)

		// Assert
		require.Equal(t, http.StatusOK, resp.Code)

		var result spec.EvaluatorListResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Should have evaluators with both tags
		assert.Greater(t, result.Total, int32(0))

		// All should have both tags
		for _, evaluator := range result.Evaluators {
			assert.Contains(t, evaluator.Tags, "deepeval")
			assert.Contains(t, evaluator.Tags, "tool-use")
		}
	})

	t.Run("Search by keyword 'correctness' should return matching evaluators", func(t *testing.T) {
		testClients := wiring.TestClients{}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Make request with search filter
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators?search=correctness", nil)
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)

		// Assert
		require.Equal(t, http.StatusOK, resp.Code)

		var result spec.EvaluatorListResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Should find evaluators with "correctness" in name or description
		// (argument-correctness and tool-correctness from deepeval)
		assert.GreaterOrEqual(t, result.Total, int32(2))

		// All should have "correctness" in identifier, displayName, or description
		for _, evaluator := range result.Evaluators {
			hasMatch := contains(evaluator.Identifier, "correctness") ||
				contains(evaluator.DisplayName, "correctness") ||
				contains(evaluator.Description, "correctness")
			assert.True(t, hasMatch, "Evaluator %s should contain 'correctness'", evaluator.Identifier)
		}
	})

	t.Run("Search by keyword 'agent' should return matching evaluators", func(t *testing.T) {
		testClients := wiring.TestClients{}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Make request with search filter
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators?search=agent", nil)
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)

		// Assert
		require.Equal(t, http.StatusOK, resp.Code)

		var result spec.EvaluatorListResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Should find multiple evaluators with "agent" in description
		assert.Greater(t, result.Total, int32(0))
	})

	t.Run("Combine filters: provider=deepeval and search=tool should work", func(t *testing.T) {
		testClients := wiring.TestClients{}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Make request with multiple filters
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators?provider=deepeval&search=tool", nil)
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)

		// Assert
		require.Equal(t, http.StatusOK, resp.Code)

		var result spec.EvaluatorListResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Should find deepeval tool correctness
		assert.Greater(t, result.Total, int32(0))

		for _, evaluator := range result.Evaluators {
			assert.Equal(t, "deepeval", evaluator.Provider)
			hasMatch := contains(evaluator.Identifier, "tool") ||
				contains(evaluator.DisplayName, "tool") ||
				contains(evaluator.Description, "tool")
			assert.True(t, hasMatch)
		}
	})

	t.Run("Pagination with limit should work", func(t *testing.T) {
		testClients := wiring.TestClients{}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Make request with limit
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators?limit=5", nil)
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)

		// Assert
		require.Equal(t, http.StatusOK, resp.Code)

		var result spec.EvaluatorListResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, int32(18), result.Total)
		assert.Len(t, result.Evaluators, 5)
		assert.Equal(t, int32(5), result.Limit)
		assert.Equal(t, int32(0), result.Offset)
	})

	t.Run("Pagination with offset should work", func(t *testing.T) {
		testClients := wiring.TestClients{}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Make request with offset
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators?limit=5&offset=10", nil)
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)

		// Assert
		require.Equal(t, http.StatusOK, resp.Code)

		var result spec.EvaluatorListResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, int32(18), result.Total)
		assert.Len(t, result.Evaluators, 5)
		assert.Equal(t, int32(5), result.Limit)
		assert.Equal(t, int32(10), result.Offset)
	})

	t.Run("Default limit should be 20", func(t *testing.T) {
		testClients := wiring.TestClients{}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Make request without limit
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators", nil)
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)

		// Assert
		require.Equal(t, http.StatusOK, resp.Code)

		var result spec.EvaluatorListResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, int32(18), result.Total)
		assert.Equal(t, int32(20), result.Limit) // Default limit
	})

	t.Run("Limit greater than 100 should be capped at 100", func(t *testing.T) {
		testClients := wiring.TestClients{}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Make request with large limit
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators?limit=200", nil)
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)

		// Assert
		require.Equal(t, http.StatusOK, resp.Code)

		var result spec.EvaluatorListResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, int32(100), result.Limit) // Capped at 100
	})
}

func TestGetEvaluator(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("Get specific evaluator by identifier should return 200", func(t *testing.T) {
		testClients := wiring.TestClients{}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Make request for a known builtin evaluator
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators/answer_relevancy", nil)
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)

		// Assert
		require.Equal(t, http.StatusOK, resp.Code)

		var result spec.EvaluatorResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, "answer_relevancy", result.Identifier)
		assert.Equal(t, "Answer Relevancy", result.DisplayName)
		assert.Equal(t, "standard", result.Provider)
		assert.True(t, result.IsBuiltin)
		assert.NotEmpty(t, result.Id)
		assert.NotEmpty(t, result.Version)
	})

	t.Run("Get evaluator with URL-encoded identifier should work", func(t *testing.T) {
		testClients := wiring.TestClients{}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Make request with URL-encoded identifier (deepeval/tool-correctness)
		encodedIdentifier := url.PathEscape("deepeval/tool-correctness")
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/orgs/test-org/evaluators/%s", encodedIdentifier), nil)
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)

		// Assert
		require.Equal(t, http.StatusOK, resp.Code)

		var result spec.EvaluatorResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, "deepeval/tool-correctness", result.Identifier)
		assert.Equal(t, "Tool Correctness", result.DisplayName)
		assert.Equal(t, "deepeval", result.Provider)
		assert.True(t, result.IsBuiltin)
	})

	t.Run("Get deepeval evaluator should return correct structure", func(t *testing.T) {
		testClients := wiring.TestClients{}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Make request
		encodedIdentifier := url.PathEscape("deepeval/argument-correctness")
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/orgs/test-org/evaluators/%s", encodedIdentifier), nil)
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)

		// Assert
		require.Equal(t, http.StatusOK, resp.Code)

		var result spec.EvaluatorResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		assert.Equal(t, "deepeval/argument-correctness", result.Identifier)
		assert.Equal(t, "deepeval", result.Provider)
		assert.Contains(t, result.Tags, "deepeval")
	})

	t.Run("Get non-existent evaluator should return 404", func(t *testing.T) {
		testClients := wiring.TestClients{}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Make request for non-existent evaluator
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators/non_existent_evaluator", nil)
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)

		// Assert
		require.Equal(t, http.StatusNotFound, resp.Code)
	})

	t.Run("Get evaluator with missing org should return 400", func(t *testing.T) {
		testClients := wiring.TestClients{}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Make request without org name (this would be caught by routing, but test the handler)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs//evaluators/answer_relevancy", nil)
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)

		// Assert - should return 404 from router or 400 from handler
		require.NotEqual(t, http.StatusOK, resp.Code)
	})

	t.Run("Verify evaluator has valid config schema", func(t *testing.T) {
		testClients := wiring.TestClients{}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Make request for an evaluator with config params
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators/answer_length", nil)
		resp := httptest.NewRecorder()
		app.ServeHTTP(resp, req)

		// Assert
		require.Equal(t, http.StatusOK, resp.Code)

		var result spec.EvaluatorResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		require.NoError(t, err)

		// Verify config schema structure
		assert.NotEmpty(t, result.ConfigSchema)

		// Check for expected config parameters
		hasMaxLength := false
		hasMinLength := false
		for _, param := range result.ConfigSchema {
			if param.Key == "max_length" {
				hasMaxLength = true
				assert.Equal(t, "integer", param.Type)
				assert.NotNil(t, param.Default)
				assert.NotNil(t, param.Min)
			}
			if param.Key == "min_length" {
				hasMinLength = true
				assert.Equal(t, "integer", param.Type)
				assert.NotNil(t, param.Default)
			}
		}
		assert.True(t, hasMaxLength, "Should have max_length config")
		assert.True(t, hasMinLength, "Should have min_length config")
	})

	t.Run("All standard evaluators should be retrievable", func(t *testing.T) {
		testClients := wiring.TestClients{}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		standardEvaluators := []string{
			"answer_length",
			"answer_relevancy",
			"contains_match",
			"exact_match",
			"iteration_count",
			"latency",
			"prohibited_content",
			"required_content",
			"required_tools",
			"step_success_rate",
			"token_efficiency",
			"tool_sequence",
		}

		for _, identifier := range standardEvaluators {
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/orgs/test-org/evaluators/%s", identifier), nil)
			resp := httptest.NewRecorder()
			app.ServeHTTP(resp, req)

			require.Equal(t, http.StatusOK, resp.Code, "Failed to get %s", identifier)

			var result spec.EvaluatorResponse
			err := json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)
			assert.Equal(t, identifier, result.Identifier)
			assert.Equal(t, "standard", result.Provider)
		}
	})

	t.Run("All deepeval evaluators should be retrievable", func(t *testing.T) {
		testClients := wiring.TestClients{}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		deepevalEvaluators := []string{
			"deepeval/argument-correctness",
			"deepeval/plan-adherence",
			"deepeval/plan-quality",
			"deepeval/step-efficiency",
			"deepeval/task-completion",
			"deepeval/tool-correctness",
		}

		for _, identifier := range deepevalEvaluators {
			encodedId := url.PathEscape(identifier)
			req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/orgs/test-org/evaluators/%s", encodedId), nil)
			resp := httptest.NewRecorder()
			app.ServeHTTP(resp, req)

			require.Equal(t, http.StatusOK, resp.Code, "Failed to get %s", identifier)

			var result spec.EvaluatorResponse
			err := json.NewDecoder(resp.Body).Decode(&result)
			require.NoError(t, err)
			assert.Equal(t, identifier, result.Identifier)
			assert.Equal(t, "deepeval", result.Provider)
		}
	})
}

// Helper function for case-insensitive substring match
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(substr) > 0 && (s[:len(substr)] == substr ||
			len(s) > len(substr) && contains(s[1:], substr) ||
			containsCaseInsensitive(s, substr)))
}

func containsCaseInsensitive(s, substr string) bool {
	s = toLower(s)
	substr = toLower(substr)
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	result := make([]rune, len(s))
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			result[i] = r + 32
		} else {
			result[i] = r
		}
	}
	return string(result)
}

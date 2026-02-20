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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/controllers"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/tests/apitestutils"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/wiring"
)

// --- Mock and helpers for unit tests ---

// MockEvaluatorService is a mock implementation of EvaluatorManagerService
type MockEvaluatorService struct {
	mock.Mock
}

func (m *MockEvaluatorService) ListEvaluators(ctx context.Context, orgID *uuid.UUID, filters services.EvaluatorFilters) ([]*models.EvaluatorResponse, int32, error) {
	args := m.Called(ctx, orgID, filters)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int32), args.Error(2)
	}
	return args.Get(0).([]*models.EvaluatorResponse), args.Get(1).(int32), args.Error(2)
}

func (m *MockEvaluatorService) GetEvaluator(ctx context.Context, orgID *uuid.UUID, identifier string) (*models.EvaluatorResponse, error) {
	args := m.Called(ctx, orgID, identifier)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.EvaluatorResponse), args.Error(1)
}

func createMockEvaluators() []*models.EvaluatorResponse {
	now := time.Now()
	return []*models.EvaluatorResponse{
		{
			ID:          uuid.New(),
			Identifier:  "answer_relevancy",
			DisplayName: "Answer Relevancy",
			Description: "Checks if the answer is relevant to the input query",
			Version:     "1.0",
			Provider:    "standard",
			Tags:        []string{},
			IsBuiltin:   true,
			ConfigSchema: []models.EvaluatorConfigParam{
				{
					Key:         "min_overlap_ratio",
					Type:        "float",
					Description: "Minimum word overlap ratio",
					Required:    false,
					Default:     0.1,
					Min:         floatPtr(0.0),
					Max:         floatPtr(1.0),
				},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:          uuid.New(),
			Identifier:  "deepeval/tool-correctness",
			DisplayName: "Tool Correctness",
			Description: "Evaluates whether the agent selects the correct tools",
			Version:     "1.0",
			Provider:    "deepeval",
			Tags:        []string{"deepeval", "llm-judge", "action", "correctness"},
			IsBuiltin:   true,
			ConfigSchema: []models.EvaluatorConfigParam{
				{
					Key:         "threshold",
					Type:        "float",
					Description: "Minimum score for passing",
					Required:    false,
					Default:     0.7,
					Min:         floatPtr(0.0),
					Max:         floatPtr(1.0),
				},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:          uuid.New(),
			Identifier:  "deepeval/argument-correctness",
			DisplayName: "Argument Correctness",
			Description: "Evaluates whether the agent generates correct arguments for tool calls",
			Version:     "1.0",
			Provider:    "deepeval",
			Tags:        []string{"deepeval", "llm-judge", "action", "correctness"},
			IsBuiltin:   true,
			ConfigSchema: []models.EvaluatorConfigParam{
				{
					Key:         "threshold",
					Type:        "float",
					Description: "Minimum score for passing",
					Required:    false,
					Default:     0.7,
					Min:         floatPtr(0.0),
					Max:         floatPtr(1.0),
				},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}

func floatPtr(f float64) *float64 {
	return &f
}

// --- Unit tests (mock-based) ---

func TestListEvaluators_Success(t *testing.T) {
	mockService := new(MockEvaluatorService)
	controller := controllers.NewEvaluatorController(mockService)

	mockEvaluators := createMockEvaluators()

	mockService.On("ListEvaluators", mock.Anything, (*uuid.UUID)(nil), services.EvaluatorFilters{
		Limit:    20,
		Offset:   0,
		Tags:     nil,
		Search:   "",
		Provider: "",
	}).Return(mockEvaluators, int32(3), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators", nil)
	req.SetPathValue("orgName", "test-org")
	resp := httptest.NewRecorder()

	controller.ListEvaluators(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result spec.EvaluatorListResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, int32(3), result.Total)
	assert.Len(t, result.Evaluators, 3)
	assert.Equal(t, "answer_relevancy", result.Evaluators[0].Identifier)
	assert.Equal(t, "standard", result.Evaluators[0].Provider)

	mockService.AssertExpectations(t)
}

func TestListEvaluators_WithProviderFilter(t *testing.T) {
	mockService := new(MockEvaluatorService)
	controller := controllers.NewEvaluatorController(mockService)

	deepevalEvaluators := []*models.EvaluatorResponse{
		createMockEvaluators()[1], // deepeval/tool-correctness
		createMockEvaluators()[2], // deepeval/argument-correctness
	}

	mockService.On("ListEvaluators", mock.Anything, (*uuid.UUID)(nil), services.EvaluatorFilters{
		Limit:    20,
		Offset:   0,
		Tags:     nil,
		Search:   "",
		Provider: "deepeval",
	}).Return(deepevalEvaluators, int32(2), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators?provider=deepeval", nil)
	req.SetPathValue("orgName", "test-org")
	resp := httptest.NewRecorder()

	controller.ListEvaluators(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result spec.EvaluatorListResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, int32(2), result.Total)
	assert.Len(t, result.Evaluators, 2)
	for _, evaluator := range result.Evaluators {
		assert.Equal(t, "deepeval", evaluator.Provider)
	}

	mockService.AssertExpectations(t)
}

func TestListEvaluators_WithTagsFilter(t *testing.T) {
	mockService := new(MockEvaluatorService)
	controller := controllers.NewEvaluatorController(mockService)

	taggedEvaluators := []*models.EvaluatorResponse{
		createMockEvaluators()[1], // has deepeval and action tags
	}

	mockService.On("ListEvaluators", mock.Anything, (*uuid.UUID)(nil), services.EvaluatorFilters{
		Limit:    20,
		Offset:   0,
		Tags:     []string{"deepeval", "action"},
		Search:   "",
		Provider: "",
	}).Return(taggedEvaluators, int32(1), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators?tags=deepeval,action", nil)
	req.SetPathValue("orgName", "test-org")
	resp := httptest.NewRecorder()

	controller.ListEvaluators(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result spec.EvaluatorListResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, int32(1), result.Total)
	mockService.AssertExpectations(t)
}

func TestListEvaluators_WithSearchFilter(t *testing.T) {
	mockService := new(MockEvaluatorService)
	controller := controllers.NewEvaluatorController(mockService)

	searchResults := []*models.EvaluatorResponse{
		createMockEvaluators()[1], // tool-correctness
		createMockEvaluators()[2], // argument-correctness
	}

	mockService.On("ListEvaluators", mock.Anything, (*uuid.UUID)(nil), services.EvaluatorFilters{
		Limit:    20,
		Offset:   0,
		Tags:     nil,
		Search:   "correctness",
		Provider: "",
	}).Return(searchResults, int32(2), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators?search=correctness", nil)
	req.SetPathValue("orgName", "test-org")
	resp := httptest.NewRecorder()

	controller.ListEvaluators(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result spec.EvaluatorListResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, int32(2), result.Total)
	mockService.AssertExpectations(t)
}

func TestListEvaluators_WithPagination(t *testing.T) {
	mockService := new(MockEvaluatorService)
	controller := controllers.NewEvaluatorController(mockService)

	mockEvaluators := createMockEvaluators()[:2]

	mockService.On("ListEvaluators", mock.Anything, (*uuid.UUID)(nil), services.EvaluatorFilters{
		Limit:    5,
		Offset:   10,
		Tags:     nil,
		Search:   "",
		Provider: "",
	}).Return(mockEvaluators, int32(18), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators?limit=5&offset=10", nil)
	req.SetPathValue("orgName", "test-org")
	resp := httptest.NewRecorder()

	controller.ListEvaluators(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result spec.EvaluatorListResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, int32(18), result.Total)
	assert.Equal(t, int32(5), result.Limit)
	assert.Equal(t, int32(10), result.Offset)
	assert.Len(t, result.Evaluators, 2)

	mockService.AssertExpectations(t)
}

func TestListEvaluators_LimitCappedAt100(t *testing.T) {
	mockService := new(MockEvaluatorService)
	controller := controllers.NewEvaluatorController(mockService)

	mockEvaluators := createMockEvaluators()

	// Expect limit to be capped at 100
	mockService.On("ListEvaluators", mock.Anything, (*uuid.UUID)(nil), services.EvaluatorFilters{
		Limit:    100,
		Offset:   0,
		Tags:     nil,
		Search:   "",
		Provider: "",
	}).Return(mockEvaluators, int32(3), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators?limit=200", nil)
	req.SetPathValue("orgName", "test-org")
	resp := httptest.NewRecorder()

	controller.ListEvaluators(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result spec.EvaluatorListResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, int32(100), result.Limit) // Capped at 100

	mockService.AssertExpectations(t)
}

func TestListEvaluators_MissingOrgName(t *testing.T) {
	mockService := new(MockEvaluatorService)
	controller := controllers.NewEvaluatorController(mockService)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs//evaluators", nil)
	// Don't set orgName path value
	resp := httptest.NewRecorder()

	controller.ListEvaluators(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	// Service should not be called
	mockService.AssertNotCalled(t, "ListEvaluators")
}

func TestListEvaluators_ServiceError(t *testing.T) {
	mockService := new(MockEvaluatorService)
	controller := controllers.NewEvaluatorController(mockService)

	mockService.On("ListEvaluators", mock.Anything, (*uuid.UUID)(nil), mock.Anything).
		Return(nil, int32(0), assert.AnError)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators", nil)
	req.SetPathValue("orgName", "test-org")
	resp := httptest.NewRecorder()

	controller.ListEvaluators(resp, req)

	assert.Equal(t, http.StatusInternalServerError, resp.Code)

	mockService.AssertExpectations(t)
}

func TestGetEvaluator_Success(t *testing.T) {
	mockService := new(MockEvaluatorService)
	controller := controllers.NewEvaluatorController(mockService)

	mockEvaluator := createMockEvaluators()[0]

	mockService.On("GetEvaluator", mock.Anything, (*uuid.UUID)(nil), "answer_relevancy").
		Return(mockEvaluator, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators/answer_relevancy", nil)
	req.SetPathValue("orgName", "test-org")
	req.SetPathValue("evaluatorId", "answer_relevancy")
	resp := httptest.NewRecorder()

	controller.GetEvaluator(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result spec.EvaluatorResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "answer_relevancy", result.Identifier)
	assert.Equal(t, "Answer Relevancy", result.DisplayName)
	assert.Equal(t, "standard", result.Provider)
	assert.True(t, result.IsBuiltin)

	mockService.AssertExpectations(t)
}

func TestGetEvaluator_URLEncodedIdentifier(t *testing.T) {
	mockService := new(MockEvaluatorService)
	controller := controllers.NewEvaluatorController(mockService)

	mockEvaluator := createMockEvaluators()[1] // deepeval/tool-correctness

	// Service should receive decoded identifier
	mockService.On("GetEvaluator", mock.Anything, (*uuid.UUID)(nil), "deepeval/tool-correctness").
		Return(mockEvaluator, nil)

	// Request with URL-encoded identifier
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators/deepeval%2Ftool-correctness", nil)
	req.SetPathValue("orgName", "test-org")
	req.SetPathValue("evaluatorId", "deepeval%2Ftool-correctness")
	resp := httptest.NewRecorder()

	controller.GetEvaluator(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result spec.EvaluatorResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.Equal(t, "deepeval/tool-correctness", result.Identifier)

	mockService.AssertExpectations(t)
}

func TestGetEvaluator_NotFound(t *testing.T) {
	mockService := new(MockEvaluatorService)
	controller := controllers.NewEvaluatorController(mockService)

	mockService.On("GetEvaluator", mock.Anything, (*uuid.UUID)(nil), "nonexistent").
		Return(nil, utils.ErrEvaluatorNotFound)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators/nonexistent", nil)
	req.SetPathValue("orgName", "test-org")
	req.SetPathValue("evaluatorId", "nonexistent")
	resp := httptest.NewRecorder()

	controller.GetEvaluator(resp, req)

	assert.Equal(t, http.StatusNotFound, resp.Code)

	mockService.AssertExpectations(t)
}

func TestGetEvaluator_MissingOrgName(t *testing.T) {
	mockService := new(MockEvaluatorService)
	controller := controllers.NewEvaluatorController(mockService)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs//evaluators/answer_relevancy", nil)
	// Don't set orgName path value
	req.SetPathValue("evaluatorId", "answer_relevancy")
	resp := httptest.NewRecorder()

	controller.GetEvaluator(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	mockService.AssertNotCalled(t, "GetEvaluator")
}

func TestGetEvaluator_MissingEvaluatorId(t *testing.T) {
	mockService := new(MockEvaluatorService)
	controller := controllers.NewEvaluatorController(mockService)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators/", nil)
	req.SetPathValue("orgName", "test-org")
	// Don't set evaluatorId path value
	resp := httptest.NewRecorder()

	controller.GetEvaluator(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	mockService.AssertNotCalled(t, "GetEvaluator")
}

func TestGetEvaluator_ServiceError(t *testing.T) {
	mockService := new(MockEvaluatorService)
	controller := controllers.NewEvaluatorController(mockService)

	mockService.On("GetEvaluator", mock.Anything, (*uuid.UUID)(nil), "answer_relevancy").
		Return(nil, assert.AnError)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators/answer_relevancy", nil)
	req.SetPathValue("orgName", "test-org")
	req.SetPathValue("evaluatorId", "answer_relevancy")
	resp := httptest.NewRecorder()

	controller.GetEvaluator(resp, req)

	assert.Equal(t, http.StatusInternalServerError, resp.Code)

	mockService.AssertExpectations(t)
}

func TestGetEvaluator_ConfigSchemaConversion(t *testing.T) {
	mockService := new(MockEvaluatorService)
	controller := controllers.NewEvaluatorController(mockService)

	// Create evaluator with complex config schema
	now := time.Now()
	mockEvaluator := &models.EvaluatorResponse{
		ID:          uuid.New(),
		Identifier:  "test_evaluator",
		DisplayName: "Test Evaluator",
		Description: "Test description",
		Version:     "1.0",
		Provider:    "standard",
		Tags:        []string{"test"},
		IsBuiltin:   true,
		ConfigSchema: []models.EvaluatorConfigParam{
			{
				Key:         "string_param",
				Type:        "string",
				Description: "A string parameter",
				Required:    true,
				Default:     "default_value",
			},
			{
				Key:         "int_param",
				Type:        "integer",
				Description: "An integer parameter",
				Required:    false,
				Default:     42,
				Min:         floatPtr(0),
				Max:         floatPtr(100),
			},
			{
				Key:         "enum_param",
				Type:        "string",
				Description: "An enum parameter",
				Required:    false,
				EnumValues:  []string{"option1", "option2", "option3"},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	mockService.On("GetEvaluator", mock.Anything, (*uuid.UUID)(nil), "test_evaluator").
		Return(mockEvaluator, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators/test_evaluator", nil)
	req.SetPathValue("orgName", "test-org")
	req.SetPathValue("evaluatorId", "test_evaluator")
	resp := httptest.NewRecorder()

	controller.GetEvaluator(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)

	var result spec.EvaluatorResponse
	err := json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	// Verify config schema was converted correctly
	assert.Len(t, result.ConfigSchema, 3)

	// Check string param
	assert.Equal(t, "string_param", result.ConfigSchema[0].Key)
	assert.Equal(t, "string", result.ConfigSchema[0].Type)
	assert.True(t, result.ConfigSchema[0].Required)
	assert.NotNil(t, result.ConfigSchema[0].Default)

	// Check int param with min/max
	assert.Equal(t, "int_param", result.ConfigSchema[1].Key)
	assert.Equal(t, "integer", result.ConfigSchema[1].Type)
	assert.NotNil(t, result.ConfigSchema[1].Min)
	assert.NotNil(t, result.ConfigSchema[1].Max)
	assert.Equal(t, float64(0), *result.ConfigSchema[1].Min)
	assert.Equal(t, float64(100), *result.ConfigSchema[1].Max)

	// Check enum param
	assert.Equal(t, "enum_param", result.ConfigSchema[2].Key)
	assert.Equal(t, []string{"option1", "option2", "option3"}, result.ConfigSchema[2].EnumValues)

	mockService.AssertExpectations(t)
}

// --- Integration tests (full app with DB) ---

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

		// Should have 19 builtin evaluators from the migration (13 standard + 6 deepeval)
		assert.Equal(t, int32(19), result.Total)
		assert.Len(t, result.Evaluators, 19)

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

		// Should have 13 standard evaluators
		assert.Equal(t, int32(13), result.Total)
		assert.Len(t, result.Evaluators, 13)

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
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/test-org/evaluators?tags=deepeval,action", nil)
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
			assert.Contains(t, evaluator.Tags, "action")
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
			hasMatch := strings.Contains(strings.ToLower(evaluator.Identifier), "correctness") ||
				strings.Contains(strings.ToLower(evaluator.DisplayName), "correctness") ||
				strings.Contains(strings.ToLower(evaluator.Description), "correctness")
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
			hasMatch := strings.Contains(strings.ToLower(evaluator.Identifier), "tool") ||
				strings.Contains(strings.ToLower(evaluator.DisplayName), "tool") ||
				strings.Contains(strings.ToLower(evaluator.Description), "tool")
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

		assert.Equal(t, int32(19), result.Total)
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

		assert.Equal(t, int32(19), result.Total)
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

		assert.Equal(t, int32(19), result.Total)
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
			"hallucination",
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

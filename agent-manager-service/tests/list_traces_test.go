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

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/clientmocks"
	traceobserversvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/traceobserversvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/tests/apitestutils"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/wiring"
)

func createMockTraceObserverClient() *clientmocks.TraceObserverClientMock {
	return &clientmocks.TraceObserverClientMock{
		ListTracesFunc: func(ctx context.Context, params traceobserversvc.ListTracesParams) (*traceobserversvc.TraceOverviewResponse, error) {
			return &traceobserversvc.TraceOverviewResponse{
				Traces: []traceobserversvc.TraceOverview{
					{
						TraceID:         "trace-id-1",
						RootSpanID:      "root-span-1",
						RootSpanName:    "GET /api/endpoint",
						StartTime:       "2025-12-16T10:00:00Z",
						EndTime:         "2025-12-16T10:00:02Z",
						DurationInNanos: 2000000000,
						SpanCount:       5,
					},
					{
						TraceID:         "trace-id-2",
						RootSpanID:      "root-span-2",
						RootSpanName:    "POST /api/data",
						StartTime:       "2025-12-16T10:05:00Z",
						EndTime:         "2025-12-16T10:05:01Z",
						DurationInNanos: 1000000000,
						SpanCount:       3,
					},
				},
				TotalCount: 2,
			}, nil
		},
	}
}

func TestListTraces(t *testing.T) {
	// Create unique test data for this test suite
	tracesOrgId := uuid.New()
	tracesUserIdpId := uuid.New()
	tracesProjId := uuid.New()
	tracesOrgName := fmt.Sprintf("traces-org-%s", uuid.New().String()[:5])
	tracesProjName := fmt.Sprintf("traces-project-%s", uuid.New().String()[:5])
	tracesAgentName := fmt.Sprintf("traces-agent-%s", uuid.New().String()[:5])

	_ = apitestutils.CreateOrganization(t, tracesOrgId, tracesUserIdpId, tracesOrgName)
	_ = apitestutils.CreateProject(t, tracesProjId, tracesOrgId, tracesProjName)
	authMiddleware := jwtassertion.NewMockMiddleware(t, tracesOrgId, tracesUserIdpId)

	t.Run("Listing traces with default parameters should return 200", func(t *testing.T) {
		traceObserverClient := createMockTraceObserverClient()
		openChoreoClient := createMockOpenChoreoClient()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
			TraceObserverClient: traceObserverClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Send the request
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/traces", tracesOrgName, tracesProjName, tracesAgentName)
		req := httptest.NewRequest(http.MethodGet, url, nil)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response
		require.Equal(t, http.StatusOK, rr.Code)

		// Read and validate response body
		b, err := io.ReadAll(rr.Body)
		require.NoError(t, err)
		t.Logf("response body: %s", string(b))

		var response traceobserversvc.TraceOverviewResponse
		require.NoError(t, json.Unmarshal(b, &response))

		// Validate response fields
		require.Equal(t, 2, response.TotalCount)
		require.Len(t, response.Traces, 2)

		// Validate first trace
		trace1 := response.Traces[0]
		require.Equal(t, "trace-id-1", trace1.TraceID)
		require.Equal(t, "root-span-1", trace1.RootSpanID)
		require.Equal(t, "GET /api/endpoint", trace1.RootSpanName)
		require.Equal(t, int64(2000000000), trace1.DurationInNanos)
		require.Equal(t, 5, trace1.SpanCount)

		// Validate service calls
		require.Len(t, traceObserverClient.ListTracesCalls(), 1)

		// Validate call parameters
		listTracesCall := traceObserverClient.ListTracesCalls()[0]
		require.Equal(t, tracesAgentName, listTracesCall.Params.ServiceName)
		require.Equal(t, 10, listTracesCall.Params.Limit)         // default limit
		require.Equal(t, 0, listTracesCall.Params.Offset)         // default offset
		require.Equal(t, "desc", listTracesCall.Params.SortOrder) // default sort order
	})

	t.Run("Listing traces with custom pagination should return 200", func(t *testing.T) {
		traceObserverClient := createMockTraceObserverClient()
		openChoreoClient := createMockOpenChoreoClient()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
			TraceObserverClient: traceObserverClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Send the request with query parameters
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/traces?limit=20&offset=10&sortOrder=asc",
			tracesOrgName, tracesProjName, tracesAgentName)
		req := httptest.NewRequest(http.MethodGet, url, nil)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response
		require.Equal(t, http.StatusOK, rr.Code)

		// Validate service calls
		require.Len(t, traceObserverClient.ListTracesCalls(), 1)

		// Validate call parameters
		listTracesCall := traceObserverClient.ListTracesCalls()[0]
		require.Equal(t, tracesAgentName, listTracesCall.Params.ServiceName)
		require.Equal(t, 20, listTracesCall.Params.Limit)
		require.Equal(t, 10, listTracesCall.Params.Offset)
		require.Equal(t, "asc", listTracesCall.Params.SortOrder)
	})

	t.Run("Listing traces with time range should return 200", func(t *testing.T) {
		traceObserverClient := createMockTraceObserverClient()
		openChoreoClient := createMockOpenChoreoClient()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
			TraceObserverClient: traceObserverClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Send the request with time range
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/traces?startTime=2025-12-16T10:00:00Z&endTime=2025-12-16T11:00:00Z",
			tracesOrgName, tracesProjName, tracesAgentName)
		req := httptest.NewRequest(http.MethodGet, url, nil)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response
		require.Equal(t, http.StatusOK, rr.Code)

		// Validate service calls
		require.Len(t, traceObserverClient.ListTracesCalls(), 1)

		// Validate call parameters
		listTracesCall := traceObserverClient.ListTracesCalls()[0]
		require.Equal(t, "2025-12-16T10:00:00Z", listTracesCall.Params.StartTime)
		require.Equal(t, "2025-12-16T11:00:00Z", listTracesCall.Params.EndTime)
	})

	t.Run("Listing traces with invalid limit should return 400", func(t *testing.T) {
		traceObserverClient := createMockTraceObserverClient()
		openChoreoClient := createMockOpenChoreoClient()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
			TraceObserverClient: traceObserverClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Send the request with invalid limit
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/traces?limit=invalid",
			tracesOrgName, tracesProjName, tracesAgentName)
		req := httptest.NewRequest(http.MethodGet, url, nil)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response
		require.Equal(t, http.StatusBadRequest, rr.Code)

		// Validate no service calls were made
		require.Len(t, traceObserverClient.ListTracesCalls(), 0)
	})

	t.Run("Listing traces with invalid sortOrder should return 400", func(t *testing.T) {
		traceObserverClient := createMockTraceObserverClient()
		openChoreoClient := createMockOpenChoreoClient()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
			TraceObserverClient: traceObserverClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Send the request with invalid sortOrder
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/traces?sortOrder=invalid",
			tracesOrgName, tracesProjName, tracesAgentName)
		req := httptest.NewRequest(http.MethodGet, url, nil)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response
		require.Equal(t, http.StatusBadRequest, rr.Code)

		// Validate no service calls were made
		require.Len(t, traceObserverClient.ListTracesCalls(), 0)
	})
}

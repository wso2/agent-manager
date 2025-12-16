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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/clientmocks"
	traceobserversvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/traceobserversvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/tests/apitestutils"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/wiring"
)

func createMockTraceObserverClientWithDetails() *clientmocks.TraceObserverClientMock {
	return &clientmocks.TraceObserverClientMock{
		TraceDetailsByIdFunc: func(ctx context.Context, params traceobserversvc.TraceDetailsByIdParams) (*traceobserversvc.TraceResponse, error) {
			return &traceobserversvc.TraceResponse{
				Spans: []traceobserversvc.Span{
					{
						TraceID:         params.TraceID,
						SpanID:          "span-1",
						ParentSpanID:    "",
						Name:            "GET /api/endpoint",
						Service:         params.ServiceName,
						StartTime:       time.Date(2025, 12, 16, 10, 0, 0, 0, time.UTC),
						EndTime:         time.Date(2025, 12, 16, 10, 0, 2, 0, time.UTC),
						DurationInNanos: 2000000000,
						Kind:            "server",
						Status:          "ok",
						Attributes: map[string]interface{}{
							"http.method": "GET",
							"http.url":    "/api/endpoint",
							"http.status": 200,
						},
						Resource: map[string]interface{}{
							"service.name": params.ServiceName,
						},
					},
					{
						TraceID:         params.TraceID,
						SpanID:          "span-2",
						ParentSpanID:    "span-1",
						Name:            "database query",
						Service:         params.ServiceName,
						StartTime:       time.Date(2025, 12, 16, 10, 0, 0, 500000000, time.UTC),
						EndTime:         time.Date(2025, 12, 16, 10, 0, 1, 0, time.UTC),
						DurationInNanos: 500000000,
						Kind:            "client",
						Status:          "ok",
						Attributes: map[string]interface{}{
							"db.system":    "postgresql",
							"db.statement": "SELECT * FROM users",
						},
						Resource: map[string]interface{}{
							"service.name": params.ServiceName,
						},
					},
				},
				TotalCount: 2,
			}, nil
		},
	}
}

func TestGetTrace(t *testing.T) {
	// Create unique test data for this test suite
	traceDetailsOrgId := uuid.New()
	traceDetailsUserIdpId := uuid.New()
	traceDetailsProjId := uuid.New()
	traceDetailsOrgName := fmt.Sprintf("trace-details-org-%s", uuid.New().String()[:5])
	traceDetailsProjName := fmt.Sprintf("trace-details-project-%s", uuid.New().String()[:5])
	traceDetailsAgentName := fmt.Sprintf("trace-details-agent-%s", uuid.New().String()[:5])

	_ = apitestutils.CreateOrganization(t, traceDetailsOrgId, traceDetailsUserIdpId, traceDetailsOrgName)
	_ = apitestutils.CreateProject(t, traceDetailsProjId, traceDetailsOrgId, traceDetailsProjName)
	authMiddleware := jwtassertion.NewMockMiddleware(t, traceDetailsOrgId, traceDetailsUserIdpId)

	t.Run("Getting trace details with valid traceId should return 200", func(t *testing.T) {
		traceObserverClient := createMockTraceObserverClientWithDetails()
		openChoreoClient := createMockOpenChoreoClient()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
			TraceObserverClient: traceObserverClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Send the request
		traceID := "trace-id-123"
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/trace/%s",
			traceDetailsOrgName, traceDetailsProjName, traceDetailsAgentName, traceID)
		req := httptest.NewRequest(http.MethodGet, url, nil)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response
		require.Equal(t, http.StatusOK, rr.Code)

		// Read and validate response body
		b, err := io.ReadAll(rr.Body)
		require.NoError(t, err)
		t.Logf("response body: %s", string(b))

		var response traceobserversvc.TraceResponse
		require.NoError(t, json.Unmarshal(b, &response))

		// Validate response fields
		require.Equal(t, 2, response.TotalCount)
		require.Len(t, response.Spans, 2)

		// Validate first span (root span)
		span1 := response.Spans[0]
		require.Equal(t, traceID, span1.TraceID)
		require.Equal(t, "span-1", span1.SpanID)
		require.Equal(t, "", span1.ParentSpanID)
		require.Equal(t, "GET /api/endpoint", span1.Name)
		require.Equal(t, traceDetailsAgentName, span1.Service)
		require.Equal(t, int64(2000000000), span1.DurationInNanos)
		require.Equal(t, "server", span1.Kind)
		require.Equal(t, "ok", span1.Status)

		// Validate second span (child span)
		span2 := response.Spans[1]
		require.Equal(t, traceID, span2.TraceID)
		require.Equal(t, "span-2", span2.SpanID)
		require.Equal(t, "span-1", span2.ParentSpanID)
		require.Equal(t, "database query", span2.Name)
		require.Equal(t, "client", span2.Kind)

		// Validate service calls
		require.Len(t, traceObserverClient.TraceDetailsByIdCalls(), 1)

		// Validate call parameters
		traceDetailsCall := traceObserverClient.TraceDetailsByIdCalls()[0]
		require.Equal(t, traceID, traceDetailsCall.Params.TraceID)
		require.Equal(t, traceDetailsAgentName, traceDetailsCall.Params.ServiceName)
		// Note: limit and sortOrder are hardcoded internally and not exposed as API parameters
	})
}

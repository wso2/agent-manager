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
	"bytes"
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

	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/clients/clientmocks"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/models"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/spec"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/tests/apitestutils"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/utils"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/wiring"
)

var (
	testExternalOrgId        = uuid.New()
	testExternalProjId       = uuid.New()
	testExternalUserIdpId    = uuid.New()
	testExternalOrgName      = fmt.Sprintf("test-org-%s", uuid.New().String()[:5])
	testExternalProjName     = fmt.Sprintf("test-project-%s", uuid.New().String()[:5])
	testExternalAgentNameOne = fmt.Sprintf("test-external-agent-%s", uuid.New().String()[:5])
	testExternalAgentNameTwo = fmt.Sprintf("test-external-agent-%s", uuid.New().String()[:5])
)

func createMockOpenChoreoClientForExternal() *clientmocks.OpenChoreoSvcClientMock {
	return &clientmocks.OpenChoreoSvcClientMock{
		GetProjectFunc: func(ctx context.Context, projectName string, orgName string) (*models.ProjectResponse, error) {
			return &models.ProjectResponse{
				Name:        projectName,
				DisplayName: projectName,
				OrgName:     orgName,
				CreatedAt:   time.Now(),
			}, nil
		},
		// External agents don't need component creation in OpenChoreo
		IsAgentComponentExistsFunc: func(ctx context.Context, orgName string, projName string, agentName string) (bool, error) {
			return false, nil
		},
	}
}

func TestCreateExternalAgent(t *testing.T) {
	setUpExternalTest(t)
	authMiddleware := jwtassertion.NewMockMiddleware(t, testExternalOrgId, testExternalUserIdpId)

	t.Run("Creating an external agent should return 202", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForExternal()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Create the request body for external agent
		reqBody := new(bytes.Buffer)
		err := json.NewEncoder(reqBody).Encode(map[string]interface{}{
			"name":        testExternalAgentNameOne,
			"displayName": "Test External Agent",
			"description": "Test External Agent Description",
			"provisioning": map[string]interface{}{
				"type": "external",
			},
		})
		require.NoError(t, err)

		// Send the request
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testExternalOrgName, testExternalProjName)
		req := httptest.NewRequest(http.MethodPost, url, reqBody)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response
		require.Equal(t, http.StatusAccepted, rr.Code)

		// Read and validate response body
		b, err := io.ReadAll(rr.Body)
		require.NoError(t, err)
		t.Logf("response body: %s", string(b))

		var payload spec.AgentResponse
		require.NoError(t, json.Unmarshal(b, &payload))

		// Validate response fields
		require.Equal(t, testExternalAgentNameOne, payload.Name)
		require.Equal(t, "Test External Agent Description", payload.Description)
		require.Equal(t, testExternalProjName, payload.ProjectName)
		require.Equal(t, "external", payload.Provisioning.Type)
		require.NotZero(t, payload.CreatedAt)
	})

	externalAgentValidationTests := []struct {
		name           string
		authMiddleware jwtassertion.Middleware
		payload        map[string]interface{}
		wantStatus     int
		wantErrMsg     string
		url            string
		setupMock      func() *clientmocks.OpenChoreoSvcClientMock
	}{
		{
			name:           "return 400 on missing agent name for external agent",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"displayName": "Test External Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "external",
				},
			},
			wantStatus: 400,
			wantErrMsg: "invalid agent name: agent name cannot be empty",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testExternalOrgName, testExternalProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForExternal()
			},
		},
		{
			name:           "return 400 on invalid agent name for external agent",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        "Invalid Agent Name!", // Invalid characters
				"displayName": "Test External Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "external",
				},
			},
			wantStatus: 400,
			wantErrMsg: "invalid agent name: agent name must contain only lowercase alphanumeric characters or '-'",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testExternalOrgName, testExternalProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForExternal()
			},
		},
		{
			name:           "return 400 on missing display name for external agent",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-external-agent-%s", uuid.New().String()[:5]),
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "external",
				},
			},
			wantStatus: 400,
			wantErrMsg: "invalid agent display name: agent name cannot be empty",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testExternalOrgName, testExternalProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForExternal()
			},
		},
		{
			name:           "return 400 on missing provisioning type for external agent",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":         fmt.Sprintf("test-external-agent-%s", uuid.New().String()[:5]),
				"displayName":  "Test External Agent",
				"description":  "Test description",
				"provisioning": map[string]interface{}{
					// Missing "type" field
				},
			},
			wantStatus: 400,
			wantErrMsg: "provisioning type must be either 'internal' or 'external'",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testExternalOrgName, testExternalProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForExternal()
			},
		},
		{
			name:           "return 400 on invalid provisioning type for external agent",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-external-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test External Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "managed", // Invalid type
				},
			},
			wantStatus: 400,
			wantErrMsg: "provisioning type must be either 'internal' or 'external'",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testExternalOrgName, testExternalProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForExternal()
			},
		},
		{
			name:           "return 404 on organization not found for external agent",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-external-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test External Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "external",
				},
			},
			wantStatus: 404,
			wantErrMsg: "Organization not found",
			url:        fmt.Sprintf("/api/v1/orgs/nonexistent-org/projects/%s/agents", testExternalProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForExternal()
			},
		},
		{
			name:           "return 404 on project not found for external agent",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-external-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test External Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "external",
				},
			},
			wantStatus: 404,
			wantErrMsg: "Project not found",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/nonexistent-project/agents", testExternalOrgName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForExternal()
			},
		},
		{
			name:           "return 409 on external agent already exists",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        testExternalAgentNameOne, // Use existing agent name
				"displayName": "Test External Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "external",
				},
			},
			wantStatus: 409,
			wantErrMsg: "Agent already exists",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testExternalOrgName, testExternalProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForExternal()
			},
		},
		{
			name: "return 401 on missing authentication for external agent",
			authMiddleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					utils.WriteErrorResponse(w, http.StatusUnauthorized, "missing header: Authorization")
				})
			},
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-external-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test External Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "external",
				},
			},
			wantStatus: 401,
			wantErrMsg: "missing header: Authorization",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testExternalOrgName, testExternalProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForExternal()
			},
		},
	}

	for _, tt := range externalAgentValidationTests {
		t.Run(tt.name, func(t *testing.T) {
			openChoreoClient := tt.setupMock()
			testClients := wiring.TestClients{
				OpenChoreoSvcClient: openChoreoClient,
			}

			app := apitestutils.MakeAppClientWithDeps(t, testClients, tt.authMiddleware)

			reqBody := new(bytes.Buffer)
			err := json.NewEncoder(reqBody).Encode(tt.payload)
			require.NoError(t, err)

			// Send the request
			req := httptest.NewRequest(http.MethodPost, tt.url, reqBody)
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			// Assert response
			require.Equal(t, tt.wantStatus, rr.Code)

			// Read response body and check error message
			body, err := io.ReadAll(rr.Body)
			require.NoError(t, err)

			if tt.wantStatus >= 400 {
				// For error responses, check that the error message is contained in the response
				bodyStr := string(body)
				require.Contains(t, bodyStr, tt.wantErrMsg)
			} else if tt.wantStatus == 202 {
				// For success responses, validate the response structure
				var payload spec.AgentResponse
				require.NoError(t, json.Unmarshal(body, &payload))
				require.Equal(t, "external", payload.Provisioning.Type)
			}
		})
	}
}

func setUpExternalTest(t *testing.T) {
	_ = apitestutils.CreateOrganization(t, testExternalOrgId, testExternalUserIdpId, testExternalOrgName)
	_ = apitestutils.CreateProject(t, testExternalProjId, testExternalOrgId, testExternalProjName)
}

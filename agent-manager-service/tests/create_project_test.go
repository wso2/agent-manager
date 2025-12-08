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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/clientmocks"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/tests/apitestutils"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/wiring"
)

var (
	testCreateProjectOrgId     = uuid.New()
	testCreateProjectUserIdpId = uuid.New()
	testCreateProjectOrgName   = fmt.Sprintf("test-org-%s", uuid.New().String()[:5])
	testCreateProjectName      = fmt.Sprintf("test-project-%s", uuid.New().String()[:5])
)

func createMockOpenChoreoClientForCreateProject() *clientmocks.OpenChoreoSvcClientMock {
	return &clientmocks.OpenChoreoSvcClientMock{
		GetDeploymentPipelinesForOrganizationFunc: func(ctx context.Context, orgName string) ([]*models.DeploymentPipelineResponse, error) {
			return []*models.DeploymentPipelineResponse{
				{
					Name:        "default",
					DisplayName: "Default Pipeline",
					OrgName:     orgName,
				},
			}, nil
		},
		CreateProjectFunc: func(ctx context.Context, orgName, projectName, deploymentPipeline, displayName string) error {
			return nil
		},
	}
}

func TestCreateProject(t *testing.T) {
	setUpCreateProjectTest(t)
	authMiddleware := jwtassertion.NewMockMiddleware(t, testCreateProjectOrgId, testCreateProjectUserIdpId)

	t.Run("Creating a project with valid data should return 202", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForCreateProject()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Create request payload
		payload := spec.CreateProjectRequest{
			Name:               testCreateProjectName,
			DisplayName:        "Test Project Display",
			Description:        stringPtr("Test project description"),
			DeploymentPipeline: "default",
		}

		body, err := json.Marshal(payload)
		require.NoError(t, err)

		// Send the create request
		url := fmt.Sprintf("/api/v1/orgs/%s/projects", testCreateProjectOrgName)
		req := httptest.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response
		require.Equal(t, http.StatusAccepted, rr.Code)

		// Parse response
		var response spec.ProjectResponse
		err = json.Unmarshal(rr.Body.Bytes(), &response)
		require.NoError(t, err)

		// Validate response fields
		require.Equal(t, payload.Name, response.Name)
		require.Equal(t, testCreateProjectOrgName, response.OrgName)
		require.Equal(t, payload.DisplayName, response.DisplayName)
		require.NotZero(t, response.CreatedAt)

		// Validate service calls
		require.Len(t, openChoreoClient.GetDeploymentPipelinesForOrganizationCalls(), 1)
		require.Len(t, openChoreoClient.CreateProjectCalls(), 1)

		// Validate call parameters
		createCall := openChoreoClient.CreateProjectCalls()[0]
		require.Equal(t, testCreateProjectOrgName, createCall.OrgName)
		require.Equal(t, payload.Name, createCall.ProjectName)
		require.Equal(t, payload.DeploymentPipeline, createCall.DeploymentPipelineRef)
		require.Equal(t, payload.DisplayName, createCall.ProjectDisplayName)
	})

	validationTests := []struct {
		name           string
		authMiddleware jwtassertion.Middleware
		wantStatus     int
		wantErrMsg     string
		url            string
		payload        interface{}
		setupMock      func() *clientmocks.OpenChoreoSvcClientMock
	}{
		{
			name:           "return 400 on invalid project name",
			authMiddleware: authMiddleware,
			wantStatus:     400,
			wantErrMsg:     "Invalid project name",
			url:            fmt.Sprintf("/api/v1/orgs/%s/projects", testCreateProjectOrgName),
			payload: spec.CreateProjectRequest{
				Name:               "INVALID-PROJECT-NAME!",
				DisplayName:        "Test Project",
				DeploymentPipeline: "default",
			},
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForCreateProject()
			},
		},
		{
			name:           "return 400 on missing deployment pipeline",
			authMiddleware: authMiddleware,
			wantStatus:     400,
			wantErrMsg:     "Missing deployment pipeline in request body",
			url:            fmt.Sprintf("/api/v1/orgs/%s/projects", testCreateProjectOrgName),
			payload: spec.CreateProjectRequest{
				Name:        "valid-project",
				DisplayName: "Test Project",
				// Missing DeploymentPipeline
			},
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForCreateProject()
			},
		},
		{
			name:           "return 404 on organization not found",
			authMiddleware: authMiddleware,
			wantStatus:     404,
			wantErrMsg:     "Organization not found",
			url:            "/api/v1/orgs/nonexistent-org/projects",
			payload: spec.CreateProjectRequest{
				Name:               "valid-project",
				DisplayName:        "Test Project",
				DeploymentPipeline: "default",
			},
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForCreateProject()
			},
		},
		{
			name:           "return 409 on project already exists",
			authMiddleware: authMiddleware,
			wantStatus:     409,
			wantErrMsg:     "Project already exists",
			url:            fmt.Sprintf("/api/v1/orgs/%s/projects", testCreateProjectOrgName),
			payload: spec.CreateProjectRequest{
				Name:               "existing-project",
				DisplayName:        "Existing Project",
				DeploymentPipeline: "default",
			},
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForCreateProject()
			},
		},
		{
			name: "return 401 on missing authentication",
			authMiddleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing header: Authorization"})
				})
			},
			wantStatus: 401,
			wantErrMsg: "missing header: Authorization",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects", testCreateProjectOrgName),
			payload: spec.CreateProjectRequest{
				Name:               "test-project",
				DisplayName:        "Test Project",
				DeploymentPipeline: "default",
			},
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForCreateProject()
			},
		},
		{
			name:           "return 500 on deployment pipeline not found",
			authMiddleware: authMiddleware,
			wantStatus:     500,
			wantErrMsg:     "Failed to create project",
			url:            fmt.Sprintf("/api/v1/orgs/%s/projects", testCreateProjectOrgName),
			payload: spec.CreateProjectRequest{
				Name:               "test-project",
				DisplayName:        "Test Project",
				DeploymentPipeline: "nonexistent-pipeline",
			},
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				mock := createMockOpenChoreoClientForCreateProject()
				return mock
			},
		},
		{
			name:           "return 500 on OpenChoreo project creation failure",
			authMiddleware: authMiddleware,
			wantStatus:     500,
			wantErrMsg:     "Failed to create project",
			url:            fmt.Sprintf("/api/v1/orgs/%s/projects", testCreateProjectOrgName),
			payload: spec.CreateProjectRequest{
				Name:               "failing-project",
				DisplayName:        "Failing Project",
				DeploymentPipeline: "default",
			},
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				mock := createMockOpenChoreoClientForCreateProject()
				mock.CreateProjectFunc = func(ctx context.Context, orgName, projectName, deploymentPipeline, displayName string) error {
					return fmt.Errorf("OpenChoreo service error")
				}
				return mock
			},
		},
	}

	for _, tt := range validationTests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test data if needed
			if tt.name == "return 409 on project already exists" {
				// Create an existing project
				existingProjectId := uuid.New()
				_ = apitestutils.CreateProject(t, existingProjectId, testCreateProjectOrgId, "existing-project")
			}

			openChoreoClient := tt.setupMock()
			testClients := wiring.TestClients{
				OpenChoreoSvcClient: openChoreoClient,
			}

			app := apitestutils.MakeAppClientWithDeps(t, testClients, tt.authMiddleware)

			// Create request body
			var body []byte
			var err error
			if str, ok := tt.payload.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.payload)
				require.NoError(t, err)
			}

			// Send the create request
			req := httptest.NewRequest(http.MethodPost, tt.url, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			// Assert response
			require.Equal(t, tt.wantStatus, rr.Code)

			// Check error message for error responses
			if tt.wantStatus >= 400 {
				body := rr.Body.String()
				require.Contains(t, body, tt.wantErrMsg)
			}
		})
	}
}

func setUpCreateProjectTest(t *testing.T) {
	_ = apitestutils.CreateOrganization(t, testCreateProjectOrgId, testCreateProjectUserIdpId, testCreateProjectOrgName)
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}

//go:build integration

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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

var testUpdateProjectOrgName = fmt.Sprintf("test-org-%s", uuid.New().String()[:5])

func TestUpdateProject(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("updating the project display name and description returns 200", func(t *testing.T) {
		platformRepo := apitestutils.CreateMockPlatformRepository()
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{PlatformRepository: platformRepo}, authMiddleware)

		payload := spec.UpdateProjectRequest{
			DisplayName:        "Updated Project",
			Description:        "updated description",
			DeploymentPipeline: "new-pipeline",
		}
		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/my-project", testUpdateProjectOrgName)
		req := httptest.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)

		require.Len(t, platformRepo.PatchProjectCalls(), 1)
		call := platformRepo.PatchProjectCalls()[0]
		require.Equal(t, "my-project", call.ProjectName)
		require.NotNil(t, call.DisplayName)
		require.Equal(t, "Updated Project", *call.DisplayName)
	})

	t.Run("returns 400 on invalid display name", func(t *testing.T) {
		platformRepo := apitestutils.CreateMockPlatformRepository()
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{PlatformRepository: platformRepo}, authMiddleware)

		payload := spec.UpdateProjectRequest{DisplayName: "", Description: "d", DeploymentPipeline: "new-pipeline"}
		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/my-project", testUpdateProjectOrgName)
		req := httptest.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Empty(t, platformRepo.PatchProjectCalls())
	})

	t.Run("returns 404 when the organization is not found", func(t *testing.T) {
		platformRepo := apitestutils.CreateMockPlatformRepository()
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{PlatformRepository: platformRepo}, jwtassertion.NewMockMiddlewareWithOUID(t, "nonexistent-org"))

		payload := spec.UpdateProjectRequest{DisplayName: "Updated", Description: "d", DeploymentPipeline: "new-pipeline"}
		body, _ := json.Marshal(payload)
		url := "/api/v1/orgs/nonexistent-org/projects/my-project"
		req := httptest.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusNotFound, rr.Code)
		require.Contains(t, rr.Body.String(), "Organization not found")
	})

	t.Run("returns 500 when the patch fails", func(t *testing.T) {
		platformRepo := apitestutils.CreateMockPlatformRepository()
		platformRepo.PatchProjectFunc = func(ctx context.Context, ouID, projectName string, displayName, description *string) error {
			return fmt.Errorf("repository error")
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{PlatformRepository: platformRepo}, authMiddleware)

		payload := spec.UpdateProjectRequest{DisplayName: "Updated", Description: "d", DeploymentPipeline: "new-pipeline"}
		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/my-project", testUpdateProjectOrgName)
		req := httptest.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusInternalServerError, rr.Code)
	})
}

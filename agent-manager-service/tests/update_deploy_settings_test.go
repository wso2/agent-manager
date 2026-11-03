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
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

var testDeploySettingsOrgName = fmt.Sprintf("test-org-%s", uuid.New().String()[:5])

// apiAgentComponentFunc returns a GetComponent mock that yields an internal API-type agent.
func apiAgentComponentFunc() func(ctx context.Context, namespaceName, projectName, componentName string) (*models.AgentResponse, error) {
	return func(ctx context.Context, namespaceName, projectName, componentName string) (*models.AgentResponse, error) {
		return &models.AgentResponse{
			UUID:         "component-uid-123",
			Name:         componentName,
			ProjectName:  projectName,
			Type:         models.AgentType{Type: string(utils.AgentTypeAPI)},
			Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
		}, nil
	}
}

func TestUpdateAgentDeploySettings(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)
	agentName := fmt.Sprintf("agent-%s", uuid.New().String()[:8])

	deploySettingsURL := func(org string) string {
		return fmt.Sprintf("/api/v1/orgs/%s/projects/my-project/agents/%s/deploy-settings", org, agentName)
	}

	t.Run("updating deploy settings for an API agent returns 204", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.GetComponentFunc = apiAgentComponentFunc()
		ocClient.GetEnvironmentFunc = func(ctx context.Context, namespaceName, environmentName string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{UUID: uuid.New().String(), Name: environmentName}, nil
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.UpdateAgentDeploySettingsRequest{
			EnvironmentName:           "development",
			EnableAutoInstrumentation: boolPtr(true),
			EnableApiKeySecurity:      boolPtr(false),
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, deploySettingsURL(testDeploySettingsOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusNoContent, rr.Code)
		require.Len(t, ocClient.UpdateReleaseBindingTraitConfigsCalls(), 1)
		call := ocClient.UpdateReleaseBindingTraitConfigsCalls()[0]
		require.Equal(t, agentName, call.ComponentName)
		require.Equal(t, "development", call.Environment)
	})

	t.Run("returns 400 when environmentName is missing", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.UpdateAgentDeploySettingsRequest{EnableAutoInstrumentation: boolPtr(true)}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, deploySettingsURL(testDeploySettingsOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), "environmentName is required")
	})

	t.Run("returns 400 when the agent is not API-type", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		// Default mock returns an agent with an empty type, which is not API-type.
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.UpdateAgentDeploySettingsRequest{EnvironmentName: "development", EnableAutoInstrumentation: boolPtr(true)}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, deploySettingsURL(testDeploySettingsOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Empty(t, ocClient.UpdateReleaseBindingTraitConfigsCalls())
	})

	t.Run("returns 404 when the agent is not found", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.GetComponentFunc = func(ctx context.Context, namespaceName, projectName, componentName string) (*models.AgentResponse, error) {
			return nil, utils.ErrNotFound
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.UpdateAgentDeploySettingsRequest{EnvironmentName: "development", EnableAutoInstrumentation: boolPtr(true)}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, deploySettingsURL(testDeploySettingsOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusNotFound, rr.Code)
		require.Contains(t, rr.Body.String(), "Agent not found")
	})
}

func TestDeploySettingsNotRefusedWithoutGateway(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)
	agentName := fmt.Sprintf("agent-%s", uuid.New().String()[:8])
	ocClient := apitestutils.CreateMockOpenChoreoClient()
	ocClient.GetComponentFunc = apiAgentComponentFunc()
	app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)
	body, _ := json.Marshal(spec.UpdateAgentDeploySettingsRequest{
		EnvironmentName:      "development",
		EnableApiKeySecurity: boolPtr(false),
	})
	url := fmt.Sprintf("/api/v1/orgs/%s/projects/my-project/agents/%s/deploy-settings", testDeploySettingsOrgName, agentName)
	req := httptest.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	app.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNoContent, rr.Code)
}

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

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

var testConfigurationsOrgName = fmt.Sprintf("test-org-%s", uuid.New().String()[:5])

func TestUpdateAgentConfigurations(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)
	agentName := fmt.Sprintf("agent-%s", uuid.New().String()[:8])

	configurationsURL := func(org string) string {
		return fmt.Sprintf("/api/v1/orgs/%s/projects/my-project/agents/%s/configurations", org, agentName)
	}

	t.Run("replacing env vars returns 204", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.ReplaceReleaseBindingWorkloadOverridesFunc = func(ctx context.Context, namespaceName, componentName, environment string, envOverrides []client.EnvVar, fileOverrides []client.FileVar, probeTimings *client.HealthCheckTimings) error {
			return nil
		}
		testClients := wiring.TestClients{
			OpenChoreoClient: ocClient,
			SecretMgmtClient: apitestutils.CreateMockSecretManagementClient(),
		}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		payload := spec.UpdateAgentConfigurationsRequest{
			EnvironmentName: "development",
			Env:             []spec.EnvironmentVariable{{Key: "LOG_LEVEL", Value: stringPtr("debug")}},
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, configurationsURL(testConfigurationsOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusNoContent, rr.Code)
		require.Len(t, ocClient.ReplaceReleaseBindingWorkloadOverridesCalls(), 1)
		call := ocClient.ReplaceReleaseBindingWorkloadOverridesCalls()[0]
		require.Equal(t, agentName, call.ComponentName)
		require.Equal(t, "development", call.Environment)
	})

	t.Run("saving health check wait times returns 204", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.GetEnvHealthChecksFunc = func(ctx context.Context, ouID, componentName, environment string) (*client.HealthChecks, error) {
			return &client.HealthChecks{}, nil
		}
		testClients := wiring.TestClients{
			OpenChoreoClient: ocClient,
			SecretMgmtClient: apitestutils.CreateMockSecretManagementClient(),
		}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		body := []byte(`{"environmentName":"development","probes":{"startup":{"failureThreshold":80}}}`)
		req := httptest.NewRequest(http.MethodPut, configurationsURL(testConfigurationsOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusNoContent, rr.Code)
		require.Len(t, ocClient.ReplaceReleaseBindingWorkloadOverridesCalls(), 1)
		call := ocClient.ReplaceReleaseBindingWorkloadOverridesCalls()[0]
		require.NotNil(t, call.ProbeTimings)
		require.NotNil(t, call.ProbeTimings.Startup)
		require.Equal(t, int32(80), *call.ProbeTimings.Startup.FailureThreshold)
	})

	t.Run("returns 400 for a health check wait time out of range", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.GetEnvHealthChecksFunc = func(ctx context.Context, ouID, componentName, environment string) (*client.HealthChecks, error) {
			return &client.HealthChecks{}, nil
		}
		testClients := wiring.TestClients{
			OpenChoreoClient: ocClient,
			SecretMgmtClient: apitestutils.CreateMockSecretManagementClient(),
		}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		body := []byte(`{"environmentName":"development","probes":{"readiness":{"periodSeconds":0}}}`)
		req := httptest.NewRequest(http.MethodPut, configurationsURL(testConfigurationsOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), "readiness periodSeconds must be between 1 and 3600")
		require.Empty(t, ocClient.ReplaceReleaseBindingWorkloadOverridesCalls())
	})

	t.Run("returns 400 when environmentName is missing", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.UpdateAgentConfigurationsRequest{
			Env: []spec.EnvironmentVariable{{Key: "LOG_LEVEL", Value: stringPtr("debug")}},
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, configurationsURL(testConfigurationsOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Contains(t, rr.Body.String(), "environmentName is required")
		require.Empty(t, ocClient.ReplaceReleaseBindingWorkloadOverridesCalls())
	})

	t.Run("returns 400 when neither env nor files are provided", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		testClients := wiring.TestClients{
			OpenChoreoClient: ocClient,
			SecretMgmtClient: apitestutils.CreateMockSecretManagementClient(),
		}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		payload := spec.UpdateAgentConfigurationsRequest{EnvironmentName: "development"}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, configurationsURL(testConfigurationsOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.Empty(t, ocClient.ReplaceReleaseBindingWorkloadOverridesCalls())
	})

	t.Run("returns 404 when the agent is not found", func(t *testing.T) {
		ocClient := apitestutils.CreateMockOpenChoreoClient()
		ocClient.GetComponentFunc = func(ctx context.Context, namespaceName, projectName, componentName string) (*models.AgentResponse, error) {
			return nil, utils.ErrNotFound
		}
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: ocClient}, authMiddleware)

		payload := spec.UpdateAgentConfigurationsRequest{
			EnvironmentName: "development",
			Env:             []spec.EnvironmentVariable{{Key: "LOG_LEVEL", Value: stringPtr("debug")}},
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPut, configurationsURL(testConfigurationsOrgName), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusNotFound, rr.Code)
		require.Contains(t, rr.Body.String(), "Agent not found")
	})
}

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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

var testAgentNameA2A = fmt.Sprintf("a2a-agent-%s", uuid.New().String()[:5])

// An a2a-agent has no REST API, so the api-configuration trait must not be
// attached at create. The positive assertion this mirrors is in
// create_agent_test.go, where a docker chat agent gets exactly two traits with
// TraitAPIManagement second.
func TestCreateA2AAgentOmitsAPIConfigurationTrait(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
	openChoreoClient.GetComponentFunc = func(ctx context.Context, namespaceName, projectName, componentName string) (*models.AgentResponse, error) {
		return &models.AgentResponse{
			UUID:         uuid.New().String(),
			Name:         componentName,
			ProjectName:  projectName,
			Provisioning: models.Provisioning{Type: "internal"},
			CreatedAt:    time.Now(),
		}, nil
	}

	testClients := wiring.TestClients{
		OpenChoreoClient: openChoreoClient,
		SecretMgmtClient: apitestutils.CreateMockSecretManagementClient(),
	}
	app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

	reqBody := new(bytes.Buffer)
	require.NoError(t, json.NewEncoder(reqBody).Encode(map[string]interface{}{
		"name":        testAgentNameA2A,
		"displayName": "A2A Trip Planner",
		"description": "A2A Agent Description",
		"provisioning": map[string]interface{}{
			"type": "internal",
			"repository": map[string]interface{}{
				"url":     "https://github.com/example/a2a-agent",
				"branch":  "main",
				"appPath": "/",
			},
		},
		"agentType": map[string]interface{}{
			"type":    "agent-api",
			"subType": "a2a-agent",
		},
		"build": map[string]interface{}{
			"type":   "docker",
			"docker": map[string]interface{}{"dockerfilePath": "/Dockerfile"},
		},
		"inputInterface": map[string]interface{}{
			"type": "HTTP",
			"port": 9099,
		},
	}))

	req := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
		reqBody,
	)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)

	require.Equal(t, http.StatusAccepted, rr.Code, "body: %s", rr.Body.String())

	attachTraitsCalls := openChoreoClient.AttachTraitsCalls()
	require.Len(t, attachTraitsCalls, 1)
	for _, tr := range attachTraitsCalls[0].TraitRequests {
		require.NotEqual(t, client.TraitAPIManagement, tr.TraitType,
			"an a2a-agent must not be given the api-configuration trait")
	}
	require.NotEmpty(t, attachTraitsCalls[0].TraitRequests,
		"the instrumentation traits are still attached")
}

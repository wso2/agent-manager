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
//go:build integration

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

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// A same-named agent must not inherit a deleted agent's card row, its URL or its source.
func TestCreateAgentClearsALeftoverA2ACardRow(t *testing.T) {
	const ouID = "mock-org-id"
	agentName := fmt.Sprintf("card-heir-%s", uuid.New().String()[:5])
	cardRepo := repositories.NewA2AAgentCardRepository(db.GetDB())
	t.Cleanup(func() { _ = cardRepo.DeleteForAgent(context.Background(), ouID, testProjName, agentName) })
	require.NoError(t, cardRepo.Enqueue(context.Background(), &models.A2AAgentCard{
		OUID: ouID, ProjectName: testProjName, AgentName: agentName, EnvironmentName: "Development", EnvironmentUUID: uuid.New(),
		Source: models.A2AAgentCardSourceExternal, SourceURL: "https://third-party.example/.well-known/agent-card.json",
	}))

	openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
	app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{
		OpenChoreoClient: openChoreoClient,
		SecretMgmtClient: apitestutils.CreateMockSecretManagementClient(),
	}, jwtassertion.NewMockMiddleware(t))

	reqBody := new(bytes.Buffer)
	require.NoError(t, json.NewEncoder(reqBody).Encode(map[string]interface{}{
		"name":        agentName,
		"displayName": "Card Heir",
		"provisioning": map[string]interface{}{
			"type": "internal",
			"repository": map[string]interface{}{
				"url": "https://github.com/test/test-repo", "branch": "main", "appPath": "/agent",
			},
		},
		"agentType": map[string]interface{}{"type": "agent-api", "subType": "a2a-agent"},
		"build": map[string]interface{}{
			"type": "buildpack",
			"buildpack": map[string]interface{}{
				"language": "python", "languageVersion": "3.11", "runCommand": "python main.py",
			},
		},
		"inputInterface": map[string]interface{}{"type": "HTTP", "port": 8000},
	}))
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName), reqBody)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)
	require.Equal(t, http.StatusAccepted, rr.Code, rr.Body.String())

	rows, err := cardRepo.ListForAgent(context.Background(), ouID, testProjName, agentName)
	require.NoError(t, err)
	require.Empty(t, rows)
}

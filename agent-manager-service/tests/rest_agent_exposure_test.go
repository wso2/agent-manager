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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// A chat-api agent is created and deployed with no route trait: the create's RestApi artifact is what the deploy
// exposes on the environment's ingress gateway, owed as a PENDING row.
func TestCreateThenDeployRestAgent(t *testing.T) {
	ouID := "mock-org-id"
	orgName := fmt.Sprintf("rest-org-%s", uuid.NewString()[:5])
	projName := fmt.Sprintf("rest-proj-%s", uuid.NewString()[:5])
	agentName := fmt.Sprintf("rest-agent-%s", uuid.NewString()[:5])
	environmentUUID := uuid.New()
	gatewayID := seedIngressGateway(t, ouID, environmentUUID)

	openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
	openChoreoClient.ComponentExistsFunc = func(context.Context, string, string, string) (bool, error) { return true, nil }
	openChoreoClient.GetComponentFunc = func(_ context.Context, _, projectName, componentName string) (*models.AgentResponse, error) {
		return &models.AgentResponse{
			UUID: uuid.NewString(), Name: componentName, ProjectName: projectName,
			Provisioning: models.Provisioning{Type: "internal"},
			Type:         models.AgentType{Type: "agent-api", SubType: "chat-api"},
			CreatedAt:    time.Now(),
		}, nil
	}
	openChoreoClient.GetEnvironmentFunc = func(_ context.Context, _, environmentName string) (*models.EnvironmentResponse, error) {
		return &models.EnvironmentResponse{UUID: environmentUUID.String(), Name: environmentName}, nil
	}
	app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{
		OpenChoreoClient: openChoreoClient,
		SecretMgmtClient: apitestutils.CreateMockSecretManagementClient(),
	}, jwtassertion.NewMockMiddleware(t))
	gdb := db.GetDB()
	exposures := repositories.NewPlatformAgentExposureRepository(gdb)
	t.Cleanup(func() { _ = exposures.DeleteForAgent(context.Background(), ouID, projName, agentName) })

	post := func(t *testing.T, url string, body map[string]interface{}) {
		t.Helper()
		buf := new(bytes.Buffer)
		require.NoError(t, json.NewEncoder(buf).Encode(body))
		req := httptest.NewRequest(http.MethodPost, url, buf)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)
		require.Equal(t, http.StatusAccepted, rr.Code, "body: %s", rr.Body.String())
	}

	post(t, fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", orgName, projName), map[string]interface{}{
		"name":        agentName,
		"displayName": "REST Agent",
		"provisioning": map[string]interface{}{
			"type":       "internal",
			"repository": map[string]interface{}{"url": "https://github.com/example/rest-agent", "branch": "main", "appPath": "/"},
		},
		"agentType":      map[string]interface{}{"type": "agent-api", "subType": "chat-api"},
		"build":          map[string]interface{}{"type": "docker", "docker": map[string]interface{}{"dockerfilePath": "/Dockerfile"}},
		"inputInterface": map[string]interface{}{"type": "HTTP", "port": 8000},
	})

	for _, call := range openChoreoClient.AttachTraitsCalls() {
		for _, trait := range call.TraitRequests {
			assert.NotEqual(t, client.TraitAPIManagement, trait.TraitType)
			assert.NotEqual(t, client.TraitA2AGatewayRoute, trait.TraitType)
		}
	}
	created, err := exposures.Get(context.Background(), ouID, projName, agentName, environmentUUID)
	require.NoError(t, err, "create exposes the agent in its first environment")
	assert.Equal(t, models.AgentExposureProtocolREST, created.Protocol)

	post(t, fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/deployments", orgName, projName, agentName),
		map[string]interface{}{"imageId": "registry.example.com/myapp:v1.0.0"})

	deployConfigs := openChoreoClient.UpdateComponentDeploymentConfigCalls()
	require.NotEmpty(t, deployConfigs)
	deployConfig := deployConfigs[len(deployConfigs)-1].Req
	assert.ElementsMatch(t, []client.TraitType{client.TraitAPIManagement, client.TraitA2AGatewayRoute}, deployConfig.TraitsToDetach)
	assert.Empty(t, deployConfig.TraitsToAttach)

	deployed, err := exposures.Get(context.Background(), ouID, projName, agentName, environmentUUID)
	require.NoError(t, err)
	assert.Equal(t, created.ArtifactUUID, deployed.ArtifactUUID, "the deploy exposes the artifact create made")
	artifact, err := repositories.NewArtifactRepo(gdb).GetByUUID(deployed.ArtifactUUID.String(), ouID)
	require.NoError(t, err)
	assert.Equal(t, models.KindRestAPI, artifact.Kind)
	row, err := repositories.NewDeploymentRepo(gdb).GetStatusRecord(context.Background(), models.Placement{
		ArtifactRef: models.ArtifactRef{OUID: ouID, ArtifactUUID: artifact.UUID, Kind: models.KindRestAPI},
		GatewayUUID: gatewayID,
	})
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, models.DeploymentStatusPending, row.Status)
}

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
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// An external agent sent with the old a2a-agent subtype is created like any external agent: no trait, no publication.
func TestCreateExternalAgentWithA2ASubTypeGetsNoGatewayPublication(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)
	platformRepo := apitestutils.CreateMockPlatformRepository()
	testClients := wiring.TestClients{
		PlatformRepository: platformRepo,
		SecretMgmtClient:   apitestutils.CreateMockSecretManagementClient(),
	}
	app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)
	agentName := fmt.Sprintf("ext-a2a-%s", uuid.New().String()[:5])

	reqBody := new(bytes.Buffer)
	require.NoError(t, json.NewEncoder(reqBody).Encode(map[string]interface{}{
		"name":         agentName,
		"displayName":  "External A2A",
		"provisioning": map[string]interface{}{"type": "external"},
		"agentType":    map[string]interface{}{"type": "external-agent-api", "subType": "a2a-agent"},
	}))
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName), reqBody)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)
	require.Equal(t, http.StatusAccepted, rr.Code, "body: %s", rr.Body.String())

	require.Len(t, platformRepo.CreateAgentCalls(), 1)

	var queued int64
	require.NoError(t, db.GetDB().Model(&models.PlatformAgentExposure{}).
		Where("agent_name = ?", agentName).Count(&queued).Error)
	require.Zero(t, queued, "an external agent is never exposed as a platform agent")
}

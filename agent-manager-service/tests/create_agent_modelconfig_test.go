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

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// TestCreateAgent_BadProviderFailsBeforeProvisioning verifies the create-time preflight:
// a modelConfig referencing a nonexistent LLM provider must fail with 404 BEFORE any
// component is provisioned (no orphaned resources).
func TestCreateAgent_BadProviderFailsBeforeProvisioning(t *testing.T) {
	seedIngressGatewayForDefaultEnvironment(t, "mock-org-id")
	authMiddleware := jwtassertion.NewMockMiddleware(t)
	openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
	// CreateMockOpenChoreoClient's GetProjectDeploymentPipelineFunc already returns a
	// PromotionPath with SourceEnvironmentRef "Development", so findLowestEnvironment
	// resolves a non-empty firstEnv and the preflight is reached.
	app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{OpenChoreoClient: openChoreoClient}, authMiddleware)

	reqBody := new(bytes.Buffer)
	require.NoError(t, json.NewEncoder(reqBody).Encode(map[string]any{
		"name":        "mc-bad-provider-agent",
		"displayName": "MC Bad Provider Agent",
		"provisioning": map[string]any{
			"type": "internal",
			"repository": map[string]any{
				"url": "https://github.com/test/test-repo", "branch": "main", "appPath": "/agent-sample",
			},
		},
		"agentType": map[string]any{"type": "agent-api", "subType": "chat-api"},
		"build": map[string]any{
			"type": "buildpack",
			"buildpack": map[string]any{
				"language": "python", "languageVersion": "3.11", "runCommand": "uvicorn app:app --host 0.0.0.0 --port 8000",
			},
		},
		"inputInterface": map[string]any{"type": "HTTP"},
		"modelConfig":    []map[string]any{{"providerName": "does-not-exist-provider"}},
	}))

	url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName)
	req := httptest.NewRequest(http.MethodPost, url, reqBody)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code, "missing provider must return 404")
	// The preflight runs before any component is provisioned, so CreateComponent must
	// never have been called. The moq-style mock records every call to CreateComponent.
	require.Empty(t, openChoreoClient.CreateComponentCalls(),
		"no component should be created when provider validation fails")
}

//go:build integration

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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

var (
	testOrgName  = fmt.Sprintf("test-org-%s", uuid.New().String()[:5])
	testProjName = fmt.Sprintf("test-project-%s", uuid.New().String()[:5])
)

func TestCreateAgent(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	validationTests := []struct {
		name           string
		authMiddleware jwtassertion.Middleware
		payload        map[string]interface{}
		wantStatus     int
		wantErrMsg     string
		url            string
	}{
		{
			// Hosted (internal) agents need an agent runtime this control
			// plane does not provide; only external agents can be created.
			name:           "return 501 on a hosted agent",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test Agent",
				"provisioning": map[string]interface{}{
					"type": "internal",
					"repository": map[string]interface{}{
						"url":     "https://github.com/test/test-repo",
						"branch":  "main",
						"appPath": "/sample-agent",
					},
				},
				"build": map[string]interface{}{
					"type": "buildpack",
					"buildpack": map[string]interface{}{
						"language":        "python",
						"languageVersion": "3.11",
						"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					},
				},
				"agentType":      map[string]interface{}{"type": "agent-api", "subType": "chat-api"},
				"inputInterface": map[string]interface{}{"type": "HTTP"},
			},
			wantStatus: http.StatusNotImplemented,
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
		},
		{
			name: "return 401 on missing authentication",
			authMiddleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					utils.WriteErrorResponse(w, http.StatusUnauthorized, "missing header: Authorization")
				})
			},
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "internal",
					"repository": map[string]interface{}{
						"url":     "https://github.com/test/test-repo",
						"branch":  "main",
						"appPath": "/sample-agent",
					},
				},
				"build": map[string]interface{}{
					"type": "buildpack",
					"buildpack": map[string]interface{}{
						"language":        "python",
						"languageVersion": "3.11",
						"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					},
				},
				"agentType": map[string]interface{}{
					"type":    "agent-api",
					"subType": "chat-api",
				},
				"inputInterface": map[string]interface{}{
					"type": "HTTP",
				},
			},
			wantStatus: 401,
			wantErrMsg: "missing header: Authorization",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
		},
	}

	for _, tt := range validationTests {
		t.Run(tt.name, func(t *testing.T) {
			testClients := wiring.TestClients{
				PlatformRepository: apitestutils.CreateMockPlatformRepository(),
				SecretMgmtClient:   apitestutils.CreateMockSecretManagementClient(),
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
			}
		})
	}
}

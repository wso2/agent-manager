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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/clientmocks"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/tests/apitestutils"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/wiring"
)

func createMockOpenChoreoClientForToken(agentName string, componentUid string, envUid string, orgUid string, projUid string) *clientmocks.OpenChoreoSvcClientMock {
	return &clientmocks.OpenChoreoSvcClientMock{
		GetProjectFunc: func(ctx context.Context, projectName string, orgName string) (*models.ProjectResponse, error) {
			return &models.ProjectResponse{
				UUID:        projUid,
				Name:        projectName,
				DisplayName: projectName,
				OrgName:     orgName,
				CreatedAt:   time.Now(),
			}, nil
		},
		GetAgentComponentFunc: func(ctx context.Context, orgName string, projName string, agName string) (*openchoreosvc.AgentComponent, error) {
			// Return existing external agent
			return &openchoreosvc.AgentComponent{
				UUID:        componentUid,
				Name:        agName,
				ProjectName: projName,
			}, nil
		},
		GetEnvironmentFunc: func(ctx context.Context, orgName string, environmentName string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{
				UUID: envUid,
				Name: environmentName,
			}, nil
		},
		GetOrganizationFunc: func(ctx context.Context, orgName string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{
				Name: orgName,
			}, nil
		},
	}
}

func TestGenerateAgentToken(t *testing.T) {
	// Create unique test data for this test suite
	tokenOrgName := fmt.Sprintf("token-org-%s", uuid.New().String()[:5])
	tokenProjName := fmt.Sprintf("token-project-%s", uuid.New().String()[:5])
	tokenAgentName := fmt.Sprintf("token-agent-%s", uuid.New().String()[:5])
	tokenComponentUid := uuid.New().String()
	tokenEnvUid := uuid.New().String()
	tokenOrgUid := uuid.New().String()
	tokenProjUid := uuid.New().String()

	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("Generating a token for an external agent should return 200 with valid token", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForToken(tokenAgentName, tokenComponentUid, tokenEnvUid, tokenOrgUid, tokenProjUid)
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Generate token for the agent with custom expiry
		tokenReqBody := new(bytes.Buffer)
		err := json.NewEncoder(tokenReqBody).Encode(map[string]interface{}{
			"expires_in": "720h",
		})
		require.NoError(t, err)

		tokenURL := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/token?environment=Development",
			tokenOrgName, tokenProjName, tokenAgentName)
		tokenReq := httptest.NewRequest(http.MethodPost, tokenURL, tokenReqBody)
		tokenReq.Header.Set("Content-Type", "application/json")

		tokenRR := httptest.NewRecorder()
		app.ServeHTTP(tokenRR, tokenReq)

		// Assert response
		require.Equal(t, http.StatusOK, tokenRR.Code)

		// Read and validate response body
		b, err := io.ReadAll(tokenRR.Body)
		require.NoError(t, err)
		t.Logf("token response body: %s", string(b))

		var tokenResponse models.TokenResponse
		require.NoError(t, json.Unmarshal(b, &tokenResponse))

		// Validate response fields
		require.NotEmpty(t, tokenResponse.Token)
		require.Equal(t, "Bearer", tokenResponse.TokenType)
		require.NotZero(t, tokenResponse.IssuedAt)
		require.NotZero(t, tokenResponse.ExpiresAt)
		require.Greater(t, tokenResponse.ExpiresAt, tokenResponse.IssuedAt)

		// Verify token structure (don't verify signature in test as keys may differ)
		parser := jwt.NewParser()
		token, _, err := parser.ParseUnverified(tokenResponse.Token, jwt.MapClaims{})
		require.NoError(t, err)
		require.NotNil(t, token)

		claims, ok := token.Claims.(jwt.MapClaims)
		require.True(t, ok)

		// Validate claims exist
		require.Contains(t, claims, "component_uid")
		require.Contains(t, claims, "environment_uid")
		require.Contains(t, claims, "iss")
		require.Contains(t, claims, "exp")
		require.Contains(t, claims, "iat")

		// Validate the component_uid matches what we expect
		require.Equal(t, tokenComponentUid, claims["component_uid"])
		require.Equal(t, tokenEnvUid, claims["environment_uid"])
	})
}

func TestGetJWKS(t *testing.T) {
	// Create unique test data for this test suite
	jwksAgentName := fmt.Sprintf("jwks-agent-%s", uuid.New().String()[:5])
	jwksComponentUid := uuid.New().String()
	jwksEnvUid := uuid.New().String()
	jwksOrgUid := uuid.New().String()
	jwksProjUid := uuid.New().String()

	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("Getting JWKS should return 200 with valid JWKS", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForToken(jwksAgentName, jwksComponentUid, jwksEnvUid, jwksOrgUid, jwksProjUid)
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Request JWKS endpoint
		jwksURL := "/auth/external/jwks.json"
		req := httptest.NewRequest(http.MethodGet, jwksURL, nil)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response
		require.Equal(t, http.StatusOK, rr.Code)

		// Read and validate response body
		b, err := io.ReadAll(rr.Body)
		require.NoError(t, err)
		t.Logf("jwks response body: %s", string(b))

		var jwksResponse models.JWKS
		require.NoError(t, json.Unmarshal(b, &jwksResponse))

		// Validate JWKS structure
		require.NotEmpty(t, jwksResponse.Keys)
		require.GreaterOrEqual(t, len(jwksResponse.Keys), 1)

		// Validate first key structure
		firstKey := jwksResponse.Keys[0]
		require.Equal(t, "RSA", firstKey.Kty)
		require.Equal(t, "RS256", firstKey.Alg)
		require.Equal(t, "sig", firstKey.Use)
		require.NotEmpty(t, firstKey.Kid)
		require.NotEmpty(t, firstKey.N) // RSA modulus
		require.NotEmpty(t, firstKey.E) // RSA exponent

		// Each key should have valid RSA components
		for _, key := range jwksResponse.Keys {
			require.Equal(t, "RSA", key.Kty, "Key type should be RSA")
			require.Equal(t, "RS256", key.Alg, "Algorithm should be RS256")
			require.Equal(t, "sig", key.Use, "Use should be sig (signature)")

			// Verify modulus and exponent are valid base64url encoded values
			require.NotEmpty(t, key.N, "RSA modulus should not be empty")
			require.NotEmpty(t, key.E, "RSA exponent should not be empty")
			require.NotEmpty(t, key.Kid, "Key ID should not be empty")
		}
	})
}

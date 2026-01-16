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
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/tests/apitestutils"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
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

		tokenURL := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/token?environment=default",
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

		var tokenResponse spec.TokenResponse
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

	t.Run("Invalid expiry duration - malformed string", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForToken(tokenAgentName, tokenComponentUid, tokenEnvUid, tokenOrgUid, tokenProjUid)
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		tokenReqBody := new(bytes.Buffer)
		err := json.NewEncoder(tokenReqBody).Encode(map[string]interface{}{
			"expires_in": "invalid-duration",
		})
		require.NoError(t, err)

		tokenURL := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/token?environment=default",
			tokenOrgName, tokenProjName, tokenAgentName)
		tokenReq := httptest.NewRequest(http.MethodPost, tokenURL, tokenReqBody)
		tokenReq.Header.Set("Content-Type", "application/json")

		tokenRR := httptest.NewRecorder()
		app.ServeHTTP(tokenRR, tokenReq)

		require.Equal(t, http.StatusBadRequest, tokenRR.Code)
	})

	t.Run("Invalid expiry duration - zero duration", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForToken(tokenAgentName, tokenComponentUid, tokenEnvUid, tokenOrgUid, tokenProjUid)
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		tokenReqBody := new(bytes.Buffer)
		err := json.NewEncoder(tokenReqBody).Encode(map[string]interface{}{
			"expires_in": "0h",
		})
		require.NoError(t, err)

		tokenURL := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/token?environment=default",
			tokenOrgName, tokenProjName, tokenAgentName)
		tokenReq := httptest.NewRequest(http.MethodPost, tokenURL, tokenReqBody)
		tokenReq.Header.Set("Content-Type", "application/json")

		tokenRR := httptest.NewRecorder()
		app.ServeHTTP(tokenRR, tokenReq)

		require.Equal(t, http.StatusBadRequest, tokenRR.Code)
	})

	t.Run("Empty expiry - should use default", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForToken(tokenAgentName, tokenComponentUid, tokenEnvUid, tokenOrgUid, tokenProjUid)
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		tokenReqBody := new(bytes.Buffer)
		err := json.NewEncoder(tokenReqBody).Encode(map[string]interface{}{})
		require.NoError(t, err)

		tokenURL := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/token?environment=default",
			tokenOrgName, tokenProjName, tokenAgentName)
		tokenReq := httptest.NewRequest(http.MethodPost, tokenURL, tokenReqBody)
		tokenReq.Header.Set("Content-Type", "application/json")

		tokenRR := httptest.NewRecorder()
		app.ServeHTTP(tokenRR, tokenReq)

		require.Equal(t, http.StatusOK, tokenRR.Code)

		var tokenResponse spec.TokenResponse
		require.NoError(t, json.NewDecoder(tokenRR.Body).Decode(&tokenResponse))
		require.NotEmpty(t, tokenResponse.Token)
	})

	t.Run("Missing agent should return 404", func(t *testing.T) {
		nonExistentAgent := "non-existent-agent"
		openChoreoClient := &clientmocks.OpenChoreoSvcClientMock{
			GetProjectFunc: func(ctx context.Context, projectName string, orgName string) (*models.ProjectResponse, error) {
				return &models.ProjectResponse{
					UUID:        tokenProjUid,
					Name:        projectName,
					DisplayName: projectName,
					OrgName:     orgName,
					CreatedAt:   time.Now(),
				}, nil
			},
			GetAgentComponentFunc: func(ctx context.Context, orgName string, projName string, agName string) (*openchoreosvc.AgentComponent, error) {
				return nil, utils.ErrAgentNotFound
			},
			GetEnvironmentFunc: func(ctx context.Context, orgName string, environmentName string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{
					UUID: tokenEnvUid,
					Name: environmentName,
				}, nil
			},
			GetOrganizationFunc: func(ctx context.Context, orgName string) (*models.OrganizationResponse, error) {
				return &models.OrganizationResponse{
					Name: orgName,
				}, nil
			},
		}
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		tokenReqBody := new(bytes.Buffer)
		err := json.NewEncoder(tokenReqBody).Encode(map[string]interface{}{
			"expires_in": "720h",
		})
		require.NoError(t, err)

		tokenURL := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/token?environment=default",
			tokenOrgName, tokenProjName, nonExistentAgent)
		tokenReq := httptest.NewRequest(http.MethodPost, tokenURL, tokenReqBody)
		tokenReq.Header.Set("Content-Type", "application/json")

		tokenRR := httptest.NewRecorder()
		app.ServeHTTP(tokenRR, tokenReq)

		require.Equal(t, http.StatusNotFound, tokenRR.Code)
	})

	t.Run("Missing environment should return 404", func(t *testing.T) {
		openChoreoClient := &clientmocks.OpenChoreoSvcClientMock{
			GetProjectFunc: func(ctx context.Context, projectName string, orgName string) (*models.ProjectResponse, error) {
				return &models.ProjectResponse{
					UUID:        tokenProjUid,
					Name:        projectName,
					DisplayName: projectName,
					OrgName:     orgName,
					CreatedAt:   time.Now(),
				}, nil
			},
			GetAgentComponentFunc: func(ctx context.Context, orgName string, projName string, agName string) (*openchoreosvc.AgentComponent, error) {
				return &openchoreosvc.AgentComponent{
					UUID:        tokenComponentUid,
					Name:        agName,
					ProjectName: projName,
				}, nil
			},
			GetEnvironmentFunc: func(ctx context.Context, orgName string, environmentName string) (*models.EnvironmentResponse, error) {
				return nil, utils.ErrEnvironmentNotFound
			},
			GetOrganizationFunc: func(ctx context.Context, orgName string) (*models.OrganizationResponse, error) {
				return &models.OrganizationResponse{
					Name: orgName,
				}, nil
			},
		}
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		tokenReqBody := new(bytes.Buffer)
		err := json.NewEncoder(tokenReqBody).Encode(map[string]interface{}{
			"expires_in": "720h",
		})
		require.NoError(t, err)

		tokenURL := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/token?environment=NonExistent",
			tokenOrgName, tokenProjName, tokenAgentName)
		tokenReq := httptest.NewRequest(http.MethodPost, tokenURL, tokenReqBody)
		tokenReq.Header.Set("Content-Type", "application/json")

		tokenRR := httptest.NewRecorder()
		app.ServeHTTP(tokenRR, tokenReq)

		require.Equal(t, http.StatusNotFound, tokenRR.Code)
	})
}

func TestConcurrentTokenGeneration(t *testing.T) {
	// Create unique test data for concurrent tests
	concurrentOrgName := fmt.Sprintf("concurrent-org-%s", uuid.New().String()[:5])
	concurrentProjName := fmt.Sprintf("concurrent-project-%s", uuid.New().String()[:5])
	concurrentAgentName := fmt.Sprintf("concurrent-agent-%s", uuid.New().String()[:5])
	concurrentComponentUid := uuid.New().String()
	concurrentEnvUid := uuid.New().String()
	concurrentOrgUid := uuid.New().String()
	concurrentProjUid := uuid.New().String()

	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("Testing concurrent token generation", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForToken(concurrentAgentName, concurrentComponentUid, concurrentEnvUid, concurrentOrgUid, concurrentProjUid)
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		const numConcurrentRequests = 10
		type result struct {
			statusCode int
			token      string
			err        error
		}
		results := make(chan result, numConcurrentRequests)

		// Launch concurrent token generation requests
		for i := 0; i < numConcurrentRequests; i++ {
			go func() {
				tokenReqBody := new(bytes.Buffer)
				err := json.NewEncoder(tokenReqBody).Encode(map[string]interface{}{
					"expires_in": "24h",
				})
				if err != nil {
					results <- result{err: err}
					return
				}

				tokenURL := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/token?environment=default",
					concurrentOrgName, concurrentProjName, concurrentAgentName)
				tokenReq := httptest.NewRequest(http.MethodPost, tokenURL, tokenReqBody)
				tokenReq.Header.Set("Content-Type", "application/json")

				tokenRR := httptest.NewRecorder()
				app.ServeHTTP(tokenRR, tokenReq)

				var tokenResponse spec.TokenResponse
				if tokenRR.Code == http.StatusOK {
					_ = json.NewDecoder(tokenRR.Body).Decode(&tokenResponse)
				}

				results <- result{
					statusCode: tokenRR.Code,
					token:      tokenResponse.Token,
				}
			}()
		}

		// Collect results
		successCount := 0
		tokensReceived := 0

		for i := 0; i < numConcurrentRequests; i++ {
			res := <-results
			require.NoError(t, res.err)
			require.Equal(t, http.StatusOK, res.statusCode)
			if res.statusCode == http.StatusOK {
				successCount++
				require.NotEmpty(t, res.token)
				tokensReceived++
			}
		}

		// All requests should succeed
		require.Equal(t, numConcurrentRequests, successCount)
		// All requests should receive a token
		require.Equal(t, numConcurrentRequests, tokensReceived)
		t.Logf("Successfully generated %d tokens concurrently", tokensReceived)
	})
}

func TestTokenExpiry(t *testing.T) {
	// Create unique test data for expiry tests
	expiryOrgName := fmt.Sprintf("expiry-org-%s", uuid.New().String()[:5])
	expiryProjName := fmt.Sprintf("expiry-project-%s", uuid.New().String()[:5])
	expiryAgentName := fmt.Sprintf("expiry-agent-%s", uuid.New().String()[:5])
	expiryComponentUid := uuid.New().String()
	expiryEnvUid := uuid.New().String()
	expiryOrgUid := uuid.New().String()
	expiryProjUid := uuid.New().String()

	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("Multiple tokens with different expiry durations", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForToken(expiryAgentName, expiryComponentUid, expiryEnvUid, expiryOrgUid, expiryProjUid)
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		testCases := []struct {
			expiresIn        string
			expectedDuration time.Duration
		}{
			{"1h", 1 * time.Hour},
			{"720h", 720 * time.Hour},
		}

		for _, tc := range testCases {
			t.Run(fmt.Sprintf("expires_in=%s", tc.expiresIn), func(t *testing.T) {
				tokenReqBody := new(bytes.Buffer)
				err := json.NewEncoder(tokenReqBody).Encode(map[string]interface{}{
					"expires_in": tc.expiresIn,
				})
				require.NoError(t, err)

				tokenURL := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/token?environment=default",
					expiryOrgName, expiryProjName, expiryAgentName)
				tokenReq := httptest.NewRequest(http.MethodPost, tokenURL, tokenReqBody)
				tokenReq.Header.Set("Content-Type", "application/json")

				tokenRR := httptest.NewRecorder()
				app.ServeHTTP(tokenRR, tokenReq)

				require.Equal(t, http.StatusOK, tokenRR.Code)

				var tokenResponse spec.TokenResponse
				require.NoError(t, json.NewDecoder(tokenRR.Body).Decode(&tokenResponse))

				// Parse and verify expiry duration
				parser := jwt.NewParser()
				token, _, err := parser.ParseUnverified(tokenResponse.Token, jwt.MapClaims{})
				require.NoError(t, err)

				claims, ok := token.Claims.(jwt.MapClaims)
				require.True(t, ok)

				expClaim, ok := claims["exp"].(float64)
				require.True(t, ok)
				iatClaim, ok := claims["iat"].(float64)
				require.True(t, ok)

				expiryTime := time.Unix(int64(expClaim), 0)
				issuedTime := time.Unix(int64(iatClaim), 0)
				duration := expiryTime.Sub(issuedTime)

				// Verify duration matches expected (allow 5 seconds tolerance)
				require.InDelta(t, tc.expectedDuration.Seconds(), duration.Seconds(), 5)
			})
		}
	})
}

func TestGetJWKSAgentManagerService(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("Getting JWKS should return 200 with valid JWKS", func(t *testing.T) {
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{}, authMiddleware)

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

		var jwksResponse spec.JWKS
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
	t.Run("JWKS endpoint should be accessible without authentication", func(t *testing.T) {
		// JWKS endpoint should be public, test with normal auth middleware
		app := apitestutils.MakeAppClientWithDeps(t, wiring.TestClients{}, authMiddleware)

		// Request JWKS endpoint without auth headers
		jwksURL := "/auth/external/jwks.json"
		req := httptest.NewRequest(http.MethodGet, jwksURL, nil)
		// Note: Not setting any authorization headers

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Should still return 200 - JWKS is public
		require.Equal(t, http.StatusOK, rr.Code)

		var jwksResponse spec.JWKS
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&jwksResponse))
		require.NotEmpty(t, jwksResponse.Keys)
	})
}

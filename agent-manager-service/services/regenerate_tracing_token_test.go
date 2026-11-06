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

package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// stubTokenManager is a minimal AgentTokenManagerService for exercising the regenerate flow
// without a real signing key.
type stubTokenManager struct {
	generateFunc func(ctx context.Context, req GenerateTokenRequest) (*spec.TokenResponse, error)
	gotRequests  []GenerateTokenRequest
}

func (s *stubTokenManager) GenerateToken(ctx context.Context, req GenerateTokenRequest) (*spec.TokenResponse, error) {
	s.gotRequests = append(s.gotRequests, req)
	return s.generateFunc(ctx, req)
}

func (s *stubTokenManager) GetJWKS(_ context.Context) (*spec.JWKS, error) {
	return &spec.JWKS{}, nil
}

func ctxWithOrg(ouID string) context.Context {
	return jwtassertion.ContextWithTokenClaims(context.Background(), &jwtassertion.TokenClaims{OuId: ouID})
}

func regenerateOCClient() *clientmocks.OpenChoreoClientMock {
	return &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{}, nil
		},
		GetEnvironmentFunc: func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{Name: name}, nil
		},
	}
}

func TestRegenerateAgentTracingToken_EnvRequired(t *testing.T) {
	s := &agentManagerService{logger: discardLogger()}

	_, err := s.RegenerateAgentTracingToken(ctxWithOrg("acme"), "acme", "proj", "agent", "", "")

	require.ErrorIs(t, err, utils.ErrInvalidInput)
}

func TestRegenerateAgentTracingToken_MissingCallerIdentity(t *testing.T) {
	s := &agentManagerService{logger: discardLogger()}

	_, err := s.RegenerateAgentTracingToken(context.Background(), "acme", "proj", "agent", "dev", "")

	require.ErrorIs(t, err, utils.ErrForbidden)
}

func TestRegenerateAgentTracingToken_HappyPath(t *testing.T) {
	ocClient := regenerateOCClient()
	secretMgmt := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, _ secretmanagersvc.SecretLocation, data map[string]string) (string, error) {
			assert.Equal(t, "signed-jwt", data[secretmanagersvc.SecretKeyAPIKey], "the minted token must be upserted into the secret store")
			return "secret-ref", nil
		},
	}
	tokenMgr := &stubTokenManager{generateFunc: func(_ context.Context, _ GenerateTokenRequest) (*spec.TokenResponse, error) {
		return &spec.TokenResponse{Token: "signed-jwt", ExpiresAt: 1234567890}, nil
	}}
	s := &agentManagerService{ocClient: ocClient, secretMgmtClient: secretMgmt, tokenManagerService: tokenMgr, logger: discardLogger()}

	result, err := s.RegenerateAgentTracingToken(ctxWithOrg("acme"), "acme", "proj", "agent", "dev", "720h")

	require.NoError(t, err)
	assert.Equal(t, "dev", result.EnvironmentName)
	assert.Equal(t, int64(1234567890), result.ExpiresAt)
	assert.NotZero(t, result.RotatedAt)
	require.Len(t, tokenMgr.gotRequests, 1)
	assert.Equal(t, "720h", tokenMgr.gotRequests[0].ExpiresIn, "the caller-supplied expiry must be honored")
	assert.Equal(t, "acme", tokenMgr.gotRequests[0].OrgId, "org id must come from the caller's claims")
	assert.Len(t, secretMgmt.CreateSecretCalls(), 1)
}

func TestRegenerateAgentTracingToken_AgentNotFound(t *testing.T) {
	ocClient := regenerateOCClient()
	ocClient.GetComponentFunc = func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
		return nil, utils.ErrAgentNotFound
	}
	tokenMgr := &stubTokenManager{generateFunc: func(_ context.Context, _ GenerateTokenRequest) (*spec.TokenResponse, error) {
		return &spec.TokenResponse{}, nil
	}}
	s := &agentManagerService{ocClient: ocClient, tokenManagerService: tokenMgr, logger: discardLogger()}

	_, err := s.RegenerateAgentTracingToken(ctxWithOrg("acme"), "acme", "proj", "agent", "dev", "")

	require.ErrorIs(t, err, utils.ErrAgentNotFound)
	assert.Empty(t, tokenMgr.gotRequests, "no token must be minted when the agent does not exist")
}

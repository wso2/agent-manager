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
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
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
		// The environment receives the agent API key by env var, so the regenerate tries to
		// force-sync it.
		GetComponentConfigurationsFunc: func(context.Context, string, string, string, string) ([]models.EnvVars, error) {
			return []models.EnvVars{
				{Key: client.EnvVarAgentAPIKey, IsSensitive: true, SecretRef: "secret-ref", SecretKey: secretmanagersvc.SecretKeyAPIKey},
			}, nil
		},
		// Without the field the regenerate skips the force-sync; tests of it override this.
		ComponentTypeDeclaresEnvironmentConfigFunc: func(context.Context, string, string, string) (bool, error) {
			return false, nil
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

// regenerateService returns a service whose tracing-token regeneration mints and stores a key
// for agent "agent" through ocClient.
func regenerateService(ocClient *clientmocks.OpenChoreoClientMock) (*agentManagerService, *clientmocks.SecretManagementClientMock) {
	secretMgmt := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			return "secret-ref", nil
		},
	}
	tokenMgr := &stubTokenManager{generateFunc: func(context.Context, GenerateTokenRequest) (*spec.TokenResponse, error) {
		return &spec.TokenResponse{Token: "signed-jwt"}, nil
	}}
	return &agentManagerService{ocClient: ocClient, secretMgmtClient: secretMgmt, tokenManagerService: tokenMgr, logger: discardLogger()}, secretMgmt
}

func TestRegenerateAgentTracingToken_ForcesSecretSync(t *testing.T) {
	// The key is stored where the old one was, so the ExternalSecret keeps the old value until it
	// is made to re-sync, and a restart before then would start the agent with the old key.
	ocClient := regenerateOCClient()
	ocClient.ComponentTypeDeclaresEnvironmentConfigFunc = func(_ context.Context, _, agentName, field string) (bool, error) {
		assert.Equal(t, "agent", agentName)
		assert.Equal(t, client.SecretSyncRevisionField, field)
		return true, nil
	}
	ocClient.SetReleaseBindingSecretSyncRevisionFunc = func(_ context.Context, _, agentName, envName, revision string) error {
		assert.Equal(t, "agent", agentName)
		assert.Equal(t, "dev", envName)
		assert.NotEmpty(t, revision)
		return nil
	}
	ocClient.IsSecretSyncRevisionAppliedFunc = func(context.Context, string, string, string, string) (bool, error) {
		return true, nil
	}
	s, secretMgmt := regenerateService(ocClient)

	_, err := s.RegenerateAgentTracingToken(ctxWithOrg("acme"), "acme", "proj", "agent", "dev", "")

	require.NoError(t, err)
	assert.Len(t, secretMgmt.CreateSecretCalls(), 1)
	setCalls := ocClient.SetReleaseBindingSecretSyncRevisionCalls()
	require.Len(t, setCalls, 1)
	appliedCalls := ocClient.IsSecretSyncRevisionAppliedCalls()
	require.Len(t, appliedCalls, 1)
	assert.Equal(t, setCalls[0].Revision, appliedCalls[0].Revision, "the wait must be for the revision just written")
}

func TestRegenerateAgentTracingToken_FieldNotDeclared_SkipsForceSync(t *testing.T) {
	// regenerateOCClient reports the field as not declared, and SetReleaseBindingSecretSyncRevision
	// is left unset so writing the revision anyway would panic.
	ocClient := regenerateOCClient()
	s, secretMgmt := regenerateService(ocClient)

	_, err := s.RegenerateAgentTracingToken(ctxWithOrg("acme"), "acme", "proj", "agent", "dev", "")

	require.NoError(t, err)
	assert.Len(t, secretMgmt.CreateSecretCalls(), 1)
	assert.Len(t, ocClient.ComponentTypeDeclaresEnvironmentConfigCalls(), 1)
}

func TestRegenerateAgentTracingToken_KeyNotDeliveredByEnvVar_SkipsForceSync(t *testing.T) {
	// An agent created before the key moved to the env var reads it through the env-injection
	// trait's ExternalSecret, which never carries the force-sync revision.
	ocClient := regenerateOCClient()
	ocClient.GetComponentConfigurationsFunc = func(context.Context, string, string, string, string) ([]models.EnvVars, error) {
		return []models.EnvVars{
			{Key: client.EnvVarAgentAPIKey, IsSensitive: true, SecretRef: "my-agent-dev-secrets", SecretKey: client.EnvVarAgentAPIKey},
		}, nil
	}
	s, secretMgmt := regenerateService(ocClient)

	_, err := s.RegenerateAgentTracingToken(ctxWithOrg("acme"), "acme", "proj", "agent", "dev", "")

	require.NoError(t, err)
	assert.Len(t, secretMgmt.CreateSecretCalls(), 1)
	assert.Empty(t, ocClient.ComponentTypeDeclaresEnvironmentConfigCalls())
}

func TestRegenerateAgentTracingToken_ForceSyncFails_StillSucceeds(t *testing.T) {
	// The key is already stored; it reaches the agent at the ExternalSecret's next refresh.
	tests := map[string]error{
		"write fails":        errors.New("openchoreo unavailable"),
		"no release binding": fmt.Errorf("no release binding found: %w", utils.ErrNotFound),
	}
	for name, setErr := range tests {
		t.Run(name, func(t *testing.T) {
			ocClient := regenerateOCClient()
			ocClient.ComponentTypeDeclaresEnvironmentConfigFunc = func(context.Context, string, string, string) (bool, error) {
				return true, nil
			}
			ocClient.SetReleaseBindingSecretSyncRevisionFunc = func(context.Context, string, string, string, string) error {
				return setErr
			}
			s, secretMgmt := regenerateService(ocClient)

			result, err := s.RegenerateAgentTracingToken(ctxWithOrg("acme"), "acme", "proj", "agent", "dev", "")

			require.NoError(t, err)
			assert.Equal(t, "dev", result.EnvironmentName)
			assert.Len(t, secretMgmt.CreateSecretCalls(), 1)
			assert.Len(t, ocClient.SetReleaseBindingSecretSyncRevisionCalls(), 1, "a permanent failure is not retried")
		})
	}
}

func TestRegenerateAgentTracingToken_ComponentTypeUnreadable_StillSucceeds(t *testing.T) {
	ocClient := regenerateOCClient()
	ocClient.ComponentTypeDeclaresEnvironmentConfigFunc = func(context.Context, string, string, string) (bool, error) {
		return false, errors.New("component type not readable")
	}
	s, secretMgmt := regenerateService(ocClient)

	_, err := s.RegenerateAgentTracingToken(ctxWithOrg("acme"), "acme", "proj", "agent", "dev", "")

	require.NoError(t, err)
	assert.Len(t, secretMgmt.CreateSecretCalls(), 1)
}

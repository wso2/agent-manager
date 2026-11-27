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

// UNIT tests that changing an agent's API keys is gated on the environment tier:
// agent:api-key-manage alone must not mint, rotate or revoke a production key.
package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// productionAPIKeyFixture is newAPIKeyServiceFixture with its one environment
// flagged production.
func productionAPIKeyFixture(t *testing.T) *apiKeyServiceFixture {
	t.Helper()
	fx := newAPIKeyServiceFixture(t, nil)
	oc, ok := fx.svc.ocClient.(*clientmocks.OpenChoreoClientMock)
	require.True(t, ok)
	oc.GetEnvironmentFunc = func(_ context.Context, _, envName string) (*models.EnvironmentResponse, error) {
		return &models.EnvironmentResponse{UUID: "dada1090-0ddb-457a-a116-7f03d3cd0c4c", Name: envName, IsProduction: true}, nil
	}
	return fx
}

func TestAgentAPIKeyService_ProductionKeyChangesNeedProductionTier(t *testing.T) {
	const org, proj, agent, env = "default", "default", "customeragent", "production"

	changes := map[string]func(fx *apiKeyServiceFixture, ctx context.Context) error{
		"create": func(fx *apiKeyServiceFixture, ctx context.Context) error {
			_, err := fx.svc.CreateAPIKey(ctx, org, proj, agent, env, &models.CreateAPIKeyRequest{Name: "prod-key"})
			return err
		},
		"rotate": func(fx *apiKeyServiceFixture, ctx context.Context) error {
			_, err := fx.svc.RotateAPIKey(ctx, org, proj, agent, env, "prod-key", &models.RotateAPIKeyRequest{})
			return err
		},
		"revoke": func(fx *apiKeyServiceFixture, ctx context.Context) error {
			return fx.svc.RevokeAPIKey(ctx, org, proj, agent, env, "prod-key")
		},
		"issue test key": func(fx *apiKeyServiceFixture, ctx context.Context) error {
			_, err := fx.svc.IssueTestAPIKey(ctx, org, proj, agent, env, "user-sub")
			return err
		},
	}
	for name, change := range changes {
		t.Run(name+" is refused with only the non-production tier", func(t *testing.T) {
			fx := productionAPIKeyFixture(t)

			err := change(fx, tierCtx(t, rbac.AgentEnvNonProduction))

			require.ErrorIs(t, err, utils.ErrForbidden)
			assert.Contains(t, err.Error(), rbac.AgentEnvProduction.Scope())
			assert.Empty(t, fx.apiKeyRepo.UpsertCalls(), "no key may be stored for a refused change")
			assert.Empty(t, fx.apiKeyRepo.GetByArtifactAndNameCalls(), "a refused change must stop before touching keys")
		})
	}

	t.Run("create succeeds with both tiers", func(t *testing.T) {
		fx := productionAPIKeyFixture(t)

		resp, err := fx.svc.CreateAPIKey(tierGrantedCtx(t), org, proj, agent, env, &models.CreateAPIKeyRequest{Name: "prod-key"})

		require.NoError(t, err)
		assert.NotEmpty(t, resp.APIKey)
	})

	t.Run("listing masked keys does not need the production tier", func(t *testing.T) {
		fx := productionAPIKeyFixture(t)
		fx.apiKeyRepo.ListPermanentByArtifactKindsFunc = func(string, []string) ([]models.StoredAPIKey, error) { return nil, nil }

		_, err := fx.svc.ListAPIKeys(tierCtx(t, rbac.AgentEnvNonProduction), org, proj, agent, env)

		require.NoError(t, err)
	})
}

func TestAgentConfigurationService_ProductionConfigKeyChangesNeedProductionTier(t *testing.T) {
	const org, proj, agent, env = "default", "default", "extagent", "production"
	configUUID := uuid.New()

	newSvc := func(t *testing.T) *agentConfigurationService {
		t.Helper()
		return &agentConfigurationService{
			ocClient: &clientmocks.OpenChoreoClientMock{
				GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
					return &models.AgentResponse{Name: agent, Provisioning: models.Provisioning{Type: string(utils.ExternalAgent)}}, nil
				},
				GetEnvironmentFunc: func(_ context.Context, _, envName string) (*models.EnvironmentResponse, error) {
					return &models.EnvironmentResponse{Name: envName, IsProduction: true}, nil
				},
			},
			agentConfigRepo: &repomocks.AgentConfigurationRepositoryMock{
				GetByUUIDFunc: func(context.Context, uuid.UUID, string) (*models.AgentConfiguration, error) {
					t.Error("a refused change must not resolve the configuration")
					return nil, utils.ErrAgentConfigNotFound
				},
			},
			apiKeyBroadcaster:     &apiKeyBroadcaster{},
			llmProxyAPIKeyService: &LLMProxyAPIKeyService{},
			logger:                discardLogger(),
		}
	}

	changes := map[string]func(s *agentConfigurationService, ctx context.Context) error{
		"create MCP key": func(s *agentConfigurationService, ctx context.Context) error {
			_, err := s.CreateMCPConfigAPIKey(ctx, org, proj, agent, configUUID, env, &models.CreateAPIKeyRequest{Name: "k1"})
			return err
		},
		"rotate MCP key": func(s *agentConfigurationService, ctx context.Context) error {
			_, err := s.RotateMCPConfigAPIKey(ctx, org, proj, agent, configUUID, env, "k1", &models.RotateAPIKeyRequest{})
			return err
		},
		"revoke MCP key": func(s *agentConfigurationService, ctx context.Context) error {
			return s.RevokeMCPConfigAPIKey(ctx, org, proj, agent, configUUID, env, "k1")
		},
		"create LLM key": func(s *agentConfigurationService, ctx context.Context) error {
			_, err := s.CreateLLMConfigAPIKey(ctx, org, proj, agent, configUUID, env, &models.CreateAPIKeyRequest{Name: "k1"})
			return err
		},
		"rotate LLM key": func(s *agentConfigurationService, ctx context.Context) error {
			_, err := s.RotateLLMConfigAPIKey(ctx, org, proj, agent, configUUID, env, "k1", &models.RotateAPIKeyRequest{})
			return err
		},
		"revoke LLM key": func(s *agentConfigurationService, ctx context.Context) error {
			return s.RevokeLLMConfigAPIKey(ctx, org, proj, agent, configUUID, env, "k1")
		},
	}
	for name, change := range changes {
		t.Run(name+" is refused with only the non-production tier", func(t *testing.T) {
			err := change(newSvc(t), tierCtx(t, rbac.AgentEnvNonProduction))

			require.ErrorIs(t, err, utils.ErrForbidden)
			assert.Contains(t, err.Error(), rbac.AgentEnvProduction.Scope())
		})
	}
}

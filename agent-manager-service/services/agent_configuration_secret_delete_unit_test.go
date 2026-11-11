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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

func TestCleanupSecretReference_AlsoDeletesLegacyReference(t *testing.T) {
	store := &clientmocks.SecretManagementClientMock{
		// The store returns success even when no row exists.
		DeleteSecretByNameFunc: func(context.Context, string, string) error { return nil },
	}
	oc := &clientmocks.OpenChoreoClientMock{
		DeleteAgentSecretReferenceFunc: func(context.Context, string, string, string, string) error { return nil },
	}
	svc := &agentManagerService{secretMgmtClient: store, ocClient: oc, logger: discardLogger()}

	svc.cleanupSecretReference(context.Background(), "tenant", "project", "agent", "legacy-secret")

	require.Len(t, store.DeleteSecretByNameCalls(), 1)
	calls := oc.DeleteAgentSecretReferenceCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "tenant", calls[0].OuID)
	assert.Equal(t, "project", calls[0].ProjectName)
	assert.Equal(t, "agent", calls[0].ComponentName)
	assert.Equal(t, "legacy-secret", calls[0].SecretRefName)
}

// TestProcessEnvRemoval_RenamedConfig_DeletesSecretByPersistedName guards the
// rename case: a configuration renamed in the same update that removes an
// environment reaches Scenario D with its new name, but the removed
// environment's proxy API key secret was created under the old one. The
// secret must be deleted by the SecretReference name persisted at creation,
// not by a name derived from the new configuration name. The location-based
// DeleteSecret mock has no func set, so using it would panic.
func TestProcessEnvRemoval_RenamedConfig_DeletesSecretByPersistedName(t *testing.T) {
	configUUID, envUUID := uuid.New(), uuid.New()
	persistedName := "old-cfg-dev-old-cfg-proxy-secrets"
	var deletedOrg, deletedName string
	secretClient := &clientmocks.SecretManagementClientMock{
		DeleteSecretByNameFunc: func(_ context.Context, orgName, secretRefName string) error {
			deletedOrg, deletedName = orgName, secretRefName
			return nil
		},
	}
	database, err := gorm.Open(postgres.New(postgres.Config{Conn: &noopTxPool{}}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	svc := &agentConfigurationService{
		db:           database,
		logger:       discardLogger(),
		secretClient: secretClient,
		ocClient: &clientmocks.OpenChoreoClientMock{
			RemoveReleaseBindingEnvVarsFunc: func(context.Context, string, string, string, string, []string) error { return nil },
		},
		envVariableRepo: &repomocks.AgentEnvConfigVariableRepositoryMock{
			ListByConfigAndEnvFunc: func(context.Context, uuid.UUID, uuid.UUID) ([]models.AgentEnvConfigVariable, error) {
				return []models.AgentEnvConfigVariable{
					{VariableName: "OLD_CFG_URL"},
					{VariableName: "OLD_CFG_API_KEY", SecretReference: persistedName},
				}, nil
			},
			DeleteByConfigAndEnvFunc: func(context.Context, *gorm.DB, uuid.UUID, uuid.UUID) error { return nil },
		},
		envMappingRepo: &repomocks.EnvAgentModelMappingRepositoryMock{
			DeleteFunc: func(context.Context, *gorm.DB, uint) error { return nil },
		},
	}
	mapping := &models.EnvAgentModelMapping{
		ID: 7, ConfigUUID: configUUID, EnvironmentUUID: envUUID,
		LLMProxy: &models.LLMProxy{Handle: "old-cfg-proxy"},
	}

	err = svc.processEnvRemoval(context.Background(), configUUID, envUUID.String(), mapping,
		"new-cfg", "dev", "acme", "proj1", "my-agent", platformAgentPolicy, nil, false)

	require.NoError(t, err)
	assert.Equal(t, "acme", deletedOrg)
	assert.Equal(t, persistedName, deletedName,
		"the secret is deleted by its persisted name, not one derived from the new configuration name")
}

// TestDeleteProxyAPIKeySecret_PicksPersistedNameOverLocation covers both
// branches of the helper that Scenario D and deleteLLMConfig share: a
// persisted SecretReference name is deleted by name, and only a missing one
// falls back to the location's derived name.
func TestDeleteProxyAPIKeySecret_PicksPersistedNameOverLocation(t *testing.T) {
	loc := secretmanagersvc.SecretLocation{
		OrgName: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "dev",
		ConfigName: "new-cfg", EntityName: "old-cfg-proxy", SecretKey: secretmanagersvc.SecretKeyAPIKey,
	}

	t.Run("persisted name", func(t *testing.T) {
		var deletedName string
		svc := &agentConfigurationService{secretClient: &clientmocks.SecretManagementClientMock{
			DeleteSecretByNameFunc: func(_ context.Context, _, secretRefName string) error {
				deletedName = secretRefName
				return nil
			},
		}}

		require.NoError(t, svc.deleteProxyAPIKeySecret(context.Background(), loc, "old-cfg-dev-old-cfg-proxy-secrets"))
		assert.Equal(t, "old-cfg-dev-old-cfg-proxy-secrets", deletedName)
	})

	t.Run("no persisted name", func(t *testing.T) {
		var deletedLoc secretmanagersvc.SecretLocation
		var deletedName string
		svc := &agentConfigurationService{secretClient: &clientmocks.SecretManagementClientMock{
			DeleteSecretFunc: func(_ context.Context, location secretmanagersvc.SecretLocation, secretRefName string) error {
				deletedLoc, deletedName = location, secretRefName
				return nil
			},
		}}

		require.NoError(t, svc.deleteProxyAPIKeySecret(context.Background(), loc, ""))
		assert.Equal(t, loc, deletedLoc)
		assert.Equal(t, loc.SecretRefName(), deletedName)
	})
}

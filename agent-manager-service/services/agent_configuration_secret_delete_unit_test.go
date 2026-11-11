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
)

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

// TestStoreProxyAPIKey_SkipsExternalAgents covers both policies: a platform agent's key is
// stored for injection, while an external agent's key is handed over and never stored.
func TestStoreProxyAPIKey_SkipsExternalAgents(t *testing.T) {
	loc := secretmanagersvc.SecretLocation{
		OrgName: "acme", ProjectName: "proj1", AgentName: "my-agent", EnvironmentName: "default",
		ConfigName: "cfg", EntityName: "cfg-proxy", SecretKey: secretmanagersvc.SecretKeyAPIKey,
	}

	t.Run("platform agent", func(t *testing.T) {
		var stored map[string]string
		svc := &agentConfigurationService{secretClient: &clientmocks.SecretManagementClientMock{
			CreateSecretFunc: func(_ context.Context, _ secretmanagersvc.SecretLocation, data map[string]string) (string, error) {
				stored = data
				return "cfg-secret", nil
			},
		}}

		refName, storedLoc, err := svc.storeProxyAPIKey(context.Background(), platformAgentPolicy, loc, "key")
		require.NoError(t, err)
		assert.Equal(t, "cfg-secret", refName)
		require.NotNil(t, storedLoc)
		assert.Equal(t, loc, *storedLoc)
		assert.Equal(t, map[string]string{secretmanagersvc.SecretKeyAPIKey: "key"}, stored)
	})

	t.Run("external agent", func(t *testing.T) {
		svc := &agentConfigurationService{secretClient: &clientmocks.SecretManagementClientMock{}}

		refName, storedLoc, err := svc.storeProxyAPIKey(context.Background(), externalAgentPolicy, loc, "key")
		require.NoError(t, err)
		assert.Empty(t, refName)
		assert.Nil(t, storedLoc)
	})
}

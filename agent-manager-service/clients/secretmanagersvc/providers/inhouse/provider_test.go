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

package inhouse_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc/providers/inhouse"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/vault/vaulttest"
)

func newClient(t *testing.T) secretmanagersvc.SecretManagementClient {
	t.Helper()
	client, err := secretmanagersvc.NewSecretManagementClient(&secretmanagersvc.StoreConfig{
		Provider: inhouse.ProviderName,
		DB:       db.GetDB(),
		Secrets:  repositories.NewSecretStore(vaulttest.Local(t)),
	}, inhouse.NewProvider())
	require.NoError(t, err)
	return client
}

func TestProviderNeedsTheDatabase(t *testing.T) {
	_, err := secretmanagersvc.NewSecretManagementClient(
		&secretmanagersvc.StoreConfig{Provider: inhouse.ProviderName}, inhouse.NewProvider())
	require.Error(t, err)
}

func TestSecretLifecycle(t *testing.T) {
	ctx, client := context.Background(), newClient(t)
	org := dbtest.Org(t, db.GetDB())
	loc := secretmanagersvc.SecretLocation{
		OrgName: org, ProjectName: "checkout", EnvironmentName: "default", AgentName: "cart", EntityName: "cart-llm",
	}
	path, err := loc.KVPath()
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.DeleteSecret(ctx, loc, "") })

	_, err = client.GetSecret(ctx, path)
	require.ErrorIs(t, err, secretmanagersvc.ErrSecretNotFound)
	_, err = client.PatchSecret(ctx, loc, map[string]string{"k": "v"}, nil)
	require.ErrorIs(t, err, secretmanagersvc.ErrSecretNotFound, "a patch never creates a secret")

	name, err := client.CreateSecret(ctx, loc, map[string]string{"api-key": "sk-123", "client-id": "abc"})
	require.NoError(t, err)
	assert.Equal(t, loc.SecretRefName(), name)

	info, err := client.GetSecret(ctx, path)
	require.NoError(t, err)
	assert.Equal(t, []string{"api-key", "client-id"}, info.Keys)

	var rows []models.Secret
	require.NoError(t, db.GetDB().Where("org_id = ? AND owner_kind = ?", org, "named_secret").Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.NotContains(t, string(rows[0].Ciphertext), "sk-123", "values are encrypted at rest")

	_, err = client.PatchSecret(ctx, loc, map[string]string{"api-key": "sk-456", "extra": "x"}, []string{"client-id"})
	require.NoError(t, err)
	info, err = client.GetSecret(ctx, path)
	require.NoError(t, err)
	assert.Equal(t, []string{"api-key", "extra"}, info.Keys)

	// A create replaces everything the secret held.
	_, err = client.CreateSecret(ctx, loc, map[string]string{"only": "one"})
	require.NoError(t, err)
	info, err = client.GetSecret(ctx, path)
	require.NoError(t, err)
	assert.Equal(t, []string{"only"}, info.Keys)
	require.NoError(t, db.GetDB().Where("org_id = ? AND owner_kind = ?", org, "named_secret").Find(&rows).Error)
	assert.Len(t, rows, 1, "one row per secret name")

	require.NoError(t, client.DeleteSecretByName(ctx, org, name))
	_, err = client.GetSecret(ctx, path)
	require.ErrorIs(t, err, secretmanagersvc.ErrSecretNotFound)
	require.NoError(t, client.DeleteSecret(ctx, loc, name), "deleting a missing secret is a no-op")
}

func TestSecretsAreScopedToTheirOrganization(t *testing.T) {
	ctx, client := context.Background(), newClient(t)
	a, b := dbtest.Org(t, db.GetDB()), dbtest.Org(t, db.GetDB())
	locA := secretmanagersvc.SecretLocation{OrgName: a, EntityName: "shared-name"}
	locB := secretmanagersvc.SecretLocation{OrgName: b, EntityName: "shared-name"}
	t.Cleanup(func() { _ = client.DeleteSecret(ctx, locA, ""); _ = client.DeleteSecret(ctx, locB, "") })

	_, err := client.CreateSecret(ctx, locA, map[string]string{"a": "1"})
	require.NoError(t, err)
	_, err = client.CreateSecret(ctx, locB, map[string]string{"b": "2"})
	require.NoError(t, err)

	pathA, _ := locA.KVPath()
	info, err := client.GetSecret(ctx, pathA)
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, info.Keys)

	require.NoError(t, client.DeleteSecret(ctx, locA, ""))
	pathB, _ := locB.KVPath()
	info, err = client.GetSecret(ctx, pathB)
	require.NoError(t, err)
	assert.Equal(t, []string{"b"}, info.Keys)
}

func TestSecretLimits(t *testing.T) {
	ctx, client := context.Background(), newClient(t)
	loc := secretmanagersvc.SecretLocation{OrgName: dbtest.Org(t, db.GetDB()), EntityName: "limits"}

	_, err := client.CreateSecret(ctx, loc, nil)
	require.ErrorIs(t, err, utils.ErrInvalidInput)

	_, err = client.CreateSecret(ctx, loc, map[string]string{"big": strings.Repeat("x", inhouse.MaxSecretBytes+1)})
	require.ErrorIs(t, err, utils.ErrInvalidInput)

	many := map[string]string{}
	for i := 0; i <= inhouse.MaxSecretKeys; i++ {
		many[strings.Repeat("k", i+1)] = "v"
	}
	_, err = client.CreateSecret(ctx, loc, many)
	require.ErrorIs(t, err, utils.ErrInvalidInput)
}

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
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc/providers/inhouse"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
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

func TestSecretsOfDifferentAgentsNeverShareARow(t *testing.T) {
	ctx, client := context.Background(), newClient(t)
	org := dbtest.Org(t, db.GetDB())
	// A long config name pushes the entity's hash past SecretRefName's 63-character cut, so both locations below
	// have the same SecretRefName. They must still be separate secrets.
	config := "customer-support-assistant-primary-model-config"
	a := secretmanagersvc.SecretLocation{
		OrgName: org, ProjectName: "p1", AgentName: "a1", EnvironmentName: "production", ConfigName: config,
		EntityName: "customer-s-1111111111111111-proxy", SecretKey: secretmanagersvc.SecretKeyAPIKey,
	}
	b := a
	b.ProjectName, b.AgentName, b.EntityName = "p2", "a2", "customer-s-2222222222222222-proxy"
	require.Equal(t, a.SecretRefName(), b.SecretRefName(), "precondition: the display names collide")
	t.Cleanup(func() { _ = client.DeleteSecret(ctx, a, ""); _ = client.DeleteSecret(ctx, b, "") })

	refA, err := client.CreateSecret(ctx, a, map[string]string{"api-key": "key-a"})
	require.NoError(t, err)
	refB, err := client.CreateSecret(ctx, b, map[string]string{"api-key": "key-b"})
	require.NoError(t, err)
	assert.NotEqual(t, refA, refB)

	var n int64
	require.NoError(t, db.GetDB().Model(&models.Secret{}).Where("org_id = ?", org).Count(&n).Error)
	assert.Equal(t, int64(2), n, "each agent's secret has its own row")

	require.NoError(t, client.DeleteSecret(ctx, b, ""))
	pathA, err := a.KVPath()
	require.NoError(t, err)
	info, err := client.GetSecret(ctx, pathA)
	require.NoError(t, err, "deleting one agent's secret leaves the other's")
	assert.Equal(t, []string{"api-key"}, info.Keys)
}

func TestScopeFieldsKeepTheirPositions(t *testing.T) {
	ctx, client := context.Background(), newClient(t)
	org := dbtest.Org(t, db.GetDB())
	project := secretmanagersvc.SecretLocation{OrgName: org, ProjectName: "p", EntityName: "model"}
	environment := secretmanagersvc.SecretLocation{OrgName: org, EnvironmentName: "p", EntityName: "model"}
	orgLevel := secretmanagersvc.SecretLocation{OrgName: org, EntityName: "model"}
	locations := []secretmanagersvc.SecretLocation{project, environment, orgLevel}
	t.Cleanup(func() {
		for _, loc := range locations {
			_ = client.DeleteSecret(ctx, loc, "")
		}
	})

	refs := map[string]string{}
	for i, loc := range locations {
		value := []string{"project", "environment", "org"}[i]
		ref, err := client.CreateSecret(ctx, loc, map[string]string{value: "v"})
		require.NoError(t, err)
		refs[ref] = value
	}
	require.Len(t, refs, 3, "a project, an environment of the same name and the organization are three secrets")
	for ref, value := range refs {
		info, err := client.GetSecret(ctx, ref)
		require.NoError(t, err, "reference %q reads back through the client", ref)
		assert.Equal(t, []string{value}, info.Keys, "reference %q finds its own secret", ref)
	}

	require.NoError(t, client.DeleteSecret(ctx, environment, ""))
	for _, loc := range []secretmanagersvc.SecretLocation{project, orgLevel} {
		ref, err := client.PatchSecret(ctx, loc, map[string]string{"k": "v"}, nil)
		require.NoError(t, err)
		_, err = client.GetSecret(ctx, ref)
		require.NoError(t, err, "deleting the environment secret leaves the others")
	}
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
	assert.Equal(t, org+"/checkout/default/cart/./cart-llm", name, "the reference keeps every scope field in place")
	byRef, err := client.GetSecret(ctx, name)
	require.NoError(t, err, "the returned reference finds the secret")
	assert.Equal(t, []string{"api-key", "client-id"}, byRef.Keys)

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

	_, err = client.PatchSecret(ctx, loc, nil, []string{"only"})
	require.NoError(t, err)
	info, err = client.GetSecret(ctx, path)
	require.NoError(t, err)
	assert.Empty(t, info.Keys, "patching away the final key preserves the named secret")

	require.NoError(t, client.DeleteSecretByName(ctx, org, name))
	_, err = client.GetSecret(ctx, path)
	require.ErrorIs(t, err, secretmanagersvc.ErrSecretNotFound)
	require.NoError(t, client.DeleteSecret(ctx, loc, name), "deleting a missing secret is a no-op")
}

func TestConcurrentPatchesKeepEveryKey(t *testing.T) {
	ctx, client := context.Background(), newClient(t)
	org := dbtest.Org(t, db.GetDB())
	loc := secretmanagersvc.SecretLocation{
		OrgName: org, ProjectName: "checkout", EnvironmentName: "default", AgentName: "cart", EntityName: "cart-llm",
	}
	path, err := loc.KVPath()
	require.NoError(t, err)
	_, err = client.CreateSecret(ctx, loc, map[string]string{"initial": "value"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.DeleteSecret(ctx, loc, "") })

	const patchCount = 16
	start := make(chan struct{})
	results := make(chan error, patchCount)
	for i := range patchCount {
		go func() {
			<-start
			_, err := client.PatchSecret(ctx, loc, map[string]string{fmt.Sprintf("key-%d", i): "value"}, nil)
			results <- err
		}()
	}
	close(start)
	for range patchCount {
		require.NoError(t, <-results)
	}
	info, err := client.GetSecret(ctx, path)
	require.NoError(t, err)
	assert.Len(t, info.Keys, patchCount+1)
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
	require.ErrorIs(t, err, httpx.ErrInvalidInput)

	_, err = client.CreateSecret(ctx, loc, map[string]string{"big": strings.Repeat("x", inhouse.MaxSecretBytes+1)})
	require.ErrorIs(t, err, httpx.ErrInvalidInput)

	many := map[string]string{}
	for i := 0; i <= inhouse.MaxSecretKeys; i++ {
		many[strings.Repeat("k", i+1)] = "v"
	}
	_, err = client.CreateSecret(ctx, loc, many)
	require.ErrorIs(t, err, httpx.ErrInvalidInput)
}

func TestPatchingAPromotedSecretKeepsEveryKey(t *testing.T) {
	ctx := context.Background()
	vaults, external := vaulttest.LocalAndExternal(t)
	client, err := secretmanagersvc.NewSecretManagementClient(&secretmanagersvc.StoreConfig{
		Provider: inhouse.ProviderName,
		DB:       db.GetDB(),
		Secrets:  repositories.NewSecretStore(vaults),
	}, inhouse.NewProvider())
	require.NoError(t, err)
	runner := repositories.NewSecretJobRunner(db.GetDB(), vaults, slog.New(slog.DiscardHandler))
	promote := func() {
		t.Helper()
		_, failed, err := runner.RunOnce(ctx, time.Now().UTC())
		require.NoError(t, err)
		require.Zero(t, failed)
	}
	org := dbtest.Org(t, db.GetDB())
	loc := secretmanagersvc.SecretLocation{OrgName: org, EntityName: "promoted"}
	ref, err := client.CreateSecret(ctx, loc, map[string]string{"a": "1"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.DeleteSecret(ctx, loc, ""); promote() })
	promote()
	require.Equal(t, 1, external.Len(), "precondition: the secret lives in the external vault")

	_, err = client.PatchSecret(ctx, loc, map[string]string{"b": "2"}, nil)
	require.NoError(t, err)
	info, err := client.GetSecret(ctx, ref)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, info.Keys)
}

// Every in-house secret is managed by the default manager, so a caller naming another one cannot delete it.
func TestDeleteRefusesAnotherManager(t *testing.T) {
	ctx := context.Background()
	low, err := inhouse.NewProvider().NewClient(&secretmanagersvc.StoreConfig{
		Provider: inhouse.ProviderName,
		DB:       db.GetDB(),
		Secrets:  repositories.NewSecretStore(vaulttest.Local(t)),
	})
	require.NoError(t, err)
	org := dbtest.Org(t, db.GetDB())
	location := secretmanagersvc.SecretLocation{OrgName: org, EntityName: "managed"}
	name, err := low.PushSecret(ctx, location, map[string]string{"k": "v"}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = low.DeleteSecretByName(ctx, org, name, nil) })

	err = low.DeleteSecretByName(ctx, org, name, &secretmanagersvc.SecretMetadata{ManagedBy: "another-client"})
	require.ErrorIs(t, err, secretmanagersvc.ErrNotManaged)
	_, err = low.GetSecret(ctx, location)
	require.NoError(t, err, "a refused delete leaves the secret")

	require.NoError(t, low.DeleteSecretByName(ctx, org, name,
		&secretmanagersvc.SecretMetadata{ManagedBy: secretmanagersvc.DefaultManagedBy}))
	_, err = low.GetSecret(ctx, location)
	require.ErrorIs(t, err, secretmanagersvc.ErrSecretNotFound)
}

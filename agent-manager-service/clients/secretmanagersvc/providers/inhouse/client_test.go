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

package inhouse

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	secretmanagersvc "github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func newTestClient(t *testing.T) *Client {
	t.Helper()
	c, err := NewClient(&secretmanagersvc.InHouseConfig{
		DB:            db.GetDB(),
		EncryptionKey: bytes.Repeat([]byte{3}, 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestInHouseSecretLifecycle(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)
	ou := "ou-" + uuid.NewString()
	loc := secretmanagersvc.SecretLocation{OrgName: ou, ProjectName: "p", AgentName: "a", EnvironmentName: "dev", EntityName: "agent"}
	meta := &secretmanagersvc.SecretMetadata{ManagedBy: secretmanagersvc.DefaultManagedBy}
	t.Cleanup(func() { db.GetDB().Exec("DELETE FROM secrets WHERE ou_id = ?", ou) })

	name, err := c.PushSecret(ctx, loc, map[string]string{"B": "2", "A": "1"}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := c.GetSecret(ctx, loc); err != nil || len(info.Keys) != 2 || info.Keys[0] != "A" {
		t.Fatalf("unexpected stored keys: %+v (%v)", info, err)
	}

	// Stored encrypted, not in plaintext.
	var ciphertext []byte
	db.GetDB().Raw("SELECT ciphertext FROM secrets WHERE ou_id = ? AND name = ?", ou, name).Scan(&ciphertext)
	if len(ciphertext) == 0 || bytes.Contains(ciphertext, []byte(`"A":"1"`)) {
		t.Fatal("secret is not stored encrypted")
	}

	if _, err := c.PatchSecret(ctx, loc, map[string]string{"C": "3"}, []string{"B"}, meta); err != nil {
		t.Fatal(err)
	}
	data, err := c.GetSecretData(ctx, ou, name)
	if err != nil || len(data) != 2 || data["A"] != "1" || data["C"] != "3" {
		t.Fatalf("after patch: %v (%v)", data, err)
	}
	if info, err := c.GetSecret(ctx, loc); err != nil || len(info.Keys) != 2 || info.Keys[0] != "A" || info.Keys[1] != "C" {
		t.Fatalf("stored keys after patch: %+v (%v)", info, err)
	}

	if v, err := c.Resolve(ctx, ou, name, "C"); err != nil || v != "3" {
		t.Fatalf("Resolve = %q (%v)", v, err)
	}
	if _, err := c.Resolve(ctx, ou, name, "B"); !errors.Is(err, secretmanagersvc.ErrSecretNotFound) {
		t.Fatalf("Resolve of a removed key: %v", err)
	}

	info, err := c.GetSecret(ctx, loc)
	if err != nil || info.Labels[secretmanagersvc.LabelKeyManagedBy] != secretmanagersvc.DefaultManagedBy {
		t.Fatalf("GetSecret = %+v (%v)", info, err)
	}

	// Another owner can neither overwrite nor delete it.
	other := &secretmanagersvc.SecretMetadata{ManagedBy: "someone-else"}
	if _, err := c.PushSecret(ctx, loc, map[string]string{"A": "x"}, other); !errors.Is(err, secretmanagersvc.ErrNotManaged) {
		t.Fatalf("push by another owner: %v", err)
	}
	if err := c.DeleteSecret(ctx, loc, other); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetSecret(ctx, loc); err != nil {
		t.Fatal("secret was deleted by another owner")
	}

	if err := c.DeleteSecret(ctx, loc, meta); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetSecret(ctx, loc); !errors.Is(err, secretmanagersvc.ErrSecretNotFound) {
		t.Fatalf("after delete: %v", err)
	}
	if err := c.DeleteSecret(ctx, loc, meta); err != nil {
		t.Fatalf("delete is not idempotent: %v", err)
	}
}

func TestImportLegacySecretPreservesExistingValues(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)
	ouID := uuid.NewString()
	const name = "legacy-agent-default-secrets"
	t.Cleanup(func() {
		require.NoError(t, c.db.Where("ou_id = ?", ouID).Delete(&secretRow{}).Error)
	})

	data := map[string]string{"TOKEN": "test-value", "config": "line 1\nline 2\n"}
	created, err := c.ImportLegacySecret(ctx, ouID, name, data)
	require.NoError(t, err)
	require.True(t, created)
	value, err := c.Resolve(ctx, ouID, name, "config")
	require.NoError(t, err)
	require.Equal(t, data["config"], value)

	created, err = c.ImportLegacySecret(ctx, ouID, name, data)
	require.NoError(t, err)
	require.False(t, created)

	created, err = c.ImportLegacySecret(ctx, ouID, name, map[string]string{"TOKEN": "different-value"})
	require.ErrorContains(t, err, "destination already contains a different secret")
	require.False(t, created)
	stored, err := c.GetSecretData(ctx, ouID, name)
	require.NoError(t, err)
	require.Equal(t, data, stored)
}

func TestInHousePatchMissingSecret(t *testing.T) {
	c := newTestClient(t)
	loc := secretmanagersvc.SecretLocation{OrgName: "ou-" + uuid.NewString(), EntityName: "missing"}
	_, err := c.PatchSecret(context.Background(), loc, map[string]string{"A": "1"}, nil, &secretmanagersvc.SecretMetadata{ManagedBy: "m"})
	if !errors.Is(err, secretmanagersvc.ErrSecretNotFound) {
		t.Fatalf("PatchSecret on a missing secret: %v", err)
	}
}

func TestGitSecretVaultLifecycle(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)
	tenant, other := uuid.NewString(), uuid.NewString()
	t.Cleanup(func() {
		require.NoError(t, c.db.Where("ou_id IN ?", []string{tenant, other}).Delete(&secretRow{}).Error)
	})
	require.NoError(t, c.CreateGitSecret(ctx, tenant, "private", "", "first-token"))
	ref := occlient.GitSecretReferenceName(tenant, "private")
	require.NotEqual(t, ref, occlient.GitSecretReferenceName(other, "private"))

	names, count, err := c.ListGitSecretNames(ctx, tenant, 10, 0)
	require.NoError(t, err)
	require.Equal(t, []string{"private"}, names)
	require.Equal(t, 1, count)
	_, err = c.ReadGitSecret(ctx, other, "private")
	require.ErrorIs(t, err, utils.ErrGitSecretNotFound)

	require.ErrorIs(t, c.CreateGitSecret(ctx, tenant, "private", "other", "second-token"), utils.ErrGitSecretAlreadyExists)
	got, err := c.ReadGitSecret(ctx, tenant, "private")
	require.NoError(t, err)
	require.Equal(t, map[string]string{"username": "git", "password": "first-token"}, got)
	value, err := c.Resolve(ctx, tenant, ref, "password")
	require.NoError(t, err)
	require.Equal(t, "first-token", value)

	require.NoError(t, c.DeleteGitSecret(ctx, tenant, "private"))
	_, err = c.Resolve(ctx, tenant, ref, "password")
	require.ErrorIs(t, err, secretmanagersvc.ErrSecretNotFound)
}

func TestPlatformVaultSeed(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)
	t.Cleanup(func() {
		require.NoError(t, c.db.Where("ou_id = ? AND name = ?", platformSecretScope, platformSecretName).Delete(&secretRow{}).Error)
	})
	require.Error(t, c.SeedPlatformSecrets(ctx, `{"other":"value"}`))
	seed := `{"workflow-plane-oauth-client-secret":"fixture"}`
	require.NoError(t, c.SeedPlatformSecrets(ctx, seed))
	row, err := c.find(ctx, c.db, platformSecretScope, platformSecretName, false)
	require.NoError(t, err)
	require.False(t, bytes.Contains(row.Ciphertext, []byte("fixture")))
	require.NoError(t, c.SeedPlatformSecrets(ctx, seed))
	repeated, err := c.find(ctx, c.db, platformSecretScope, platformSecretName, false)
	require.NoError(t, err)
	require.Equal(t, row.Ciphertext, repeated.Ciphertext)
	require.NoError(t, c.SeedPlatformSecrets(ctx, `{"workflow-plane-oauth-client-secret":"rotated"}`))
	got, err := c.Resolve(ctx, platformSecretScope, platformSecretName, "workflow-plane-oauth-client-secret")
	require.NoError(t, err)
	require.Equal(t, "rotated", got)
}

func TestInHouseSecretLimits(t *testing.T) {
	c := newTestClient(t)
	loc := secretmanagersvc.SecretLocation{OrgName: "ou-" + uuid.NewString(), EntityName: "big"}
	meta := &secretmanagersvc.SecretMetadata{ManagedBy: "m"}

	tooMany := make(map[string]string, MaxSecretKeys+1)
	for i := 0; i <= MaxSecretKeys; i++ {
		tooMany[fmt.Sprintf("K%d", i)] = "v"
	}
	_, err := c.PushSecret(context.Background(), loc, tooMany, meta)
	require.ErrorIs(t, err, utils.ErrInvalidInput)

	_, err = c.PushSecret(context.Background(), loc, map[string]string{"K": strings.Repeat("x", DefaultMaxSecretBytes)}, meta)
	require.ErrorIs(t, err, utils.ErrInvalidInput)

	// The size counts raw keys and values: a value that grows six-fold in JSON
	// (control characters escape as \u00XX) still fits when its raw size does.
	escaping := strings.Repeat("\x01", DefaultMaxSecretBytes/2)
	_, err = c.PushSecret(context.Background(), loc, map[string]string{"K": escaping}, meta)
	require.NoError(t, err)
}

func TestInHouseSecretLimitFollowsConfig(t *testing.T) {
	c := newTestClient(t)
	c.maxBytes = 300 << 10
	loc := secretmanagersvc.SecretLocation{OrgName: "ou-" + uuid.NewString(), EntityName: "file"}
	// A 290 KiB sensitive file fits a store sized for the file mount budget.
	_, err := c.PushSecret(context.Background(), loc, map[string]string{"cert.pem": strings.Repeat("x", 290<<10)},
		&secretmanagersvc.SecretMetadata{ManagedBy: "m"})
	require.NoError(t, err)
}

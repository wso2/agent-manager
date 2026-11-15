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

package repositories

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/vault"
	"github.com/wso2/agent-manager/agent-manager-service/vault/vaulttest"
)

func secretRow(t *testing.T, id uuid.UUID) models.Secret {
	t.Helper()
	var row models.Secret
	require.NoError(t, db.GetDB().Where("id = ?", id).Take(&row).Error)
	return row
}

func TestSecretStoreSealsValuesIntoTheRow(t *testing.T) {
	ctx, gdb, store := context.Background(), db.GetDB(), testSecrets(t)
	org, owner, id := newOrg(t), newID(), newID()
	t.Cleanup(func() { _ = store.Delete(ctx, gdb, id) })

	require.NoError(t, store.Write(ctx, gdb, org, "test_owner", owner, id, []byte("first")))
	row := secretRow(t, id)
	assert.Equal(t, vault.ProviderLocal, row.Provider)
	assert.NotNil(t, row.KeyID, "the sealing key is recorded for rotation")
	assert.Nil(t, row.ExternalRef)
	assert.NotContains(t, string(row.Ciphertext), "first")
	assert.Equal(t, 1, row.Version)

	value, found, err := store.Read(ctx, gdb, id)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "first", string(value))

	require.NoError(t, store.Write(ctx, gdb, org, "test_owner", owner, id, []byte("second")))
	assert.Equal(t, 2, secretRow(t, id).Version)
	value, _, err = store.Read(ctx, gdb, id)
	require.NoError(t, err)
	assert.Equal(t, "second", string(value))

	require.Error(t, store.Write(ctx, gdb, newOrg(t), "test_owner", owner, id, []byte("x")),
		"another organization cannot overwrite the secret")

	require.NoError(t, store.Delete(ctx, gdb, id))
	_, found, err = store.Read(ctx, gdb, id)
	require.NoError(t, err)
	assert.False(t, found)
	require.NoError(t, store.Delete(ctx, gdb, id), "deleting a missing secret is a no-op")
}

// A ciphertext copied into another row must not open there.
func TestSecretStoreBindsACiphertextToItsRow(t *testing.T) {
	ctx, gdb, store := context.Background(), db.GetDB(), testSecrets(t)
	org, a, b := newOrg(t), newID(), newID()
	t.Cleanup(func() { _ = store.Delete(ctx, gdb, a); _ = store.Delete(ctx, gdb, b) })

	require.NoError(t, store.Write(ctx, gdb, org, "test_owner", a, a, []byte("a-secret")))
	require.NoError(t, store.Write(ctx, gdb, org, "test_owner", b, b, []byte("b-secret")))
	require.NoError(t, gdb.Model(&models.Secret{}).Where("id = ?", b).
		Update("ciphertext", secretRow(t, a).Ciphertext).Error)

	_, _, err := store.Read(ctx, gdb, b)
	require.Error(t, err)
}

// Rows written before the vault hold what the owning service encrypted itself.
func TestSecretStoreReadsRowsWrittenBeforeTheVault(t *testing.T) {
	ctx, gdb, store := context.Background(), db.GetDB(), testSecrets(t)
	org, id, now := newOrg(t), newID(), dbx.Now()
	t.Cleanup(func() { _ = store.Delete(ctx, gdb, id) })

	legacy, err := utils.EncryptBytes([]byte("old-value"), vaulttest.Key)
	require.NoError(t, err)
	require.NoError(t, gdb.Create(&models.Secret{
		ID: id, OrgID: org, OwnerKind: "test_owner", OwnerID: id, Provider: vault.ProviderLocal,
		Ciphertext: legacy, Version: 1, CreatedAt: now, UpdatedAt: now,
	}).Error)

	value, err := store.get(gdb, &id)
	require.NoError(t, err)
	assert.Equal(t, "old-value", value)

	stored, err := store.getAsStored(gdb, &id)
	require.NoError(t, err)
	assert.Equal(t, string(legacy), stored, "an owner that encrypts itself gets its own form back")

	require.NoError(t, store.Write(ctx, gdb, org, "test_owner", id, id, []byte("old-value")))
	assert.NotNil(t, secretRow(t, id).KeyID, "the next write reseals the row under the vault")
	stored, err = store.getAsStored(gdb, &id)
	require.NoError(t, err)
	assert.Equal(t, "old-value", stored)
}

// externalVault keeps values outside the database; the row holds a reference.
type externalVault struct {
	values map[string][]byte
}

func (*externalVault) Provider() string { return "test-external" }

func (e *externalVault) Seal(_ context.Context, ref vault.Ref, plaintext []byte, current *vault.Sealed) (vault.Sealed, error) {
	path := "amp/" + ref.OrgID + "/" + ref.OwnerKind + "/" + ref.SecretID.String()
	if current != nil && current.ExternalRef != "" {
		path = current.ExternalRef
	}
	e.values[path] = plaintext
	return vault.Sealed{ExternalRef: path}, nil
}

func (e *externalVault) Open(_ context.Context, _ vault.Ref, sealed vault.Sealed) ([]byte, error) {
	return e.values[sealed.ExternalRef], nil
}

func (e *externalVault) Destroy(_ context.Context, _ vault.Ref, sealed vault.Sealed) error {
	delete(e.values, sealed.ExternalRef)
	return nil
}

func TestSecretStoreKeepsOnlyAReferenceForAnExternalVault(t *testing.T) {
	ctx, gdb := context.Background(), db.GetDB()
	local, err := vault.NewLocal(vaulttest.Key)
	require.NoError(t, err)
	external := &externalVault{values: map[string][]byte{}}
	localStore := NewSecretStore(vault.Of(local))
	store := NewSecretStore(vault.Of(external, local))
	org, id := newOrg(t), newID()
	t.Cleanup(func() { _ = store.Delete(ctx, gdb, id) })

	// A value the local vault sealed moves to the external one on its next write.
	require.NoError(t, localStore.Write(ctx, gdb, org, "test_owner", id, id, []byte("v1")))
	value, _, err := store.Read(ctx, gdb, id)
	require.NoError(t, err)
	assert.Equal(t, "v1", string(value), "the previous vault's values stay readable")

	require.NoError(t, store.Write(ctx, gdb, org, "test_owner", id, id, []byte("v2")))
	row := secretRow(t, id)
	assert.Equal(t, "test-external", row.Provider)
	assert.Empty(t, row.Ciphertext, "nothing of the value stays in the database")
	assert.Nil(t, row.KeyID)
	require.NotNil(t, row.ExternalRef)
	assert.Equal(t, "v2", string(external.values[*row.ExternalRef]))

	require.NoError(t, store.Write(ctx, gdb, org, "test_owner", id, id, []byte("v3")))
	assert.Equal(t, *row.ExternalRef, *secretRow(t, id).ExternalRef, "a rewrite keeps the same entry")
	assert.Len(t, external.values, 1)

	require.ErrorIs(t, deleteRowSecret(gdb, &id), errSecretHeldExternally,
		"a delete path without the store must not orphan the external value")

	require.NoError(t, store.Delete(ctx, gdb, id))
	assert.Empty(t, external.values, "deleting the secret removes it from the vault")
	require.NoError(t, deleteRowSecret(gdb, &id), "a missing row is a no-op")
}

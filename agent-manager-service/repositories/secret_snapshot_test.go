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
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/vault"
)

func TestSecretSnapshots(t *testing.T) {
	ctx, gdb, store := context.Background(), db.GetDB(), testSecrets(t)
	org, owner, id := newOrg(t), NewID(), NewID()
	t.Cleanup(func() { _ = store.Delete(ctx, gdb, id) })
	require.NoError(t, store.Write(ctx, gdb, org, "test_owner", owner, id, []byte("v1")))

	// A missing reference is no credential; a dangling one is an error for a deployment credential but none for a
	// secret an owner may lack.
	none, err := store.SnapshotUpstreamCredential(ctx, gdb, nil)
	require.NoError(t, err)
	assert.Nil(t, none)
	missing := NewID()
	_, err = store.SnapshotUpstreamCredential(ctx, gdb, &missing)
	require.ErrorContains(t, err, "is missing")
	snap, err := store.SnapshotSecret(ctx, gdb, &missing)
	require.NoError(t, err)
	assert.Nil(t, snap)
	snap, err = store.SnapshotSecret(ctx, gdb, nil)
	require.NoError(t, err)
	assert.Nil(t, snap)

	var noStore *SecretStore
	_, err = noStore.SnapshotUpstreamCredential(ctx, gdb, &id)
	require.ErrorIs(t, err, ErrNoSecretStore)
	_, err = noStore.SnapshotSecret(ctx, gdb, &id)
	require.ErrorIs(t, err, ErrNoSecretStore)

	first, err := store.SnapshotUpstreamCredential(ctx, gdb, &id)
	require.NoError(t, err)
	assert.False(t, first.External(), "sealed by the local vault")
	value, err := first.Open(ctx)
	require.NoError(t, err)
	assert.Equal(t, "v1", value)
	value, err = first.OpenAsStored(ctx)
	require.NoError(t, err)
	assert.Equal(t, "v1", value, "a vault-sealed row is opened, not returned raw")

	same, err := store.SnapshotSecret(ctx, gdb, &id)
	require.NoError(t, err)
	assert.True(t, first.SameVersion(same))

	require.NoError(t, store.Write(ctx, gdb, org, "test_owner", owner, id, []byte("v2")))
	second, err := store.SnapshotSecret(ctx, gdb, &id)
	require.NoError(t, err)
	assert.False(t, first.SameVersion(second), "a rewrite is a new version")
	assert.False(t, first.SameVersion(nil))
	value, err = first.Open(ctx)
	require.NoError(t, err)
	assert.Equal(t, "v1", value, "a snapshot opens the value it was taken of")

	// LockVersion holds only the version it was taken of.
	err = gdb.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, second.LockVersion(ctx, tx))
		require.ErrorIs(t, first.LockVersion(ctx, tx), gorm.ErrRecordNotFound, "an older snapshot is stale")
		return errors.New("rollback")
	})
	require.EqualError(t, err, "rollback")

	var nilSnap *UpstreamCredential
	assert.False(t, nilSnap.External())
}

func TestSecretSnapshotOfPreVaultRowIsReturnedAsStored(t *testing.T) {
	ctx, gdb, store := context.Background(), db.GetDB(), testSecrets(t)
	org, id, now := newOrg(t), NewID(), dbx.Now()
	t.Cleanup(func() { _ = store.Delete(ctx, gdb, id) })
	require.NoError(t, gdb.Create(&models.Secret{
		ID: id, OrgID: org, OwnerKind: "test_owner", OwnerID: id, Provider: vault.ProviderLocal,
		Ciphertext: []byte("owner-encrypted-form"), Version: 1, CreatedAt: now, UpdatedAt: now,
	}).Error)

	snap, err := store.SnapshotSecret(ctx, gdb, &id)
	require.NoError(t, err)
	stored, err := snap.OpenAsStored(ctx)
	require.NoError(t, err)
	assert.Equal(t, "owner-encrypted-form", stored)
	_, err = snap.Open(ctx)
	require.ErrorIs(t, err, ErrSecretUnreadable, "the local vault cannot open a form it did not seal")
}

func TestSecretPutGetAndAdopt(t *testing.T) {
	ctx, gdb, store := context.Background(), db.GetDB(), testSecrets(t)
	org, owner := newOrg(t), NewID()

	id, err := store.Put(gdb.WithContext(ctx), org, "test_owner", owner, nil, "")
	require.NoError(t, err)
	assert.Nil(t, id, "an empty value stores nothing")

	id, err = store.Put(gdb.WithContext(ctx), org, "test_owner", owner, nil, "secret")
	require.NoError(t, err)
	require.NotNil(t, id)
	t.Cleanup(func() { _ = store.Delete(ctx, gdb, *id) })
	again, err := store.Put(gdb, org, "test_owner", owner, id, "rotated")
	require.NoError(t, err)
	assert.Equal(t, *id, *again, "a current id is rewritten in place")
	value, err := store.Get(gdb, id)
	require.NoError(t, err)
	assert.Equal(t, "rotated", value)
	value, err = store.Get(gdb, nil)
	require.NoError(t, err)
	assert.Empty(t, value)
	value, err = store.GetAsStored(gdb, nil)
	require.NoError(t, err)
	assert.Empty(t, value)

	newOwner := NewID()
	require.NoError(t, AdoptSecret(gdb, *id, newOwner))
	assert.Equal(t, newOwner, secretRow(t, *id).OwnerID)

	require.NoError(t, ReleaseSecret(gdb, nil))
	require.NoError(t, store.Delete(ctx, gdb, NewID()), "deleting a missing secret is a no-op")
}

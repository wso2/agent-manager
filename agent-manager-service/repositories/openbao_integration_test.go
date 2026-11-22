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
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/vault"
	"github.com/wso2/agent-manager/agent-manager-service/vault/openbao"
)

func TestStandaloneOpenBaoReferenceOnly(t *testing.T) {
	if os.Getenv("OPENBAO_ADDR") == "" {
		t.Skip("set OPENBAO_ADDR and OPENBAO_TOKEN to test a standalone OpenBao KV v2 server")
	}
	ctx := t.Context()
	gdb := db.GetDB()
	org := newOrg(t)
	id := NewID()
	vaults, err := vault.Open(openbao.Provider, vault.Config{EncryptionKey: make([]byte, vault.KeySize)})
	require.NoError(t, err)
	store := NewSecretStore(vaults)
	runner := NewSecretJobRunner(gdb, vaults, slog.New(slog.DiscardHandler))
	writer := vaults.Writer()

	write := func(tx *gorm.DB, value string) error {
		return store.Write(ctx, tx, org, "test_owner", id, id, []byte(value))
	}
	checkValue := func(want string) models.Secret {
		t.Helper()
		row := secretRow(t, id)
		require.Equal(t, openbao.Provider, row.Provider)
		require.Nil(t, row.Ciphertext)
		require.Nil(t, row.KeyID)
		var referenceOnly int64
		require.NoError(t, gdb.Model(&models.Secret{}).Where("id = ? AND ciphertext IS NULL AND key_id IS NULL", id).Count(&referenceOnly).Error)
		require.EqualValues(t, 1, referenceOnly)
		require.NotNil(t, row.ExternalRef)
		got, found, err := store.Read(ctx, gdb, id)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, want, string(got))
		return row
	}

	local, err := vaults.For(vault.ProviderLocal)
	require.NoError(t, err)
	require.NoError(t, NewSecretStore(vault.Of(local)).Write(ctx, gdb, org, "test_owner", id, id, []byte("legacy")))
	require.NotEmpty(t, secretRow(t, id).Ciphertext)
	legacy, found, err := store.Read(ctx, gdb, id)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "legacy", string(legacy))

	require.NoError(t, write(gdb, "first"))
	first := checkValue("first")
	n, err := PendingSecretJobs(ctx, gdb, id)
	require.NoError(t, err)
	require.Zero(t, n, "there is no SQL-staged promotion")
	rollback := errors.New("rollback")
	var uncommitted models.Secret
	err = gdb.Transaction(func(tx *gorm.DB) error {
		if err := write(tx, "discarded"); err != nil {
			return err
		}
		if err := tx.Take(&uncommitted, "id = ?", id).Error; err != nil {
			return err
		}
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	require.Equal(t, first.ExternalRef, checkValue("first").ExternalRef)
	require.NoError(t, writer.Destroy(ctx, secretRef(&uncommitted), sealedOf(&uncommitted)))

	bad, err := openbao.New(os.Getenv("OPENBAO_ADDR"), "invalid-test-token", os.Getenv("OPENBAO_KV_MOUNT"))
	require.NoError(t, err)
	badStore := NewSecretStore(vault.Of(bad))
	require.Error(t, badStore.Write(ctx, gdb, org, "test_owner", id, id, []byte("unavailable")))
	checkValue("first")

	require.NoError(t, write(gdb, "rotated"))
	current := checkValue("rotated")
	require.NotEqual(t, *first.ExternalRef, *current.ExternalRef)
	_, failed, err := runner.RunOnce(ctx, time.Now().UTC())
	require.NoError(t, err)
	require.Zero(t, failed)
	_, err = writer.Open(ctx, secretRef(&first), sealedOf(&first))
	require.ErrorContains(t, err, "HTTP 404")

	require.NoError(t, store.Delete(ctx, gdb, id))
	_, failed, err = runner.RunOnce(ctx, time.Now().UTC())
	require.NoError(t, err)
	require.Zero(t, failed)
	_, err = writer.Open(ctx, secretRef(&current), sealedOf(&current))
	require.ErrorContains(t, err, "HTTP 404")
	require.NoError(t, writer.Destroy(ctx, secretRef(&current), sealedOf(&current)), "destroy can be retried")
}

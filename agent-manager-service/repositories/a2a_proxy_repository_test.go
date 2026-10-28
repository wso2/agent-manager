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

//go:build integration

package repositories

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func newTestProxy(agentName string) *models.A2AProxy {
	return &models.A2AProxy{
		OUID:               "ou-" + uuid.New().String()[:8],
		ProjectName:        "checkout",
		AgentName:          agentName,
		EnvironmentName:    "Development",
		EnvironmentUUID:    uuid.New(),
		UpstreamAuthHeader: "X-Upstream-Key",
		UpstreamAuthSecret: "c2VjcmV0",
	}
}

func cleanupProxy(t *testing.T, repo A2AProxyRepository, proxy *models.A2AProxy) {
	t.Helper()
	t.Cleanup(func() {
		_ = repo.DeleteForAgent(context.Background(), proxy.OUID, proxy.ProjectName, proxy.AgentName)
	})
}

func TestA2AProxyUpsertCreatesAndUpdates(t *testing.T) {
	repo := NewA2AProxyRepository(db.GetDB())
	ctx := context.Background()
	proxy := newTestProxy("upsert-" + uuid.New().String()[:8])
	cleanupProxy(t, repo, proxy)
	require.NoError(t, repo.Upsert(ctx, proxy))

	byName, err := repo.Get(ctx, proxy.OUID, proxy.ProjectName, proxy.AgentName, "Development")
	require.NoError(t, err)
	assert.Equal(t, "X-Upstream-Key", byName.UpstreamAuthHeader)
	byUUID, err := repo.GetByEnvUUID(ctx, proxy.OUID, proxy.ProjectName, proxy.AgentName, proxy.EnvironmentUUID)
	require.NoError(t, err)
	assert.Equal(t, byName.ID, byUUID.ID)

	cleared := *proxy
	cleared.UpstreamAuthHeader, cleared.UpstreamAuthSecret = "", ""
	require.NoError(t, repo.Upsert(ctx, &cleared))
	got, err := repo.GetByEnvUUID(ctx, proxy.OUID, proxy.ProjectName, proxy.AgentName, proxy.EnvironmentUUID)
	require.NoError(t, err)
	assert.Equal(t, byName.ID, got.ID, "the same row is updated")
	assert.Empty(t, got.UpstreamAuthHeader)
	assert.Empty(t, got.UpstreamAuthSecret, "clearing auth drops the secret")
}

// A re-created environment of the same name must not inherit the old proxy or its secret.
func TestA2AProxyUpsertReplacesARowFromADeletedEnvironment(t *testing.T) {
	repo := NewA2AProxyRepository(db.GetDB())
	ctx := context.Background()
	old := newTestProxy("recreated-" + uuid.New().String()[:8])
	cleanupProxy(t, repo, old)
	require.NoError(t, repo.Upsert(ctx, old))

	recreated := newTestProxy(old.AgentName)
	recreated.OUID, recreated.UpstreamAuthHeader, recreated.UpstreamAuthSecret = old.OUID, "", ""
	require.NoError(t, repo.Upsert(ctx, recreated))

	_, err := repo.GetByEnvUUID(ctx, old.OUID, old.ProjectName, old.AgentName, old.EnvironmentUUID)
	assert.ErrorIs(t, err, ErrA2AProxyNotFound)
	got, err := repo.Get(ctx, old.OUID, old.ProjectName, old.AgentName, "Development")
	require.NoError(t, err)
	assert.Equal(t, recreated.EnvironmentUUID, got.EnvironmentUUID)
	assert.Empty(t, got.UpstreamAuthSecret)
}

func TestA2AProxyLookupsAreOrgScopedAndReportAMissingRow(t *testing.T) {
	repo := NewA2AProxyRepository(db.GetDB())
	ctx := context.Background()
	proxy := newTestProxy("scoped-" + uuid.New().String()[:8])
	cleanupProxy(t, repo, proxy)
	require.NoError(t, repo.Upsert(ctx, proxy))

	_, err := repo.Get(ctx, "other-org", proxy.ProjectName, proxy.AgentName, "Development")
	assert.ErrorIs(t, err, ErrA2AProxyNotFound)
	_, err = repo.GetByEnvUUID(ctx, proxy.OUID, proxy.ProjectName, proxy.AgentName, uuid.New())
	assert.ErrorIs(t, err, ErrA2AProxyNotFound)
}

func TestA2AProxyDeleteReportsAMissingRow(t *testing.T) {
	repo := NewA2AProxyRepository(db.GetDB())
	ctx := context.Background()
	proxy := newTestProxy("delete-" + uuid.New().String()[:8])
	cleanupProxy(t, repo, proxy)
	require.NoError(t, repo.Upsert(ctx, proxy))

	require.NoError(t, repo.Delete(ctx, proxy.OUID, proxy.ProjectName, proxy.AgentName, proxy.EnvironmentUUID))
	assert.ErrorIs(t, repo.Delete(ctx, proxy.OUID, proxy.ProjectName, proxy.AgentName, proxy.EnvironmentUUID), ErrA2AProxyNotFound)
}

func TestA2AProxyDeletesForAgentAndEnvironment(t *testing.T) {
	repo := NewA2AProxyRepository(db.GetDB())
	ctx := context.Background()
	dev := newTestProxy("bulk-" + uuid.New().String()[:8])
	prod := newTestProxy(dev.AgentName)
	prod.OUID, prod.EnvironmentName = dev.OUID, "Production"
	cleanupProxy(t, repo, dev)
	require.NoError(t, repo.Upsert(ctx, dev))
	require.NoError(t, repo.Upsert(ctx, prod))

	require.NoError(t, repo.DeleteForEnvironment(ctx, dev.OUID, prod.EnvironmentUUID))
	_, err := repo.GetByEnvUUID(ctx, prod.OUID, prod.ProjectName, prod.AgentName, prod.EnvironmentUUID)
	assert.ErrorIs(t, err, ErrA2AProxyNotFound)
	_, err = repo.GetByEnvUUID(ctx, dev.OUID, dev.ProjectName, dev.AgentName, dev.EnvironmentUUID)
	require.NoError(t, err, "another environment's row stays")

	require.NoError(t, repo.DeleteForAgent(ctx, dev.OUID, dev.ProjectName, dev.AgentName))
	_, err = repo.GetByEnvUUID(ctx, dev.OUID, dev.ProjectName, dev.AgentName, dev.EnvironmentUUID)
	assert.ErrorIs(t, err, ErrA2AProxyNotFound)
}

func TestA2AProxyWithTxRollsBack(t *testing.T) {
	repo := NewA2AProxyRepository(db.GetDB())
	ctx := context.Background()
	proxy := newTestProxy("tx-" + uuid.New().String()[:8])
	cleanupProxy(t, repo, proxy)

	err := db.GetDB().Transaction(func(tx *gorm.DB) error {
		require.NoError(t, repo.WithTx(tx).Upsert(ctx, proxy))
		return errors.New("roll back")
	})
	require.Error(t, err)

	_, err = repo.GetByEnvUUID(ctx, proxy.OUID, proxy.ProjectName, proxy.AgentName, proxy.EnvironmentUUID)
	assert.ErrorIs(t, err, ErrA2AProxyNotFound)
}

// The lock holds a second transaction on the same environment until the first ends, and only that one.
func TestA2AProxyLockAgentEnvSerialisesOneEnvironment(t *testing.T) {
	repo := NewA2AProxyRepository(db.GetDB())
	ctx := context.Background()
	ouID, envUUID := "ou-"+uuid.New().String()[:8], uuid.New()

	holder := db.GetDB().Begin()
	require.NoError(t, holder.Error)
	defer holder.Rollback()
	require.NoError(t, repo.WithTx(holder).LockAgentEnv(ctx, ouID, "checkout", "weather", envUUID))

	tryLock := func(agentName string, envUUID uuid.UUID) bool {
		var acquired bool
		err := db.GetDB().Transaction(func(tx *gorm.DB) error {
			key := a2aProxyLockKey(ouID, "checkout", agentName, envUUID)
			return tx.Raw("SELECT pg_try_advisory_xact_lock(?, hashtext(?))", a2aProxyLockNamespace, key).Scan(&acquired).Error
		})
		require.NoError(t, err)
		return acquired
	}
	assert.False(t, tryLock("weather", envUUID), "the same environment waits")
	assert.True(t, tryLock("weather", uuid.New()), "another environment does not")
	assert.True(t, tryLock("weather-2", envUUID), "nor another agent")

	require.NoError(t, holder.Rollback().Error)
	assert.True(t, tryLock("weather", envUUID), "the lock ends with the transaction")
}

func TestA2AProxyListsTheEnvironmentUUIDsUnderAName(t *testing.T) {
	repo := NewA2AProxyRepository(db.GetDB())
	ctx := context.Background()
	old := newTestProxy("names-" + uuid.New().String()[:8])
	cleanupProxy(t, repo, old)
	require.NoError(t, repo.Upsert(ctx, old))

	got, err := repo.ListEnvironmentUUIDs(ctx, old.OUID, old.EnvironmentName)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{old.EnvironmentUUID}, got)

	none, err := repo.ListEnvironmentUUIDs(ctx, old.OUID, "Staging")
	require.NoError(t, err)
	assert.Empty(t, none)
}

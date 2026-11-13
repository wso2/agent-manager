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
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func newTestProxy(t *testing.T, agentName string) *models.A2AProxy {
	t.Helper()
	return newTestProxyIn(t, "Development", agentName)
}

// newTestProxyIn is newTestProxy in the environment named env.
func newTestProxyIn(t *testing.T, env, agentName string) *models.A2AProxy {
	t.Helper()
	ou := testOrg(t)
	ensureAgent(t, ou, "checkout", agentName)
	envID := envOf(t, ou, env)
	return &models.A2AProxy{
		OUID:               ou,
		ProjectName:        "checkout",
		AgentName:          agentName,
		EnvironmentName:    env,
		EnvironmentUUID:    envID,
		ArtifactUUID:       newArtifact(t, ou, models.KindAgent, "checkout/"+agentName+"/"+envID.String()),
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
	proxy := newTestProxy(t, "upsert-"+uuid.New().String()[:8])
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

func TestA2AProxyLookupsAreOrgScopedAndReportAMissingRow(t *testing.T) {
	repo := NewA2AProxyRepository(db.GetDB())
	ctx := context.Background()
	proxy := newTestProxy(t, "scoped-"+uuid.New().String()[:8])
	cleanupProxy(t, repo, proxy)
	require.NoError(t, repo.Upsert(ctx, proxy))

	_, err := repo.Get(ctx, newOrg(t), proxy.ProjectName, proxy.AgentName, "Development")
	assert.ErrorIs(t, err, ErrA2AProxyNotFound)
	_, err = repo.GetByEnvUUID(ctx, proxy.OUID, proxy.ProjectName, proxy.AgentName, uuid.New())
	assert.ErrorIs(t, err, ErrA2AProxyNotFound)
}

func TestA2AProxyDeleteReportsAMissingRow(t *testing.T) {
	repo := NewA2AProxyRepository(db.GetDB())
	ctx := context.Background()
	proxy := newTestProxy(t, "delete-"+uuid.New().String()[:8])
	cleanupProxy(t, repo, proxy)
	require.NoError(t, repo.Upsert(ctx, proxy))

	require.NoError(t, repo.Delete(ctx, proxy.OUID, proxy.ProjectName, proxy.AgentName, proxy.EnvironmentUUID))
	assert.ErrorIs(t, repo.Delete(ctx, proxy.OUID, proxy.ProjectName, proxy.AgentName, proxy.EnvironmentUUID), ErrA2AProxyNotFound)
}

func TestA2AProxyDeletesForAgentAndEnvironment(t *testing.T) {
	repo := NewA2AProxyRepository(db.GetDB())
	ctx := context.Background()
	dev := newTestProxy(t, "bulk-"+uuid.New().String()[:8])
	prod := newTestProxyIn(t, "Production", dev.AgentName)
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
	proxy := newTestProxy(t, "tx-"+uuid.New().String()[:8])
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
	gormDB := db.GetDB()
	repo := NewA2AProxyRepository(gormDB)
	ctx := context.Background()
	ouID := dbtest.Org(t, gormDB)
	dbtest.Agent(t, gormDB, ouID, dbtest.Project(t, gormDB, ouID, "checkout"), "weather")
	envUUID := dbtest.Environment(t, gormDB, ouID)

	holder := gormDB.Begin()
	require.NoError(t, holder.Error)
	defer holder.Rollback()
	require.NoError(t, repo.WithTx(holder).LockAgentEnv(ctx, ouID, "checkout", "weather", envUUID))

	var order []string
	var mu sync.Mutex
	note := func(s string) { mu.Lock(); order = append(order, s); mu.Unlock() }
	done := make(chan error, 1)
	go func() {
		done <- gormDB.Transaction(func(tx *gorm.DB) error {
			if err := repo.WithTx(tx).LockAgentEnv(ctx, ouID, "checkout", "weather", envUUID); err != nil {
				return err
			}
			note("second")
			return nil
		})
	}()
	time.Sleep(200 * time.Millisecond)
	note("first released")
	require.NoError(t, holder.Commit().Error)
	require.NoError(t, <-done)
	assert.Equal(t, []string{"first released", "second"}, order, "the second lock waits for the first")
}

func TestA2AProxyListsTheEnvironmentUUIDsUnderAName(t *testing.T) {
	repo := NewA2AProxyRepository(db.GetDB())
	ctx := context.Background()
	old := newTestProxy(t, "names-"+uuid.New().String()[:8])
	cleanupProxy(t, repo, old)
	require.NoError(t, repo.Upsert(ctx, old))

	got, err := repo.ListEnvironmentUUIDs(ctx, old.OUID, old.EnvironmentName)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{old.EnvironmentUUID}, got)

	none, err := repo.ListEnvironmentUUIDs(ctx, old.OUID, "Staging")
	require.NoError(t, err)
	assert.Empty(t, none)
}

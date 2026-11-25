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

package gwrepo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

func covKeyIDs(keys []gwmodels.StoredAPIKey) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(keys))
	for _, k := range keys {
		ids = append(ids, k.UUID)
	}
	return ids
}

// covKey upserts a key named name on artifact and returns it.
func covKey(t *testing.T, repo APIKeyRepository, ou string, artifact uuid.UUID, name string, purpose int, expires *time.Time) gwmodels.StoredAPIKey {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	key := gwmodels.StoredAPIKey{
		Name: name, ArtifactUUID: artifact, OUID: ou, APIKeyHash: "hash-" + name, MaskedAPIKey: "****" + name,
		Status: "active", Purpose: purpose, CreatedAt: now, UpdatedAt: now, ExpiresAt: expires,
	}
	require.NoError(t, repo.Upsert(&key))
	return key
}

// An API key upserts by artifact and name, is listed per artifact, kind, environment and purpose, and is deleted.
func TestCovAPIKeyRepoUpsertsListsAndDeletes(t *testing.T) {
	gdb := db.GetDB()
	repo := NewAPIKeyRepo(gdb)
	ou := newOrg(t)
	env1, env2 := uuid.New(), uuid.New()
	provider := newArtifact(t, ou, gwmodels.KindLLMProvider, "p-"+uuid.NewString()[:8])
	agent1 := newArtifact(t, ou, gwmodels.KindAgent, "proj/a/"+env1.String())
	rest2 := newArtifact(t, ou, gwmodels.KindRestAPI, "proj/r/"+env2.String())
	other := newOrg(t)
	otherOrgProvider := newArtifact(t, other, gwmodels.KindLLMProvider, "p-"+uuid.NewString()[:8])

	userKey := covKey(t, repo, ou, provider, "user", 0, nil)
	assert.Equal(t, gwmodels.APIKeyPurposeUserManaged, userKey.Purpose, "a key without a purpose is a user's")
	testKey := covKey(t, repo, ou, provider, "test", gwmodels.APIKeyPurposeTest, nil)
	agentKey := covKey(t, repo, ou, agent1, "a", gwmodels.APIKeyPurposeUserManaged, nil)
	restKey := covKey(t, repo, ou, rest2, "r", gwmodels.APIKeyPurposeUserManaged, nil)
	covKey(t, repo, other, otherOrgProvider, "elsewhere", 0, nil)

	// The same name on the same artifact updates the key in place.
	later := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	rotated := gwmodels.StoredAPIKey{
		Name: "user", ArtifactUUID: provider, OUID: ou, APIKeyHash: "rotated", MaskedAPIKey: "****rot",
		Status: "revoked", UpdatedAt: later, ExpiresAt: &later,
	}
	require.NoError(t, repo.Upsert(&rotated))
	got, err := repo.GetByArtifactAndName(provider.String(), "user")
	require.NoError(t, err)
	assert.Equal(t, userKey.UUID, got.UUID)
	assert.Equal(t, "rotated", got.APIKeyHash)
	assert.Equal(t, "****rot", got.MaskedAPIKey)
	assert.Equal(t, "revoked", got.Status)
	require.NotNil(t, got.ExpiresAt)
	assert.WithinDuration(t, later, *got.ExpiresAt, time.Second)
	_, err = repo.GetByArtifactAndName(provider.String(), "missing")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	byArtifact, err := repo.ListByArtifact(context.Background(), provider.String())
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{userKey.UUID, testKey.UUID}, covKeyIDs(byArtifact))
	byApp, err := repo.ListByApplicationUUID(provider.String())
	require.NoError(t, err)
	assert.Len(t, byApp, 2)

	byKind, err := repo.ListByArtifactKind(ou, gwmodels.KindLLMProvider)
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{userKey.UUID, testKey.UUID}, covKeyIDs(byKind), "test keys sync too; other orgs' don't")

	inEnvs, err := repo.ListByArtifactKindAndEnvs(ou, gwmodels.AgentArtifactKinds, []string{env1.String()})
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{agentKey.UUID}, covKeyIDs(inEnvs))
	inEnvs, err = repo.ListByArtifactKindAndEnvs(ou, gwmodels.AgentArtifactKinds, []string{env1.String(), env2.String()})
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{agentKey.UUID, restKey.UUID}, covKeyIDs(inEnvs))
	none, err := repo.ListByArtifactKindAndEnvs(ou, gwmodels.AgentArtifactKinds, nil)
	require.NoError(t, err)
	assert.Empty(t, none)

	permanent, err := repo.ListPermanentByArtifactKinds(ou, []string{gwmodels.KindLLMProvider, gwmodels.KindAgent})
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{userKey.UUID, agentKey.UUID}, covKeyIDs(permanent), "test keys are not permanent")

	require.NoError(t, repo.Delete(provider.String(), "test"))
	byArtifact, err = repo.ListByArtifact(context.Background(), provider.String())
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{userKey.UUID}, covKeyIDs(byArtifact))
}

// The keys a gateway would still accept are the active, unexpired ones, oldest first.
func TestCovAPIKeyRepoListsActiveKeysInATransaction(t *testing.T) {
	gdb := db.GetDB()
	repo := NewAPIKeyRepo(gdb)
	ou := newOrg(t)
	artifact := newArtifact(t, ou, gwmodels.KindAgent, "proj/a/"+uuid.NewString())
	past, future := time.Now().UTC().Add(-time.Hour), time.Now().UTC().Add(time.Hour)
	first := covKey(t, repo, ou, artifact, "first", 0, nil)
	second := covKey(t, repo, ou, artifact, "second", 0, &future)
	covKey(t, repo, ou, artifact, "expired", 0, &past)
	revoked := covKey(t, repo, ou, artifact, "revoked", 0, nil)
	require.NoError(t, gdb.Model(&gwmodels.StoredAPIKey{}).Where("id = ?", revoked.UUID).Update("status", "revoked").Error)
	// Make the order explicit: second was created after first.
	require.NoError(t, gdb.Model(&gwmodels.StoredAPIKey{}).Where("id = ?", second.UUID).
		Update("created_at", first.CreatedAt.Add(time.Minute)).Error)

	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		keys, err := repo.ListActiveByArtifactTx(context.Background(), tx, artifact, time.Now().UTC())
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{first.UUID, second.UUID}, covKeyIDs(keys))
		return nil
	}))
}

// An artifact is found by handle and id within its organization, counted, listed by prefix, updated and deleted.
func TestCovArtifactRepoReadsUpdatesAndDeletes(t *testing.T) {
	gdb := db.GetDB()
	repo := NewArtifactRepo(gdb, gwtest.NewSecrets())
	ou, other := newOrg(t), newOrg(t)
	prefix := "team_" + uuid.NewString()[:6] + "/"
	a1 := newArtifact(t, ou, gwmodels.KindAgent, prefix+"one")
	a2 := newArtifact(t, ou, gwmodels.KindRestAPI, prefix+"two")
	newArtifact(t, ou, gwmodels.KindAgent, "teamX"+prefix[5:]+"three") // "_" must not match any character
	newArtifact(t, other, gwmodels.KindAgent, prefix+"elsewhere")

	exists, err := repo.Exists(gwmodels.KindAgent, prefix+"one", ou)
	require.NoError(t, err)
	assert.True(t, exists)
	exists, err = repo.Exists(gwmodels.KindRestAPI, prefix+"one", ou)
	require.NoError(t, err)
	assert.False(t, exists, "the kind must match")

	byHandle, err := repo.GetByHandle(prefix+"two", ou)
	require.NoError(t, err)
	assert.Equal(t, a2, byHandle.UUID)
	_, err = repo.GetByHandle(prefix+"two", other)
	require.ErrorIs(t, err, gwerr.ErrArtifactNotFound)
	byID, err := repo.GetByUUID(a1.String(), ou)
	require.NoError(t, err)
	assert.Equal(t, prefix+"one", byID.Handle)
	_, err = repo.GetByUUID(a1.String(), other)
	require.ErrorIs(t, err, gwerr.ErrArtifactNotFound)

	listed, err := repo.ListByHandlePrefix(context.Background(), []string{gwmodels.KindAgent, gwmodels.KindRestAPI}, prefix, ou)
	require.NoError(t, err)
	ids := make([]uuid.UUID, 0, len(listed))
	for _, a := range listed {
		ids = append(ids, a.UUID)
	}
	assert.ElementsMatch(t, []uuid.UUID{a1, a2}, ids)
	n, err := repo.CountByKindAndOrg(gwmodels.KindAgent, ou)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	require.NoError(t, repo.WithTx(gdb).Update(gdb, &gwmodels.Artifact{UUID: a1, OUID: ou, Name: "renamed", Version: "v2"}))
	byID, err = repo.GetByUUID(a1.String(), ou)
	require.NoError(t, err)
	assert.Equal(t, "renamed", byID.Name)
	assert.Equal(t, "v2", byID.Version)
	assert.Equal(t, gwmodels.KindAgent, byID.Kind, "an empty field is left as it was")
	require.ErrorIs(t, repo.Update(gdb, &gwmodels.Artifact{UUID: a1, OUID: other, Name: "x"}), gorm.ErrRecordNotFound)

	require.NoError(t, repo.UpdateCatalogStatus(gdb, a1.String(), ou, true))
	byID, err = repo.GetByUUID(a1.String(), ou)
	require.NoError(t, err)
	assert.True(t, byID.InCatalog)
	require.ErrorIs(t, repo.UpdateCatalogStatus(gdb, a1.String(), other, true), gorm.ErrRecordNotFound)

	require.NoError(t, repo.Delete(gdb, a1.String()))
	_, err = repo.GetByUUID(a1.String(), ou)
	require.ErrorIs(t, err, gwerr.ErrArtifactNotFound)
	require.ErrorIs(t, repo.Delete(gdb, a1.String()), gorm.ErrRecordNotFound)

	existing, err := repo.ListExisting(context.Background(), ou, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, existing)
}

// An AI application is created once per agent and environment, found by agent and environment, and deleted.
func TestCovAIApplicationRepoLifecycle(t *testing.T) {
	gdb := db.GetDB()
	repo := NewAIApplicationRepository(gdb)
	ou := newOrg(t)
	agentID := ensureAgent(t, ou, "shop", "cart")
	dev, prod := envOf(t, ou, "dev"), envOf(t, ou, "prod")
	app := func(env uuid.UUID, handle string) *gwmodels.AIApplication {
		now := time.Now().UTC().Truncate(time.Microsecond)
		return &gwmodels.AIApplication{
			UUID: uuid.Must(uuid.NewV7()), Handle: handle, Name: handle, AgentUUID: &agentID, EnvironmentUUID: env,
			OUID: ou, CreatedAt: now, UpdatedAt: now,
		}
	}
	devApp := app(dev, "cart-dev")
	inserted, err := repo.Create(context.Background(), nil, devApp)
	require.NoError(t, err)
	assert.True(t, inserted)
	inserted, err = repo.Create(context.Background(), nil, app(dev, "cart-dev-again"))
	require.NoError(t, err)
	assert.False(t, inserted, "one application per agent and environment")
	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		inserted, err := repo.Create(context.Background(), tx, app(prod, "cart-prod"))
		require.NoError(t, err)
		assert.True(t, inserted)
		return nil
	}))

	got, err := repo.GetByAgentEnv(context.Background(), ou, "shop", "cart", "dev")
	require.NoError(t, err)
	assert.Equal(t, devApp.UUID, got.UUID)
	_, err = repo.GetByAgentEnv(context.Background(), ou, "shop", "cart", "staging")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	byAgent, err := repo.ListByAgent(context.Background(), ou, "shop", "cart")
	require.NoError(t, err)
	assert.Len(t, byAgent, 2)
	byOrg, err := repo.ListByOrg(context.Background(), ou)
	require.NoError(t, err)
	assert.Len(t, byOrg, 2)

	require.NoError(t, repo.DeleteByAgentEnv(context.Background(), nil, ou, "shop", "cart", "dev"))
	byAgent, err = repo.ListByAgent(context.Background(), ou, "shop", "cart")
	require.NoError(t, err)
	require.Len(t, byAgent, 1)
	assert.Equal(t, prod, byAgent[0].EnvironmentUUID)
	require.NoError(t, repo.DeleteByAgent(context.Background(), ou, "shop", "cart"))
	byOrg, err = repo.ListByOrg(context.Background(), ou)
	require.NoError(t, err)
	assert.Empty(t, byOrg)
}

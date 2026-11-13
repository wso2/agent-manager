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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// newTestExposure is an exposure of a fresh per-environment agent artifact, removed with it at cleanup.
func newTestExposure(t *testing.T, ouID, agentName string, envUUID uuid.UUID) *models.PlatformAgentExposure {
	t.Helper()
	gdb := db.GetDB()
	ensureAgent(t, ouID, "checkout", agentName)
	envWithID(t, ouID, envUUID, "Development")
	artifactUUID := uuid.New()
	require.NoError(t, NewArtifactRepo(gdb).Create(gdb, &models.Artifact{
		UUID: artifactUUID, Handle: "checkout/" + agentName + "/" + envUUID.String(), Name: "agent-" + artifactUUID.String()[:8],
		Version: "v1.0", Kind: models.KindAgent, OUID: ouID,
	}))
	return &models.PlatformAgentExposure{
		OUID:            ouID,
		ProjectName:     "checkout",
		AgentName:       agentName,
		EnvironmentName: "Development",
		EnvironmentUUID: envUUID,
		ArtifactUUID:    artifactUUID,
	}
}

func testOU(t *testing.T) string { t.Helper(); return newOrg(t) }

func exposureRowsFor(t *testing.T, ouID, agentName string) []models.PlatformAgentExposure {
	t.Helper()
	var rows []models.PlatformAgentExposure
	gdb := db.GetDB()
	require.NoError(t, gdb.Where("org_id = ? AND agent_id IN (?)", ouID, models.AgentIDQuery(gdb, ouID, "checkout", agentName)).
		Find(&rows).Error)
	return rows
}

// A redeploy upserts the agent-environment pair rather than adding a second row.
func TestPlatformAgentExposureUpsertKeepsOneRowPerPair(t *testing.T) {
	repo := NewPlatformAgentExposureRepository(db.GetDB())
	ctx := context.Background()
	exposure := newTestExposure(t, testOU(t), "trip-planner", uuid.New())
	require.NoError(t, repo.Upsert(ctx, exposure))

	again := *exposure
	again.ID = uuid.Nil
	again.Protocol = models.AgentExposureProtocolREST
	require.NoError(t, repo.Upsert(ctx, &again))

	rows := exposureRowsFor(t, exposure.OUID, exposure.AgentName)
	require.Len(t, rows, 1)
	assert.Equal(t, models.AgentExposureProtocolREST, rows[0].Protocol, "an upsert rewrites the pair's row")
}

func TestPlatformAgentExposureGetIsByEnvironmentUUID(t *testing.T) {
	repo := NewPlatformAgentExposureRepository(db.GetDB())
	ctx := context.Background()
	exposure := newTestExposure(t, testOU(t), "get", uuid.New())
	require.NoError(t, repo.Upsert(ctx, exposure))

	got, err := repo.Get(ctx, exposure.OUID, exposure.ProjectName, exposure.AgentName, exposure.EnvironmentUUID)
	require.NoError(t, err)
	assert.Equal(t, exposure.ID, got.ID)

	_, err = repo.Get(ctx, exposure.OUID, exposure.ProjectName, exposure.AgentName, uuid.New())
	assert.ErrorIs(t, err, ErrPlatformAgentExposureNotFound)
}

func TestPlatformAgentExposureGetByArtifact(t *testing.T) {
	repo := NewPlatformAgentExposureRepository(db.GetDB())
	ctx := context.Background()
	exposure := newTestExposure(t, testOU(t), "by-artifact", uuid.New())
	require.NoError(t, repo.Upsert(ctx, exposure))

	got, err := repo.GetByArtifact(ctx, exposure.OUID, exposure.ArtifactUUID)
	require.NoError(t, err)
	assert.Equal(t, exposure.ID, got.ID)

	_, err = repo.GetByArtifact(ctx, testOU(t), exposure.ArtifactUUID)
	assert.ErrorIs(t, err, ErrPlatformAgentExposureNotFound, "scoped to the org")
	_, err = repo.GetByArtifact(ctx, exposure.OUID, uuid.New())
	assert.ErrorIs(t, err, ErrPlatformAgentExposureNotFound)
}

// The exposure has no life of its own: deleting the agent's artifact removes it.
func TestPlatformAgentExposureGoesWithItsArtifact(t *testing.T) {
	repo := NewPlatformAgentExposureRepository(db.GetDB())
	ctx := context.Background()
	exposure := newTestExposure(t, testOU(t), "cascade", uuid.New())
	require.NoError(t, repo.Upsert(ctx, exposure))

	gdb := db.GetDB()
	require.NoError(t, NewArtifactRepo(gdb).Delete(gdb, exposure.ArtifactUUID.String()))

	_, err := repo.GetByArtifact(ctx, exposure.OUID, exposure.ArtifactUUID)
	assert.ErrorIs(t, err, ErrPlatformAgentExposureNotFound)
}

func TestPlatformAgentExposureListsAndDeletesByEnvironment(t *testing.T) {
	repo := NewPlatformAgentExposureRepository(db.GetDB())
	ctx := context.Background()
	ouID, prodUUID := testOU(t), uuid.New()
	envWithID(t, ouID, prodUUID, "Production")
	dev := newTestExposure(t, ouID, "env", uuid.New())
	prod := newTestExposure(t, ouID, "env", prodUUID)
	prod.EnvironmentName = "Production"
	neighbour := newTestExposure(t, ouID, "env-other", prodUUID)
	neighbour.EnvironmentName = "Production"
	for _, exposure := range []*models.PlatformAgentExposure{dev, prod, neighbour} {
		require.NoError(t, repo.Upsert(ctx, exposure))
	}

	inProd, err := repo.ListForEnvironment(ctx, ouID, prodUUID)
	require.NoError(t, err)
	var ids []uuid.UUID
	for _, row := range inProd {
		ids = append(ids, row.ID)
	}
	assert.ElementsMatch(t, []uuid.UUID{prod.ID, neighbour.ID}, ids)

	envUUIDs, err := repo.ListEnvironmentUUIDs(ctx, ouID, "Production")
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{prodUUID}, envUUIDs)

	require.NoError(t, repo.DeleteForAgentEnv(ctx, ouID, dev.ProjectName, dev.AgentName, dev.EnvironmentUUID))
	_, err = repo.Get(ctx, ouID, dev.ProjectName, dev.AgentName, dev.EnvironmentUUID)
	assert.ErrorIs(t, err, ErrPlatformAgentExposureNotFound)
	_, err = repo.Get(ctx, ouID, prod.ProjectName, prod.AgentName, prodUUID)
	require.NoError(t, err, "only that environment's row is gone")

	require.NoError(t, repo.DeleteForEnvironment(ctx, ouID, prodUUID))
	inProd, err = repo.ListForEnvironment(ctx, ouID, prodUUID)
	require.NoError(t, err)
	assert.Empty(t, inProd)
}

func TestPlatformAgentExposureDeleteForAgent(t *testing.T) {
	repo := NewPlatformAgentExposureRepository(db.GetDB())
	ctx := context.Background()
	ouID := testOU(t)
	for _, exposure := range []*models.PlatformAgentExposure{
		newTestExposure(t, ouID, "deleted", uuid.New()),
		newTestExposure(t, ouID, "deleted", uuid.New()),
		newTestExposure(t, ouID, "kept", uuid.New()),
	} {
		exposure.EnvironmentName = exposure.EnvironmentUUID.String()
		require.NoError(t, repo.Upsert(ctx, exposure))
	}

	require.NoError(t, repo.DeleteForAgent(ctx, ouID, "checkout", "deleted"))

	assert.Empty(t, exposureRowsFor(t, ouID, "deleted"))
	assert.Len(t, exposureRowsFor(t, ouID, "kept"), 1)
}

// Writes through a transaction-bound repository roll back with the transaction.
func TestPlatformAgentExposureWithTxRollsBack(t *testing.T) {
	repo := NewPlatformAgentExposureRepository(db.GetDB())
	ctx := context.Background()
	exposure := newTestExposure(t, testOU(t), "tx", uuid.New())

	err := db.GetDB().Transaction(func(tx *gorm.DB) error {
		require.NoError(t, repo.WithTx(tx).Upsert(ctx, exposure))
		return errors.New("roll back")
	})
	require.Error(t, err)

	_, err = repo.Get(ctx, exposure.OUID, exposure.ProjectName, exposure.AgentName, exposure.EnvironmentUUID)
	assert.ErrorIs(t, err, ErrPlatformAgentExposureNotFound)
}

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

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

var errRollback = errors.New("roll back the test transaction")

// lostInsertRace commits an artifact for a fresh handle and returns a repo whose first lookup
// misses it, as when a concurrent first enable inserts between the lookup and the insert.
func lostInsertRace(t *testing.T) (ouID, envID string, winner *models.Artifact, racing repositories.ArtifactRepository) {
	t.Helper()
	artifacts := repositories.NewArtifactRepo(db.GetDB())
	ouID, envID = dbtest.Org(t, db.GetDB()), uuid.New().String()
	winner = &models.Artifact{
		UUID: uuid.New(), Handle: agentEnvAPIArtifactHandle("checkout", "weather", envID),
		Name: "weather-winner-" + envID[:8], Version: "v1.0", Kind: models.KindAgent, OUID: ouID,
	}
	require.NoError(t, artifacts.Create(db.GetDB(), winner))
	t.Cleanup(func() { _ = artifacts.Delete(db.GetDB(), winner.UUID.String()) })

	lookups := 0
	reads := artifacts
	mock := &repomocks.ArtifactRepositoryMock{
		GetByHandleFunc: func(handle, ouID string) (*models.Artifact, error) {
			lookups++
			if lookups == 1 {
				return nil, utils.ErrArtifactNotFound
			}
			return reads.GetByHandle(handle, ouID)
		},
		CreateFunc: artifacts.Create,
	}
	mock.WithTxFunc = func(tx *gorm.DB) repositories.ArtifactRepository {
		reads = artifacts.WithTx(tx)
		return mock
	}
	return ouID, envID, winner, mock
}

// Inside a transaction the artifact is read through it, so one the transaction created is found, not inserted again.
func TestEnsureAgentEnvAPIArtifactReadsThroughTheCallersTransaction(t *testing.T) {
	artifacts := repositories.NewArtifactRepo(db.GetDB())
	ouID, envID := dbtest.Org(t, db.GetDB()), uuid.New().String()

	err := db.GetDB().Transaction(func(tx *gorm.DB) error {
		created, err := ensureAgentEnvAPIArtifact(tx, artifacts, ouID, "checkout", "weather", envID, models.KindAgent)
		require.NoError(t, err)
		again, err := ensureAgentEnvAPIArtifact(tx, artifacts, ouID, "checkout", "weather", envID, models.KindAgent)
		require.NoError(t, err)
		assert.Equal(t, created.UUID, again.UUID)
		return errRollback
	})
	require.ErrorIs(t, err, errRollback)
}

func TestEnsureAgentEnvAPIArtifactSurvivesLosingTheInsertRaceInsideATransaction(t *testing.T) {
	ouID, envID, winner, racing := lostInsertRace(t)

	err := db.GetDB().Transaction(func(tx *gorm.DB) error {
		got, err := ensureAgentEnvAPIArtifact(tx, racing, ouID, "checkout", "weather", envID, models.KindAgent)
		require.NoError(t, err)
		assert.Equal(t, winner.UUID, got.UUID)
		require.NoError(t, tx.Exec("SELECT 1").Error, "the caller's transaction is still usable")
		return errRollback
	})
	require.ErrorIs(t, err, errRollback)
}

func TestEnsureAgentEnvAPIArtifactReturnsTheWinnerOutsideATransaction(t *testing.T) {
	ouID, envID, winner, racing := lostInsertRace(t)

	got, err := ensureAgentEnvAPIArtifact(db.GetDB(), racing, ouID, "checkout", "weather", envID, models.KindAgent)

	require.NoError(t, err)
	assert.Equal(t, winner.UUID, got.UUID)
}

func TestEnsureAgentEnvAPIArtifactCreatesKind(t *testing.T) {
	artifacts := repositories.NewArtifactRepo(db.GetDB())
	ouID, envID := dbtest.Org(t, db.GetDB()), uuid.New().String()

	err := db.GetDB().Transaction(func(tx *gorm.DB) error {
		created, err := ensureAgentEnvAPIArtifact(tx, artifacts, ouID, "checkout", "weather", envID, models.KindRestAPI)
		require.NoError(t, err)
		assert.Equal(t, models.KindRestAPI, created.Kind)

		_, err = ensureAgentEnvAPIArtifact(tx, artifacts, ouID, "checkout", "weather", envID, models.KindAgent)
		assert.Error(t, err, "an existing artifact of another kind is not switched")
		return errRollback
	})
	require.ErrorIs(t, err, errRollback)
}

func TestEnsureAgentEnvAPIArtifactRefusesTheWinnerOfAnotherKindInTheInsertRace(t *testing.T) {
	ouID, envID, _, racing := lostInsertRace(t)

	_, err := ensureAgentEnvAPIArtifact(db.GetDB(), racing, ouID, "checkout", "weather", envID, models.KindRestAPI)

	assert.Error(t, err)
}

// An agent's prefix must not match a longer agent name, or a name its LIKE wildcards would.
func TestListByHandlePrefixFindsOnlyThatAgentsArtifacts(t *testing.T) {
	artifacts := repositories.NewArtifactRepo(db.GetDB())
	ouID := dbtest.Org(t, db.GetDB())
	create := func(agentName, kind string) *models.Artifact {
		envID := uuid.New().String()
		a := &models.Artifact{
			UUID: uuid.New(), Handle: agentEnvAPIArtifactHandle("checkout", agentName, envID),
			Name: agentName + "-" + envID[:8], Version: "v1.0", Kind: kind, OUID: ouID,
		}
		require.NoError(t, artifacts.Create(db.GetDB(), a))
		t.Cleanup(func() { _ = artifacts.Delete(db.GetDB(), a.UUID.String()) })
		return a
	}
	dev, qa := create("we_ther", models.KindAgent), create("we_ther", models.KindRestAPI)
	create("we_ther", models.KindWebSubAPI)
	create("we_ther-2", models.KindAgent)
	create("weather", models.KindAgent)

	got, err := artifacts.ListByHandlePrefix(context.Background(), models.AgentArtifactKinds, agentAPIArtifactHandlePrefix("checkout", "we_ther"), ouID)

	require.NoError(t, err)
	gotUUIDs := make([]uuid.UUID, 0, len(got))
	for _, a := range got {
		gotUUIDs = append(gotUUIDs, a.UUID)
	}
	assert.ElementsMatch(t, []uuid.UUID{dev.UUID, qa.UUID}, gotUUIDs)
}

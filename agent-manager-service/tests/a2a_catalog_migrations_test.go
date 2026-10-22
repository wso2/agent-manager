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

package tests

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	dbmigrations "github.com/wso2/agent-manager/agent-manager-service/db_migrations"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

const seededCardAttemptCount = 3

// catalogMigrationTx opens a transaction rolled back at test end, so seeded rows never
// persist for another test to observe.
func catalogMigrationTx(t *testing.T) *gorm.DB {
	t.Helper()
	tx := db.DB(context.Background()).Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { require.NoError(t, tx.Rollback().Error) })
	return tx
}

// seedCard inserts a card row with a non-zero attempt count, so a reset is observable.
// A nil nextAttemptAt leaves the row parked.
func seedCard(t *testing.T, tx *gorm.DB, status models.A2AAgentCardStatus, card []byte, nextAttemptAt *time.Time) uuid.UUID {
	t.Helper()
	row := &models.A2AAgentCard{
		OUID:            "catalog-migration-" + t.Name(),
		ProjectName:     "p1",
		AgentName:       "agent-" + uuid.NewString(),
		EnvironmentName: "development",
		EnvironmentUUID: uuid.New(),
		Source:          models.A2AAgentCardSourcePlatform,
		Card:            card,
		Status:          status,
		AttemptCount:    seededCardAttemptCount,
		NextAttemptAt:   nextAttemptAt,
	}
	require.NoError(t, tx.Create(row).Error)
	return row.ID
}

func cardRow(t *testing.T, tx *gorm.DB, id uuid.UUID) models.A2AAgentCard {
	t.Helper()
	var row models.A2AAgentCard
	require.NoError(t, tx.First(&row, "id = ?", id).Error)
	return row
}

func TestMigration049RequeuesOnlyFetchedCards(t *testing.T) {
	tx := catalogMigrationTx(t)
	now := time.Now()
	fetched := seedCard(t, tx, models.A2AAgentCardStatusFetched, []byte(`{"name":"x"}`), nil)
	failed := seedCard(t, tx, models.A2AAgentCardStatusFailed, []byte(`{"name":"x"}`), nil)
	pending := seedCard(t, tx, models.A2AAgentCardStatusPending, nil, &now)
	parked := seedCard(t, tx, models.A2AAgentCardStatusPending, nil, nil)
	untouched := []uuid.UUID{failed, pending, parked}
	before := make(map[uuid.UUID]models.A2AAgentCard, len(untouched))
	for _, id := range untouched {
		before[id] = cardRow(t, tx, id)
	}

	require.NoError(t, dbmigrations.RequeueFetchedA2ACards(tx))

	row := cardRow(t, tx, fetched)
	assert.Equal(t, models.A2AAgentCardStatusPending, row.Status)
	assert.Equal(t, 0, row.AttemptCount)
	require.NotNil(t, row.NextAttemptAt)
	assert.False(t, row.NextAttemptAt.After(time.Now()))
	assert.JSONEq(t, `{"name":"x"}`, string(row.Card))

	for _, id := range untouched {
		want, got := before[id], cardRow(t, tx, id)
		assert.Equal(t, want.Status, got.Status)
		assert.Equal(t, want.AttemptCount, got.AttemptCount)
		assert.Equal(t, want.NextAttemptAt, got.NextAttemptAt)
	}
}

func TestMigration049And050AreIdempotent(t *testing.T) {
	tx := catalogMigrationTx(t)
	require.NoError(t, dbmigrations.AddA2ACardProjectionColumns(tx))
	require.NoError(t, dbmigrations.AddA2ACardProjectionColumns(tx))
	require.NoError(t, dbmigrations.AddA2APublicationTransportsColumn(tx))
	require.NoError(t, dbmigrations.AddA2APublicationTransportsColumn(tx))
}

func TestMigration050LeavesExistingTransportsNull(t *testing.T) {
	tx := catalogMigrationTx(t)
	seeded := &models.A2APublication{
		OUID:            "catalog-migration-" + t.Name(),
		ProjectName:     "p1",
		AgentName:       "agent-" + uuid.NewString(),
		EnvironmentName: "development",
		EnvironmentUUID: uuid.New(),
		ArtifactUUID:    uuid.New(),
		Status:          models.A2APublicationStatusPublished,
	}
	require.NoError(t, tx.Create(seeded).Error)

	var got models.A2APublication
	require.NoError(t, tx.First(&got, "id = ?", seeded.ID).Error)
	assert.Nil(t, got.PublishedTransports)
}

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
	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// Each gateway reports the shared totals: a row keeps the higher of each, months are kept
// apart, and months older than the kept ones are deleted.
func TestCostUsageRepository_RecordSinceAndDeleteBefore(t *testing.T) {
	gdb := db.GetDB()
	ctx := context.Background()
	ou, other := dbtest.Org(t, gdb), dbtest.Org(t, gdb)
	repo := NewCostUsageRepository(gdb)
	project, model := "api-platform", "gpt-4o"
	key := uuid.New()
	november := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	row := func(org string, month time.Time, cost string, missing int64) gwmodels.CostUsage {
		return gwmodels.CostUsage{
			OUID: org, Month: month, CostKey: key, Project: &project, LLMProvider: "openai", Model: &model,
			Cost: cost, MissingPriceRequests: missing,
		}
	}

	require.NoError(t, repo.Record(ctx, []gwmodels.CostUsage{row(ou, november, "80.14", 3)}))
	require.NoError(t, repo.Record(ctx, []gwmodels.CostUsage{row(ou, november, "75", 4)}), "a lagging gateway's lower cost")
	require.NoError(t, repo.Record(ctx, []gwmodels.CostUsage{row(ou, november.AddDate(0, -1, 0), "10", 0)}))
	require.NoError(t, repo.Record(ctx, []gwmodels.CostUsage{row(ou, november.AddDate(0, -4, 0), "999", 0)}))
	require.NoError(t, repo.Record(ctx, []gwmodels.CostUsage{row(other, november, "1", 0)}))

	rows, err := repo.Since(ctx, ou, november.AddDate(0, -2, 0))
	require.NoError(t, err)
	require.Len(t, rows, 2, "July is older than the kept months; another organization's row is never read")
	byMonth := map[time.Month]gwmodels.CostUsage{}
	for _, r := range rows {
		byMonth[r.Month.UTC().Month()] = r
	}
	assert.Equal(t, "80.14", byMonth[time.November].Cost, "the higher cost is kept")
	assert.Equal(t, int64(4), byMonth[time.November].MissingPriceRequests, "the higher count is kept")
	assert.Equal(t, "api-platform", *byMonth[time.November].Project)
	assert.Equal(t, "10", byMonth[time.October].Cost)

	deleted, err := repo.DeleteBefore(ctx, ou, november.AddDate(0, -2, 0))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, deleted, int64(1))
	all, err := repo.Since(ctx, ou, time.Time{})
	require.NoError(t, err)
	assert.Len(t, all, 2, "July went")
}

func TestCostUsageRepository_ABatchSpansOrganizationsAndMonths(t *testing.T) {
	gdb := db.GetDB()
	ctx := context.Background()
	repo := NewCostUsageRepository(gdb)
	acme, globex := dbtest.Org(t, gdb), dbtest.Org(t, gdb)
	october := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	november := october.AddDate(0, 1, 0)
	gpt, claude := uuid.New(), uuid.New()
	row := func(ou string, month time.Time, key uuid.UUID, model, cost string) gwmodels.CostUsage {
		return gwmodels.CostUsage{
			OUID: ou, Month: month, CostKey: key, LLMProvider: "openai", Model: &model, Cost: cost,
		}
	}
	// Out of key order, across organizations and months, in one batch.
	require.NoError(t, repo.Record(ctx, []gwmodels.CostUsage{
		row(globex, november, claude, "claude-sonnet", "4.00"),
		row(acme, november, gpt, "gpt-4o", "2.00"),
		row(acme, october, claude, "claude-sonnet", "3.00"),
		row(acme, november, claude, "claude-sonnet", "1.00"),
	}))
	require.NoError(t, repo.Record(ctx, nil), "an empty batch writes nothing")

	got, err := repo.Since(ctx, acme, october)
	require.NoError(t, err)
	assert.Len(t, got, 3, "the organization's own rows only")

	deleted, err := repo.DeleteBefore(ctx, acme, november)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)
	got, err = repo.Since(ctx, acme, october)
	require.NoError(t, err)
	assert.Len(t, got, 2)
	other, err := repo.Since(ctx, globex, october)
	require.NoError(t, err)
	assert.Len(t, other, 1, "another organization keeps its rows")
}

func TestCostUsageRepository_ARowWrittenFirstByAnotherWriterIsMergedInto(t *testing.T) {
	gdb := db.GetDB()
	ctx := context.Background()
	repo := NewCostUsageRepository(gdb)
	ou := dbtest.Org(t, gdb)
	november := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	key := uuid.New()
	row := func(cost string, missing, unavailable int64) gwmodels.CostUsage {
		return gwmodels.CostUsage{
			OUID: ou, Month: november, CostKey: key, LLMProvider: "openai", Cost: cost,
			MissingPriceRequests: missing, UsageUnavailableRequests: unavailable,
		}
	}

	// Two reports of one key in one batch: the second is merged into the first.
	require.NoError(t, repo.Record(ctx, []gwmodels.CostUsage{row("1.00", 0, 1), row("2.00", 3, 0)}))
	got, err := repo.Since(ctx, ou, november)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "2.00", got[0].Cost)
	assert.EqualValues(t, 3, got[0].MissingPriceRequests)
	assert.EqualValues(t, 1, got[0].UsageUnavailableRequests, "each count keeps its highest")

	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		ok, err := updateCostIf(tx, row("1.50", 3, 1), map[string]any{"cost": "9.00"})
		require.NoError(t, err)
		assert.False(t, ok, "the row no longer holds what was read")
		require.NoError(t, mergeCostRetrying(tx, row("9.00", 3, 1)), "read again and raised")
		require.NoError(t, mergeCostRetrying(tx, row("4.00", 0, 0)), "a lower report changes nothing")
		missing := row("1.00", 0, 0)
		missing.CostKey = uuid.New()
		assert.Error(t, mergeCostRetrying(tx, missing), "a row that is not there cannot be merged into")
		return nil
	}))
	got, err = repo.Since(ctx, ou, november)
	require.NoError(t, err)
	assert.Equal(t, "9.00", got[0].Cost)
}

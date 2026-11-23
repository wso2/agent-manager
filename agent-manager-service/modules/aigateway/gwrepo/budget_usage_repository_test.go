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
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/notifyv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

func TestBudgetUsageRepository_RecordMergesAndRaisesWarningsOnce(t *testing.T) {
	gdb := db.GetDB()
	ctx := context.Background()
	ou := dbtest.Org(t, gdb)
	budgetID := createBudget(t, gdb, ou, "per-agent")
	repo := NewBudgetUsageRepository(gdb)

	november := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	allowance := uuid.New()
	report := func(period time.Time, spent string, unpriced int64) gwmodels.BudgetUsage {
		return gwmodels.BudgetUsage{
			BudgetID: budgetID, AllowanceID: allowance, OUID: ou, Allowance: map[string]string{"agent": "support-agent"},
			PeriodStart: period, Spent: spent, UnpricedRequests: unpriced,
		}
	}
	var seen []gwmodels.BudgetUsage
	warn := func(stored gwmodels.BudgetUsage) []gwmodels.BudgetWarning {
		seen = append(seen, stored)
		return []gwmodels.BudgetWarning{{
			BudgetID: budgetID, AllowanceID: allowance, PeriodStart: stored.PeriodStart, Amount: "80.00",
			OUID: ou, Threshold: 80, Notice: notifyv1.Notification{Subject: "per-agent"},
		}}
	}
	var published []gwmodels.BudgetWarning
	publish := func(_ context.Context, tx *gorm.DB, w gwmodels.BudgetWarning) error {
		require.NotNil(t, tx, "published in the usage write's transaction")
		published = append(published, w)
		return nil
	}

	require.NoError(t, repo.Record(ctx, []gwmodels.BudgetUsage{report(november, "80.14", 3)}, warn, publish))
	require.Len(t, published, 1, "the first crossing is raised")
	assert.Equal(t, "per-agent", published[0].Notice.Subject)

	require.NoError(t, repo.Record(ctx, []gwmodels.BudgetUsage{report(november, "90", 2)}, warn, publish))
	assert.Len(t, published, 1, "the same warning is raised once")
	rows, err := repo.ForBudget(ctx, budgetID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "90", rows[0].Spent, "the higher total is kept")
	assert.Equal(t, int64(3), rows[0].UnpricedRequests, "and the higher unpriced count")
	assert.Equal(t, map[string]string{"agent": "support-agent"}, rows[0].Allowance)

	seen = nil
	require.NoError(t, repo.Record(ctx, []gwmodels.BudgetUsage{report(november, "85", 3)}, warn, publish))
	assert.Empty(t, seen, "a lower total changes nothing, so nothing is checked")

	// A publish that fails rolls the usage write back, so the crossing is tried again.
	failing := func(context.Context, *gorm.DB, gwmodels.BudgetWarning) error { return errors.New("outbox down") }
	december := november.AddDate(0, 1, 0)
	require.Error(t, repo.Record(ctx, []gwmodels.BudgetUsage{report(december, "99", 0)}, warn, failing))
	rows, _ = repo.ForBudget(ctx, budgetID)
	assert.Equal(t, "90", rows[0].Spent, "nothing of the failed write is kept")
	require.NoError(t, repo.Record(ctx, []gwmodels.BudgetUsage{report(december, "99", 0)}, warn, publish))
	assert.Len(t, published, 2, "a new period warns again")

	require.NoError(t, repo.Record(ctx, []gwmodels.BudgetUsage{report(november.AddDate(0, -1, 0), "500", 9)}, nil, nil))
	rows, _ = repo.ForBudget(ctx, budgetID)
	require.Len(t, rows, 1, "reads show each allowance once")
	assert.Equal(t, "99", rows[0].Spent, "reads show the latest period")
	assert.True(t, rows[0].PeriodStart.Equal(december))
	var periods int64
	require.NoError(t, gdb.Model(&gwmodels.BudgetUsage{}).Where("budget_id = ?", budgetID).Count(&periods).Error)
	assert.Equal(t, int64(3), periods, "October, November and December each keep their own row")

	// An edit's check records only warnings not raised before.
	require.NoError(t, repo.RecordWarnings(ctx, []gwmodels.BudgetWarning{
		{BudgetID: budgetID, AllowanceID: allowance, PeriodStart: december, Amount: "80.00", OUID: ou, Threshold: 80},
		{BudgetID: budgetID, AllowanceID: allowance, PeriodStart: december, Amount: "56.00", OUID: ou, Threshold: 80},
	}, publish))
	assert.Len(t, published, 3)
	assert.Equal(t, "56.00", published[2].Amount)

	// Deleting the budget deletes its usage and warnings.
	require.NoError(t, newBudgetRepo(gdb).Delete(ctx, ou, "per-agent"))
	var left int64
	require.NoError(t, gdb.Model(&gwmodels.BudgetWarning{}).Where("budget_id = ?", budgetID).Count(&left).Error)
	assert.Zero(t, left)
}

// The budgets list reads the usage of a page of budgets in one query, within the organization.
func TestBudgetUsageRepository_ForBudgets(t *testing.T) {
	gdb := db.GetDB()
	ctx := context.Background()
	ou, other := dbtest.Org(t, gdb), dbtest.Org(t, gdb)
	a := createBudget(t, gdb, ou, "team-a")
	b := createBudget(t, gdb, ou, "team-b")
	elsewhere := createBudget(t, gdb, other, "team-a")
	repo := NewBudgetUsageRepository(gdb)
	november := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for _, row := range []struct {
		budget uuid.UUID
		org    string
	}{{a, ou}, {a, ou}, {b, ou}, {elsewhere, other}} {
		err := repo.Record(ctx, []gwmodels.BudgetUsage{{
			BudgetID: row.budget, AllowanceID: uuid.New(), OUID: row.org, Allowance: map[string]string{},
			PeriodStart: november, Spent: "1", UpdatedAt: november,
		}}, nil, nil)
		require.NoError(t, err)
	}
	rows, err := repo.ForBudgets(ctx, ou, []uuid.UUID{a, b, elsewhere})
	require.NoError(t, err)
	assert.Len(t, rows, 3, "another organization's usage is never read")
	none, err := repo.ForBudgets(ctx, ou, nil)
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestBudgetUsageRepository_ABatchKeepsTheHighestUnderConcurrentWriters(t *testing.T) {
	gdb := db.GetDB()
	ctx := context.Background()
	ou := dbtest.Org(t, gdb)
	budgetID := createBudget(t, gdb, ou, "batch-writers")
	repo := NewBudgetUsageRepository(gdb)
	november := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	// 300 allowances: more than one multi-row insert, some already stored.
	allowances := make([]uuid.UUID, 300)
	for i := range allowances {
		allowances[i] = uuid.New()
	}
	batch := func(spent string) []gwmodels.BudgetUsage {
		rows := make([]gwmodels.BudgetUsage, len(allowances))
		for i, id := range allowances {
			rows[i] = gwmodels.BudgetUsage{
				BudgetID: budgetID, AllowanceID: id, OUID: ou, Allowance: map[string]string{},
				PeriodStart: november, Spent: spent,
			}
		}
		return rows
	}
	err := repo.Record(ctx, batch("1.00")[:50], nil, nil)
	require.NoError(t, err)

	// Several replicas write the same allowances at once with different totals.
	var wg sync.WaitGroup
	for _, spent := range []string{"2.00", "9.50", "4.25", "9.49", "7.00"} {
		wg.Add(1)
		go func(spent string) {
			defer wg.Done()
			err := repo.Record(ctx, batch(spent), nil, nil)
			assert.NoError(t, err)
		}(spent)
	}
	wg.Wait()

	rows, err := repo.ForBudget(ctx, budgetID)
	require.NoError(t, err)
	require.Len(t, rows, len(allowances), "one row per allowance and period, never a duplicate")
	for _, row := range rows {
		assert.Equal(t, "9.50", row.Spent, "every row keeps the highest total any writer reported")
	}

	// A lower total later changes nothing.
	err = repo.Record(ctx, batch("3.00"), nil, nil)
	require.NoError(t, err)
	rows, err = repo.ForBudget(ctx, budgetID)
	require.NoError(t, err)
	assert.Equal(t, "9.50", rows[0].Spent)
}

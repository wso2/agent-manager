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
	"fmt"
	"math/big"
	"strconv"
	"strings"
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

// crossing warns for each threshold amount a write took the total from below to or past, as
// the warning service does.
func crossing(thresholds ...string) WarnFunc {
	return func(before string, after gwmodels.BudgetUsage) []notifyv1.Notification {
		was, _ := new(big.Rat).SetString(before)
		now, _ := new(big.Rat).SetString(after.Spent)
		var out []notifyv1.Notification
		for _, threshold := range thresholds {
			at, _ := new(big.Rat).SetString(threshold)
			if was.Cmp(at) < 0 && now.Cmp(at) >= 0 {
				out = append(out, notifyv1.Notification{
					ID:      fmt.Sprintf("%s:%s:%s", after.BudgetUsageID, after.PeriodStart.Format(time.DateOnly), threshold),
					Subject: "per-agent",
				})
			}
		}
		return out
	}
}

// published collects the warnings a usage write raised.
type published struct {
	mu      sync.Mutex
	ids     []string
	noTx    bool
	failing error
}

func (p *published) publish(_ context.Context, tx *gorm.DB, n notifyv1.Notification) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if tx == nil {
		p.noTx = true
	}
	if p.failing != nil {
		return p.failing
	}
	p.ids = append(p.ids, n.ID)
	return nil
}

func (p *published) thresholds() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.ids))
	for i, id := range p.ids {
		out[i] = id[strings.LastIndex(id, ":")+1:]
	}
	return out
}

func usageReporter(budgetID uuid.UUID, ou string, usage uuid.UUID) func(time.Time, string, int64) gwmodels.BudgetUsage {
	return func(period time.Time, spent string, unpriced int64) gwmodels.BudgetUsage {
		return gwmodels.BudgetUsage{
			BudgetID: budgetID, BudgetUsageID: usage, OUID: ou, Usage: map[string]string{"agent": "support-agent"},
			PeriodStart: period, Spent: spent, UnpricedRequests: unpriced,
		}
	}
}

func TestBudgetUsageRepository_RecordMergesAndRaisesACrossingOnce(t *testing.T) {
	gdb := db.GetDB()
	ctx := context.Background()
	ou := dbtest.Org(t, gdb)
	budgetID := createBudget(t, gdb, ou, "per-agent")
	repo := NewBudgetUsageRepository(gdb)

	november := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	usage := uuid.New()
	report := usageReporter(budgetID, ou, usage)
	var seen []string // "before>after" of each write warn was asked about
	warn80 := crossing("80.00")
	warn := func(before string, after gwmodels.BudgetUsage) []notifyv1.Notification {
		seen = append(seen, before+">"+after.Spent)
		return warn80(before, after)
	}
	got := &published{}

	require.NoError(t, repo.Record(ctx, []gwmodels.BudgetUsage{report(november, "80.14", 3)}, warn, got.publish))
	assert.Equal(t, []string{"80.00"}, got.thresholds(), "the write that crossed 80 raises it")
	assert.False(t, got.noTx, "published in the usage write's transaction")
	assert.Equal(t, []string{"0>80.14"}, seen, "a new row held 0 before")

	require.NoError(t, repo.Record(ctx, []gwmodels.BudgetUsage{report(november, "90", 2)}, warn, got.publish))
	assert.Equal(t, []string{"80.00"}, got.thresholds(), "a later write above the threshold raises nothing")
	assert.Equal(t, []string{"0>80.14", "80.14>90"}, seen, "the total it replaced is what warn is given")
	rows, err := repo.ForBudget(ctx, budgetID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "90", rows[0].Spent, "the higher total is kept")
	assert.Equal(t, int64(3), rows[0].UnpricedRequests, "and the higher unpriced count")
	assert.Equal(t, map[string]string{"agent": "support-agent"}, rows[0].Usage)

	seen = nil
	require.NoError(t, repo.Record(ctx, []gwmodels.BudgetUsage{report(november, "85", 3)}, warn, got.publish))
	assert.Empty(t, seen, "a lower total changes nothing, so nothing is checked")
	require.NoError(t, repo.Record(ctx, []gwmodels.BudgetUsage{report(november, "90", 5)}, warn, got.publish))
	assert.Equal(t, []string{"90>90"}, seen, "a write that only raises the unpriced count has no crossing to find")
	assert.Equal(t, []string{"80.00"}, got.thresholds())

	// A publish that fails rolls the usage write back, so the crossing is tried again.
	december := november.AddDate(0, 1, 0)
	got.failing = errors.New("outbox down")
	require.Error(t, repo.Record(ctx, []gwmodels.BudgetUsage{report(december, "99", 0)}, warn, got.publish))
	rows, _ = repo.ForBudget(ctx, budgetID)
	assert.Equal(t, "90", rows[0].Spent, "nothing of the failed write is kept")
	got.failing = nil
	require.NoError(t, repo.Record(ctx, []gwmodels.BudgetUsage{report(december, "99", 0)}, warn, got.publish))
	assert.Equal(t, []string{"80.00", "80.00"}, got.thresholds(), "a new period warns again")

	require.NoError(t, repo.Record(ctx, []gwmodels.BudgetUsage{report(november.AddDate(0, -1, 0), "500", 9)}, nil, nil))
	rows, _ = repo.ForBudget(ctx, budgetID)
	require.Len(t, rows, 1, "reads show each usage once")
	assert.Equal(t, "99", rows[0].Spent, "reads show the latest period")
	assert.True(t, rows[0].PeriodStart.Equal(december))
	var periods int64
	require.NoError(t, gdb.Model(&gwmodels.BudgetUsage{}).Where("budget_id = ?", budgetID).Count(&periods).Error)
	assert.Equal(t, int64(3), periods, "October, November and December each keep their own row")

	// Deleting the budget deletes its usage.
	require.NoError(t, newBudgetRepo(gdb).Delete(ctx, ou, "per-agent"))
	var left int64
	require.NoError(t, gdb.Model(&gwmodels.BudgetUsage{}).Where("budget_id = ?", budgetID).Count(&left).Error)
	assert.Zero(t, left)
}

func TestBudgetUsageRepository_AThresholdIsCrossedByOneWrite(t *testing.T) {
	gdb := db.GetDB()
	ctx := context.Background()
	ou := dbtest.Org(t, gdb)
	november := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	t.Run("exactly reaching a threshold crosses it", func(t *testing.T) {
		budgetID := createBudget(t, gdb, ou, "exact")
		repo, report, got := NewBudgetUsageRepository(gdb), usageReporter(budgetID, ou, uuid.New()), &published{}
		require.NoError(t, repo.Record(ctx, []gwmodels.BudgetUsage{report(november, "79.99", 0)}, crossing("80.00"), got.publish))
		assert.Empty(t, got.thresholds())
		require.NoError(t, repo.Record(ctx, []gwmodels.BudgetUsage{report(november, "80.00", 0)}, crossing("80.00"), got.publish))
		assert.Equal(t, []string{"80.00"}, got.thresholds())
	})

	t.Run("one write over several thresholds raises each once", func(t *testing.T) {
		budgetID := createBudget(t, gdb, ou, "several")
		repo, report, got := NewBudgetUsageRepository(gdb), usageReporter(budgetID, ou, uuid.New()), &published{}
		warn := crossing("50.00", "80.00", "100.00")
		require.NoError(t, repo.Record(ctx, []gwmodels.BudgetUsage{report(november, "120", 0)}, warn, got.publish))
		assert.ElementsMatch(t, []string{"50.00", "80.00", "100.00"}, got.thresholds())
		require.NoError(t, repo.Record(ctx, []gwmodels.BudgetUsage{report(november, "130", 0)}, warn, got.publish))
		assert.Len(t, got.thresholds(), 3)
	})

	t.Run("two totals of one usage in a batch cross it once", func(t *testing.T) {
		budgetID := createBudget(t, gdb, ou, "one-batch")
		repo, report, got := NewBudgetUsageRepository(gdb), usageReporter(budgetID, ou, uuid.New()), &published{}
		require.NoError(t, repo.Record(ctx, []gwmodels.BudgetUsage{report(november, "70", 0), report(november, "85", 0)},
			crossing("80.00"), got.publish))
		assert.Equal(t, []string{"80.00"}, got.thresholds())
	})
}

// Replicas write the same usage at once, each with the totals its gateways report: every
// threshold is crossed by exactly one of the writes, never twice and never missed.
func TestBudgetUsageRepository_ConcurrentWritersCrossEachThresholdOnce(t *testing.T) {
	gdb := db.GetDB()
	ctx := context.Background()
	ou := dbtest.Org(t, gdb)
	budgetID := createBudget(t, gdb, ou, "replicas")
	repo := NewBudgetUsageRepository(gdb)
	november := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	usages := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	warn := crossing("25.00", "50.00", "75.00", "100.00")
	got := &published{}

	var wg sync.WaitGroup
	for replica := range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each replica reports every total from 0 to 120 in its own steps, lagging or ahead of the others.
			for spent := replica; spent <= 120; spent += replica + 1 {
				for _, id := range usages {
					row := usageReporter(budgetID, ou, id)(november, strconv.Itoa(spent), 0)
					assert.NoError(t, repo.Record(ctx, []gwmodels.BudgetUsage{row}, warn, got.publish))
				}
			}
		}()
	}
	wg.Wait()

	counts := map[string]int{}
	for _, id := range got.ids {
		counts[id]++
	}
	assert.Len(t, counts, len(usages)*4, "each usage crossed each of the four thresholds")
	for id, n := range counts {
		assert.Equal(t, 1, n, "%s was raised once", id)
	}
	rows, err := repo.ForBudget(ctx, budgetID)
	require.NoError(t, err)
	for _, row := range rows {
		assert.Equal(t, "120", row.Spent)
	}
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
			BudgetID: row.budget, BudgetUsageID: uuid.New(), OUID: row.org, Usage: map[string]string{},
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

	// 300 usages: more than one multi-row insert, some already stored.
	usages := make([]uuid.UUID, 300)
	for i := range usages {
		usages[i] = uuid.New()
	}
	batch := func(spent string) []gwmodels.BudgetUsage {
		rows := make([]gwmodels.BudgetUsage, len(usages))
		for i, id := range usages {
			rows[i] = gwmodels.BudgetUsage{
				BudgetID: budgetID, BudgetUsageID: id, OUID: ou, Usage: map[string]string{},
				PeriodStart: november, Spent: spent,
			}
		}
		return rows
	}
	err := repo.Record(ctx, batch("1.00")[:50], nil, nil)
	require.NoError(t, err)

	// Several replicas write the same usages at once with different totals.
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
	require.Len(t, rows, len(usages), "one row per usage and period, never a duplicate")
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

func TestBudgetUsageRepository_DeletePeriodsBeforeKeepsTheCurrentPeriod(t *testing.T) {
	gdb := db.GetDB()
	ctx := context.Background()
	ou := dbtest.Org(t, gdb)
	budgetID := createBudget(t, gdb, ou, "per-agent-daily")
	otherID := createBudget(t, gdb, ou, "org-daily")
	repo := NewBudgetUsageRepository(gdb)

	usage := uuid.New()
	nov22 := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)
	nov23 := nov22.AddDate(0, 0, 1)
	nov24 := nov23.AddDate(0, 0, 1)
	row := func(budget uuid.UUID, period time.Time) gwmodels.BudgetUsage {
		return gwmodels.BudgetUsage{
			BudgetID: budget, BudgetUsageID: usage, OUID: ou,
			Usage: map[string]string{"agent": "support-agent"}, PeriodStart: period, Spent: "9.00",
		}
	}
	require.NoError(t, repo.Record(ctx, []gwmodels.BudgetUsage{
		row(budgetID, nov22), row(budgetID, nov23), row(budgetID, nov24), row(otherID, nov22),
	}, nil, nil))

	deleted, err := repo.DeletePeriodsBefore(ctx, budgetID, nov24)
	require.NoError(t, err)
	assert.EqualValues(t, 2, deleted, "the two past periods go")

	rows, err := repo.ForBudget(ctx, budgetID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.True(t, rows[0].PeriodStart.Equal(nov24), "the current period is kept")

	other, err := repo.ForBudget(ctx, otherID)
	require.NoError(t, err)
	assert.Len(t, other, 1, "other budgets keep their periods")

	deleted, err = repo.DeletePeriodsBefore(ctx, budgetID, nov24)
	require.NoError(t, err)
	assert.Zero(t, deleted, "nothing left to delete")
}

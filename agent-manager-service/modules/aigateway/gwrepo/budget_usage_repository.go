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
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/notifyv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
)

// BudgetUsageRepository keeps each budget usage's spend in its current period and
// raises the warnings the spend crosses.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg gwrepomocks -out gwrepomocks/budget_usage_repository_mock.go . BudgetUsageRepository:BudgetUsageRepositoryMock
type BudgetUsageRepository interface {
	// Record merges a gateway's reports into their usages' rows, all in one
	// transaction: each period has its own row, and the same period keeps the higher
	// totals (each gateway reports the shared total; one may lag). warn gets each row that
	// changed, with the total it held before (0 for a new row), and returns the warnings
	// that change crosses; Record calls publish for each in the same transaction, so a
	// warning's event commits with the spend that crossed it, and a failed publish leaves
	// the spend unwritten, to be reported again. A total is replaced from one value to a
	// higher one only once, so a threshold is crossed by exactly one write.
	Record(ctx context.Context, rows []gwmodels.BudgetUsage, warn WarnFunc, publish WarningPublisher) error
	// ForBudget returns the budget's usages.
	ForBudget(ctx context.Context, budgetID uuid.UUID) ([]gwmodels.BudgetUsage, error)
	// ForBudgets returns the usages of the organization's budgets with those IDs.
	ForBudgets(ctx context.Context, ouID string, budgetIDs []uuid.UUID) ([]gwmodels.BudgetUsage, error)
	// DeletePeriodsBefore deletes the budget's usages of periods that started before
	// before, and returns how many went.
	DeletePeriodsBefore(ctx context.Context, budgetID uuid.UUID, before time.Time) (int64, error)
}

// WarnFunc returns the warnings a changed usage row calls for: before is the total it
// held until this write (a decimal string, "0" for a new row), after is the row as stored.
type WarnFunc func(before string, after gwmodels.BudgetUsage) []notifyv1.Notification

// WarningPublisher sends a warning inside tx.
type WarningPublisher func(ctx context.Context, tx *gorm.DB, n notifyv1.Notification) error

type budgetUsageRepository struct {
	db *gorm.DB
}

// NewBudgetUsageRepository creates a BudgetUsageRepository.
func NewBudgetUsageRepository(db *gorm.DB) BudgetUsageRepository {
	return &budgetUsageRepository{db: db}
}

func (r *budgetUsageRepository) DeletePeriodsBefore(ctx context.Context, budgetID uuid.UUID,
	before time.Time,
) (int64, error) {
	var deleted int64
	res := r.db.WithContext(ctx).Where("budget_id = ? AND period_start < ?", budgetID, before.UTC()).
		Delete(&gwmodels.BudgetUsage{})
	deleted = res.RowsAffected
	return deleted, res.Error
}

func (r *budgetUsageRepository) Record(ctx context.Context, rows []gwmodels.BudgetUsage,
	warn WarnFunc, publish WarningPublisher,
) error {
	if len(rows) == 0 {
		return nil
	}
	now := dbx.Now()
	batch := make([]gwmodels.BudgetUsage, len(rows))
	for i, u := range rows {
		u.PeriodStart = u.PeriodStart.UTC()
		u.UpdatedAt = now
		batch[i] = u
	}
	// Rows are locked in key order, so two batches never wait on each other's rows.
	sort.Slice(batch, func(i, j int) bool {
		a, b := batch[i], batch[j]
		if a.BudgetID != b.BudgetID {
			return a.BudgetID.String() < b.BudgetID.String()
		}
		if a.BudgetUsageID != b.BudgetUsageID {
			return a.BudgetUsageID.String() < b.BudgetUsageID.String()
		}
		return a.PeriodStart.Before(b.PeriodStart)
	})
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		changed, err := writeUsageBatch(tx, batch)
		if err != nil {
			return err
		}
		if warn == nil || publish == nil {
			return nil
		}
		for _, c := range changed {
			for _, n := range warn(c.before, c.after) {
				if err := publish(ctx, tx, n); err != nil {
					return fmt.Errorf("failed to publish a budget warning: %w", err)
				}
			}
		}
		return nil
	})
}

// usageChange is a row a write changed: the row as stored, and the total it held before.
type usageChange struct {
	before string
	after  gwmodels.BudgetUsage
}

// newRowSpent is the total a row that did not exist is taken to have held.
const newRowSpent = "0"

// usageBatchSize bounds one multi-row insert (8 columns; SQL Server allows 2,100 parameters).
const usageBatchSize = 200

type usageRowKey struct {
	budget, usage uuid.UUID
	period        int64
}

func usageRowKeyOf(u gwmodels.BudgetUsage) usageRowKey {
	return usageRowKey{u.BudgetID, u.BudgetUsageID, u.PeriodStart.UTC().UnixMicro()}
}

// writeUsageBatch writes a batch of reported totals with one read and, per row that is
// new or higher, one write: new rows in multi-row inserts, higher ones by a conditional
// update on the value just read, with no row lock. A row another writer changed in
// between is read again and merged; a row another writer inserted first is merged into.
// It returns every row as stored after a change, with the total it replaced, for the
// warnings.
func writeUsageBatch(tx *gorm.DB, batch []gwmodels.BudgetUsage) ([]usageChange, error) {
	stored, err := storedUsage(tx, batch)
	if err != nil {
		return nil, err
	}
	var inserts []gwmodels.BudgetUsage
	type update struct{ current, merged gwmodels.BudgetUsage }
	var updates []update
	for _, u := range batch {
		current, ok := stored[usageRowKeyOf(u)]
		if !ok {
			inserts = append(inserts, u)
			continue
		}
		if merged, changed := mergeUsage(current, u); changed {
			updates = append(updates, update{current, merged})
		}
	}
	var changed []usageChange
	for start := 0; start < len(inserts); start += usageBatchSize {
		chunk := inserts[start:min(start+usageBatchSize, len(inserts))]
		inserted, err := dbx.InsertIfAbsent(tx, &chunk)
		if err != nil {
			return nil, err
		}
		if inserted {
			for _, u := range chunk {
				changed = append(changed, usageChange{before: newRowSpent, after: u})
			}
			continue
		}
		// Another writer inserted one of them first: row by row, merging into those.
		for _, u := range chunk {
			row := u
			inserted, err := dbx.InsertIfAbsent(tx, &row)
			if err != nil {
				return nil, err
			}
			if inserted {
				changed = append(changed, usageChange{before: newRowSpent, after: row})
				continue
			}
			before, merged, ok, err := mergeUsageRetrying(tx, u)
			if err != nil {
				return nil, err
			}
			if ok {
				changed = append(changed, usageChange{before: before, after: merged})
			}
		}
	}
	for _, up := range updates {
		ok, err := updateUsageIf(tx, up.current, up.merged)
		if err != nil {
			return nil, err
		}
		if ok {
			changed = append(changed, usageChange{before: up.current.Spent, after: up.merged})
			continue
		}
		before, merged, ok, err := mergeUsageRetrying(tx, up.merged)
		if err != nil {
			return nil, err
		}
		if ok {
			changed = append(changed, usageChange{before: before, after: merged})
		}
	}
	return changed, nil
}

// storedUsage reads the stored rows of the batch's usages, and only those, in one query
// per chunk of usages: a budget split many ways is not read whole for one change.
func storedUsage(tx *gorm.DB, batch []gwmodels.BudgetUsage) (map[usageRowKey]gwmodels.BudgetUsage, error) {
	usageSet, budgetSet := map[uuid.UUID]bool{}, map[uuid.UUID]bool{}
	periodSet := map[int64]time.Time{}
	for _, u := range batch {
		usageSet[u.BudgetUsageID] = true
		budgetSet[u.BudgetID] = true
		periodSet[u.PeriodStart.UTC().UnixMicro()] = u.PeriodStart.UTC()
	}
	usages := make([]uuid.UUID, 0, len(usageSet))
	for id := range usageSet {
		usages = append(usages, id)
	}
	budgets := make([]uuid.UUID, 0, len(budgetSet))
	for id := range budgetSet {
		budgets = append(budgets, id)
	}
	periods := make([]time.Time, 0, len(periodSet))
	for _, t := range periodSet {
		periods = append(periods, t)
	}
	out := make(map[usageRowKey]gwmodels.BudgetUsage, len(batch))
	for _, chunk := range dbx.Chunks(usages) {
		var rows []gwmodels.BudgetUsage
		// budget_id leads the primary key, so the lookup uses it.
		if err := tx.Where("budget_id IN ? AND budget_usage_id IN ? AND period_start IN ?", budgets, chunk, periods).
			Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("failed to read budget usage: %w", err)
		}
		for _, row := range rows {
			out[usageRowKeyOf(row)] = row
		}
	}
	return out, nil
}

// updateUsageIf raises a row to merged only if it still holds current; false when another
// writer changed it in between.
func updateUsageIf(tx *gorm.DB, current, merged gwmodels.BudgetUsage) (bool, error) {
	res := tx.Model(&gwmodels.BudgetUsage{}).
		Where("budget_id = ? AND budget_usage_id = ? AND period_start = ? AND spent = ? AND unpriced_requests = ?",
			current.BudgetID, current.BudgetUsageID, current.PeriodStart, current.Spent, current.UnpricedRequests).
		Updates(map[string]any{
			"budget_usage":      dbx.JSON(merged.Usage),
			"spent":             merged.Spent,
			"unpriced_requests": merged.UnpricedRequests,
			"updated_at":        merged.UpdatedAt,
		})
	return res.RowsAffected == 1, res.Error
}

// mergeUsageRetrying reads a row another writer just changed and merges the report into
// it, a few times; false when the stored row is already as high. It returns the total the
// write replaced, and the row as stored.
func mergeUsageRetrying(tx *gorm.DB, report gwmodels.BudgetUsage) (string, gwmodels.BudgetUsage, bool, error) {
	for range 5 {
		var current gwmodels.BudgetUsage
		err := tx.Where("budget_id = ? AND budget_usage_id = ? AND period_start = ?",
			report.BudgetID, report.BudgetUsageID, report.PeriodStart).Take(&current).Error
		if err != nil {
			return "", report, false, err
		}
		merged, changed := mergeUsage(current, report)
		if !changed {
			return current.Spent, current, false, nil
		}
		ok, err := updateUsageIf(tx, current, merged)
		if err != nil || ok {
			return current.Spent, merged, ok, err
		}
	}
	return "", report, false, fmt.Errorf("budget usage %s/%s kept changing while it was written", report.BudgetID, report.BudgetUsageID)
}

func mergeUsage(current, report gwmodels.BudgetUsage) (gwmodels.BudgetUsage, bool) {
	switch {
	case report.PeriodStart.Before(current.PeriodStart):
		return current, false
	case report.PeriodStart.After(current.PeriodStart):
		return report, true
	}
	merged, changed := current, false
	if gwutil.DecimalGreater(report.Spent, current.Spent) {
		merged.Spent, changed = report.Spent, true
	}
	if report.UnpricedRequests > current.UnpricedRequests {
		merged.UnpricedRequests, changed = report.UnpricedRequests, true
	}
	if changed {
		merged.UpdatedAt = report.UpdatedAt
		if len(report.Usage) > 0 {
			merged.Usage = report.Usage
		}
	}
	return merged, changed
}

// ForBudget returns each of a budget's usages in its latest period.
func (r *budgetUsageRepository) ForBudget(ctx context.Context, budgetID uuid.UUID) ([]gwmodels.BudgetUsage, error) {
	var rows []gwmodels.BudgetUsage
	err := r.db.WithContext(ctx).Where("budget_id = ?", budgetID).Order("budget_usage_id").Find(&rows).Error
	return latestPeriods(rows), err
}

// latestPeriods keeps each usage's latest period, in the order the rows came.
func latestPeriods(rows []gwmodels.BudgetUsage) []gwmodels.BudgetUsage {
	type key struct{ budget, usage uuid.UUID }
	latest := map[key]int{}
	out := make([]gwmodels.BudgetUsage, 0, len(rows))
	for _, row := range rows {
		k := key{row.BudgetID, row.BudgetUsageID}
		if i, seen := latest[k]; seen {
			if row.PeriodStart.After(out[i].PeriodStart) {
				out[i] = row
			}
			continue
		}
		latest[k] = len(out)
		out = append(out, row)
	}
	return out
}

func (r *budgetUsageRepository) ForBudgets(ctx context.Context, ouID string, budgetIDs []uuid.UUID) ([]gwmodels.BudgetUsage, error) {
	rows := []gwmodels.BudgetUsage{}
	for _, chunk := range dbx.Chunks(budgetIDs) {
		var page []gwmodels.BudgetUsage
		if err := r.db.WithContext(ctx).Where("org_id = ? AND budget_id IN ?", ouID, chunk).Find(&page).Error; err != nil {
			return nil, fmt.Errorf("failed to read budget usage: %w", err)
		}
		rows = append(rows, page...)
	}
	return latestPeriods(rows), nil
}

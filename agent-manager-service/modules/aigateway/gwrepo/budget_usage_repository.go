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

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
)

// BudgetUsageRepository keeps each budget allowance's spend in its current period
// and records the warnings it crosses.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg gwrepomocks -out gwrepomocks/budget_usage_repository_mock.go . BudgetUsageRepository:BudgetUsageRepositoryMock
type BudgetUsageRepository interface {
	// Record merges a gateway's reports into their allowances' rows, all in one
	// transaction: each period has its own row, and the same period keeps the higher
	// totals (each gateway reports the shared total; one may lag). warn gets each row that
	// changed and returns the warnings it calls for; Record records them in the same
	// transaction and, for each not recorded before, calls publish there, so a warning's
	// event commits with the spend that crossed it.
	Record(ctx context.Context, rows []gwmodels.BudgetUsage,
		warn func(stored gwmodels.BudgetUsage) []gwmodels.BudgetWarning, publish WarningPublisher) error
	// ForBudget returns the budget's allowances.
	ForBudget(ctx context.Context, budgetID uuid.UUID) ([]gwmodels.BudgetUsage, error)
	// ForBudgets returns the allowances of the organization's budgets with those IDs.
	ForBudgets(ctx context.Context, ouID string, budgetIDs []uuid.UUID) ([]gwmodels.BudgetUsage, error)
	// RecordWarnings records warnings and publishes those not recorded before, in one
	// transaction.
	RecordWarnings(ctx context.Context, warnings []gwmodels.BudgetWarning, publish WarningPublisher) error
}

// WarningPublisher publishes a newly recorded warning's event inside tx.
type WarningPublisher func(ctx context.Context, tx *gorm.DB, w gwmodels.BudgetWarning) error

type budgetUsageRepository struct {
	db *gorm.DB
}

// NewBudgetUsageRepository creates a BudgetUsageRepository.
func NewBudgetUsageRepository(db *gorm.DB) BudgetUsageRepository {
	return &budgetUsageRepository{db: db}
}

func (r *budgetUsageRepository) Record(ctx context.Context, rows []gwmodels.BudgetUsage,
	warn func(stored gwmodels.BudgetUsage) []gwmodels.BudgetWarning, publish WarningPublisher,
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
		if a.AllowanceID != b.AllowanceID {
			return a.AllowanceID.String() < b.AllowanceID.String()
		}
		return a.PeriodStart.Before(b.PeriodStart)
	})
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		changed, err := writeUsageBatch(tx, batch)
		if err != nil {
			return err
		}
		if warn == nil {
			return nil
		}
		for _, stored := range changed {
			if err := recordWarnings(ctx, tx, warn(stored), publish); err != nil {
				return err
			}
		}
		return nil
	})
}

// usageBatchSize bounds one multi-row insert (8 columns; SQL Server allows 2,100 parameters).
const usageBatchSize = 200

type usageRowKey struct {
	budget, allowance uuid.UUID
	period            int64
}

func usageRowKeyOf(u gwmodels.BudgetUsage) usageRowKey {
	return usageRowKey{u.BudgetID, u.AllowanceID, u.PeriodStart.UTC().UnixMicro()}
}

// writeUsageBatch writes a batch of reported totals with one read and, per row that is
// new or higher, one write: new rows in multi-row inserts, higher ones by a conditional
// update on the value just read, with no row lock. A row another writer changed in
// between is read again and merged; a row another writer inserted first is merged into.
// It returns every row as stored after a change, for the warnings.
func writeUsageBatch(tx *gorm.DB, batch []gwmodels.BudgetUsage) ([]gwmodels.BudgetUsage, error) {
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
	var changed []gwmodels.BudgetUsage
	for start := 0; start < len(inserts); start += usageBatchSize {
		chunk := inserts[start:min(start+usageBatchSize, len(inserts))]
		inserted, err := dbx.InsertIfAbsent(tx, &chunk)
		if err != nil {
			return nil, err
		}
		if inserted {
			changed = append(changed, chunk...)
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
				changed = append(changed, row)
				continue
			}
			merged, ok, err := mergeUsageRetrying(tx, u)
			if err != nil {
				return nil, err
			}
			if ok {
				changed = append(changed, merged)
			}
		}
	}
	for _, up := range updates {
		ok, err := updateUsageIf(tx, up.current, up.merged)
		if err != nil {
			return nil, err
		}
		if ok {
			changed = append(changed, up.merged)
			continue
		}
		merged, ok, err := mergeUsageRetrying(tx, up.merged)
		if err != nil {
			return nil, err
		}
		if ok {
			changed = append(changed, merged)
		}
	}
	return changed, nil
}

// storedUsage reads the stored rows of the batch's allowances, and only those, in one query
// per chunk of allowances: a budget split many ways is not read whole for one change.
func storedUsage(tx *gorm.DB, batch []gwmodels.BudgetUsage) (map[usageRowKey]gwmodels.BudgetUsage, error) {
	allowanceSet, budgetSet := map[uuid.UUID]bool{}, map[uuid.UUID]bool{}
	periodSet := map[int64]time.Time{}
	for _, u := range batch {
		allowanceSet[u.AllowanceID] = true
		budgetSet[u.BudgetID] = true
		periodSet[u.PeriodStart.UTC().UnixMicro()] = u.PeriodStart.UTC()
	}
	allowances := make([]uuid.UUID, 0, len(allowanceSet))
	for id := range allowanceSet {
		allowances = append(allowances, id)
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
	for _, chunk := range dbx.Chunks(allowances) {
		var rows []gwmodels.BudgetUsage
		// budget_id leads the primary key, so the lookup uses it.
		if err := tx.Where("budget_id IN ? AND allowance_id IN ? AND period_start IN ?", budgets, chunk, periods).
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
		Where("budget_id = ? AND allowance_id = ? AND period_start = ? AND spent = ? AND unpriced_requests = ?",
			current.BudgetID, current.AllowanceID, current.PeriodStart, current.Spent, current.UnpricedRequests).
		Updates(map[string]any{
			"allowance":         dbx.JSON(merged.Allowance),
			"spent":             merged.Spent,
			"unpriced_requests": merged.UnpricedRequests,
			"updated_at":        merged.UpdatedAt,
		})
	return res.RowsAffected == 1, res.Error
}

// mergeUsageRetrying reads a row another writer just changed and merges the report into
// it, a few times; false when the stored row is already as high.
func mergeUsageRetrying(tx *gorm.DB, report gwmodels.BudgetUsage) (gwmodels.BudgetUsage, bool, error) {
	for range 5 {
		var current gwmodels.BudgetUsage
		err := tx.Where("budget_id = ? AND allowance_id = ? AND period_start = ?",
			report.BudgetID, report.AllowanceID, report.PeriodStart).Take(&current).Error
		if err != nil {
			return report, false, err
		}
		merged, changed := mergeUsage(current, report)
		if !changed {
			return current, false, nil
		}
		ok, err := updateUsageIf(tx, current, merged)
		if err != nil || ok {
			return merged, ok, err
		}
	}
	return report, false, fmt.Errorf("budget usage %s/%s kept changing while it was written", report.BudgetID, report.AllowanceID)
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
		if len(report.Allowance) > 0 {
			merged.Allowance = report.Allowance
		}
	}
	return merged, changed
}

// ForBudget returns each of a budget's allowances in its latest period.
func (r *budgetUsageRepository) ForBudget(ctx context.Context, budgetID uuid.UUID) ([]gwmodels.BudgetUsage, error) {
	var rows []gwmodels.BudgetUsage
	err := r.db.WithContext(ctx).Where("budget_id = ?", budgetID).Order("allowance_id").Find(&rows).Error
	return latestPeriods(rows), err
}

// latestPeriods keeps each allowance's latest period, in the order the rows came.
func latestPeriods(rows []gwmodels.BudgetUsage) []gwmodels.BudgetUsage {
	type key struct{ budget, allowance uuid.UUID }
	latest := map[key]int{}
	out := make([]gwmodels.BudgetUsage, 0, len(rows))
	for _, row := range rows {
		k := key{row.BudgetID, row.AllowanceID}
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

func (r *budgetUsageRepository) RecordWarnings(ctx context.Context, warnings []gwmodels.BudgetWarning, publish WarningPublisher) error {
	if len(warnings) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return recordWarnings(ctx, tx, warnings, publish)
	})
}

// recordWarnings records warnings in tx and publishes each one not recorded before: a
// threshold amount warns once per allowance and period.
func recordWarnings(ctx context.Context, tx *gorm.DB, warnings []gwmodels.BudgetWarning, publish WarningPublisher) error {
	now := dbx.Now()
	for i := range warnings {
		w := warnings[i]
		w.PeriodStart = w.PeriodStart.UTC()
		w.CreatedAt = now
		inserted, err := dbx.InsertIfAbsent(tx, &w)
		if err != nil {
			return fmt.Errorf("failed to record a budget warning: %w", err)
		}
		if !inserted || publish == nil {
			continue
		}
		if err := publish(ctx, tx, w); err != nil {
			return fmt.Errorf("failed to publish a budget warning: %w", err)
		}
	}
	return nil
}

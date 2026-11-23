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

// CostUsageRepository keeps what LLM traffic cost per month and combination of values
// (AI cost stats, Interfaces 1.3).
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg gwrepomocks -out gwrepomocks/cost_usage_repository_mock.go . CostUsageRepository:CostUsageRepositoryMock
type CostUsageRepository interface {
	// Record merges a gateway's reports into their rows, all in one transaction: each
	// total keeps the higher value (every gateway reports the shared total; one may lag).
	Record(ctx context.Context, rows []gwmodels.CostUsage) error
	// Since returns the organization's rows from the month starting at from.
	Since(ctx context.Context, ouID string, from time.Time) ([]gwmodels.CostUsage, error)
	// DeleteBefore deletes the organization's rows of months before before, and returns
	// how many went.
	DeleteBefore(ctx context.Context, ouID string, before time.Time) (int64, error)
}

type costUsageRepository struct {
	db *gorm.DB
}

// NewCostUsageRepository creates a CostUsageRepository.
func NewCostUsageRepository(db *gorm.DB) CostUsageRepository {
	return &costUsageRepository{db: db}
}

func (r *costUsageRepository) Record(ctx context.Context, rows []gwmodels.CostUsage) error {
	if len(rows) == 0 {
		return nil
	}
	now := dbx.Now()
	batch := make([]gwmodels.CostUsage, len(rows))
	for i, u := range rows {
		u.Month = u.Month.UTC()
		u.UpdatedAt = now
		batch[i] = u
	}
	// Rows are locked in key order, so two batches never wait on each other's rows.
	sort.Slice(batch, func(i, j int) bool {
		a, b := batch[i], batch[j]
		if a.OUID != b.OUID {
			return a.OUID < b.OUID
		}
		if !a.Month.Equal(b.Month) {
			return a.Month.Before(b.Month)
		}
		return a.CostKey.String() < b.CostKey.String()
	})
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return writeCostBatch(tx, batch)
	})
}

type costRowKey struct {
	ou    string
	month int64
	key   uuid.UUID
}

func costRowKeyOf(u gwmodels.CostUsage) costRowKey {
	return costRowKey{u.OUID, u.Month.UTC().UnixMicro(), u.CostKey}
}

// writeCostBatch writes a batch of reported totals as writeUsageBatch does: one read,
// multi-row inserts for new rows and a conditional update for each higher one.
func writeCostBatch(tx *gorm.DB, batch []gwmodels.CostUsage) error {
	stored := map[costRowKey]gwmodels.CostUsage{}
	byOU := map[string]map[int64]time.Time{}
	for _, u := range batch {
		if byOU[u.OUID] == nil {
			byOU[u.OUID] = map[int64]time.Time{}
		}
		byOU[u.OUID][u.Month.UTC().UnixMicro()] = u.Month.UTC()
	}
	for ou, monthSet := range byOU {
		months := make([]time.Time, 0, len(monthSet))
		for _, m := range monthSet {
			months = append(months, m)
		}
		var keys []uuid.UUID
		for _, u := range batch {
			if u.OUID == ou {
				keys = append(keys, u.CostKey)
			}
		}
		for _, chunk := range dbx.Chunks(keys) {
			var rows []gwmodels.CostUsage
			if err := tx.Where("org_id = ? AND month IN ? AND cost_key IN ?", ou, months, chunk).Find(&rows).Error; err != nil {
				return fmt.Errorf("failed to read cost usage: %w", err)
			}
			for _, row := range rows {
				stored[costRowKeyOf(row)] = row
			}
		}
	}
	var inserts []gwmodels.CostUsage
	for _, u := range batch {
		current, ok := stored[costRowKeyOf(u)]
		if !ok {
			inserts = append(inserts, u)
			continue
		}
		if updates := costIncrease(current, u); len(updates) > 0 {
			ok, err := updateCostIf(tx, current, updates)
			if err != nil {
				return err
			}
			if !ok {
				if err := mergeCostRetrying(tx, u); err != nil {
					return err
				}
			}
		}
	}
	for start := 0; start < len(inserts); start += usageBatchSize {
		chunk := inserts[start:min(start+usageBatchSize, len(inserts))]
		inserted, err := dbx.InsertIfAbsent(tx, &chunk)
		if err != nil || inserted {
			if err != nil {
				return err
			}
			continue
		}
		for _, u := range chunk {
			row := u
			inserted, err := dbx.InsertIfAbsent(tx, &row)
			if err != nil {
				return err
			}
			if !inserted {
				if err := mergeCostRetrying(tx, u); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// costIncrease returns the fields of report higher than current; none when no change.
func costIncrease(current, report gwmodels.CostUsage) map[string]any {
	updates := map[string]any{}
	if gwutil.DecimalGreater(report.Cost, current.Cost) {
		updates["cost"] = report.Cost
	}
	if report.MissingPriceRequests > current.MissingPriceRequests {
		updates["missing_price_requests"] = report.MissingPriceRequests
	}
	if report.UsageUnavailableRequests > current.UsageUnavailableRequests {
		updates["usage_unavailable_requests"] = report.UsageUnavailableRequests
	}
	if len(updates) > 0 {
		updates["updated_at"] = report.UpdatedAt
	}
	return updates
}

// updateCostIf applies updates only if the row still holds current.
func updateCostIf(tx *gorm.DB, current gwmodels.CostUsage, updates map[string]any) (bool, error) {
	res := tx.Model(&gwmodels.CostUsage{}).
		Where("org_id = ? AND month = ? AND cost_key = ? AND cost = ? AND missing_price_requests = ? AND usage_unavailable_requests = ?",
			current.OUID, current.Month, current.CostKey, current.Cost, current.MissingPriceRequests, current.UsageUnavailableRequests).
		Updates(updates)
	return res.RowsAffected == 1, res.Error
}

// mergeCostRetrying reads a row another writer just changed and merges the report in.
func mergeCostRetrying(tx *gorm.DB, report gwmodels.CostUsage) error {
	for range 5 {
		var current gwmodels.CostUsage
		err := tx.Where("org_id = ? AND month = ? AND cost_key = ?", report.OUID, report.Month, report.CostKey).
			Take(&current).Error
		if err != nil {
			return err
		}
		updates := costIncrease(current, report)
		if len(updates) == 0 {
			return nil
		}
		ok, err := updateCostIf(tx, current, updates)
		if err != nil || ok {
			return err
		}
	}
	return fmt.Errorf("cost usage %s kept changing while it was written", report.CostKey)
}

func (r *costUsageRepository) Since(ctx context.Context, ouID string, from time.Time) ([]gwmodels.CostUsage, error) {
	rows := []gwmodels.CostUsage{}
	if err := r.db.WithContext(ctx).Where("org_id = ? AND month >= ?", ouID, from.UTC()).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to read cost usage: %w", err)
	}
	return rows, nil
}

func (r *costUsageRepository) DeleteBefore(ctx context.Context, ouID string, before time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Where("org_id = ? AND month < ?", ouID, before.UTC()).Delete(&gwmodels.CostUsage{})
	if res.Error != nil {
		return 0, fmt.Errorf("failed to delete old cost usage: %w", res.Error)
	}
	return res.RowsAffected, nil
}

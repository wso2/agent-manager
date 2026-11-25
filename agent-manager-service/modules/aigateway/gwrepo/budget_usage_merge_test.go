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

func TestBudgetUsageRepository_ARowWrittenFirstByAnotherWriterIsMergedInto(t *testing.T) {
	gdb := db.GetDB()
	ctx := context.Background()
	ou := dbtest.Org(t, gdb)
	budgetID := createBudget(t, gdb, ou, "org-daily")
	repo := NewBudgetUsageRepository(gdb)
	day := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	allowance := uuid.New()
	report := func(spent string, unpriced int64) gwmodels.BudgetUsage {
		return gwmodels.BudgetUsage{
			BudgetID: budgetID, AllowanceID: allowance, OUID: ou, Allowance: map[string]string{},
			PeriodStart: day, Spent: spent, UnpricedRequests: unpriced,
		}
	}

	// Two reports of one allowance in one batch: the batch insert meets the first, so the
	// second is merged into it, as when another writer inserts the row first.
	require.NoError(t, repo.Record(ctx, []gwmodels.BudgetUsage{report("5.00", 1), report("7.00", 2)}, nil, nil))
	rows, err := repo.ForBudget(ctx, budgetID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "7.00", rows[0].Spent)
	assert.EqualValues(t, 2, rows[0].UnpricedRequests)

	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		// Another writer changed the row since it was read: the conditional update misses.
		ok, err := updateUsageIf(tx, report("6.00", 2), report("9.00", 2))
		require.NoError(t, err)
		assert.False(t, ok, "the row no longer holds what was read")

		merged, ok, err := mergeUsageRetrying(tx, report("9.00", 2))
		require.NoError(t, err)
		assert.True(t, ok, "read again and raised")
		assert.Equal(t, "9.00", merged.Spent)

		_, ok, err = mergeUsageRetrying(tx, report("8.00", 1))
		require.NoError(t, err)
		assert.False(t, ok, "a lower report changes nothing")

		missing := report("1.00", 0)
		missing.AllowanceID = uuid.New()
		_, _, err = mergeUsageRetrying(tx, missing)
		assert.Error(t, err, "a row that is not there cannot be merged into")
		return nil
	}))
}

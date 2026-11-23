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

package gwmodels

import (
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/notifyv1"
)

// BudgetUsage is one budget allowance's spend in one period, as gateways report it
// (Interfaces 1.2); each period keeps its own row. Spent is a decimal string in USD.
type BudgetUsage struct {
	BudgetID         uuid.UUID         `gorm:"column:budget_id;primaryKey"`
	AllowanceID      uuid.UUID         `gorm:"column:allowance_id;primaryKey"`
	OUID             string            `gorm:"column:org_id"`
	Allowance        map[string]string `gorm:"column:allowance;serializer:json"`
	PeriodStart      time.Time         `gorm:"column:period_start;primaryKey"`
	Spent            string            `gorm:"column:spent"`
	UnpricedRequests int64             `gorm:"column:unpriced_requests"`
	UpdatedAt        time.Time         `gorm:"column:updated_at"`
}

func (BudgetUsage) TableName() string { return "mod_ai_gateway_budget_usage" }

// BudgetWarning is a warning threshold an allowance crossed in a period. The row is
// written in the transaction that records the crossing, with the warning's event, so
// each threshold amount warns once per allowance and period; raising the limit gives new
// amounts, which warn again.
type BudgetWarning struct {
	BudgetID    uuid.UUID `gorm:"column:budget_id;primaryKey"`
	AllowanceID uuid.UUID `gorm:"column:allowance_id;primaryKey"`
	PeriodStart time.Time `gorm:"column:period_start;primaryKey"`
	// Amount is the threshold amount in USD, two decimals.
	Amount    string    `gorm:"column:amount;primaryKey"`
	OUID      string    `gorm:"column:org_id"`
	Threshold int       `gorm:"column:threshold"`
	CreatedAt time.Time `gorm:"column:created_at"`

	// Notice is the warning sent to the budget's channels when the row is new.
	Notice notifyv1.Notification `gorm:"-"`
}

func (BudgetWarning) TableName() string { return "mod_ai_gateway_budget_warnings" }

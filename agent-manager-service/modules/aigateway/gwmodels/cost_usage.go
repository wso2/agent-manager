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
)

// CostUsage is what one combination of project, agent, API key, user, LLM provider and
// model cost in one month, as gateways report it (AI cost stats, Interfaces 1.3). The
// values are the names gateways report, empty when unknown; Cost is a decimal string in
// USD.
type CostUsage struct {
	OUID                     string    `gorm:"column:org_id;primaryKey"`
	Month                    time.Time `gorm:"column:month;primaryKey"`
	CostKey                  uuid.UUID `gorm:"column:cost_key;primaryKey"`
	Project                  *string   `gorm:"column:project"`
	Agent                    *string   `gorm:"column:agent"`
	APIKey                   *string   `gorm:"column:api_key"`
	UserID                   *string   `gorm:"column:user_id"`
	LLMProvider              string    `gorm:"column:llm_provider"`
	Model                    *string   `gorm:"column:model"`
	Cost                     string    `gorm:"column:cost"`
	MissingPriceRequests     int64     `gorm:"column:missing_price_requests"`
	UsageUnavailableRequests int64     `gorm:"column:usage_unavailable_requests"`
	UpdatedAt                time.Time `gorm:"column:updated_at"`
}

// TableName names the table.
func (CostUsage) TableName() string { return "mod_ai_gateway_cost_usage" }

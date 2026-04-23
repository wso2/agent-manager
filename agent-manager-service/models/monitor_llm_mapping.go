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

package models

import (
	"time"

	"github.com/google/uuid"
)

// MonitorLLMMapping links a monitor to an LLM proxy in the shared llm_proxies table.
// This mirrors the env_agent_model_mapping pattern used by agents.
type MonitorLLMMapping struct {
	ID                  uint        `gorm:"column:id;primaryKey;autoIncrement" json:"-"`
	MonitorID           uuid.UUID   `gorm:"column:monitor_id;type:uuid;not null" json:"monitorId"`
	LLMProxyUUID        uuid.UUID   `gorm:"column:llm_proxy_uuid;type:uuid;not null" json:"llmProxyUuid"`
	PolicyConfiguration LLMPolicies `gorm:"column:policy_configuration;type:jsonb;default:[]" json:"policyConfiguration,omitempty"`
	CreatedAt           time.Time   `gorm:"column:created_at;type:timestamp;default:CURRENT_TIMESTAMP" json:"createdAt"`

	// Relations (for preloading — used during cleanup to derive proxy handle)
	LLMProxy *LLMProxy `gorm:"foreignKey:LLMProxyUUID" json:"llmProxy,omitempty"`
}

// TableName returns the table name for the MonitorLLMMapping model
func (MonitorLLMMapping) TableName() string {
	return "monitor_llm_mapping"
}

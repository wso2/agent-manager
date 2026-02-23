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

// AgentConfig is the GORM model for the agent_configs table.
// Stores per-environment configuration for agents.
type AgentConfig struct {
	ID                        uuid.UUID `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OrgName                   string    `gorm:"column:org_name;not null"`
	ProjectName               string    `gorm:"column:project_name;not null"`
	AgentName                 string    `gorm:"column:agent_name;not null"`
	EnvironmentName           string    `gorm:"column:environment_name;not null"`
	EnableAutoInstrumentation bool      `gorm:"column:enable_auto_instrumentation;not null"`
	CreatedAt                 time.Time `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt                 time.Time `gorm:"column:updated_at;not null;default:NOW()"`
}

func (AgentConfig) TableName() string { return "agent_configs" }

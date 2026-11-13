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
	"gorm.io/gorm"
)

// AIApplication tracks a per-agent-per-environment application record used for
// consumer-level rate limiting. One application is shared across all LLM configs
// of the same agent in the same environment; all proxy API keys for that agent+env
// are bound to it so the gateway policy engine can apply per-consumer limits.
type AIApplication struct {
	UUID            uuid.UUID  `gorm:"column:id;primaryKey" json:"uuid"`
	Handle          string     `gorm:"column:handle" json:"handle"`
	Name            string     `gorm:"column:name" json:"name"`
	AgentUUID       *uuid.UUID `gorm:"column:agent_id" json:"-"`
	EnvironmentUUID uuid.UUID  `gorm:"column:environment_id" json:"-"`
	AgentID         string     `gorm:"-" json:"agentId"`
	ProjectName     string     `gorm:"-" json:"projectName"`
	EnvironmentName string     `gorm:"-" json:"environmentName"`
	OUID            string     `gorm:"column:org_id" json:"-"`
	CreatedAt       time.Time  `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt       time.Time  `gorm:"column:updated_at" json:"updatedAt"`
}

// TableName returns the table name for the AIApplication model.
func (AIApplication) TableName() string {
	return "ai_applications"
}

func (a *AIApplication) ref() agentRef {
	return agentRef{&a.OUID, &a.ProjectName, &a.AgentID, &a.AgentUUID, a.EnvironmentUUID, &a.EnvironmentName}
}

// BeforeCreate resolves the agent's and environment's ids from their names.
func (a *AIApplication) BeforeCreate(tx *gorm.DB) error {
	if a.UUID == uuid.Nil {
		a.UUID = uuid.Must(uuid.NewV7())
	}
	if err := resolveEnvironment(tx, a.OUID, a.EnvironmentName, &a.EnvironmentUUID); err != nil {
		return err
	}
	return a.ref().resolve(tx)
}

// AfterFind fills the agent's and environment's names.
func (a *AIApplication) AfterFind(tx *gorm.DB) error { return a.ref().fill(tx) }

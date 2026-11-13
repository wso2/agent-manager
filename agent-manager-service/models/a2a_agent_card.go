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
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// A2AAgentCardStatus is where one agent-environment card stands in the fetch cycle.
type A2AAgentCardStatus string

const (
	// A2AAgentCardStatusPending means a fetch is due or being retried.
	A2AAgentCardStatusPending A2AAgentCardStatus = "pending"
	// A2AAgentCardStatusFetched means Card holds the last successful fetch.
	A2AAgentCardStatusFetched A2AAgentCardStatus = "fetched"
	// A2AAgentCardStatusFailed means the attempt budget ran out; a refresh restarts it.
	A2AAgentCardStatusFailed A2AAgentCardStatus = "failed"
)

// A2AAgentCardSource says where the card URL comes from.
type A2AAgentCardSource string

const (
	// A2AAgentCardSourcePlatform cards are fetched through the environment's gateway.
	A2AAgentCardSourcePlatform A2AAgentCardSource = "platform"
	// A2AAgentCardSourceExternal cards are fetched from a URL the user registered.
	A2AAgentCardSourceExternal A2AAgentCardSource = "external"
)

// A2AAgentCard is one agent-environment pair's public agent card and its fetch state. Services name the agent and
// environment; the hooks translate them to the stored ids.
type A2AAgentCard struct {
	ID              uuid.UUID          `gorm:"column:id;primaryKey"`
	OUID            string             `gorm:"column:org_id"`
	ProjectName     string             `gorm:"-"`
	AgentName       string             `gorm:"-"`
	EnvironmentName string             `gorm:"-"`
	EnvironmentUUID uuid.UUID          `gorm:"column:environment_id"`
	Source          A2AAgentCardSource `gorm:"column:source"`
	// AgentUUID is the agent the card belongs to; nil until resolved from the names.
	AgentUUID *uuid.UUID `gorm:"column:agent_id"`
	// SourceURL is the registered URL for external cards, and the last URL fetched for platform cards.
	SourceURL string          `gorm:"column:source_url"`
	Card      json.RawMessage `gorm:"column:card"`
	CardHash  string          `gorm:"column:card_hash"`
	FetchedAt *time.Time      `gorm:"column:fetched_at"`
	// ReleaseName and AttemptedReleaseName named the binding release a platform card came from; Lite has none.
	ReleaseName          string             `gorm:"-"`
	AttemptedReleaseName string             `gorm:"-"`
	Status               A2AAgentCardStatus `gorm:"column:status"`
	AttemptCount         int                `gorm:"column:attempt_count"`
	LastError            string             `gorm:"column:last_error"`
	NextAttemptAt        *time.Time         `gorm:"column:next_attempt_at"`
	CreatedAt            time.Time          `gorm:"column:created_at"`
	UpdatedAt            time.Time          `gorm:"column:updated_at"`
}

func (A2AAgentCard) TableName() string { return "agent_cards" }

func (c *A2AAgentCard) ref() agentRef {
	return agentRef{&c.OUID, &c.ProjectName, &c.AgentName, &c.AgentUUID, c.EnvironmentUUID, &c.EnvironmentName}
}

// BeforeCreate resolves the agent's id from its names.
func (c *A2AAgentCard) BeforeCreate(tx *gorm.DB) error { return c.ref().resolve(tx) }

// AfterFind fills the agent's and environment's names.
func (c *A2AAgentCard) AfterFind(tx *gorm.DB) error { return c.ref().fill(tx) }

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

// A2AAgentCard is one agent-environment pair's public agent card and its fetch state.
type A2AAgentCard struct {
	ID              uuid.UUID          `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OUID            string             `gorm:"column:ou_id;not null"`
	ProjectName     string             `gorm:"column:project_name;not null"`
	AgentName       string             `gorm:"column:agent_name;not null"`
	EnvironmentName string             `gorm:"column:environment_name;not null"`
	EnvironmentUUID uuid.UUID          `gorm:"column:environment_uuid;type:uuid;not null"`
	Source          A2AAgentCardSource `gorm:"column:source;not null"`
	// SourceURL is the registered URL for external cards, and the last URL fetched for platform cards.
	SourceURL string          `gorm:"column:source_url;not null;default:''"`
	Card      json.RawMessage `gorm:"column:card;type:jsonb"`
	CardHash  string          `gorm:"column:card_hash;not null;default:''"`
	FetchedAt *time.Time      `gorm:"column:fetched_at"`
	// ReleaseName is the binding release a platform card was fetched from.
	ReleaseName   string             `gorm:"column:release_name;not null;default:''"`
	Status        A2AAgentCardStatus `gorm:"column:status;not null;default:'pending'"`
	AttemptCount  int                `gorm:"column:attempt_count;not null;default:0"`
	LastError     string             `gorm:"column:last_error;not null;default:''"`
	NextAttemptAt *time.Time         `gorm:"column:next_attempt_at"`
	CreatedAt     time.Time          `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt     time.Time          `gorm:"column:updated_at;not null;default:NOW()"`
}

func (A2AAgentCard) TableName() string { return "a2a_agent_cards" }

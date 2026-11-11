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

// AgentExposureProtocol is how the gateway exposes a platform agent.
type AgentExposureProtocol string

const (
	AgentExposureProtocolA2A  AgentExposureProtocol = "a2a"
	AgentExposureProtocolREST AgentExposureProtocol = "rest"
)

// PlatformAgentExposure records that AMS exposes an agent it hosts through the environment's ingress
// gateway. Nothing else in AMS says so: the agent's subtype lives in the agent record, and its card row only appears once
// the gateway serves it. External agents are recorded by A2AProxy instead. Whether the gateway holds the agent is
// the artifact's deployment_status row.
type PlatformAgentExposure struct {
	ID          uuid.UUID `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OUID        string    `gorm:"column:ou_id;not null"`
	ProjectName string    `gorm:"column:project_name;not null"`
	AgentName   string    `gorm:"column:agent_name;not null"`
	// EnvironmentName is kept because platform lookups are by name.
	EnvironmentName string    `gorm:"column:environment_name;not null"`
	EnvironmentUUID uuid.UUID `gorm:"column:environment_uuid;type:uuid;not null"`
	// ArtifactUUID is the per-environment artifact (one of models.AgentArtifactKinds): the agentId the gateway is told about and the
	// key its API keys are bound to. The exposure goes with it.
	ArtifactUUID uuid.UUID `gorm:"column:artifact_uuid;type:uuid;not null"`
	// AgentUUID is the agent's id; nil when it couldn't be read.
	AgentUUID *uuid.UUID            `gorm:"column:agent_uuid;type:uuid"`
	Protocol  AgentExposureProtocol `gorm:"column:protocol;not null;default:'a2a'"`
	CreatedAt time.Time             `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt time.Time             `gorm:"column:updated_at;not null;default:NOW()"`
}

func (PlatformAgentExposure) TableName() string { return "platform_agent_exposures" }

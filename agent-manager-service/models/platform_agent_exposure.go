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
	ID              uuid.UUID `gorm:"column:id;primaryKey"`
	OUID            string    `gorm:"column:org_id"`
	ProjectName     string    `gorm:"-"`
	AgentName       string    `gorm:"-"`
	EnvironmentName string    `gorm:"-"`
	EnvironmentUUID uuid.UUID `gorm:"column:environment_id"`
	// ArtifactUUID is the per-environment artifact (one of models.AgentArtifactKinds): the agentId the gateway is told
	// about and the key its API keys are bound to. The exposure goes with it.
	ArtifactUUID uuid.UUID             `gorm:"column:artifact_id"`
	AgentUUID    *uuid.UUID            `gorm:"column:agent_id"`
	Protocol     AgentExposureProtocol `gorm:"column:protocol"`
	CreatedAt    time.Time             `gorm:"column:created_at"`
	UpdatedAt    time.Time             `gorm:"column:updated_at"`
}

func (PlatformAgentExposure) TableName() string { return "agent_exposures" }

func (e *PlatformAgentExposure) ref() agentRef {
	return agentRef{&e.OUID, &e.ProjectName, &e.AgentName, &e.AgentUUID, e.EnvironmentUUID, &e.EnvironmentName}
}

// BeforeCreate resolves the agent's id from its names.
func (e *PlatformAgentExposure) BeforeCreate(tx *gorm.DB) error {
	if e.Protocol == "" {
		e.Protocol = AgentExposureProtocolA2A
	}
	return e.ref().resolve(tx)
}

// AfterFind fills the agent's and environment's names.
func (e *PlatformAgentExposure) AfterFind(tx *gorm.DB) error { return e.ref().fill(tx) }

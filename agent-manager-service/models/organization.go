// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

// Organization is the root of every tenant-owned row. ThunderOUID links it to
// the Thunder organization unit carried in callers' tokens.
type Organization struct {
	ID          uuid.UUID `gorm:"column:id;primaryKey"`
	Handle      string    `gorm:"column:handle"`
	DisplayName string    `gorm:"column:display_name"`
	Description *string   `gorm:"column:description"`
	ThunderOUID *string   `gorm:"column:thunder_ou_id"`
	CreatedBy   *string   `gorm:"column:created_by"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (Organization) TableName() string { return "organizations" }

// Project groups agents within an organization.
type Project struct {
	ID          uuid.UUID `gorm:"column:id;primaryKey"`
	OrgID       uuid.UUID `gorm:"column:org_id"`
	Name        string    `gorm:"column:name"`
	DisplayName string    `gorm:"column:display_name"`
	Description *string   `gorm:"column:description"`
	CreatedBy   *string   `gorm:"column:created_by"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (Project) TableName() string { return "projects" }

// Environment is internal: every organization has exactly one, named
// DefaultEnvironmentName, and the API never exposes or accepts another.
type Environment struct {
	ID           uuid.UUID `gorm:"column:id;primaryKey"`
	OrgID        uuid.UUID `gorm:"column:org_id"`
	Name         string    `gorm:"column:name"`
	DisplayName  string    `gorm:"column:display_name"`
	IsProduction bool      `gorm:"column:is_production"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (Environment) TableName() string { return "environments" }

// DefaultEnvironmentName is the name of every organization's only environment.
const DefaultEnvironmentName = "default"

// DeploymentPipeline orders an organization's environments for promotion. Every organization gets one named
// DefaultPipelineName holding its default environment; only the Agent Runtime tier reads it.
type DeploymentPipeline struct {
	ID          uuid.UUID `gorm:"column:id;primaryKey"`
	OrgID       uuid.UUID `gorm:"column:org_id"`
	Name        string    `gorm:"column:name"`
	DisplayName string    `gorm:"column:display_name"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (DeploymentPipeline) TableName() string { return "deployment_pipelines" }

// DeploymentPipelineEnvironment places an environment in a pipeline; position 0 is the first stage.
type DeploymentPipelineEnvironment struct {
	PipelineID    uuid.UUID `gorm:"column:pipeline_id;primaryKey"`
	EnvironmentID uuid.UUID `gorm:"column:environment_id;primaryKey"`
	OrgID         uuid.UUID `gorm:"column:org_id"`
	Position      int       `gorm:"column:position"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

func (DeploymentPipelineEnvironment) TableName() string { return "deployment_pipeline_environments" }

// DefaultPipelineName is the name of the pipeline every organization starts with.
const DefaultPipelineName = "default"

// Agent is an agent registered in a project. Only external agents exist
// without a hosted runtime.
type Agent struct {
	ID               uuid.UUID    `gorm:"column:id;primaryKey"`
	OrgID            uuid.UUID    `gorm:"column:org_id"`
	ProjectID        uuid.UUID    `gorm:"column:project_id"`
	Name             string       `gorm:"column:name"`
	DisplayName      string       `gorm:"column:display_name"`
	Description      *string      `gorm:"column:description"`
	ProvisioningType string       `gorm:"column:provisioning_type"`
	CreatedBy        *string      `gorm:"column:created_by"`
	CreatedAt        time.Time    `gorm:"column:created_at"`
	UpdatedAt        time.Time    `gorm:"column:updated_at"`
	Labels           []AgentLabel `gorm:"foreignKey:AgentID"`
}

func (Agent) TableName() string { return "agents" }

type AgentLabel struct {
	AgentID    uuid.UUID `gorm:"column:agent_id;primaryKey"`
	OrgID      uuid.UUID `gorm:"column:org_id"`
	LabelKey   string    `gorm:"column:label_key;primaryKey"`
	LabelValue *string   `gorm:"column:label_value"`
}

func (AgentLabel) TableName() string { return "agent_labels" }

// OrganizationResponse is the API shape of an organization.
type OrganizationResponse struct {
	Name        string    `json:"name"`
	DisplayName string    `json:"displayName,omitempty"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	Status      string    `json:"status,omitempty"`
	Handle      string    `json:"handle,omitempty"` // caller's OU handle, for console URLs; set from the token
}

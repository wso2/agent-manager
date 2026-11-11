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
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LLMPolicies is a custom type for []LLMPolicy that handles JSONB scanning
type LLMPolicies []LLMPolicy

func (p LLMPolicies) Value() (driver.Value, error) {
	return json.Marshal(p)
}

func (p *LLMPolicies) Scan(value interface{}) error {
	if value == nil {
		*p = LLMPolicies{}
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("unsupported type for LLMPolicies: %T", value)
	}
	return json.Unmarshal(bytes, p)
}

// AgentConfiguration represents an agent's model configuration. Services set and read the agent by its names
// (AgentID, ProjectName) and the type by TypeID; the hooks translate them to the stored agent id and type text.
type AgentConfiguration struct {
	UUID        uuid.UUID `gorm:"column:id;primaryKey" json:"uuid"`
	Name        string    `gorm:"column:name" json:"name"`
	Description string    `gorm:"column:description" json:"description,omitempty"`
	OUID        string    `gorm:"column:org_id" json:"organizationName"`
	AgentUUID   uuid.UUID `gorm:"column:agent_id" json:"-"`
	Type        string    `gorm:"column:type" json:"-"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt   time.Time `gorm:"column:updated_at" json:"updatedAt"`

	// AgentID is the agent's name and ProjectName its project's; TypeID is Type as a number.
	AgentID     string `gorm:"-" json:"agentId"`
	ProjectName string `gorm:"-" json:"projectName"`
	TypeID      uint   `gorm:"-" json:"-"`

	// Relationships
	EnvMappings    []EnvAgentModelMapping   `gorm:"foreignKey:ConfigUUID" json:"envMappings,omitempty"`
	EnvMCPMappings []EnvAgentMCPMapping     `gorm:"foreignKey:ConfigUUID" json:"mcpEnvMappings,omitempty"`
	EnvVariables   []AgentEnvConfigVariable `gorm:"foreignKey:ConfigUUID" json:"-"`

	// MCPProxyRef is the environment-agnostic MCP proxy an MCP-type configuration
	// references, independent of which environments currently hold a deployed mapping.
	MCPProxyRef *AgentMCPConfigProxy `gorm:"foreignKey:ConfigUUID;references:UUID" json:"mcpProxyRef,omitempty"`

	// MCPEnvExclusions are the environments an MCP connection was deliberately removed from.
	MCPEnvExclusions []AgentMCPConfigEnvExclusion `gorm:"foreignKey:ConfigUUID;references:UUID" json:"-"`

	// MCPScope is non-nil once the connection's environment scope has been recorded.
	MCPScope *AgentMCPConfigScope `gorm:"foreignKey:ConfigUUID;references:UUID" json:"-"`
}

// TableName returns the table name for the AgentConfiguration model
func (AgentConfiguration) TableName() string {
	return "agent_model_configs"
}

// BeforeSave stores the type as text and resolves the agent's id from its names.
func (c *AgentConfiguration) BeforeSave(tx *gorm.DB) error {
	if c.TypeID != 0 {
		c.Type = AgentConfigTypeFromID(c.TypeID)
	} else if c.Type == "" {
		c.Type = AgentConfigTypeLLM
	}
	if c.AgentUUID == uuid.Nil && c.AgentID != "" {
		id, err := AgentIDByNames(tx, c.OUID, c.ProjectName, c.AgentID)
		if err != nil {
			return err
		}
		c.AgentUUID = id
	}
	return nil
}

// AfterFind fills the agent's names and the numeric type.
func (c *AgentConfiguration) AfterFind(tx *gorm.DB) error {
	c.TypeID = AgentConfigTypeToID(c.Type)
	if c.AgentUUID == uuid.Nil || c.AgentID != "" {
		return nil
	}
	project, agent, err := AgentNamesByID(tx, c.AgentUUID)
	if err != nil {
		return err
	}
	c.ProjectName, c.AgentID = project, agent
	return nil
}

// AgentMCPConfigProxy binds one MCP-type AgentConfiguration to the org-level MCP proxy it references.
type AgentMCPConfigProxy struct {
	ConfigUUID   uuid.UUID `gorm:"column:config_id;primaryKey" json:"configUuid"`
	OrgID        string    `gorm:"column:org_id" json:"-"`
	MCPProxyUUID uuid.UUID `gorm:"column:mcp_proxy_id" json:"mcpProxyUuid"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt    time.Time `gorm:"column:updated_at" json:"updatedAt"`
	TypeID       uint      `gorm:"-" json:"-"`
}

// TableName returns the table name for the AgentMCPConfigProxy model.
func (AgentMCPConfigProxy) TableName() string {
	return "agent_model_config_mcp"
}

// BeforeCreate copies the configuration's organization.
func (p *AgentMCPConfigProxy) BeforeCreate(tx *gorm.DB) (err error) {
	if p.OrgID == "" {
		p.OrgID, err = orgIDOf(tx, "agent_model_configs", p.ConfigUUID)
	}
	return err
}

// AgentMCPConfigEnvExclusion records an environment an MCP connection was deliberately removed from. It shares
// agent_model_config_mcp_envs with EnvAgentMCPMapping, as a row with excluded set and no mapping.
type AgentMCPConfigEnvExclusion struct {
	ConfigUUID      uuid.UUID `gorm:"column:config_id;primaryKey" json:"configUuid"`
	EnvironmentUUID uuid.UUID `gorm:"column:environment_id;primaryKey" json:"environmentUuid"`
	OrgID           string    `gorm:"column:org_id" json:"-"`
	Excluded        bool      `gorm:"column:excluded" json:"-"`
	CreatedAt       time.Time `gorm:"column:created_at" json:"createdAt"`
	TypeID          uint      `gorm:"-" json:"-"`
}

// TableName returns the table name for the AgentMCPConfigEnvExclusion model.
func (AgentMCPConfigEnvExclusion) TableName() string {
	return "agent_model_config_mcp_envs"
}

// BeforeCreate marks the row as an exclusion and copies the configuration's organization.
func (e *AgentMCPConfigEnvExclusion) BeforeCreate(tx *gorm.DB) (err error) {
	e.Excluded = true
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
	}
	if e.OrgID == "" {
		e.OrgID, err = orgIDOf(tx, "agent_model_configs", e.ConfigUUID)
	}
	return err
}

// AgentMCPConfigScope marks an MCP connection whose environment scope has been recorded. It is the
// scope_recorded_at column of the connection's agent_model_config_mcp row.
type AgentMCPConfigScope struct {
	ConfigUUID uuid.UUID `gorm:"column:config_id;primaryKey" json:"configUuid"`
	RecordedAt time.Time `gorm:"column:scope_recorded_at" json:"recordedAt"`
	TypeID     uint      `gorm:"-" json:"-"`
}

// TableName returns the table name for the AgentMCPConfigScope model.
func (AgentMCPConfigScope) TableName() string {
	return "agent_model_config_mcp"
}

// EnvAgentModelMapping represents per-environment LLM proxy mapping
type EnvAgentModelMapping struct {
	ConfigUUID          uuid.UUID   `gorm:"column:config_id;primaryKey" json:"configUuid"`
	EnvironmentUUID     uuid.UUID   `gorm:"column:environment_id;primaryKey" json:"environmentUuid"`
	OrgID               string      `gorm:"column:org_id" json:"-"`
	LLMProxyUUID        uuid.UUID   `gorm:"column:llm_proxy_id" json:"llmProxyUuid"`
	CreatedAt           time.Time   `gorm:"column:created_at" json:"createdAt"`
	PolicyConfiguration LLMPolicies `gorm:"column:policy_configuration" json:"policyConfiguration,omitempty"`

	// Relationships
	LLMProxy *LLMProxy `gorm:"foreignKey:LLMProxyUUID" json:"llmProxy,omitempty"`
}

// TableName returns the table name for the EnvAgentModelMapping model
func (EnvAgentModelMapping) TableName() string {
	return "agent_model_config_llm"
}

// BeforeCreate copies the configuration's organization.
func (m *EnvAgentModelMapping) BeforeCreate(tx *gorm.DB) (err error) {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
	}
	if m.PolicyConfiguration == nil {
		m.PolicyConfiguration = LLMPolicies{}
	}
	if m.OrgID == "" {
		m.OrgID, err = orgIDOf(tx, "agent_model_configs", m.ConfigUUID)
	}
	return err
}

// EnvAgentMCPMapping is an MCP connection's binding in one environment: the agent's mapping artifact of the MCP
// proxy. It shares agent_model_config_mcp_envs with AgentMCPConfigEnvExclusion, as a row that is not excluded.
type EnvAgentMCPMapping struct {
	ConfigUUID      uuid.UUID `gorm:"column:config_id;primaryKey" json:"configUuid"`
	EnvironmentUUID uuid.UUID `gorm:"column:environment_id;primaryKey" json:"environmentUuid"`
	OrgID           string    `gorm:"column:org_id" json:"-"`
	MCPProxyUUID    uuid.UUID `gorm:"column:mcp_proxy_id" json:"mcpProxyUuid"`
	ArtifactUUID    uuid.UUID `gorm:"column:mapping_artifact_id" json:"artifactUuid"`
	Excluded        bool      `gorm:"column:excluded" json:"-"`
	CreatedAt       time.Time `gorm:"column:created_at" json:"createdAt"`

	// Relationships
	MCPProxy        *MCPProxy        `gorm:"foreignKey:MCPProxyUUID" json:"mcpProxy,omitempty"`
	MCPProxyMapping *MCPProxyMapping `gorm:"foreignKey:ArtifactUUID;references:UUID" json:"mcpProxyMapping,omitempty"`
	Artifact        *Artifact        `gorm:"foreignKey:ArtifactUUID;references:UUID" json:"artifact,omitempty"`
}

// TableName returns the table name for the EnvAgentMCPMapping model.
func (EnvAgentMCPMapping) TableName() string {
	return "agent_model_config_mcp_envs"
}

// BeforeCreate copies the configuration's organization.
func (m *EnvAgentMCPMapping) BeforeCreate(tx *gorm.DB) (err error) {
	m.Excluded = false
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
	}
	if m.OrgID == "" {
		m.OrgID, err = orgIDOf(tx, "agent_model_configs", m.ConfigUUID)
	}
	return err
}

// AgentEnvConfigVariable is the name an environment variable of a configuration takes in one environment.
type AgentEnvConfigVariable struct {
	ConfigUUID      uuid.UUID `gorm:"column:config_id;primaryKey" json:"-"`
	EnvironmentUUID uuid.UUID `gorm:"column:environment_id;primaryKey" json:"-"`
	VariableKey     string    `gorm:"column:variable_key;primaryKey" json:"key"`
	OrgID           string    `gorm:"column:org_id" json:"-"`
	VariableName    string    `gorm:"column:variable_name" json:"name"`
	CreatedAt       time.Time `gorm:"column:created_at" json:"-"`
	// SecretReference named a secret only hosted workloads mounted; external agents are handed their keys.
	SecretReference string `gorm:"-" json:"-"`
}

// TableName returns the table name for the AgentEnvConfigVariable model
func (AgentEnvConfigVariable) TableName() string {
	return "agent_model_config_vars"
}

// BeforeCreate copies the configuration's organization.
func (v *AgentEnvConfigVariable) BeforeCreate(tx *gorm.DB) (err error) {
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
	}
	if v.OrgID == "" {
		v.OrgID, err = orgIDOf(tx, "agent_model_configs", v.ConfigUUID)
	}
	return err
}

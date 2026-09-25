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

// AgentConfiguration represents an agent's model configuration
type AgentConfiguration struct {
	UUID        uuid.UUID `gorm:"column:uuid;type:uuid;primaryKey;default:gen_random_uuid()" json:"uuid"`
	Name        string    `gorm:"column:name;type:varchar(255);not null" json:"name"`
	Description string    `gorm:"column:description;type:text" json:"description,omitempty"`
	AgentID     string    `gorm:"column:agent_id;type:varchar(255);not null" json:"agentId"`
	TypeID      uint      `gorm:"column:type_id;type:integer;not null;default:1" json:"-"`
	OUID        string    `gorm:"column:ou_id;type:varchar(255);not null" json:"organizationName"`
	ProjectName string    `gorm:"column:project_name;type:varchar(255);not null" json:"projectName"`
	CreatedAt   time.Time `gorm:"column:created_at;type:timestamp;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt   time.Time `gorm:"column:updated_at;type:timestamp;default:CURRENT_TIMESTAMP" json:"updatedAt"`

	// Relations (eager loaded)
	EnvMappings    []EnvAgentModelMapping   `gorm:"foreignKey:ConfigUUID;constraint:OnDelete:CASCADE" json:"envMappings,omitempty"`
	EnvMCPMappings []EnvAgentMCPMapping     `gorm:"foreignKey:ConfigUUID;constraint:OnDelete:CASCADE" json:"mcpEnvMappings,omitempty"`
	EnvVariables   []AgentEnvConfigVariable `gorm:"foreignKey:ConfigUUID;constraint:OnDelete:CASCADE" json:"-"`

	// MCPProxyRef is the environment-agnostic MCP proxy an MCP-type configuration
	// references. Nil for a non-MCP configuration, and for an MCP configuration whose
	// environments name different proxies: there is no single answer then, so readers fall
	// back to the per-environment mapping rows.
	//
	// Lives in its own table rather than as a column here because this row is polymorphic
	// across LLM, MCP and agent configurations — see migration044.
	MCPProxyRef *AgentMCPConfigProxy `gorm:"foreignKey:ConfigUUID;references:UUID" json:"mcpProxyRef,omitempty"`
}

// AgentMCPConfigProxy binds one MCP-type AgentConfiguration to the org-level MCP proxy it
// references, independent of any environment.
//
// The reference is kept here rather than on agent_configurations so that table stays
// generic, and so the database can enforce that only an MCP configuration carries one: the
// TypeID column is pinned to AgentConfigTypeIDMCP by a CHECK and joined back to
// agent_configurations through a composite foreign key on (uuid, type_id). TypeID is
// therefore redundant by design — it exists to make that key expressible.
//
// A row's presence is the whole signal. There is no NULL proxy state: a configuration
// either records an environment-agnostic proxy or has no row at all.
type AgentMCPConfigProxy struct {
	ConfigUUID   uuid.UUID `gorm:"column:config_uuid;type:uuid;primaryKey" json:"configUuid"`
	TypeID       uint      `gorm:"column:type_id;type:integer;not null;default:2" json:"-"`
	MCPProxyUUID uuid.UUID `gorm:"column:mcp_proxy_uuid;type:uuid;not null" json:"mcpProxyUuid"`
	CreatedAt    time.Time `gorm:"column:created_at;type:timestamp;default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt    time.Time `gorm:"column:updated_at;type:timestamp;default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

// TableName returns the table name for the AgentMCPConfigProxy model.
func (AgentMCPConfigProxy) TableName() string {
	return "agent_mcp_config_proxy"
}

// TableName returns the table name for the AgentConfiguration model
func (AgentConfiguration) TableName() string {
	return "agent_configurations"
}

// EnvAgentModelMapping represents environment-specific model configuration
type EnvAgentModelMapping struct {
	ID                  uint        `gorm:"column:id;primaryKey;autoIncrement" json:"-"`
	ConfigUUID          uuid.UUID   `gorm:"column:config_uuid;type:uuid;not null" json:"configUuid"`
	EnvironmentUUID     uuid.UUID   `gorm:"column:environment_uuid;type:uuid;not null" json:"environmentUuid"`
	LLMProxyUUID        uuid.UUID   `gorm:"column:llm_proxy_uuid;type:uuid;not null" json:"llmProxyUuid"`
	CreatedAt           time.Time   `gorm:"column:created_at;type:timestamp;default:CURRENT_TIMESTAMP" json:"createdAt"`
	PolicyConfiguration LLMPolicies `gorm:"column:policy_configuration;type:jsonb;default:[]" json:"policyConfiguration,omitempty"`

	// Relations (for preloading)
	LLMProxy *LLMProxy `gorm:"foreignKey:LLMProxyUUID" json:"llmProxy,omitempty"`
}

// TableName returns the table name for the EnvAgentModelMapping model
func (EnvAgentModelMapping) TableName() string {
	return "env_agent_model_mapping"
}

// EnvAgentMCPMapping represents environment-specific MCP configuration.
type EnvAgentMCPMapping struct {
	ID              uint      `gorm:"column:id;primaryKey;autoIncrement" json:"-"`
	ConfigUUID      uuid.UUID `gorm:"column:config_uuid;type:uuid;not null" json:"configUuid"`
	EnvironmentUUID uuid.UUID `gorm:"column:environment_uuid;type:uuid;not null" json:"environmentUuid"`
	MCPProxyUUID    uuid.UUID `gorm:"column:mcp_proxy_uuid;type:uuid;not null" json:"mcpProxyUuid"`
	ArtifactUUID    uuid.UUID `gorm:"column:artifact_uuid;type:uuid;not null" json:"artifactUuid"`
	CreatedAt       time.Time `gorm:"column:created_at;type:timestamp;default:CURRENT_TIMESTAMP" json:"createdAt"`

	// Relations (for preloading)
	MCPProxy        *MCPProxy        `gorm:"foreignKey:MCPProxyUUID" json:"mcpProxy,omitempty"`
	MCPProxyMapping *MCPProxyMapping `gorm:"foreignKey:ArtifactUUID;references:UUID" json:"mcpProxyMapping,omitempty"`
	Artifact        *Artifact        `gorm:"foreignKey:ArtifactUUID;references:UUID" json:"artifact,omitempty"`
}

// TableName returns the table name for the EnvAgentMCPMapping model.
func (EnvAgentMCPMapping) TableName() string {
	return "env_agent_mcp_mapping"
}

// AgentEnvConfigVariable represents environment variable configuration
type AgentEnvConfigVariable struct {
	ID              uint      `gorm:"column:id;primaryKey;autoIncrement" json:"-"`
	ConfigUUID      uuid.UUID `gorm:"column:config_uuid;type:uuid;not null" json:"-"`
	EnvironmentUUID uuid.UUID `gorm:"column:environment_uuid;type:uuid;not null" json:"-"`
	VariableName    string    `gorm:"column:variable_name;type:varchar(255);not null" json:"name"`
	VariableKey     string    `gorm:"column:variable_key;type:varchar(255);not null" json:"key"`
	SecretReference string    `gorm:"column:secret_reference;type:text;not null" json:"-"` // NEVER expose in API
	CreatedAt       time.Time `gorm:"column:created_at;type:timestamp;default:CURRENT_TIMESTAMP" json:"-"`
}

// TableName returns the table name for the AgentEnvConfigVariable model
func (AgentEnvConfigVariable) TableName() string {
	return "agent_env_config_variables_mapping"
}

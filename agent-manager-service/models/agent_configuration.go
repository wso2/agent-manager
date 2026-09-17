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

	// MCPProxyUUID is the org-level MCP proxy an MCP-type configuration references,
	// independent of any environment. An MCP connection is environment-agnostic by
	// construction — every environment maps to the same proxy — so recording it here
	// keeps the reference resolvable in environments that have no EnvAgentMCPMapping
	// row yet, which is what lets the binding reconcile find a connection it has
	// never managed to bind anywhere.
	//
	// Nil for non-MCP configurations, and for an MCP configuration whose environments
	// name different proxies: there is no single environment-agnostic answer then, so
	// readers fall back to the per-environment mapping rows.
	//
	// The foreign key is ON DELETE RESTRICT, so a proxy cannot be deleted while a
	// configuration still points at it — see migration044 for why SET NULL was unsafe.
	MCPProxyUUID *uuid.UUID `gorm:"column:mcp_proxy_uuid;type:uuid" json:"mcpProxyUuid,omitempty"`

	// Relations (eager loaded)
	EnvMappings    []EnvAgentModelMapping   `gorm:"foreignKey:ConfigUUID;constraint:OnDelete:CASCADE" json:"envMappings,omitempty"`
	EnvMCPMappings []EnvAgentMCPMapping     `gorm:"foreignKey:ConfigUUID;constraint:OnDelete:CASCADE" json:"mcpEnvMappings,omitempty"`
	EnvVariables   []AgentEnvConfigVariable `gorm:"foreignKey:ConfigUUID;constraint:OnDelete:CASCADE" json:"-"`
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

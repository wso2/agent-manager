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

// -----------------------------------------------------------------------------
// DB Structs
// -----------------------------------------------------------------------------

type AgentKind struct {
	ID          uuid.UUID          `gorm:"column:id;primaryKey"`
	Name        string             `gorm:"column:name"`
	DisplayName string             `gorm:"column:display_name"`
	Description string             `gorm:"column:description"`
	OUID        string             `gorm:"column:ou_id"`
	ProjectName string             `gorm:"column:project_name"`
	AgentName   string             `gorm:"column:agent_name"`
	Labels      map[string]string  `gorm:"column:labels;type:jsonb;serializer:json"`
	CreatedAt   time.Time          `gorm:"column:created_at"`
	UpdatedAt   time.Time          `gorm:"column:updated_at"`
	Versions    []AgentKindVersion `gorm:"foreignKey:AgentKindID"`
}

func (AgentKind) TableName() string { return "agent_kinds" }

type AgentKindVersion struct {
	ID           uuid.UUID              `gorm:"column:id;primaryKey"`
	AgentKindID  uuid.UUID              `gorm:"column:agent_kind_id"`
	Version      string                 `gorm:"column:version"`
	BuildName    string                 `gorm:"column:build_name"`
	ImageId      string                 `gorm:"column:image_id"`
	ConfigSchema []KindConfigSchemaItem `gorm:"column:config_schema;type:jsonb;serializer:json"`
	Metadata     json.RawMessage        `gorm:"column:metadata;type:jsonb"`
	// HealthChecks are the source agent's health checks when the version was
	// published. nil for versions published before they were recorded.
	HealthChecks json.RawMessage `gorm:"column:health_checks;type:jsonb"`
	CreatedAt    time.Time       `gorm:"column:created_at"`
	Kind         *AgentKind      `gorm:"foreignKey:AgentKindID"`
}

func (AgentKindVersion) TableName() string { return "agent_kind_versions" }

// KindConfigSchemaItem defines a single configurable parameter in an Agent Kind version.
type KindConfigSchemaItem struct {
	Name         string  `json:"name"`
	Description  string  `json:"description,omitempty"`
	IsSecret     bool    `json:"isSecret"`
	IsMandatory  bool    `json:"isMandatory"`
	DefaultValue *string `json:"defaultValue,omitempty"`
}

// RedactedSecretDefaultPlaceholder replaces a secret config item's real default value
// in any client-facing response. It exists so a client can still tell a mandatory
// secret with a baked-in default apart from one that has none at all (and therefore
// must be supplied by whoever creates an agent from the kind) without ever learning
// what the real value is.
const RedactedSecretDefaultPlaceholder = "••••••••"

// RedactSecretDefaults returns a copy of schema where every secret item's real
// DefaultValue has been replaced with RedactedSecretDefaultPlaceholder (or left empty
// if it had none), so a secret's default value is never serialized back to a client
// once it has been submitted. Callers needing the true default for internal use
// (validation, applying it server-side) must read the un-redacted schema instead.
func RedactSecretDefaults(schema []KindConfigSchemaItem) []KindConfigSchemaItem {
	out := make([]KindConfigSchemaItem, len(schema))
	copy(out, schema)
	for i := range out {
		if !out[i].IsSecret {
			continue
		}
		if out[i].DefaultValue != nil && *out[i].DefaultValue != "" {
			placeholder := RedactedSecretDefaultPlaceholder
			out[i].DefaultValue = &placeholder
		} else {
			out[i].DefaultValue = nil
		}
	}
	return out
}

// -----------------------------------------------------------------------------
// Response DTOs
// -----------------------------------------------------------------------------

type AgentKindVersionResponse struct {
	Version           string                 `json:"version"`
	BuildName         string                 `json:"buildName,omitempty"`
	ImageId           string                 `json:"imageId"`
	SourceAgentName   string                 `json:"sourceAgentName,omitempty"`
	SourceProjectName string                 `json:"sourceProjectName,omitempty"`
	ConfigSchema      []KindConfigSchemaItem `json:"configSchema"`
	Metadata          json.RawMessage        `json:"metadata,omitempty"`
	CreatedAt         time.Time              `json:"createdAt"`
}

type AgentKindResponse struct {
	UUID          string                     `json:"uuid"`
	Name          string                     `json:"name"`
	Kind          string                     `json:"kind"`
	DisplayName   string                     `json:"displayName"`
	Description   string                     `json:"description,omitempty"`
	ProjectName   string                     `json:"projectName"`
	AgentName     string                     `json:"agentName"`
	LatestVersion string                     `json:"latestVersion,omitempty"`
	Labels        map[string]string          `json:"labels"`
	Versions      []AgentKindVersionResponse `json:"versions"`
	CreatedAt     time.Time                  `json:"createdAt"`
	UpdatedAt     time.Time                  `json:"updatedAt"`
}

type AgentKindListResponse struct {
	Kinds  []AgentKindResponse `json:"kinds"`
	Total  int64               `json:"total"`
	Limit  int                 `json:"limit"`
	Offset int                 `json:"offset"`
}

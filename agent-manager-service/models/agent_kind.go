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

// -----------------------------------------------------------------------------
// DB Structs
// -----------------------------------------------------------------------------

type AgentKind struct {
	ID          uuid.UUID          `gorm:"column:id;primaryKey"`
	Name        string             `gorm:"column:name"`
	DisplayName string             `gorm:"column:display_name"`
	Description string             `gorm:"column:description"`
	OrgName     string             `gorm:"column:org_name"`
	ProjectName string             `gorm:"column:project_name"`
	AgentName   string             `gorm:"column:agent_name"`
	CreatedAt   time.Time          `gorm:"column:created_at"`
	UpdatedAt   time.Time          `gorm:"column:updated_at"`
	DeletedAt   gorm.DeletedAt     `gorm:"column:deleted_at"`
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
	CreatedAt    time.Time              `gorm:"column:created_at"`
	Kind         *AgentKind             `gorm:"foreignKey:AgentKindID"`
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
	CreatedAt         time.Time              `json:"createdAt"`
}

type AgentKindResponse struct {
	UUID          string                     `json:"uuid"`
	Name          string                     `json:"name"`
	Kind          string                     `json:"kind"`
	DisplayName   string                     `json:"displayName"`
	Description   string                     `json:"description,omitempty"`
	OrgName       string                     `json:"orgName"`
	ProjectName   string                     `json:"projectName"`
	AgentName     string                     `json:"agentName"`
	LatestVersion string                     `json:"latestVersion,omitempty"`
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

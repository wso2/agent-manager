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
	"gorm.io/gorm"
)

// API Response DTO
type AgentResponse struct {
	Name         string       `json:"name"`
	DisplayName  string       `json:"displayName,omitempty"`
	Description  string       `json:"description,omitempty"`
	ProjectName  string       `json:"projectName"`
	CreatedAt    time.Time    `json:"createdAt"`
	Status       string       `json:"status,omitempty"`
	Provisioning Provisioning `json:"provisioning,omitempty"`
	AgentType    AgentType    `json:"agentType,omitempty"`
	Language     string       `json:"language,omitempty"`
}

type AgentType struct {
	// Type of the agent
	Type string `json:"type"`
	// Sub-type of the agent
	SubType string `json:"subType"`
}

type Provisioning struct {
	Type       string     `json:"type"`
	Repository Repository `json:"repository,omitempty"`
}

type Repository struct {
	Url     string `json:"url"`
	AppPath string `json:"appPath"`
	Branch  string `json:"branch"`
}

// DB Model
type Agent struct {
	ID               uuid.UUID      `gorm:"column:id;primaryKey"`
	ProvisioningType string         `gorm:"column:provisioning_type"`
	Name             string         `gorm:"column:name"`
	DisplayName      string         `gorm:"column:display_name"`
	Description      string         `gorm:"column:description"`
	ProjectId        uuid.UUID      `gorm:"column:project_id"`
	OrgID            uuid.UUID      `gorm:"column:org_id"`
	CreatedAt        time.Time      `gorm:"column:created_at"`
	UpdatedAt        time.Time      `gorm:"column:updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"column:deleted_at"`
	AgentDetails     *InternalAgent
}

type InternalAgent struct {
	ID           uuid.UUID              `gorm:"column:id;primaryKey"`
	AgentType    string                 `gorm:"column:agent_type"`
	AgentSubType string                 `gorm:"column:agent_subtype"`
	Language     string                 `gorm:"column:language"`
	WorkloadSpec map[string]interface{} `gorm:"column:workload_spec;type:jsonb;serializer:json"`
}

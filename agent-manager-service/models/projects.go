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
type ProjectResponse struct {
	Name               string    `json:"name"`
	OrgName            string    `json:"orgName"`
	DisplayName        string    `json:"displayName,omitempty"`
	Description        string    `json:"description,omitempty"`
	DeploymentPipeline string    `json:"deploymentPipeline,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	Status             string    `json:"status,omitempty"`
}

// DB Model
type Project struct {
	ID                uuid.UUID      `gorm:"column:id;primaryKey" json:"projectID" binding:"required"`
	Name              string         `gorm:"column:name" json:"name" binding:"required"`
	OrgID             uuid.UUID      `gorm:"column:org_id" json:"orgID" binding:"required"`
	OpenChoreoProject string         `gorm:"column:open_choreo_project" json:"openChoreoProject,omitempty"`
	DisplayName       string         `gorm:"column:display_name" json:"displayName,omitempty"`
	Description       string         `gorm:"column:description" json:"description,omitempty"`
	CreatedAt         time.Time      `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt         time.Time      `gorm:"column:updated_at" json:"updatedAt"`
	DeletedAt         gorm.DeletedAt `gorm:"column:deleted_at" json:"deletedAt"`
}

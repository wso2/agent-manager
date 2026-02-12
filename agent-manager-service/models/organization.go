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

// Organization is the database model for organizations
type Organization struct {
	UUID      uuid.UUID      `gorm:"column:uuid;primaryKey"`
	Name      string         `gorm:"column:name;uniqueIndex"`
	Handle    string         `gorm:"column:handle;uniqueIndex"`
	Region    string         `gorm:"column:region"`
	CreatedAt time.Time      `gorm:"column:created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

// TableName returns the table name for GORM
func (Organization) TableName() string {
	return "organizations"
}

// API Response DTO (from OpenChoreo)
type OrganizationResponse struct {
	Name        string    `json:"name"`
	DisplayName string    `json:"displayName,omitempty"`
	Description string    `json:"description,omitempty"`
	Namespace   string    `json:"namespace,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	Status      string    `json:"status,omitempty"`
}

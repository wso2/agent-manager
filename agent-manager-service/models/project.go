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

// Project represents an API Platform project within an organization
type Project struct {
	ID               string         `gorm:"column:uuid;primaryKey" json:"id" db:"uuid"`
	OrganizationUUID string         `gorm:"column:organization_uuid" json:"organizationId" db:"organization_uuid"`
	Name             string         `gorm:"column:name" json:"name" db:"name"`
	Description      string         `gorm:"column:description" json:"description,omitempty" db:"description"`
	CreatedAt        time.Time      `gorm:"column:created_at" json:"createdAt" db:"created_at"`
	UpdatedAt        time.Time      `gorm:"column:updated_at" json:"updatedAt" db:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"column:deleted_at;index" json:"-" db:"deleted_at"`
}

// TableName returns the table name for the Project model
func (Project) TableName() string {
	return "ap_projects"
}

// BeforeCreate will set a UUID if one is not provided
func (p *Project) BeforeCreate(tx *gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	return nil
}

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
)

// APProject represents an API Platform project entity
type APProject struct {
	UUID             uuid.UUID `gorm:"column:uuid;primaryKey" json:"id"`
	Name             string    `gorm:"column:name" json:"name"`
	OrganizationUUID uuid.UUID `gorm:"column:organization_uuid" json:"organizationId"`
	Description      string    `gorm:"column:description" json:"description"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt        time.Time `gorm:"column:updated_at" json:"updatedAt"`
}

// TableName returns the table name for the APProject model
func (APProject) TableName() string {
	return "ap_projects"
}

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

// AssociationMapping represents the association between an API and a resource (gateway or dev portal)
type AssociationMapping struct {
	ID               int64     `gorm:"column:id;primaryKey" json:"id"`
	ArtifactUUID     uuid.UUID `gorm:"column:artifact_uuid" json:"artifactId"`
	OrganizationUUID uuid.UUID `gorm:"column:organization_uuid" json:"organizationId"`
	ResourceUUID     uuid.UUID `gorm:"column:resource_uuid" json:"resourceId"`
	AssociationType  string    `gorm:"column:association_type" json:"associationType"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt        time.Time `gorm:"column:updated_at" json:"updatedAt"`
}

// TableName returns the table name for the AssociationMapping model
func (AssociationMapping) TableName() string {
	return "association_mappings"
}

// AssociationType constants
const (
	AssociationTypeGateway   = "gateway"
	AssociationTypeDevPortal = "dev_portal"
)

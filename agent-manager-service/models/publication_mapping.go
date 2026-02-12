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
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PublicationStatus represents the status of an API publication to a DevPortal
type PublicationStatus string

const (
	PublishedStatus  PublicationStatus = "published"
	FailedStatus     PublicationStatus = "failed"
	PublishingStatus PublicationStatus = "publishing"
)

// PublicationMapping represents the current publication status of an API to a DevPortal
// Note: This table uses a composite primary key (api_uuid, devportal_uuid, organization_uuid)
type PublicationMapping struct {
	APIUUID          uuid.UUID         `gorm:"column:api_uuid;primaryKey" json:"apiUuid"`
	DevPortalUUID    uuid.UUID         `gorm:"column:devportal_uuid;primaryKey" json:"devPortalUuid"`
	OrganizationUUID uuid.UUID         `gorm:"column:organization_uuid;primaryKey" json:"organizationUuid"`
	Status           PublicationStatus `gorm:"column:status" json:"status"`
	APIVersion       *string           `gorm:"column:api_version" json:"apiVersion,omitempty"`
	DevPortalRefID   *string           `gorm:"column:devportal_ref_id" json:"devPortalRefId,omitempty"`

	// Gateway endpoints for sandbox and production
	SandboxEndpointURL    string `gorm:"column:sandbox_endpoint_url" json:"sandboxEndpointUrl"`
	ProductionEndpointURL string `gorm:"column:production_endpoint_url" json:"productionEndpointUrl"`

	// Timestamps
	CreatedAt time.Time `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updatedAt"`
}

// TableName returns the table name for the PublicationMapping model
func (PublicationMapping) TableName() string {
	return "publication_mappings"
}

// Validate validates the PublicationMapping model
func (pm *PublicationMapping) Validate() error {
	if pm.APIUUID == uuid.Nil {
		return fmt.Errorf("API UUID is required")
	}

	if pm.DevPortalUUID == uuid.Nil {
		return fmt.Errorf("DevPortal UUID is required")
	}

	if pm.OrganizationUUID == uuid.Nil {
		return fmt.Errorf("Organization UUID is required")
	}

	if pm.SandboxEndpointURL == "" {
		return fmt.Errorf("Sandbox endpoint URL is required")
	}

	if pm.ProductionEndpointURL == "" {
		return fmt.Errorf("Production endpoint URL is required")
	}

	return nil
}

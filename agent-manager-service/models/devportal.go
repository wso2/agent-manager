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

// DevPortal represents a developer portal associated with an organization
type DevPortal struct {
	UUID             uuid.UUID `gorm:"column:uuid;primaryKey" json:"uuid"`
	OrganizationUUID uuid.UUID `gorm:"column:organization_uuid" json:"organizationUuid"`
	Name             string    `gorm:"column:name" json:"name"`
	Identifier       string    `gorm:"column:identifier" json:"identifier"`
	APIUrl           string    `gorm:"column:api_url" json:"apiUrl"`
	Hostname         string    `gorm:"column:hostname" json:"hostname"`
	IsActive         bool      `gorm:"column:is_active" json:"isActive"`
	IsEnabled        bool      `gorm:"column:is_enabled" json:"isEnabled"`
	APIKey           string    `gorm:"column:api_key" json:"apiKey"`
	HeaderKeyName    string    `gorm:"column:header_key_name" json:"headerKeyName"`
	IsDefault        bool      `gorm:"column:is_default" json:"isDefault"`
	Visibility       string    `gorm:"column:visibility" json:"visibility"`
	Description      string    `gorm:"column:description" json:"description"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt        time.Time `gorm:"column:updated_at" json:"updatedAt"`
}

// TableName returns the table name for the DevPortal model
func (DevPortal) TableName() string {
	return "devportals"
}

// GetUIUrl returns the computed UI URL based on API URL and identifier
func (d *DevPortal) GetUIUrl() string {
	return fmt.Sprintf("%s/%s/views/default/apis", d.APIUrl, d.Identifier)
}

// Validate performs basic validation of DevPortal fields
func (d *DevPortal) Validate() error {
	if d.Name == "" {
		return fmt.Errorf("devportal name is required")
	}
	if d.Identifier == "" {
		return fmt.Errorf("devportal identifier is required")
	}
	if d.APIUrl == "" {
		return fmt.Errorf("devportal API URL is required")
	}
	if d.Hostname == "" {
		return fmt.Errorf("devportal hostname is required")
	}
	if d.APIKey == "" {
		return fmt.Errorf("devportal API key is required")
	}
	if d.HeaderKeyName == "" {
		return fmt.Errorf("devportal header key name is required")
	}
	if d.Visibility != "public" && d.Visibility != "private" {
		return fmt.Errorf("devportal visibility must be 'public' or 'private'")
	}
	return nil
}

// APIDevPortalWithDetails represents a DevPortal with its association and publication details for an API
type APIDevPortalWithDetails struct {
	// DevPortal information
	UUID             uuid.UUID `json:"uuid" db:"uuid"`
	OrganizationUUID uuid.UUID `json:"organizationUuid" db:"organization_uuid"`
	Name             string    `json:"name" db:"name"`
	Identifier       string    `json:"identifier" db:"identifier"`
	APIUrl           string    `json:"apiUrl" db:"api_url"`
	Hostname         string    `json:"hostname" db:"hostname"`
	IsActive         bool      `json:"isActive" db:"is_active"`
	IsEnabled        bool      `json:"isEnabled" db:"is_enabled"`
	IsDefault        bool      `json:"isDefault" db:"is_default"`
	Visibility       string    `json:"visibility" db:"visibility"`
	Description      string    `json:"description" db:"description"`
	CreatedAt        time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt        time.Time `json:"updatedAt" db:"updated_at"`

	// Association information (from association_mappings table)
	AssociatedAt         time.Time `json:"associatedAt" db:"associated_at"`
	AssociationUpdatedAt time.Time `json:"associationUpdatedAt" db:"association_updated_at"`

	// Publication information (nullable if not published - from publication_mappings table)
	IsPublished           bool       `json:"isPublished" db:"is_published"`
	PublicationStatus     *string    `json:"publicationStatus,omitempty" db:"publication_status"`
	APIVersion            *string    `json:"apiVersion,omitempty" db:"api_version"`
	DevPortalRefID        *string    `json:"devPortalRefId,omitempty" db:"devportal_ref_id"`
	SandboxEndpointURL    *string    `json:"sandboxEndpointUrl,omitempty" db:"sandbox_endpoint_url"`
	ProductionEndpointURL *string    `json:"productionEndpointUrl,omitempty" db:"production_endpoint_url"`
	PublishedAt           *time.Time `json:"publishedAt,omitempty" db:"published_at"`
	PublicationUpdatedAt  *time.Time `json:"publicationUpdatedAt,omitempty" db:"publication_updated_at"`
}

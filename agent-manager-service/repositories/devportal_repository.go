/*
 *  Copyright (c) 2025, WSO2 LLC. (http://www.wso2.org) All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package repositories

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"gorm.io/gorm"
)

// DevPortalRepository defines the interface for DevPortal-related database operations
type DevPortalRepository interface {
	// Basic CRUD operations
	Create(devPortal *models.DevPortal) error
	GetByUUID(uuid, orgUUID string) (*models.DevPortal, error)
	GetByOrganizationUUID(orgUUID string, isDefault, isActive *bool, limit, offset int) ([]*models.DevPortal, error)
	Update(devPortal *models.DevPortal, orgUUID string) error
	Delete(uuid, orgUUID string) error

	// Special operations
	GetDefaultByOrganizationUUID(orgUUID string) (*models.DevPortal, error)
	CountByOrganizationUUID(orgUUID string, isDefault, isActive *bool) (int, error)
	UpdateEnabledStatus(uuid, orgUUID string, isEnabled bool) error
	SetAsDefault(uuid, orgUUID string) error
}

// DevPortalRepo implements DevPortalRepository using GORM
type DevPortalRepo struct {
	db *gorm.DB
}

// NewDevPortalRepository creates a new instance of DevPortalRepository
func NewDevPortalRepository(db *gorm.DB) DevPortalRepository {
	return &DevPortalRepo{db: db}
}

// Create creates a new DevPortal in the database
func (r *DevPortalRepo) Create(devPortal *models.DevPortal) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Generate UUID if not provided
		if devPortal.UUID == uuid.Nil {
			devPortal.UUID = uuid.New()
		}

		// Set timestamps
		now := time.Now()
		devPortal.CreatedAt = now
		devPortal.UpdatedAt = now

		// Check for conflicts within the transaction
		if err := r.checkForConflictsTx(tx, devPortal); err != nil {
			return err
		}

		// Create the devportal
		return tx.Create(devPortal).Error
	})
}

// checkForConflictsTx checks for conflicts within a transaction
func (r *DevPortalRepo) checkForConflictsTx(tx *gorm.DB, devPortal *models.DevPortal) error {
	// Check for existing DevPortal with same API URL in the same organization
	var count int64
	if err := tx.Model(&models.DevPortal{}).
		Where("organization_uuid = ? AND api_url = ?", devPortal.OrganizationUUID, devPortal.APIUrl).
		Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check for existing API URL: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("devportal with API URL %s already exists in organization %s", devPortal.APIUrl, devPortal.OrganizationUUID)
	}

	// Check for existing DevPortal with same hostname in the same organization
	if err := tx.Model(&models.DevPortal{}).
		Where("organization_uuid = ? AND hostname = ?", devPortal.OrganizationUUID, devPortal.Hostname).
		Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check for existing hostname: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("devportal with hostname %s already exists in organization %s", devPortal.Hostname, devPortal.OrganizationUUID)
	}

	// Check for existing default DevPortal if this one is set as default
	if devPortal.IsDefault {
		if err := tx.Model(&models.DevPortal{}).
			Where("organization_uuid = ? AND is_default = ?", devPortal.OrganizationUUID, true).
			Count(&count).Error; err != nil {
			return fmt.Errorf("failed to check for existing default devportal: %w", err)
		}
		if count > 0 {
			return fmt.Errorf("default devportal already exists for organization %s", devPortal.OrganizationUUID)
		}
	}

	return nil
}

// checkForUpdateConflictsTx checks for update conflicts within a transaction
func (r *DevPortalRepo) checkForUpdateConflictsTx(tx *gorm.DB, devPortal *models.DevPortal, orgUUID string) error {
	// Check for existing DevPortal with same API URL in the same organization (excluding this one)
	var count int64
	if err := tx.Model(&models.DevPortal{}).
		Where("organization_uuid = ? AND api_url = ? AND uuid != ?", orgUUID, devPortal.APIUrl, devPortal.UUID).
		Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check for existing API URL during update: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("devportal with API URL %s already exists in organization %s", devPortal.APIUrl, orgUUID)
	}

	// Check for existing DevPortal with same hostname in the same organization (excluding this one)
	if err := tx.Model(&models.DevPortal{}).
		Where("organization_uuid = ? AND hostname = ? AND uuid != ?", orgUUID, devPortal.Hostname, devPortal.UUID).
		Count(&count).Error; err != nil {
		return fmt.Errorf("failed to check for existing hostname during update: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("devportal with hostname %s already exists in organization %s", devPortal.Hostname, orgUUID)
	}

	return nil
}

// GetByUUID retrieves a DevPortal by its UUID
func (r *DevPortalRepo) GetByUUID(uuid, orgUUID string) (*models.DevPortal, error) {
	var devPortal models.DevPortal
	err := r.db.Where("uuid = ? AND organization_uuid = ?", uuid, orgUUID).First(&devPortal).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("devportal with UUID %s not found in organization %s", uuid, orgUUID)
		}
		return nil, fmt.Errorf("failed to get devportal with UUID %s for organization %s: %w", uuid, orgUUID, err)
	}
	return &devPortal, nil
}

// GetByOrganizationUUID retrieves DevPortals for an organization with optional filters
func (r *DevPortalRepo) GetByOrganizationUUID(orgUUID string, isDefault, isActive *bool, limit, offset int) ([]*models.DevPortal, error) {
	var devPortals []*models.DevPortal
	query := r.db.Where("organization_uuid = ?", orgUUID)

	// Add filters if provided
	if isDefault != nil {
		query = query.Where("is_default = ?", *isDefault)
	}
	if isActive != nil {
		query = query.Where("is_active = ?", *isActive)
	}

	err := query.Order("is_default DESC, created_at ASC").
		Limit(limit).
		Offset(offset).
		Find(&devPortals).Error

	return devPortals, err
}

// Update updates an existing DevPortal
func (r *DevPortalRepo) Update(devPortal *models.DevPortal, orgUUID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Update timestamp
		devPortal.UpdatedAt = time.Now()

		// Check for conflicts within the transaction
		if err := r.checkForUpdateConflictsTx(tx, devPortal, orgUUID); err != nil {
			return err
		}

		result := tx.Model(&models.DevPortal{}).
			Where("uuid = ? AND organization_uuid = ?", devPortal.UUID, orgUUID).
			Updates(map[string]interface{}{
				"name":            devPortal.Name,
				"api_url":         devPortal.APIUrl,
				"hostname":        devPortal.Hostname,
				"api_key":         devPortal.APIKey,
				"header_key_name": devPortal.HeaderKeyName,
				"is_active":       devPortal.IsActive,
				"is_enabled":      devPortal.IsEnabled,
				"visibility":      devPortal.Visibility,
				"description":     devPortal.Description,
				"updated_at":      devPortal.UpdatedAt,
			})

		if result.Error != nil {
			return fmt.Errorf("failed to update devportal %s in organization %s: %w", devPortal.UUID, orgUUID, result.Error)
		}

		if result.RowsAffected == 0 {
			return fmt.Errorf("devportal with UUID %s not found in organization %s", devPortal.UUID, orgUUID)
		}

		return nil
	})
}

// Delete deletes a DevPortal by UUID
func (r *DevPortalRepo) Delete(uuid, orgUUID string) error {
	result := r.db.Where("uuid = ? AND organization_uuid = ?", uuid, orgUUID).Delete(&models.DevPortal{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete devportal %s from organization %s: %w", uuid, orgUUID, result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("devportal with UUID %s not found in organization %s", uuid, orgUUID)
	}

	return nil
}

// GetDefaultByOrganizationUUID retrieves the default DevPortal for an organization
func (r *DevPortalRepo) GetDefaultByOrganizationUUID(orgUUID string) (*models.DevPortal, error) {
	var devPortal models.DevPortal
	err := r.db.Where("organization_uuid = ? AND is_default = ?", orgUUID, true).First(&devPortal).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("no default devportal found for organization %s", orgUUID)
		}
		return nil, fmt.Errorf("failed to get default devportal for organization %s: %w", orgUUID, err)
	}
	return &devPortal, nil
}

// CountByOrganizationUUID counts DevPortals for an organization with optional filters
func (r *DevPortalRepo) CountByOrganizationUUID(orgUUID string, isDefault, isActive *bool) (int, error) {
	var count int64
	query := r.db.Model(&models.DevPortal{}).Where("organization_uuid = ?", orgUUID)

	// Add filters if provided
	if isDefault != nil {
		query = query.Where("is_default = ?", *isDefault)
	}
	if isActive != nil {
		query = query.Where("is_active = ?", *isActive)
	}

	err := query.Count(&count).Error
	return int(count), err
}

// UpdateEnabledStatus updates the enabled status of a DevPortal
func (r *DevPortalRepo) UpdateEnabledStatus(uuid, orgUUID string, isEnabled bool) error {
	result := r.db.Model(&models.DevPortal{}).
		Where("uuid = ? AND organization_uuid = ?", uuid, orgUUID).
		Updates(map[string]interface{}{
			"is_enabled": isEnabled,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update enabled status for devportal %s in organization %s: %w", uuid, orgUUID, result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("devportal with UUID %s not found in organization %s", uuid, orgUUID)
	}

	return nil
}

// SetAsDefault sets a DevPortal as the default for its organization
func (r *DevPortalRepo) SetAsDefault(uuid, orgUUID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Get the DevPortal to verify it exists
		var devPortal models.DevPortal
		if err := tx.Where("uuid = ? AND organization_uuid = ?", uuid, orgUUID).First(&devPortal).Error; err != nil {
			return err
		}

		// Unset previous default
		if err := tx.Model(&models.DevPortal{}).
			Where("organization_uuid = ? AND is_default = ?", devPortal.OrganizationUUID, true).
			Update("is_default", false).Error; err != nil {
			return fmt.Errorf("failed to unset previous default devportal for organization %s: %w", devPortal.OrganizationUUID, err)
		}

		// Set the new default
		result := tx.Model(&models.DevPortal{}).
			Where("uuid = ? AND organization_uuid = ?", uuid, orgUUID).
			Updates(map[string]interface{}{
				"is_default": true,
				"updated_at": time.Now(),
			})

		if result.Error != nil {
			return fmt.Errorf("failed to set devportal %s as default for organization %s: %w", uuid, orgUUID, result.Error)
		}

		if result.RowsAffected == 0 {
			return fmt.Errorf("devportal with UUID %s not found in organization %s", uuid, orgUUID)
		}

		return nil
	})
}

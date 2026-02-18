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

package repositories

import (
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// CatalogRepository defines the interface for catalog data access
type CatalogRepository interface {
	// ListByKind lists catalog entries filtered by kind with pagination
	ListByKind(orgUUID, kind string, limit, offset int) ([]models.CatalogEntry, int64, error)
	// ListAll lists all catalog entries with pagination
	ListAll(orgUUID string, limit, offset int) ([]models.CatalogEntry, int64, error)
}

// CatalogRepo implements CatalogRepository using GORM
type CatalogRepo struct {
	db *gorm.DB
}

// NewCatalogRepo creates a new catalog repository
func NewCatalogRepo(db *gorm.DB) CatalogRepository {
	return &CatalogRepo{db: db}
}

// ListByKind lists catalog entries filtered by kind with pagination
func (r *CatalogRepo) ListByKind(orgUUID, kind string, limit, offset int) ([]models.CatalogEntry, int64, error) {
	var entries []models.CatalogEntry
	var total int64

	// Count total matching records
	if err := r.db.Model(&models.CatalogEntry{}).
		Where("organization_uuid = ? AND kind = ? AND in_catalog = ?", orgUUID, kind, true).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Retrieve paginated results
	if err := r.db.
		Where("organization_uuid = ? AND kind = ? AND in_catalog = ?", orgUUID, kind, true).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&entries).Error; err != nil {
		return nil, 0, err
	}

	return entries, total, nil
}

// ListAll lists all catalog entries with pagination
func (r *CatalogRepo) ListAll(orgUUID string, limit, offset int) ([]models.CatalogEntry, int64, error) {
	var entries []models.CatalogEntry
	var total int64

	// Count total matching records
	if err := r.db.Model(&models.CatalogEntry{}).
		Where("organization_uuid = ? AND in_catalog = ?", orgUUID, true).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Retrieve paginated results
	if err := r.db.
		Where("organization_uuid = ? AND in_catalog = ?", orgUUID, true).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&entries).Error; err != nil {
		return nil, 0, err
	}

	return entries, total, nil
}

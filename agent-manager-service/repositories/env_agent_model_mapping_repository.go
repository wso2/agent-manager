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
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// EnvAgentModelMappingRepository defines data access for environment mappings
type EnvAgentModelMappingRepository interface {
	// Create creates a new environment mapping (use within transaction)
	Create(ctx context.Context, tx *gorm.DB, mapping *models.EnvAgentModelMapping) error

	// GetByConfigAndEnv retrieves mapping by config UUID and environment
	GetByConfigAndEnv(ctx context.Context, configUUID, envUUID uuid.UUID) (*models.EnvAgentModelMapping, error)

	// ListByConfig retrieves all mappings for a configuration
	ListByConfig(ctx context.Context, configUUID uuid.UUID) ([]models.EnvAgentModelMapping, error)

	// Update updates an existing mapping (use within transaction)
	Update(ctx context.Context, tx *gorm.DB, mapping *models.EnvAgentModelMapping) error

	// Delete deletes a mapping by ID (use within transaction)
	Delete(ctx context.Context, tx *gorm.DB, mappingID uint) error

	// DeleteByConfig deletes all mappings for a configuration (use within transaction)
	DeleteByConfig(ctx context.Context, tx *gorm.DB, configUUID uuid.UUID) error
}

type envAgentModelMappingRepository struct {
	db *gorm.DB
}

// NewEnvAgentModelMappingRepository creates a new repository
func NewEnvAgentModelMappingRepository(db *gorm.DB) EnvAgentModelMappingRepository {
	return &envAgentModelMappingRepository{db: db}
}

func (r *envAgentModelMappingRepository) Create(ctx context.Context, tx *gorm.DB, mapping *models.EnvAgentModelMapping) error {
	return tx.WithContext(ctx).Create(mapping).Error
}

func (r *envAgentModelMappingRepository) GetByConfigAndEnv(ctx context.Context, configUUID, envUUID uuid.UUID) (*models.EnvAgentModelMapping, error) {
	var mapping models.EnvAgentModelMapping
	err := r.db.WithContext(ctx).
		Preload("LLMProxy").
		Where("config_uuid = ? AND environment_uuid = ?", configUUID, envUUID).
		First(&mapping).Error
	return &mapping, err
}

func (r *envAgentModelMappingRepository) ListByConfig(ctx context.Context, configUUID uuid.UUID) ([]models.EnvAgentModelMapping, error) {
	var mappings []models.EnvAgentModelMapping
	err := r.db.WithContext(ctx).
		Preload("LLMProxy").
		Where("config_uuid = ?", configUUID).
		Find(&mappings).Error
	return mappings, err
}

func (r *envAgentModelMappingRepository) Update(ctx context.Context, tx *gorm.DB, mapping *models.EnvAgentModelMapping) error {
	return tx.WithContext(ctx).Save(mapping).Error
}

func (r *envAgentModelMappingRepository) Delete(ctx context.Context, tx *gorm.DB, mappingID uint) error {
	return tx.WithContext(ctx).Delete(&models.EnvAgentModelMapping{}, mappingID).Error
}

func (r *envAgentModelMappingRepository) DeleteByConfig(ctx context.Context, tx *gorm.DB, configUUID uuid.UUID) error {
	return tx.WithContext(ctx).
		Where("config_uuid = ?", configUUID).
		Delete(&models.EnvAgentModelMapping{}).Error
}

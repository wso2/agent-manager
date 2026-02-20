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

// AgentConfigurationRepository defines data access for agent configurations
type AgentConfigurationRepository interface {
	// Create creates a new agent configuration (use within transaction)
	Create(ctx context.Context, tx *gorm.DB, config *models.AgentConfiguration) error

	// GetByUUID retrieves configuration by UUID
	GetByUUID(ctx context.Context, configUUID uuid.UUID, orgName string) (*models.AgentConfiguration, error)

	// GetByAgentID retrieves configuration by agent ID
	GetByAgentID(ctx context.Context, agentID, orgName string) (*models.AgentConfiguration, error)

	// List retrieves configurations with pagination
	List(ctx context.Context, orgName string, limit, offset int) ([]models.AgentConfiguration, error)

	// Count counts total configurations
	Count(ctx context.Context, orgName string) (int64, error)

	// Update updates an existing configuration (use within transaction)
	Update(ctx context.Context, tx *gorm.DB, config *models.AgentConfiguration) error

	// Delete deletes a configuration by UUID (use within transaction)
	Delete(ctx context.Context, tx *gorm.DB, configUUID uuid.UUID, orgName string) error

	// Exists checks if configuration exists
	Exists(ctx context.Context, configUUID uuid.UUID, orgName string) (bool, error)
}

type agentConfigurationRepository struct {
	db *gorm.DB
}

// NewAgentConfigurationRepository creates a new repository
func NewAgentConfigurationRepository(db *gorm.DB) AgentConfigurationRepository {
	return &agentConfigurationRepository{db: db}
}

func (r *agentConfigurationRepository) Create(ctx context.Context, tx *gorm.DB, config *models.AgentConfiguration) error {
	return tx.WithContext(ctx).Create(config).Error
}

func (r *agentConfigurationRepository) GetByUUID(ctx context.Context, configUUID uuid.UUID, orgName string) (*models.AgentConfiguration, error) {
	var config models.AgentConfiguration
	err := r.db.WithContext(ctx).
		Preload("EnvMappings").
		Preload("EnvMappings.LLMProxy").
		Preload("EnvVariables").
		Where("uuid = ? AND organization_name = ?", configUUID, orgName).
		First(&config).Error
	return &config, err
}

func (r *agentConfigurationRepository) GetByAgentID(ctx context.Context, agentID, orgName string) (*models.AgentConfiguration, error) {
	var config models.AgentConfiguration
	err := r.db.WithContext(ctx).
		Preload("EnvMappings").
		Preload("EnvMappings.LLMProxy").
		Preload("EnvVariables").
		Where("agent_id = ? AND organization_name = ?", agentID, orgName).
		First(&config).Error
	return &config, err
}

func (r *agentConfigurationRepository) List(ctx context.Context, orgName string, limit, offset int) ([]models.AgentConfiguration, error) {
	var configs []models.AgentConfiguration
	err := r.db.WithContext(ctx).
		Where("organization_name = ?", orgName).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&configs).Error
	return configs, err
}

func (r *agentConfigurationRepository) Count(ctx context.Context, orgName string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.AgentConfiguration{}).
		Where("organization_name = ?", orgName).
		Count(&count).Error
	return count, err
}

func (r *agentConfigurationRepository) Update(ctx context.Context, tx *gorm.DB, config *models.AgentConfiguration) error {
	return tx.WithContext(ctx).Save(config).Error
}

func (r *agentConfigurationRepository) Delete(ctx context.Context, tx *gorm.DB, configUUID uuid.UUID, orgName string) error {
	return tx.WithContext(ctx).
		Where("uuid = ? AND organization_name = ?", configUUID, orgName).
		Delete(&models.AgentConfiguration{}).Error
}

func (r *agentConfigurationRepository) Exists(ctx context.Context, configUUID uuid.UUID, orgName string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.AgentConfiguration{}).
		Where("uuid = ? AND organization_name = ?", configUUID, orgName).
		Count(&count).Error
	return count > 0, err
}

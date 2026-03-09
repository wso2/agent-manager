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
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// AgentEnvConfigVariableRepository defines data access for environment variables
type AgentEnvConfigVariableRepository interface {
	// CreateBatch creates multiple variables (use within transaction)
	CreateBatch(ctx context.Context, tx *gorm.DB, variables []models.AgentEnvConfigVariable) error

	// ListByConfigAndEnv retrieves variables for a config and environment
	ListByConfigAndEnv(ctx context.Context, configUUID, envUUID uuid.UUID) ([]models.AgentEnvConfigVariable, error)

	// ListByConfig retrieves all variables for a config across all environments
	ListByConfig(ctx context.Context, configUUID uuid.UUID) ([]models.AgentEnvConfigVariable, error)

	// UpdateVariableNames updates variable_name for matching variable_key entries across all envs (use within transaction)
	UpdateVariableNames(ctx context.Context, tx *gorm.DB, configUUID uuid.UUID, keyNameMap map[string]string) error

	// DeleteByConfig deletes all variables for a configuration (use within transaction)
	DeleteByConfig(ctx context.Context, tx *gorm.DB, configUUID uuid.UUID) error

	// DeleteByConfigAndEnv deletes variables for config and environment (use within transaction)
	DeleteByConfigAndEnv(ctx context.Context, tx *gorm.DB, configUUID, envUUID uuid.UUID) error
}

type agentEnvConfigVariableRepository struct {
	db *gorm.DB
}

// NewAgentEnvConfigVariableRepository creates a new repository
func NewAgentEnvConfigVariableRepository(db *gorm.DB) AgentEnvConfigVariableRepository {
	return &agentEnvConfigVariableRepository{db: db}
}

func (r *agentEnvConfigVariableRepository) CreateBatch(ctx context.Context, tx *gorm.DB, variables []models.AgentEnvConfigVariable) error {
	if len(variables) == 0 {
		return nil
	}
	return tx.WithContext(ctx).Create(&variables).Error
}

func (r *agentEnvConfigVariableRepository) ListByConfigAndEnv(ctx context.Context, configUUID, envUUID uuid.UUID) ([]models.AgentEnvConfigVariable, error) {
	var variables []models.AgentEnvConfigVariable
	err := r.db.WithContext(ctx).
		Where("config_uuid = ? AND environment_uuid = ?", configUUID, envUUID).
		Find(&variables).Error
	return variables, err
}

func (r *agentEnvConfigVariableRepository) ListByConfig(ctx context.Context, configUUID uuid.UUID) ([]models.AgentEnvConfigVariable, error) {
	var variables []models.AgentEnvConfigVariable
	err := r.db.WithContext(ctx).
		Where("config_uuid = ?", configUUID).
		Find(&variables).Error
	return variables, err
}

func (r *agentEnvConfigVariableRepository) UpdateVariableNames(ctx context.Context, tx *gorm.DB, configUUID uuid.UUID, keyNameMap map[string]string) error {
	for key, name := range keyNameMap {
		result := tx.WithContext(ctx).
			Model(&models.AgentEnvConfigVariable{}).
			Where("config_uuid = ? AND variable_key = ?", configUUID, key).
			Update("variable_name", name)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("no rows affected for key %q: unknown environment variable key", key)
		}
	}
	return nil
}

func (r *agentEnvConfigVariableRepository) DeleteByConfig(ctx context.Context, tx *gorm.DB, configUUID uuid.UUID) error {
	return tx.WithContext(ctx).
		Where("config_uuid = ?", configUUID).
		Delete(&models.AgentEnvConfigVariable{}).Error
}

func (r *agentEnvConfigVariableRepository) DeleteByConfigAndEnv(ctx context.Context, tx *gorm.DB, configUUID, envUUID uuid.UUID) error {
	return tx.WithContext(ctx).
		Where("config_uuid = ? AND environment_uuid = ?", configUUID, envUUID).
		Delete(&models.AgentEnvConfigVariable{}).Error
}

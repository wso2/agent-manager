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
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// LLMProviderGatewayMappingRepository handles database operations for LLM provider-gateway mappings
type LLMProviderGatewayMappingRepository interface {
	// Create creates a new mapping
	Create(mapping *models.LLMProviderGatewayMapping) error

	// CreateBatch creates multiple mappings in a single transaction
	CreateBatch(mappings []*models.LLMProviderGatewayMapping) error

	// Delete deletes a specific mapping
	Delete(providerUUID, gatewayUUID uuid.UUID) error

	// DeleteByProvider deletes all mappings for a provider
	DeleteByProvider(providerUUID uuid.UUID) error

	// DeleteByGateway deletes all mappings for a gateway
	DeleteByGateway(gatewayUUID uuid.UUID) error

	// GetByProvider gets all gateway UUIDs for a provider
	GetByProvider(providerUUID uuid.UUID) ([]string, error)

	// GetByGateway gets all provider UUIDs for a gateway
	GetByGateway(gatewayUUID uuid.UUID) ([]string, error)

	// Exists checks if a mapping exists
	Exists(providerUUID, gatewayUUID uuid.UUID) (bool, error)

	// ReplaceForProvider replaces all mappings for a provider with new ones
	ReplaceForProvider(providerUUID uuid.UUID, gatewayUUIDs []uuid.UUID) error
}

type llmProviderGatewayMappingRepository struct {
	db *gorm.DB
}

// NewLLMProviderGatewayMappingRepository creates a new LLM provider-gateway mapping repository
func NewLLMProviderGatewayMappingRepository(db *gorm.DB) LLMProviderGatewayMappingRepository {
	return &llmProviderGatewayMappingRepository{
		db: db,
	}
}

func (r *llmProviderGatewayMappingRepository) Create(mapping *models.LLMProviderGatewayMapping) error {
	return r.db.Create(mapping).Error
}

func (r *llmProviderGatewayMappingRepository) CreateBatch(mappings []*models.LLMProviderGatewayMapping) error {
	if len(mappings) == 0 {
		return nil
	}
	return r.db.Create(mappings).Error
}

func (r *llmProviderGatewayMappingRepository) Delete(providerUUID, gatewayUUID uuid.UUID) error {
	return r.db.Where("llm_provider_uuid = ? AND gateway_uuid = ?", providerUUID, gatewayUUID).
		Delete(&models.LLMProviderGatewayMapping{}).Error
}

func (r *llmProviderGatewayMappingRepository) DeleteByProvider(providerUUID uuid.UUID) error {
	return r.db.Where("llm_provider_uuid = ?", providerUUID).
		Delete(&models.LLMProviderGatewayMapping{}).Error
}

func (r *llmProviderGatewayMappingRepository) DeleteByGateway(gatewayUUID uuid.UUID) error {
	return r.db.Where("gateway_uuid = ?", gatewayUUID).
		Delete(&models.LLMProviderGatewayMapping{}).Error
}

func (r *llmProviderGatewayMappingRepository) GetByProvider(providerUUID uuid.UUID) ([]string, error) {
	var mappings []models.LLMProviderGatewayMapping
	if err := r.db.Where("llm_provider_uuid = ?", providerUUID).
		Find(&mappings).Error; err != nil {
		return nil, err
	}

	gatewayUUIDs := make([]string, len(mappings))
	for i, mapping := range mappings {
		gatewayUUIDs[i] = mapping.GatewayUUID
	}
	return gatewayUUIDs, nil
}

func (r *llmProviderGatewayMappingRepository) GetByGateway(gatewayUUID uuid.UUID) ([]string, error) {
	var mappings []models.LLMProviderGatewayMapping
	if err := r.db.Where("gateway_uuid = ?", gatewayUUID).
		Find(&mappings).Error; err != nil {
		return nil, err
	}

	providerUUIDs := make([]string, len(mappings))
	for i, mapping := range mappings {
		providerUUIDs[i] = mapping.LLMProviderUUID
	}
	return providerUUIDs, nil
}

func (r *llmProviderGatewayMappingRepository) Exists(providerUUID, gatewayUUID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.Model(&models.LLMProviderGatewayMapping{}).
		Where("llm_provider_uuid = ? AND gateway_uuid = ?", providerUUID, gatewayUUID).
		Count(&count).Error
	return count > 0, err
}

func (r *llmProviderGatewayMappingRepository) ReplaceForProvider(providerUUID uuid.UUID, gatewayUUIDs []uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Delete all existing mappings for this provider
		if err := tx.Where("llm_provider_uuid = ?", providerUUID).
			Delete(&models.LLMProviderGatewayMapping{}).Error; err != nil {
			return err
		}

		// Create new mappings
		if len(gatewayUUIDs) > 0 {
			mappings := make([]*models.LLMProviderGatewayMapping, len(gatewayUUIDs))
			for i, gatewayUUID := range gatewayUUIDs {
				mappings[i] = &models.LLMProviderGatewayMapping{
					LLMProviderUUID: providerUUID.String(),
					GatewayUUID:     gatewayUUID.String(),
				}
			}
			if err := tx.Create(mappings).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

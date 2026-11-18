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

package gwrepo

import (
	"context"
	"errors"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// AgentConsumer holds the proxy and agent names needed to build a consumer list item.
type AgentConsumer struct {
	ProxyHandle string
	ProxyName   string
	ProjectName string
	AgentID     string
}

// EnvAgentModelMappingRepository defines data access for environment mappings
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg gwrepomocks -out gwrepomocks/env_agent_model_mapping_repository_mock.go . EnvAgentModelMappingRepository:EnvAgentModelMappingRepositoryMock
type EnvAgentModelMappingRepository interface {
	// Create creates a new environment mapping (use within transaction)
	Create(ctx context.Context, tx *gorm.DB, mapping *models.EnvAgentModelMapping) error

	// GetByConfigAndEnv retrieves mapping by config UUID and environment
	GetByConfigAndEnv(ctx context.Context, configUUID, envUUID uuid.UUID) (*models.EnvAgentModelMapping, error)

	// ListByConfig retrieves all mappings for a configuration
	ListByConfig(ctx context.Context, configUUID uuid.UUID) ([]models.EnvAgentModelMapping, error)

	// ListAgentConsumersByProxyUUIDs returns distinct agent consumers for the given proxy UUIDs.
	ListAgentConsumersByProxyUUIDs(ctx context.Context, proxyUUIDs []uuid.UUID) ([]AgentConsumer, error)

	// Update updates an existing mapping (use within transaction)
	Update(ctx context.Context, tx *gorm.DB, mapping *models.EnvAgentModelMapping) error

	// Delete deletes a mapping by ID (use within transaction)
	Delete(ctx context.Context, tx *gorm.DB, configUUID, envUUID uuid.UUID) error

	// DeleteByConfig deletes all mappings for a configuration (use within transaction)
	DeleteByConfig(ctx context.Context, tx *gorm.DB, configUUID uuid.UUID) error
}

type envAgentModelMappingRepository struct {
	db   *gorm.DB
	core modkit.CoreAPI
}

// NewEnvAgentModelMappingRepository creates a new repository
func NewEnvAgentModelMappingRepository(db *gorm.DB) EnvAgentModelMappingRepository {
	return &envAgentModelMappingRepository{db: db, core: repositories.NewCoreReadAPI(db)}
}

func (r *envAgentModelMappingRepository) Create(ctx context.Context, tx *gorm.DB, mapping *models.EnvAgentModelMapping) error {
	return tx.WithContext(ctx).Create(mapping).Error
}

func (r *envAgentModelMappingRepository) GetByConfigAndEnv(ctx context.Context, configUUID, envUUID uuid.UUID) (*models.EnvAgentModelMapping, error) {
	var mapping models.EnvAgentModelMapping
	err := r.db.WithContext(ctx).
		Preload("LLMProxy").
		Where("config_id = ? AND environment_id = ?", configUUID, envUUID).
		First(&mapping).Error
	if err == nil {
		backfillLLMProxyHandlesInMappings([]models.EnvAgentModelMapping{mapping})
	}
	return &mapping, err
}

func (r *envAgentModelMappingRepository) ListByConfig(ctx context.Context, configUUID uuid.UUID) ([]models.EnvAgentModelMapping, error) {
	var mappings []models.EnvAgentModelMapping
	err := r.db.WithContext(ctx).
		Preload("LLMProxy").
		Where("config_id = ?", configUUID).
		Find(&mappings).Error
	if err == nil {
		backfillLLMProxyHandlesInMappings(mappings)
	}
	return mappings, err
}

func (r *envAgentModelMappingRepository) Update(ctx context.Context, tx *gorm.DB, mapping *models.EnvAgentModelMapping) error {
	return tx.WithContext(ctx).Save(mapping).Error
}

func (r *envAgentModelMappingRepository) Delete(ctx context.Context, tx *gorm.DB, configUUID, envUUID uuid.UUID) error {
	return tx.WithContext(ctx).Where("config_id = ? AND environment_id = ?", configUUID, envUUID).
		Delete(&models.EnvAgentModelMapping{}).Error
}

func (r *envAgentModelMappingRepository) DeleteByConfig(ctx context.Context, tx *gorm.DB, configUUID uuid.UUID) error {
	return tx.WithContext(ctx).
		Where("config_id = ?", configUUID).
		Delete(&models.EnvAgentModelMapping{}).Error
}

func (r *envAgentModelMappingRepository) ListAgentConsumersByProxyUUIDs(ctx context.Context, proxyUUIDs []uuid.UUID) ([]AgentConsumer, error) {
	if len(proxyUUIDs) == 0 {
		return nil, nil
	}
	// A single agent gets a distinct LLM proxy artifact per environment, so grouping by
	// agent — rather than including the per-env proxy handle/name in the projection —
	// collapses those per-env rows into one consumer entry per agent. The agent's and its
	// project's names come from the core.
	var rows []struct {
		ProxyHandle string
		ProxyName   string
		OrgID       string
		AgentUUID   string
	}
	err := r.db.WithContext(ctx).
		Table("mod_ai_gateway_agent_model_config_llm eam").
		Select("MIN(a.handle) AS proxy_handle, MIN(a.name) AS proxy_name, ac.org_id AS org_id, ac.agent_id AS agent_uuid").
		Joins("JOIN mod_ai_gateway_artifacts a ON a.id = eam.llm_proxy_id").
		Joins("JOIN mod_ai_gateway_agent_model_configs ac ON ac.id = eam.config_id").
		Where("eam.llm_proxy_id IN ?", proxyUUIDs).
		Group("ac.org_id, ac.agent_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	results := make([]AgentConsumer, 0, len(rows))
	for _, row := range rows {
		agent, err := r.core.GetAgent(ctx, row.OrgID, row.AgentUUID)
		if errors.Is(err, modkit.ErrNotFound) {
			continue // deleted meanwhile; its rows go with its finalize hook
		}
		if err != nil {
			return nil, err
		}
		results = append(results, AgentConsumer{
			ProxyHandle: row.ProxyHandle, ProxyName: row.ProxyName,
			ProjectName: agent.ProjectName, AgentID: agent.Name,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].ProjectName != results[j].ProjectName {
			return results[i].ProjectName < results[j].ProjectName
		}
		return results[i].AgentID < results[j].AgentID
	})
	return results, nil
}

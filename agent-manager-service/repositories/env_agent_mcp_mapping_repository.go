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
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// EnvAgentMCPMappingRepository defines data access for per-environment MCP mappings.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/env_agent_mcp_mapping_repository_mock.go . EnvAgentMCPMappingRepository:EnvAgentMCPMappingRepositoryMock
type EnvAgentMCPMappingRepository interface {
	Create(ctx context.Context, tx *gorm.DB, mapping *models.EnvAgentMCPMapping, proxyMapping *models.MCPProxyMapping, handle, name, version, ouID string) error
	ListByConfig(ctx context.Context, configUUID uuid.UUID) ([]models.EnvAgentMCPMapping, error)
	// ListByMCPProxy returns every mapping that derives from the given source proxy.
	// Used by the redeploy-on-update cascade so agent-scoped artifacts pick up new
	// upstream/auth/policies on their respective gateways.
	ListByMCPProxy(ctx context.Context, mcpProxyUUID uuid.UUID) ([]models.EnvAgentMCPMapping, error)
	// ListByEnvironment returns every mapping deployed into the given environment,
	// across all agent configs. Used by the environment-deletion cascade to tear down
	// all agent-scoped MCP artifacts for a disappearing environment.
	ListByEnvironment(ctx context.Context, envUUID uuid.UUID) ([]models.EnvAgentMCPMapping, error)
	// ListAgentMCPServersByOu returns every MCP proxy each agent in the organization is
	// configured with, per environment.
	ListAgentMCPServersByOu(ctx context.Context, ouID string) ([]AgentEnvironmentResource, error)
	Update(ctx context.Context, tx *gorm.DB, mapping *models.EnvAgentMCPMapping) error
	Delete(ctx context.Context, tx *gorm.DB, mappingID uint) error
	DeleteByConfig(ctx context.Context, tx *gorm.DB, configUUID uuid.UUID) error
}

type envAgentMCPMappingRepository struct {
	db           *gorm.DB
	artifactRepo ArtifactRepository
}

// NewEnvAgentMCPMappingRepository creates a new repository.
func NewEnvAgentMCPMappingRepository(db *gorm.DB) EnvAgentMCPMappingRepository {
	return &envAgentMCPMappingRepository{
		db:           db,
		artifactRepo: NewArtifactRepo(db),
	}
}

func (r *envAgentMCPMappingRepository) Create(ctx context.Context, tx *gorm.DB, mapping *models.EnvAgentMCPMapping, proxyMapping *models.MCPProxyMapping, handle, name, version, ouID string) error {
	if mapping.ArtifactUUID == uuid.Nil {
		mapping.ArtifactUUID = uuid.New()
	}
	if proxyMapping == nil {
		return fmt.Errorf("MCP proxy mapping is required")
	}
	if proxyMapping.UUID == uuid.Nil {
		proxyMapping.UUID = mapping.ArtifactUUID
	}
	if proxyMapping.UUID != mapping.ArtifactUUID {
		return fmt.Errorf("MCP proxy mapping UUID must match environment mapping artifact UUID")
	}
	if proxyMapping.SourceMCPProxyUUID == uuid.Nil {
		proxyMapping.SourceMCPProxyUUID = mapping.MCPProxyUUID
	}
	if proxyMapping.Status == "" {
		proxyMapping.Status = models.StatusCreated
	}
	now := time.Now()
	if err := r.artifactRepo.Create(tx, &models.Artifact{
		UUID:      mapping.ArtifactUUID,
		Handle:    handle,
		Name:      name,
		Version:   version,
		Kind:      models.KindMCPMapping,
		OUID:      ouID,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		return fmt.Errorf("failed to create MCP config artifact: %w", err)
	}
	if err := tx.WithContext(ctx).Create(proxyMapping).Error; err != nil {
		return fmt.Errorf("failed to create MCP proxy mapping: %w", err)
	}
	return tx.WithContext(ctx).Create(mapping).Error
}

func (r *envAgentMCPMappingRepository) ListByConfig(ctx context.Context, configUUID uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
	var mappings []models.EnvAgentMCPMapping
	err := r.db.WithContext(ctx).
		Preload("Artifact").
		Preload("MCPProxy").
		Preload("MCPProxy.Artifact").
		Preload("MCPProxy.Endpoints").
		Preload("MCPProxy.Endpoints.Environments").
		Preload("MCPProxyMapping").
		Preload("MCPProxyMapping.Artifact").
		Preload("MCPProxyMapping.SourceMCPProxy").
		Preload("MCPProxyMapping.SourceMCPProxy.Artifact").
		Where("config_uuid = ?", configUUID).
		Find(&mappings).Error
	return mappings, err
}

func (r *envAgentMCPMappingRepository) ListByMCPProxy(ctx context.Context, mcpProxyUUID uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
	var mappings []models.EnvAgentMCPMapping
	err := r.db.WithContext(ctx).
		Preload("Artifact").
		Preload("MCPProxy").
		Preload("MCPProxy.Artifact").
		Preload("MCPProxy.Endpoints").
		Preload("MCPProxy.Endpoints.Environments").
		Preload("MCPProxyMapping").
		Preload("MCPProxyMapping.Artifact").
		Preload("MCPProxyMapping.SourceMCPProxy").
		Preload("MCPProxyMapping.SourceMCPProxy.Artifact").
		Where("mcp_proxy_uuid = ?", mcpProxyUUID).
		Find(&mappings).Error
	return mappings, err
}

func (r *envAgentMCPMappingRepository) ListAgentMCPServersByOu(ctx context.Context, ouID string) ([]AgentEnvironmentResource, error) {
	var results []AgentEnvironmentResource
	err := r.db.WithContext(ctx).
		Table("env_agent_mcp_mapping m").
		Select("DISTINCT ac.project_name, ac.agent_id, m.environment_uuid, a.handle, a.name").
		Joins("JOIN agent_configurations ac ON ac.uuid = m.config_uuid").
		Joins("JOIN artifacts a ON a.uuid = m.mcp_proxy_uuid").
		Where("ac.ou_id = ?", ouID).
		Order("ac.project_name, ac.agent_id, a.name").
		Scan(&results).Error
	if err != nil {
		return nil, fmt.Errorf("list agent MCP servers: %w", err)
	}
	return results, nil
}

func (r *envAgentMCPMappingRepository) ListByEnvironment(ctx context.Context, envUUID uuid.UUID) ([]models.EnvAgentMCPMapping, error) {
	var mappings []models.EnvAgentMCPMapping
	err := r.db.WithContext(ctx).
		Preload("Artifact").
		Preload("MCPProxy").
		Preload("MCPProxy.Artifact").
		Preload("MCPProxy.Endpoints").
		Preload("MCPProxy.Endpoints.Environments").
		Preload("MCPProxyMapping").
		Preload("MCPProxyMapping.Artifact").
		Preload("MCPProxyMapping.SourceMCPProxy").
		Preload("MCPProxyMapping.SourceMCPProxy.Artifact").
		Where("environment_uuid = ?", envUUID).
		Find(&mappings).Error
	return mappings, err
}

func (r *envAgentMCPMappingRepository) Update(ctx context.Context, tx *gorm.DB, mapping *models.EnvAgentMCPMapping) error {
	return tx.WithContext(ctx).Save(mapping).Error
}

func (r *envAgentMCPMappingRepository) Delete(ctx context.Context, tx *gorm.DB, mappingID uint) error {
	return tx.WithContext(ctx).Delete(&models.EnvAgentMCPMapping{}, mappingID).Error
}

func (r *envAgentMCPMappingRepository) DeleteByConfig(ctx context.Context, tx *gorm.DB, configUUID uuid.UUID) error {
	return tx.WithContext(ctx).
		Where("config_uuid = ?", configUUID).
		Delete(&models.EnvAgentMCPMapping{}).Error
}

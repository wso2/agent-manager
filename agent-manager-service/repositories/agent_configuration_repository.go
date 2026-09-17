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
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// AgentConfigurationRepository defines data access for agent configurations
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/agent_configuration_repository_mock.go . AgentConfigurationRepository:AgentConfigurationRepositoryMock
type AgentConfigurationRepository interface {
	// Create creates a new agent configuration (use within transaction)
	Create(ctx context.Context, tx *gorm.DB, config *models.AgentConfiguration) error

	// GetByUUID retrieves configuration by UUID
	GetByUUID(ctx context.Context, configUUID uuid.UUID, ouID string) (*models.AgentConfiguration, error)

	// GetByAgentID retrieves configuration by agent ID. An agent can have
	// several configuration rows of the same type (e.g. one per bound MCP
	// proxy — see ListMCPConfigsByAgent's doc comment), so this returns an
	// ARBITRARY single row when more than one exists; callers that need
	// every row of a given type must use ListMCPConfigsByAgent (MCP) or
	// ListByAgentAndType instead.
	GetByAgentID(ctx context.Context, agentID, ouID string) (*models.AgentConfiguration, error)

	// ListMCPConfigsByAgent returns every MCP-type AgentConfiguration row for
	// one agent, each fully preloaded with its EnvMCPMappings (and their
	// MCPProxy/Artifact) — one row per MCP proxy the agent is configured
	// with (see createMCPConfig: each configured MCP proxy gets its own
	// AgentConfiguration row, all sharing TypeID=AgentConfigTypeIDMCP).
	// Callers that need the full set of MCP proxies an agent is bound to
	// (e.g. resolving the union of every proxy's scopes) must use this
	// instead of GetByAgentID, which only ever returns one arbitrary row.
	ListMCPConfigsByAgent(ctx context.Context, ouID, projectName, agentID string) ([]models.AgentConfiguration, error)

	// ListMCPConfigsByProxy returns every MCP-type AgentConfiguration row in the
	// organization whose mcp_proxy_uuid is proxyUUID, preloaded with the mapping and
	// env var rows a binding reconcile reads.
	//
	// This is the proxy-side counterpart to ListMCPConfigsByAgent, and deliberately
	// keys off the configuration's own proxy column rather than its mapping rows: a
	// connection with no mapping in some (or any) environment is exactly the one a
	// reconcile needs to find, and a mapping-row join cannot see it.
	ListMCPConfigsByProxy(ctx context.Context, ouID string, proxyUUID uuid.UUID) ([]models.AgentConfiguration, error)

	// List retrieves configurations with pagination
	List(ctx context.Context, ouID string, limit, offset int) ([]models.AgentConfiguration, error)

	// Count counts total configurations
	Count(ctx context.Context, ouID string) (int64, error)

	// ListByAgent retrieves configurations scoped to a specific agent with pagination
	ListByAgent(ctx context.Context, ouID, projectName, agentName string, limit, offset int) ([]models.AgentConfiguration, error)

	// CountByAgent counts total configurations for a specific agent
	CountByAgent(ctx context.Context, ouID, projectName, agentName string) (int64, error)

	// ListByAgentAndType retrieves configurations scoped to a specific agent and config type with pagination
	ListByAgentAndType(ctx context.Context, ouID, projectName, agentName string, typeID uint, limit, offset int) ([]models.AgentConfiguration, error)

	// CountByAgentAndType counts configurations scoped to a specific agent and config type
	CountByAgentAndType(ctx context.Context, ouID, projectName, agentName string, typeID uint) (int64, error)

	// Update updates an existing configuration (use within transaction)
	Update(ctx context.Context, tx *gorm.DB, config *models.AgentConfiguration) error

	// Delete deletes a configuration by UUID (use within transaction)
	Delete(ctx context.Context, tx *gorm.DB, configUUID uuid.UUID, ouID string) error

	// Exists checks if configuration exists
	Exists(ctx context.Context, configUUID uuid.UUID, ouID string) (bool, error)
}

type agentConfigurationRepository struct {
	db *gorm.DB
}

// NewAgentConfigurationRepository creates a new repository
func NewAgentConfigurationRepository(db *gorm.DB) AgentConfigurationRepository {
	return &agentConfigurationRepository{db: db}
}

func (r *agentConfigurationRepository) Create(ctx context.Context, tx *gorm.DB, config *models.AgentConfiguration) error {
	err := tx.WithContext(ctx).Create(config).Error
	if err != nil {
		// Use PostgreSQL error code 23505 (unique_violation) for reliable duplicate detection
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return utils.ErrAgentConfigAlreadyExists
		}
		return err
	}
	return nil
}

func (r *agentConfigurationRepository) GetByUUID(ctx context.Context, configUUID uuid.UUID, ouID string) (*models.AgentConfiguration, error) {
	var config models.AgentConfiguration
	err := r.db.WithContext(ctx).
		Preload("EnvMappings").
		Preload("EnvMappings.LLMProxy").
		Preload("EnvMCPMappings").
		Preload("EnvMCPMappings.Artifact").
		Preload("EnvMCPMappings.MCPProxy").
		Preload("EnvMCPMappings.MCPProxy.Artifact").
		Preload("EnvVariables").
		Where("uuid = ? AND ou_id = ?", configUUID, ouID).
		First(&config).Error
	if err == nil {
		backfillLLMProxyHandles(&config)
	}
	return &config, err
}

func (r *agentConfigurationRepository) GetByAgentID(ctx context.Context, agentID, ouID string) (*models.AgentConfiguration, error) {
	var config models.AgentConfiguration
	err := r.db.WithContext(ctx).
		Preload("EnvMappings").
		Preload("EnvMappings.LLMProxy").
		Preload("EnvMCPMappings").
		Preload("EnvMCPMappings.Artifact").
		Preload("EnvMCPMappings.MCPProxy").
		Preload("EnvMCPMappings.MCPProxy.Artifact").
		Preload("EnvVariables").
		Where("agent_id = ? AND ou_id = ?", agentID, ouID).
		First(&config).Error
	if err == nil {
		backfillLLMProxyHandles(&config)
	}
	return &config, err
}

func (r *agentConfigurationRepository) ListMCPConfigsByAgent(ctx context.Context, ouID, projectName, agentID string) ([]models.AgentConfiguration, error) {
	var configs []models.AgentConfiguration
	err := r.db.WithContext(ctx).
		Preload("EnvMCPMappings").
		Preload("EnvMCPMappings.Artifact").
		Preload("EnvMCPMappings.MCPProxy").
		Preload("EnvMCPMappings.MCPProxy.Artifact").
		Where("ou_id = ? AND project_name = ? AND agent_id = ? AND type_id = ?", ouID, projectName, agentID, models.AgentConfigTypeIDMCP).
		Find(&configs).Error
	return configs, err
}

func (r *agentConfigurationRepository) ListMCPConfigsByProxy(ctx context.Context, ouID string, proxyUUID uuid.UUID) ([]models.AgentConfiguration, error) {
	var configs []models.AgentConfiguration
	err := r.db.WithContext(ctx).
		Preload("EnvMCPMappings").
		Preload("EnvMCPMappings.Artifact").
		Preload("EnvMCPMappings.MCPProxy").
		Preload("EnvMCPMappings.MCPProxy.Artifact").
		Preload("EnvVariables").
		Where("ou_id = ? AND type_id = ? AND mcp_proxy_uuid = ?", ouID, models.AgentConfigTypeIDMCP, proxyUUID).
		Find(&configs).Error
	return configs, err
}

// backfillLLMProxyHandles populates the Handle field of each preloaded LLM proxy
// from its Configuration.Name. LLMProxy.Handle is gorm:"-" (derived from the
// artifact table, not a column on llm_proxies), so GORM Preload leaves it empty.
// For agent-scoped proxies the handle and Configuration.Name are the same value by
// construction (LLMProxyService.Create sets handle := Configuration.Name), so this
// lets every consumer read mapping.LLMProxy.Handle directly instead of treating
// Configuration.Name as a stand-in for the handle.
func backfillLLMProxyHandles(config *models.AgentConfiguration) {
	if config == nil {
		return
	}

	backfillLLMProxyHandlesInMappings(config.EnvMappings)
}

// backfillLLMProxyHandlesInMappings is the shared implementation used by any
// repository that preloads EnvAgentModelMapping.LLMProxy directly (see
// backfillLLMProxyHandles for why this backfill is needed).
func backfillLLMProxyHandlesInMappings(mappings []models.EnvAgentModelMapping) {
	for i := range mappings {
		proxy := mappings[i].LLMProxy
		if proxy != nil && proxy.Handle == "" {
			proxy.Handle = proxy.Configuration.Name
		}
	}
}

func (r *agentConfigurationRepository) List(ctx context.Context, ouID string, limit, offset int) ([]models.AgentConfiguration, error) {
	var configs []models.AgentConfiguration
	err := r.db.WithContext(ctx).
		Where("ou_id = ?", ouID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&configs).Error
	return configs, err
}

func (r *agentConfigurationRepository) Count(ctx context.Context, ouID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.AgentConfiguration{}).
		Where("ou_id = ?", ouID).
		Count(&count).Error
	return count, err
}

func (r *agentConfigurationRepository) ListByAgent(ctx context.Context, ouID, projectName, agentName string, limit, offset int) ([]models.AgentConfiguration, error) {
	var configs []models.AgentConfiguration
	err := r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ? AND agent_id = ?", ouID, projectName, agentName).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&configs).Error
	return configs, err
}

func (r *agentConfigurationRepository) CountByAgent(ctx context.Context, ouID, projectName, agentName string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.AgentConfiguration{}).
		Where("ou_id = ? AND project_name = ? AND agent_id = ?", ouID, projectName, agentName).
		Count(&count).Error
	return count, err
}

func (r *agentConfigurationRepository) ListByAgentAndType(
	ctx context.Context, ouID, projectName, agentName string, typeID uint, limit, offset int,
) ([]models.AgentConfiguration, error) {
	var configs []models.AgentConfiguration
	err := r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ? AND agent_id = ? AND type_id = ?", ouID, projectName, agentName, typeID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&configs).Error
	return configs, err
}

func (r *agentConfigurationRepository) CountByAgentAndType(
	ctx context.Context, ouID, projectName, agentName string, typeID uint,
) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.AgentConfiguration{}).
		Where("ou_id = ? AND project_name = ? AND agent_id = ? AND type_id = ?", ouID, projectName, agentName, typeID).
		Count(&count).Error
	return count, err
}

func (r *agentConfigurationRepository) Update(ctx context.Context, tx *gorm.DB, config *models.AgentConfiguration) error {
	return tx.WithContext(ctx).Save(config).Error
}

func (r *agentConfigurationRepository) Delete(ctx context.Context, tx *gorm.DB, configUUID uuid.UUID, ouID string) error {
	return tx.WithContext(ctx).
		Where("uuid = ? AND ou_id = ?", configUUID, ouID).
		Delete(&models.AgentConfiguration{}).Error
}

func (r *agentConfigurationRepository) Exists(ctx context.Context, configUUID uuid.UUID, ouID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.AgentConfiguration{}).
		Where("uuid = ? AND ou_id = ?", configUUID, ouID).
		Count(&count).Error
	return count > 0, err
}

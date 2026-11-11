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

	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
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
	// organization whose agent_mcp_config_proxy reference names proxyUUID, preloaded with
	// the mapping and env var rows a binding reconcile reads.
	//
	// This is the proxy-side counterpart to ListMCPConfigsByAgent, and deliberately
	// keys off the configuration's proxy reference rather than its mapping rows: a
	// connection with no mapping in some (or any) environment is exactly the one a
	// reconcile needs to find, and a mapping-row join cannot see it.
	ListMCPConfigsByProxy(ctx context.Context, ouID string, proxyUUID uuid.UUID) ([]models.AgentConfiguration, error)

	// SetMCPProxyRef records the environment-agnostic MCP proxy a configuration
	// references, inserting or re-pointing its agent_mcp_config_proxy row.
	SetMCPProxyRef(ctx context.Context, tx *gorm.DB, configUUID, proxyUUID uuid.UUID) error

	// ClearMCPProxyRef removes a configuration's proxy reference, for when its
	// environments no longer agree on a single proxy.
	ClearMCPProxyRef(ctx context.Context, tx *gorm.DB, configUUID uuid.UUID) error

	// SetMCPEnvScope records which environments an MCP connection was deliberately
	// removed from (exclude) and which it was explicitly asked into (include), in one
	// transaction. Include wins for an environment listed in both.
	SetMCPEnvScope(ctx context.Context, tx *gorm.DB, configUUID uuid.UUID, exclude, include []uuid.UUID) error

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
	if config.UUID == uuid.Nil {
		config.UUID = uuid.Must(uuid.NewV7())
	}
	now := dbx.Now()
	if config.CreatedAt.IsZero() {
		config.CreatedAt = now
	}
	config.UpdatedAt = now
	inserted, err := dbx.InsertIfAbsent(tx.WithContext(ctx), config,
		"EnvMappings", "EnvMCPMappings", "EnvVariables", "MCPProxyRef", "MCPEnvExclusions", "MCPScope")
	if err != nil {
		return err
	}
	if !inserted {
		return utils.ErrAgentConfigAlreadyExists
	}
	return nil
}

// withRelations preloads a configuration's mappings. MCP mappings and exclusions share one table, told apart by
// excluded; the scope is the proxy row once its scope_recorded_at is set.
func withRelations(q *gorm.DB, envVariables bool) *gorm.DB {
	q = q.
		Preload("EnvMappings").
		Preload("EnvMappings.LLMProxy").
		Preload("EnvMCPMappings", "excluded = ?", false).
		Preload("MCPProxyRef").
		Preload("MCPEnvExclusions", "excluded = ?", true).
		Preload("MCPScope", "scope_recorded_at IS NOT NULL").
		Preload("EnvMCPMappings.Artifact").
		Preload("EnvMCPMappings.MCPProxy").
		Preload("EnvMCPMappings.MCPProxy.Artifact").
		Preload("EnvMCPMappings.MCPProxy.Endpoints").
		Preload("EnvMCPMappings.MCPProxy.Endpoints.Environments")
	if envVariables {
		q = q.Preload("EnvVariables")
	}
	return q
}

// GetByUUID loads one configuration with its LLM and MCP mappings. MCP mappings carry their
// proxy's endpoint → environment bindings, which per-environment gateway resolution reads.
func (r *agentConfigurationRepository) GetByUUID(ctx context.Context, configUUID uuid.UUID, ouID string) (*models.AgentConfiguration, error) {
	var config models.AgentConfiguration
	err := withRelations(r.db.WithContext(ctx), true).
		Where("id = ? AND org_id = ?", configUUID, ouID).
		First(&config).Error
	if err == nil {
		backfillLLMProxyHandles(&config)
	}
	return &config, err
}

// GetByAgentID loads an agent's configuration with the same relations as GetByUUID. agentID is the agent's name,
// which is unique in its organization.
func (r *agentConfigurationRepository) GetByAgentID(ctx context.Context, agentID, ouID string) (*models.AgentConfiguration, error) {
	var config models.AgentConfiguration
	err := withRelations(r.db.WithContext(ctx), true).
		Where("org_id = ? AND agent_id IN (?)", ouID,
			r.db.Session(&gorm.Session{NewDB: true}).Table("agents").Select("id").Where("org_id = ? AND name = ?", ouID, agentID)).
		First(&config).Error
	if err == nil {
		backfillLLMProxyHandles(&config)
	}
	return &config, err
}

// byAgent narrows q to one agent's configurations.
func (r *agentConfigurationRepository) byAgent(q *gorm.DB, ouID, projectName, agentName string) *gorm.DB {
	return q.Where("agent_model_configs.org_id = ? AND agent_model_configs.agent_id IN (?)", ouID,
		models.AgentIDQuery(r.db, ouID, projectName, agentName))
}

// ListMCPConfigsByAgent lists an agent's MCP configurations with their mappings, proxies and
// the proxies' endpoint → environment bindings.
func (r *agentConfigurationRepository) ListMCPConfigsByAgent(ctx context.Context, ouID, projectName, agentID string) ([]models.AgentConfiguration, error) {
	var configs []models.AgentConfiguration
	err := r.byAgent(withRelations(r.db.WithContext(ctx), false), ouID, projectName, agentID).
		Where("agent_model_configs.type = ?", models.AgentConfigTypeMCP).
		Find(&configs).Error
	return configs, err
}

// ListMCPConfigsByProxy lists the MCP configurations bound to the given proxy, with the same
// relations as ListMCPConfigsByAgent plus their env var rows.
func (r *agentConfigurationRepository) ListMCPConfigsByProxy(ctx context.Context, ouID string, proxyUUID uuid.UUID) ([]models.AgentConfiguration, error) {
	var configs []models.AgentConfiguration
	err := withRelations(r.db.WithContext(ctx), true).
		Joins("JOIN agent_model_config_mcp p ON p.config_id = agent_model_configs.id").
		Where("agent_model_configs.org_id = ? AND agent_model_configs.type = ? AND p.mcp_proxy_id = ?",
			ouID, models.AgentConfigTypeMCP, proxyUUID).
		Find(&configs).Error
	return configs, err
}

// SetMCPProxyRef points a configuration at proxyUUID, inserting the reference row or
// re-pointing the existing one. Idempotent, so a reconcile converging a stale reference
// and a write path recording a new one take the same path.
func (r *agentConfigurationRepository) SetMCPProxyRef(ctx context.Context, tx *gorm.DB, configUUID, proxyUUID uuid.UUID) error {
	now := dbx.Now()
	ref := models.AgentMCPConfigProxy{ConfigUUID: configUUID, MCPProxyUUID: proxyUUID, CreatedAt: now, UpdatedAt: now}
	_, err := dbx.Upsert(tx.WithContext(ctx), &ref, func(db *gorm.DB) error {
		return db.Model(&models.AgentMCPConfigProxy{}).Where("config_id = ?", configUUID).
			Updates(map[string]any{"mcp_proxy_id": proxyUUID, "updated_at": now}).Error
	})
	return err
}

// SetMCPEnvScope adds exclude as exclusions and removes include from them, and marks the
// connection's scope as recorded so the reconcile trusts those exclusions from now on.
// Idempotent: excluding an already-excluded environment, including a never-excluded one and
// re-marking a marked connection are all no-ops.
func (r *agentConfigurationRepository) SetMCPEnvScope(ctx context.Context, tx *gorm.DB, configUUID uuid.UUID, exclude, include []uuid.UUID) error {
	db := tx.WithContext(ctx)
	if err := db.Model(&models.AgentMCPConfigProxy{}).
		Where("config_id = ? AND scope_recorded_at IS NULL", configUUID).
		Update("scope_recorded_at", dbx.Now()).Error; err != nil {
		return err
	}
	if len(include) > 0 {
		if err := db.Where("config_id = ? AND environment_id IN ? AND excluded = ?", configUUID, include, true).
			Delete(&models.AgentMCPConfigEnvExclusion{}).Error; err != nil {
			return err
		}
	}
	keep := make(map[uuid.UUID]struct{}, len(include))
	for _, envUUID := range include {
		keep[envUUID] = struct{}{}
	}
	for _, envUUID := range exclude {
		if _, included := keep[envUUID]; included {
			continue
		}
		if _, err := dbx.InsertIfAbsent(db, &models.AgentMCPConfigEnvExclusion{
			ConfigUUID: configUUID, EnvironmentUUID: envUUID,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ClearMCPProxyRef drops a configuration's proxy reference. Used when its environments stop
// agreeing on a single proxy, which leaves no environment-agnostic answer to record.
func (r *agentConfigurationRepository) ClearMCPProxyRef(ctx context.Context, tx *gorm.DB, configUUID uuid.UUID) error {
	return tx.WithContext(ctx).
		Where("config_id = ?", configUUID).
		Delete(&models.AgentMCPConfigProxy{}).Error
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
		Where("org_id = ?", ouID).
		Order("created_at DESC, id").
		Limit(limit).
		Offset(offset).
		Find(&configs).Error
	return configs, err
}

func (r *agentConfigurationRepository) Count(ctx context.Context, ouID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.AgentConfiguration{}).
		Where("org_id = ?", ouID).
		Count(&count).Error
	return count, err
}

func (r *agentConfigurationRepository) ListByAgent(ctx context.Context, ouID, projectName, agentName string, limit, offset int) ([]models.AgentConfiguration, error) {
	var configs []models.AgentConfiguration
	err := r.byAgent(r.db.WithContext(ctx), ouID, projectName, agentName).
		Order("created_at DESC, id").
		Limit(limit).
		Offset(offset).
		Find(&configs).Error
	return configs, err
}

func (r *agentConfigurationRepository) CountByAgent(ctx context.Context, ouID, projectName, agentName string) (int64, error) {
	var count int64
	err := r.byAgent(r.db.WithContext(ctx).Model(&models.AgentConfiguration{}), ouID, projectName, agentName).
		Count(&count).Error
	return count, err
}

func (r *agentConfigurationRepository) ListByAgentAndType(
	ctx context.Context, ouID, projectName, agentName string, typeID uint, limit, offset int,
) ([]models.AgentConfiguration, error) {
	var configs []models.AgentConfiguration
	err := r.byAgent(r.db.WithContext(ctx), ouID, projectName, agentName).
		Where("type = ?", models.AgentConfigTypeFromID(typeID)).
		Order("created_at DESC, id").
		Limit(limit).
		Offset(offset).
		Find(&configs).Error
	return configs, err
}

func (r *agentConfigurationRepository) CountByAgentAndType(
	ctx context.Context, ouID, projectName, agentName string, typeID uint,
) (int64, error) {
	var count int64
	err := r.byAgent(r.db.WithContext(ctx).Model(&models.AgentConfiguration{}), ouID, projectName, agentName).
		Where("type = ?", models.AgentConfigTypeFromID(typeID)).
		Count(&count).Error
	return count, err
}

func (r *agentConfigurationRepository) Update(ctx context.Context, tx *gorm.DB, config *models.AgentConfiguration) error {
	config.UpdatedAt = dbx.Now()
	return tx.WithContext(ctx).
		Omit("EnvMappings", "EnvMCPMappings", "EnvVariables", "MCPProxyRef", "MCPEnvExclusions", "MCPScope").
		Save(config).Error
}

func (r *agentConfigurationRepository) Delete(ctx context.Context, tx *gorm.DB, configUUID uuid.UUID, ouID string) error {
	return tx.WithContext(ctx).
		Where("id = ? AND org_id = ?", configUUID, ouID).
		Delete(&models.AgentConfiguration{}).Error
}

func (r *agentConfigurationRepository) Exists(ctx context.Context, configUUID uuid.UUID, ouID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.AgentConfiguration{}).
		Where("id = ? AND org_id = ?", configUUID, ouID).
		Count(&count).Error
	return count > 0, err
}

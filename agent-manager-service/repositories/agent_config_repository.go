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

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// ErrAgentConfigNotFound is returned when no agent config exists for the given agent and environment.
var ErrAgentConfigNotFound = errors.New("agent config not found")

// AgentConfigRepository defines the interface for agent configuration operations
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/agent_config_repository_mock.go . AgentConfigRepository:AgentConfigRepositoryMock
type AgentConfigRepository interface {
	// Upsert creates or updates an agent config for a specific environment
	Upsert(ctx context.Context, config *models.AgentConfig) error

	// CreateIfAbsent inserts the config unless the environment already has one, which it leaves untouched.
	CreateIfAbsent(ctx context.Context, config *models.AgentConfig) error

	// WithTx returns the repository bound to tx.
	WithTx(tx *gorm.DB) AgentConfigRepository

	// Get retrieves agent config for a specific agent and environment
	Get(ctx context.Context, ouID, projectName, agentName, environmentName string) (*models.AgentConfig, error)

	// DeleteAllByAgent removes all configs for an agent (used when agent is deleted)
	DeleteAllByAgent(ctx context.Context, ouID, projectName, agentName string) error
	// DeleteForEnvironment removes every agent's config for a deleted environment.
	DeleteForEnvironment(ctx context.Context, ouID, environmentName string) error
}

// AgentConfigRepo implements AgentConfigRepository using GORM
type AgentConfigRepo struct {
	db *gorm.DB
}

// NewAgentConfigRepo creates a new agent config repository
func NewAgentConfigRepo(db *gorm.DB) AgentConfigRepository {
	return &AgentConfigRepo{db: db}
}

// agentConfigColumns are the settings an Upsert rewrites.
var agentConfigColumns = []string{
	"api_key_security", "cors_enabled", "cors_allow_origins", "cors_allow_methods", "cors_allow_headers",
	"cors_allow_credentials", "oauth_enabled", "oauth_issuers", "oauth_audiences", "oauth_header_name",
	"oauth_header_prefix", "oauth_forward_token", "timeout_seconds", "card_cors_enabled", "card_cors_allow_origins",
	"card_cors_allow_headers", "card_cors_allow_credentials", "updated_at",
}

// prepare resolves the config's ids and stamps it. The required JSON lists are never NULL: a nil list is empty.
func (r *AgentConfigRepo) prepare(db *gorm.DB, config *models.AgentConfig) error {
	if err := config.Resolve(db); err != nil {
		return err
	}
	for _, list := range []*[]string{
		&config.CORSAllowOrigins, &config.CORSAllowMethods, &config.CORSAllowHeaders, &config.OAuthIssuers, &config.OAuthAudiences,
	} {
		if *list == nil {
			*list = []string{}
		}
	}
	now := dbx.Now()
	if config.CreatedAt.IsZero() {
		config.CreatedAt = now
	}
	config.UpdatedAt = now
	return nil
}

// Upsert creates or updates an agent config for a specific environment
func (r *AgentConfigRepo) Upsert(ctx context.Context, config *models.AgentConfig) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.prepare(tx, config); err != nil {
			return err
		}
		_, err := dbx.Upsert(tx, config, func(tx *gorm.DB) error {
			return tx.Model(&models.AgentConfig{}).
				Where("agent_id = ? AND environment_id = ?", *config.AgentUUID, config.EnvironmentUUID).
				Select(agentConfigColumns).Updates(config).Error
		})
		return err
	})
}

// CreateIfAbsent inserts the config unless the environment already has one.
func (r *AgentConfigRepo) CreateIfAbsent(ctx context.Context, config *models.AgentConfig) error {
	db := r.db.WithContext(ctx)
	if err := r.prepare(db, config); err != nil {
		return err
	}
	_, err := dbx.InsertIfAbsent(db, config)
	return err
}

// WithTx returns the repository bound to tx.
func (r *AgentConfigRepo) WithTx(tx *gorm.DB) AgentConfigRepository {
	return &AgentConfigRepo{db: tx}
}

// Get retrieves agent config for a specific agent and environment
func (r *AgentConfigRepo) Get(ctx context.Context, ouID, projectName, agentName, environmentName string) (*models.AgentConfig, error) {
	var config models.AgentConfig
	cond, args := agentEnvCond(r.db, ouID, projectName, agentName, environmentName)
	err := r.db.WithContext(ctx).Where(cond, args...).Take(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentConfigNotFound
		}
		return nil, err
	}
	return &config, nil
}

// DeleteAllByAgent removes all configs for an agent (used when agent is deleted)
func (r *AgentConfigRepo) DeleteAllByAgent(ctx context.Context, ouID, projectName, agentName string) error {
	cond, args := agentCond(r.db, ouID, projectName, agentName)
	return r.db.WithContext(ctx).Where(cond, args...).Delete(&models.AgentConfig{}).Error
}

// DeleteForEnvironment removes every agent's config for a deleted environment.
func (r *AgentConfigRepo) DeleteForEnvironment(ctx context.Context, ouID, environmentName string) error {
	return r.db.WithContext(ctx).
		Where("org_id = ? AND environment_id IN (?)", ouID, models.EnvironmentIDQuery(r.db, ouID, environmentName)).
		Delete(&models.AgentConfig{}).Error
}

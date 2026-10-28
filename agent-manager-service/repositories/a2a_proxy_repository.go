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
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// ErrA2AProxyNotFound means the agent's environment is not proxied.
var ErrA2AProxyNotFound = errors.New("a2a proxy not found")

// A2AProxyRepository stores which external A2A agent environments are published through their gateway.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/a2a_proxy_repository_mock.go . A2AProxyRepository:A2AProxyRepositoryMock
type A2AProxyRepository interface {
	// WithTx returns the repository bound to tx, so its writes commit or roll back with the caller's.
	WithTx(tx *gorm.DB) A2AProxyRepository

	// LockAgentEnv serialises the environment's proxy transitions until the transaction ends; call it
	// first, on WithTx, so every transition takes its row locks in the same order.
	LockAgentEnv(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) error

	// Get looks the row up by the environment name in the URL path.
	Get(ctx context.Context, ouID, projectName, agentName, environmentName string) (*models.A2AProxy, error)
	// GetByEnvUUID looks the row up by environment UUID.
	GetByEnvUUID(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) (*models.A2AProxy, error)

	// Upsert writes the row, replacing a same-named row from a deleted environment.
	Upsert(ctx context.Context, proxy *models.A2AProxy) error

	// Delete returns ErrA2AProxyNotFound when the environment was not proxied.
	Delete(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) error
	// DeleteForAgent removes every environment's row for a deleted agent.
	DeleteForAgent(ctx context.Context, ouID, projectName, agentName string) error
	// DeleteForEnvironment removes every agent's row in a deleted environment.
	DeleteForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) error
	// ListEnvironmentUUIDs returns the distinct environment UUIDs with rows recorded under environmentName.
	ListEnvironmentUUIDs(ctx context.Context, ouID, environmentName string) ([]uuid.UUID, error)
}

type a2aProxyRepository struct {
	db *gorm.DB
}

// NewA2AProxyRepository creates an A2AProxyRepository.
func NewA2AProxyRepository(db *gorm.DB) A2AProxyRepository {
	return &a2aProxyRepository{db: db}
}

func (r *a2aProxyRepository) WithTx(tx *gorm.DB) A2AProxyRepository {
	return &a2aProxyRepository{db: tx}
}

func (r *a2aProxyRepository) LockAgentEnv(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) error {
	return advisoryXactLock(r.db.WithContext(ctx), a2aProxyLockNamespace, a2aProxyLockKey(ouID, projectName, agentName, environmentUUID))
}

func a2aProxyLockKey(ouID, projectName, agentName string, environmentUUID uuid.UUID) string {
	return strings.Join([]string{ouID, projectName, agentName, environmentUUID.String()}, "/")
}

func (r *a2aProxyRepository) Get(ctx context.Context, ouID, projectName, agentName, environmentName string) (*models.A2AProxy, error) {
	return r.first(ctx, "ou_id = ? AND project_name = ? AND agent_name = ? AND environment_name = ?",
		ouID, projectName, agentName, environmentName)
}

func (r *a2aProxyRepository) GetByEnvUUID(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) (*models.A2AProxy, error) {
	return r.first(ctx, "ou_id = ? AND project_name = ? AND agent_name = ? AND environment_uuid = ?",
		ouID, projectName, agentName, environmentUUID)
}

func (r *a2aProxyRepository) first(ctx context.Context, query string, args ...interface{}) (*models.A2AProxy, error) {
	var row models.A2AProxy
	err := r.db.WithContext(ctx).Where(query, args...).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrA2AProxyNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *a2aProxyRepository) Upsert(ctx context.Context, proxy *models.A2AProxy) error {
	proxy.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A same-named row from a deleted environment must not pass its secret on.
		err := tx.Where("ou_id = ? AND project_name = ? AND agent_name = ? AND environment_name = ? AND environment_uuid <> ?",
			proxy.OUID, proxy.ProjectName, proxy.AgentName, proxy.EnvironmentName, proxy.EnvironmentUUID).
			Delete(&models.A2AProxy{}).Error
		if err != nil {
			return err
		}
		// Select("*") writes an empty header and secret explicitly, so clearing auth clears the row.
		return tx.Select("*").Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "ou_id"},
				{Name: "project_name"},
				{Name: "agent_name"},
				{Name: "environment_uuid"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"environment_name", "upstream_auth_header", "upstream_auth_secret", "updated_at",
			}),
		}).Create(proxy).Error
	})
}

func (r *a2aProxyRepository) Delete(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ? AND agent_name = ? AND environment_uuid = ?",
			ouID, projectName, agentName, environmentUUID).
		Delete(&models.A2AProxy{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrA2AProxyNotFound
	}
	return nil
}

func (r *a2aProxyRepository) DeleteForAgent(ctx context.Context, ouID, projectName, agentName string) error {
	return r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ? AND agent_name = ?", ouID, projectName, agentName).
		Delete(&models.A2AProxy{}).Error
}

func (r *a2aProxyRepository) DeleteForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) error {
	return deleteForEnvironment[models.A2AProxy](ctx, r.db, ouID, environmentUUID)
}

func (r *a2aProxyRepository) ListEnvironmentUUIDs(ctx context.Context, ouID, environmentName string) ([]uuid.UUID, error) {
	return listEnvironmentUUIDs[models.A2AProxy](ctx, r.db, ouID, environmentName)
}

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
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// ErrPlatformAgentExposureNotFound means the agent is not exposed in the environment.
var ErrPlatformAgentExposureNotFound = errors.New("platform agent exposure not found")

// PlatformAgentExposureRepository records which platform-hosted agents AMS exposes through each environment's gateway.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/platform_agent_exposure_repository_mock.go . PlatformAgentExposureRepository:PlatformAgentExposureRepositoryMock
type PlatformAgentExposureRepository interface {
	// WithTx returns the repository bound to tx, so its writes commit or roll back with the caller's.
	WithTx(tx *gorm.DB) PlatformAgentExposureRepository

	// Upsert records the agent-environment pair, replacing a same-named row from a deleted environment.
	Upsert(ctx context.Context, exposure *models.PlatformAgentExposure) error

	// Get returns ErrPlatformAgentExposureNotFound when no row exists.
	Get(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) (*models.PlatformAgentExposure, error)

	// GetByArtifact returns the exposure of the per-environment agent artifact, or ErrPlatformAgentExposureNotFound.
	GetByArtifact(ctx context.Context, ouID string, artifactUUID uuid.UUID) (*models.PlatformAgentExposure, error)

	// ListForEnvironment returns every agent's row in one environment.
	ListForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) ([]models.PlatformAgentExposure, error)

	// ListEnvironmentUUIDs returns the distinct environment UUIDs with rows recorded under environmentName.
	ListEnvironmentUUIDs(ctx context.Context, ouID, environmentName string) ([]uuid.UUID, error)

	// DeleteForAgentEnv removes one environment's row.
	DeleteForAgentEnv(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) error

	// DeleteForEnvironment removes every agent's row in a deleted environment.
	DeleteForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) error

	// DeleteForAgent removes every environment's row for a deleted agent.
	DeleteForAgent(ctx context.Context, ouID, projectName, agentName string) error
}

type platformAgentExposureRepository struct {
	db *gorm.DB
}

// NewPlatformAgentExposureRepository creates a PlatformAgentExposureRepository.
func NewPlatformAgentExposureRepository(db *gorm.DB) PlatformAgentExposureRepository {
	return &platformAgentExposureRepository{db: db}
}

func (r *platformAgentExposureRepository) WithTx(tx *gorm.DB) PlatformAgentExposureRepository {
	return &platformAgentExposureRepository{db: tx}
}

func (r *platformAgentExposureRepository) Upsert(ctx context.Context, exposure *models.PlatformAgentExposure) error {
	if exposure.Protocol == "" {
		exposure.Protocol = models.AgentExposureProtocolA2A
	}
	now := dbx.Now()
	exposure.UpdatedAt = now
	if exposure.ID == uuid.Nil {
		exposure.ID = newID()
	}
	if exposure.CreatedAt.IsZero() {
		exposure.CreatedAt = now
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if exposure.AgentUUID == nil || *exposure.AgentUUID == uuid.Nil {
			id, err := models.AgentIDByNames(tx, exposure.OUID, exposure.ProjectName, exposure.AgentName)
			if err != nil {
				return err
			}
			exposure.AgentUUID = &id
		}
		_, err := dbx.Upsert(tx, exposure, func(tx *gorm.DB) error {
			return tx.Model(&models.PlatformAgentExposure{}).
				Where("agent_id = ? AND environment_id = ?", *exposure.AgentUUID, exposure.EnvironmentUUID).
				Updates(map[string]any{"artifact_id": exposure.ArtifactUUID, "protocol": exposure.Protocol, "updated_at": now}).Error
		})
		return err
	})
}

func (r *platformAgentExposureRepository) Get(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) (*models.PlatformAgentExposure, error) {
	cond, args := agentEnvIDCond(r.db, ouID, projectName, agentName, environmentUUID)
	return r.first(ctx, cond, args...)
}

func (r *platformAgentExposureRepository) GetByArtifact(ctx context.Context, ouID string, artifactUUID uuid.UUID) (*models.PlatformAgentExposure, error) {
	return r.first(ctx, "org_id = ? AND artifact_id = ?", ouID, artifactUUID)
}

func (r *platformAgentExposureRepository) first(ctx context.Context, query string, args ...interface{}) (*models.PlatformAgentExposure, error) {
	var row models.PlatformAgentExposure
	err := r.db.WithContext(ctx).Where(query, args...).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPlatformAgentExposureNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *platformAgentExposureRepository) ListForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) ([]models.PlatformAgentExposure, error) {
	return listForEnvironment[models.PlatformAgentExposure](ctx, r.db, models.PlatformAgentExposure{}.TableName(), ouID, environmentUUID)
}

func (r *platformAgentExposureRepository) ListEnvironmentUUIDs(ctx context.Context, ouID, environmentName string) ([]uuid.UUID, error) {
	return listEnvironmentUUIDs[models.PlatformAgentExposure](ctx, r.db, ouID, environmentName)
}

func (r *platformAgentExposureRepository) DeleteForAgentEnv(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) error {
	cond, args := agentEnvIDCond(r.db, ouID, projectName, agentName, environmentUUID)
	return r.db.WithContext(ctx).Where(cond, args...).Delete(&models.PlatformAgentExposure{}).Error
}

func (r *platformAgentExposureRepository) DeleteForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) error {
	return deleteForEnvironment[models.PlatformAgentExposure](ctx, r.db, ouID, environmentUUID)
}

func (r *platformAgentExposureRepository) DeleteForAgent(ctx context.Context, ouID, projectName, agentName string) error {
	cond, args := agentCond(r.db, ouID, projectName, agentName)
	return r.db.WithContext(ctx).Where(cond, args...).Delete(&models.PlatformAgentExposure{}).Error
}

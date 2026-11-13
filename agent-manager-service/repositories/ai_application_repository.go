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

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// AIApplicationRepository defines the interface for AI application persistence.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/ai_application_repository_mock.go . AIApplicationRepository:AIApplicationRepositoryMock
type AIApplicationRepository interface {
	// Create inserts an AIApplication row. Uses ON CONFLICT DO NOTHING so it is safe to
	// call idempotently on retry. Returns (true, nil) when a new row was inserted, or
	// (false, nil) when the conflict path fired (caller must re-fetch with GetByAgentEnv).
	Create(ctx context.Context, tx *gorm.DB, app *models.AIApplication) (bool, error)
	// GetByAgentEnv returns the AIApplication for the given org/project/agent/environment.
	// Returns gorm.ErrRecordNotFound when no row exists.
	GetByAgentEnv(ctx context.Context, ouID, projectName, agentID, envName string) (*models.AIApplication, error)
	// ListByOrg returns all AIApplication rows for the given organisation.
	ListByOrg(ctx context.Context, ouID string) ([]models.AIApplication, error)
	// ListByAgent returns all AIApplication rows for the given org+project+agent across all environments.
	ListByAgent(ctx context.Context, ouID, projectName, agentID string) ([]models.AIApplication, error)
	// DeleteByAgent removes all AIApplication rows for the given org+project+agent across all environments.
	// Used during agent deletion after all per-config resources are cleaned up.
	DeleteByAgent(ctx context.Context, ouID, projectName, agentID string) error
	// DeleteByAgentEnv removes the AIApplication row for the given agent+environment. No-op if absent.
	DeleteByAgentEnv(ctx context.Context, tx *gorm.DB, ouID, projectName, agentID, envName string) error
}

// AIApplicationRepo implements AIApplicationRepository using GORM.
type AIApplicationRepo struct {
	db *gorm.DB
}

// NewAIApplicationRepository creates a new AIApplicationRepo.
func NewAIApplicationRepository(db *gorm.DB) *AIApplicationRepo {
	return &AIApplicationRepo{db: db}
}

// Create inserts an AIApplication, ignoring conflicts on (ou_id, project_name, agent_id, environment_name).
// Returns (true, nil) when a new row was inserted; (false, nil) when ON CONFLICT DO NOTHING fired.
func (r *AIApplicationRepo) Create(ctx context.Context, tx *gorm.DB, app *models.AIApplication) (bool, error) {
	db := tx
	if db == nil {
		db = r.db
	}
	return dbx.InsertIfAbsent(db.WithContext(ctx), app)
}

// GetByAgentEnv fetches the AIApplication for a specific agent+environment in an org.
func (r *AIApplicationRepo) GetByAgentEnv(ctx context.Context, ouID, projectName, agentID, envName string) (*models.AIApplication, error) {
	var app models.AIApplication
	err := r.db.WithContext(ctx).
		Where(appCond(r.db, ouID, projectName, agentID, envName)).
		Take(&app).Error
	if err != nil {
		return nil, err
	}
	return &app, nil
}

// ListByOrg returns all AIApplication rows for an organisation.
func (r *AIApplicationRepo) ListByOrg(ctx context.Context, ouID string) ([]models.AIApplication, error) {
	var apps []models.AIApplication
	err := r.db.WithContext(ctx).
		Where("org_id = ?", ouID).
		Find(&apps).Error
	return apps, err
}

// ListByAgent returns all AIApplication rows for the given org+project+agent across all environments.
func (r *AIApplicationRepo) ListByAgent(ctx context.Context, ouID, projectName, agentID string) ([]models.AIApplication, error) {
	var apps []models.AIApplication
	err := r.db.WithContext(ctx).
		Where(appCond(r.db, ouID, projectName, agentID, "")).
		Find(&apps).Error
	return apps, err
}

// DeleteByAgent removes all AIApplication rows for the given org+project+agent across all environments.
func (r *AIApplicationRepo) DeleteByAgent(ctx context.Context, ouID, projectName, agentID string) error {
	return r.db.WithContext(ctx).
		Where(appCond(r.db, ouID, projectName, agentID, "")).
		Delete(&models.AIApplication{}).Error
}

// DeleteByAgentEnv removes the AIApplication row for the given agent+environment. No-op if absent.
func (r *AIApplicationRepo) DeleteByAgentEnv(ctx context.Context, tx *gorm.DB, ouID, projectName, agentID, envName string) error {
	db := tx
	if db == nil {
		db = r.db
	}
	return db.WithContext(ctx).
		Where(appCond(r.db, ouID, projectName, agentID, envName)).
		Delete(&models.AIApplication{}).Error
}

// appCond matches the named agent's applications, in the named environment unless envName is empty.
func appCond(db *gorm.DB, ouID, projectName, agentName, envName string) *gorm.DB {
	q := db.Session(&gorm.Session{NewDB: true}).
		Where("org_id = ? AND agent_id IN (?)", ouID, models.AgentIDQuery(db, ouID, projectName, agentName))
	if envName != "" {
		q = q.Where("environment_id IN (?)", models.EnvironmentIDQuery(db, ouID, envName))
	}
	return q
}

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
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// CancelledBuildRepository defines data access for the record of builds that
// were cancelled before they finished. See models.CancelledBuild and
// migration044: these rows are the only surviving evidence of a cancelled
// build, because cancelling deletes the underlying WorkflowRun.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/cancelled_build_repository_mock.go . CancelledBuildRepository:CancelledBuildRepositoryMock
type CancelledBuildRepository interface {
	// Record stores rec. Recording the same build twice is not an error: the
	// second call leaves the first row untouched, so a retried cancel (the
	// delete succeeded but the response was lost) cannot rewrite who cancelled
	// the build or when.
	Record(ctx context.Context, rec *models.CancelledBuild) error
	// ListByAgent returns every cancelled build for an agent, newest first.
	// An agent with no cancelled builds yields an empty slice, not an error.
	ListByAgent(ctx context.Context, ouID, projectName, agentName string) ([]models.CancelledBuild, error)
	// Get returns one cancelled build, or (nil, gorm.ErrRecordNotFound) when
	// the build was never cancelled — callers map that to a build-not-found.
	Get(ctx context.Context, ouID, projectName, agentName, buildName string) (*models.CancelledBuild, error)
	// DeleteAllByAgent removes every cancelled-build record for an agent, so a
	// deleted agent does not leave rows that a later same-named agent would
	// inherit. Deleting when there are no rows is not an error.
	DeleteAllByAgent(ctx context.Context, ouID, projectName, agentName string) error
}

type cancelledBuildRepo struct {
	db *gorm.DB
}

// NewCancelledBuildRepo creates a new cancelled-build repository.
func NewCancelledBuildRepo(db *gorm.DB) CancelledBuildRepository {
	return &cancelledBuildRepo{db: db}
}

func (r *cancelledBuildRepo) Record(ctx context.Context, rec *models.CancelledBuild) error {
	// DoNothing rather than an upsert: the first record of a cancellation is
	// the true one.
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "ou_id"}, {Name: "project_name"}, {Name: "agent_name"}, {Name: "build_name"}},
			DoNothing: true,
		}).
		Create(rec).Error
}

func (r *cancelledBuildRepo) ListByAgent(ctx context.Context, ouID, projectName, agentName string) ([]models.CancelledBuild, error) {
	records := make([]models.CancelledBuild, 0)
	err := r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ? AND agent_name = ?", ouID, projectName, agentName).
		Order("cancelled_at DESC").
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (r *cancelledBuildRepo) Get(ctx context.Context, ouID, projectName, agentName, buildName string) (*models.CancelledBuild, error) {
	var record models.CancelledBuild
	err := r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ? AND agent_name = ? AND build_name = ?", ouID, projectName, agentName, buildName).
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *cancelledBuildRepo) DeleteAllByAgent(ctx context.Context, ouID, projectName, agentName string) error {
	return r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ? AND agent_name = ?", ouID, projectName, agentName).
		Delete(&models.CancelledBuild{}).Error
}

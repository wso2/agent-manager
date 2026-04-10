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
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// APIKeyRepository defines the interface for API key persistence
type APIKeyRepository interface {
	Upsert(key *models.StoredAPIKey) error
	Delete(artifactUUID, name string) error
	ListByArtifactKind(orgName, kind string) ([]models.StoredAPIKey, error)
}

// APIKeyRepo implements APIKeyRepository using GORM
type APIKeyRepo struct {
	db *gorm.DB
}

// NewAPIKeyRepo creates a new API key repository
func NewAPIKeyRepo(db *gorm.DB) *APIKeyRepo {
	return &APIKeyRepo{db: db}
}

// Upsert creates or updates an API key record
func (r *APIKeyRepo) Upsert(key *models.StoredAPIKey) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "artifact_uuid"}, {Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"api_key_hash", "masked_api_key", "status", "updated_at", "expires_at"}),
	}).Create(key).Error
}

// Delete removes an API key by artifact UUID and key name
func (r *APIKeyRepo) Delete(artifactUUID, name string) error {
	return r.db.Where("artifact_uuid = ? AND name = ?", artifactUUID, name).
		Delete(&models.StoredAPIKey{}).Error
}

// ListByArtifactKind returns all active API keys for artifacts of a given kind (e.g., "LlmProvider", "LlmProxy")
func (r *APIKeyRepo) ListByArtifactKind(orgName, kind string) ([]models.StoredAPIKey, error) {
	var keys []models.StoredAPIKey
	err := r.db.
		Joins("JOIN artifacts a ON api_keys.artifact_uuid = a.uuid").
		Where("a.organization_name = ? AND a.kind = ?", orgName, kind).
		Find(&keys).Error
	return keys, err
}

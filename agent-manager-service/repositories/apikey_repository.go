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
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// APIKeyRepository defines the interface for API key persistence
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/apikey_repository_mock.go . APIKeyRepository:APIKeyRepositoryMock
type APIKeyRepository interface {
	Upsert(key *models.StoredAPIKey) error
	// Insert stores key as a new row, and returns utils.ErrAPIKeyNameTaken when the artifact already has a key of
	// that name, so a caller can claim a name it read as free without racing another request for it.
	Insert(key *models.StoredAPIKey) error
	// ReplaceIfUnchanged gives the stored key key.Name of key.ArtifactUUID the value, status, update time and expiry
	// of key, only while its hash is still prevHash. It returns utils.ErrAPIKeyChanged when another request
	// rotated or removed the key first.
	ReplaceIfUnchanged(key *models.StoredAPIKey, prevHash string) error
	Delete(artifactUUID, name string) error
	ListByArtifact(ctx context.Context, artifactUUID string) ([]models.StoredAPIKey, error)
	ListByArtifactKind(ouID, kind string) ([]models.StoredAPIKey, error)
	// ListByArtifactKindAndEnvs returns all active API keys for artifacts of any of the given kinds
	// whose handle encodes one of the supplied environment UUIDs (handle format: "project/agent/<envUUID>").
	// Used by the gateway bulk-sync path to scope agent keys to the gateway's own environments.
	ListByArtifactKindAndEnvs(ouID string, kinds []string, envUUIDs []string) ([]models.StoredAPIKey, error)
	// ListPermanentByArtifactKinds returns only user-managed (permanent) keys.
	// Used by the Credentials list so console-managed test keys stay hidden.
	ListPermanentByArtifactKinds(ouID string, kinds []string) ([]models.StoredAPIKey, error)
	// GetByArtifactAndName returns gorm.ErrRecordNotFound when no row matches.
	GetByArtifactAndName(artifactUUID, name string) (*models.StoredAPIKey, error)
	// ListByApplicationUUID returns all API keys whose artifact_uuid matches the given AI application UUID.
	// Used by the gateway bulk-sync path to populate per-application key bindings.
	ListByApplicationUUID(applicationUUID string) ([]models.StoredAPIKey, error)
	// ListActiveByArtifactTx returns the artifact's keys a gateway would still accept at now.
	ListActiveByArtifactTx(ctx context.Context, tx *gorm.DB, artifactUUID uuid.UUID, now time.Time) ([]models.StoredAPIKey, error)
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

// Insert stores key as a new row; see APIKeyRepository.Insert.
func (r *APIKeyRepo) Insert(key *models.StoredAPIKey) error {
	err := r.db.Create(key).Error
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return fmt.Errorf("%w: %q", utils.ErrAPIKeyNameTaken, key.Name)
	}
	return err
}

// ReplaceIfUnchanged replaces the key's value only while its hash is prevHash; see APIKeyRepository.ReplaceIfUnchanged.
func (r *APIKeyRepo) ReplaceIfUnchanged(key *models.StoredAPIKey, prevHash string) error {
	res := r.db.Model(&models.StoredAPIKey{}).
		Where("artifact_uuid = ? AND name = ? AND api_key_hash = ?", key.ArtifactUUID, key.Name, prevHash).
		Updates(map[string]any{
			"api_key_hash":   key.APIKeyHash,
			"masked_api_key": key.MaskedAPIKey,
			"status":         key.Status,
			"updated_at":     key.UpdatedAt,
			"expires_at":     key.ExpiresAt,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("%w: %q", utils.ErrAPIKeyChanged, key.Name)
	}
	return nil
}

// Delete removes an API key by artifact UUID and key name
func (r *APIKeyRepo) Delete(artifactUUID, name string) error {
	return r.db.Where("artifact_uuid = ? AND name = ?", artifactUUID, name).
		Delete(&models.StoredAPIKey{}).Error
}

// ListByArtifact returns all persisted API keys for a single artifact UUID.
func (r *APIKeyRepo) ListByArtifact(ctx context.Context, artifactUUID string) ([]models.StoredAPIKey, error) {
	var keys []models.StoredAPIKey
	err := r.db.WithContext(ctx).Where("artifact_uuid = ?", artifactUUID).Find(&keys).Error
	return keys, err
}

// ListByArtifactKind returns all active API keys for artifacts of a given kind (e.g., "LlmProvider", "LlmProxy").
// Used by the gateway bulk-sync path — must include test keys so the gateway can enforce them.
func (r *APIKeyRepo) ListByArtifactKind(ouID, kind string) ([]models.StoredAPIKey, error) {
	var keys []models.StoredAPIKey
	err := r.db.
		Joins("JOIN artifacts a ON api_keys.artifact_uuid = a.uuid").
		Where("a.ou_id = ? AND a.kind = ?", ouID, kind).
		Find(&keys).Error
	return keys, err
}

// ListByArtifactKindAndEnvs returns all active API keys for artifacts whose handle ends with
// "/<envUUID>" for any of the provided environment UUIDs. Empty envUUIDs returns nil, nil.
func (r *APIKeyRepo) ListByArtifactKindAndEnvs(ouID string, kinds []string, envUUIDs []string) ([]models.StoredAPIKey, error) {
	if len(envUUIDs) == 0 {
		return nil, nil
	}
	query := r.db.
		Joins("JOIN artifacts a ON api_keys.artifact_uuid = a.uuid").
		Where("a.ou_id = ? AND a.kind IN ?", ouID, kinds)

	// Build OR conditions matching the environment UUID suffix in the artifact handle.
	// Artifact handle format: "projectName/agentName/<environmentUUID>"
	orCond := r.db.Where("a.handle LIKE ?", "%/"+envUUIDs[0])
	for _, envUUID := range envUUIDs[1:] {
		orCond = orCond.Or("a.handle LIKE ?", "%/"+envUUID)
	}

	var keys []models.StoredAPIKey
	err := query.Where(orCond).Find(&keys).Error
	return keys, err
}

// ListPermanentByArtifactKinds returns only user-managed permanent keys.
func (r *APIKeyRepo) ListPermanentByArtifactKinds(ouID string, kinds []string) ([]models.StoredAPIKey, error) {
	var keys []models.StoredAPIKey
	err := r.db.
		Joins("JOIN artifacts a ON api_keys.artifact_uuid = a.uuid").
		Where("a.ou_id = ? AND a.kind IN ? AND api_keys.purpose = ?",
			ouID, kinds, models.APIKeyPurposeUserManaged).
		Find(&keys).Error
	return keys, err
}

// GetByArtifactAndName returns gorm.ErrRecordNotFound when no row matches; other non-nil errors are real failures.
func (r *APIKeyRepo) GetByArtifactAndName(artifactUUID, name string) (*models.StoredAPIKey, error) {
	var key models.StoredAPIKey
	err := r.db.Where("artifact_uuid = ? AND name = ?", artifactUUID, name).First(&key).Error
	if err != nil {
		return nil, err
	}
	return &key, nil
}

// ListByApplicationUUID returns all API keys whose artifact_uuid matches the given AI application UUID.
func (r *APIKeyRepo) ListByApplicationUUID(applicationUUID string) ([]models.StoredAPIKey, error) {
	var keys []models.StoredAPIKey
	err := r.db.Where("artifact_uuid = ?", applicationUUID).Find(&keys).Error
	return keys, err
}

// ListActiveByArtifactTx returns the artifact's active keys that have not expired at now, oldest first. The rows are
// held FOR SHARE until tx commits, so a concurrent revoke waits for the keys tx publishes instead of racing them.
func (r *APIKeyRepo) ListActiveByArtifactTx(
	ctx context.Context, tx *gorm.DB, artifactUUID uuid.UUID, now time.Time,
) ([]models.StoredAPIKey, error) {
	var keys []models.StoredAPIKey
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "SHARE"}).
		Where("artifact_uuid = ? AND status = 'active' AND (expires_at IS NULL OR expires_at > ?)", artifactUUID, now).
		Order("created_at, uuid").
		Find(&keys).Error
	return keys, err
}

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

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// EnvThunderURLRepository defines data access for per-environment env-Thunder
// URL handles.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/env_thunder_url_repository_mock.go . EnvThunderURLRepository:EnvThunderURLRepositoryMock
type EnvThunderURLRepository interface {
	// Get returns the handle record for (ouID, envName), or
	// (nil, gorm.ErrRecordNotFound) if none exists.
	Get(ctx context.Context, ouID, envName string) (*models.EnvThunderURL, error)
	// Insert atomically CREATES the handle record for (rec.OUID, rec.EnvName) —
	// insert-only, deliberately with NO "ON CONFLICT ... DO UPDATE": Thunder's
	// issuer is immutable once minted, so two requests racing to provision the
	// SAME (ouID, envName) for the first time must never have the second one
	// silently overwrite the first's already-committed handle. Returns:
	//   - nil: rec was inserted; this call won the claim.
	//   - utils.ErrEnvThunderURLAlreadyClaimed: a DIFFERENT concurrent request
	//     already inserted a row for this exact (OUID, EnvName) first — the
	//     caller should Get the row and adopt/reject whatever handle won,
	//     never retry this same insert.
	//   - utils.ErrThunderHandleTaken: rec.ThunderHandle is already registered
	//     to a DIFFERENT (OUID, EnvName) — a genuine cross-environment
	//     collision, unrelated to concurrency.
	//   - utils.ErrThunderURLTaken: rec.ThunderURL is already registered to a
	//     DIFFERENT (OUID, EnvName) — same idea, for a caller-supplied url
	//     that has no handle to collide on instead.
	Insert(ctx context.Context, rec *models.EnvThunderURL) error
	// Delete removes the handle record for (ouID, envName). Deleting a
	// non-existent row is not an error.
	Delete(ctx context.Context, ouID, envName string) error
	// ExistsByHandle reports whether handle is already registered to ANY
	// (ouID, envName) — the handle is globally unique across the whole
	// platform (see uq_env_thunder_urls_handle), so this is deliberately not
	// scoped to a single environment. Used for a pre-flight availability
	// check so a caller can be told a handle is taken before ever attempting
	// to claim it (Insert already enforces this atomically; this is a
	// non-authoritative, racy-by-nature read for UX purposes only).
	ExistsByHandle(ctx context.Context, handle string) (bool, error)
}

type envThunderURLRepo struct {
	db *gorm.DB
}

// NewEnvThunderURLRepo creates a new EnvThunderURLRepository.
func NewEnvThunderURLRepo(db *gorm.DB) EnvThunderURLRepository {
	return &envThunderURLRepo{db: db}
}

func (r *envThunderURLRepo) Get(ctx context.Context, ouID, envName string) (*models.EnvThunderURL, error) {
	var rec models.EnvThunderURL
	result := r.db.WithContext(ctx).Where("ou_id = ? AND env_name = ?", ouID, envName).First(&rec)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, result.Error
	}
	return &rec, nil
}

func (r *envThunderURLRepo) Insert(ctx context.Context, rec *models.EnvThunderURL) error {
	err := r.db.WithContext(ctx).Create(rec).Error
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		// Distinguish WHICH unique constraint fired: (ou_id, env_name) means a
		// concurrent request beat this one to the SAME environment (a race,
		// not a real conflict — the caller adopts the winner); thunder_handle
		// or thunder_url means that exact value is already owned by a
		// DIFFERENT environment (a genuine collision, one per registration
		// path — see EnvironmentService.SetThunderURL).
		switch pgErr.ConstraintName {
		case "uq_env_thunder_urls_ou_env":
			return utils.ErrEnvThunderURLAlreadyClaimed
		case "uq_env_thunder_urls_handle":
			return utils.ErrThunderHandleTaken
		case "uq_env_thunder_urls_url":
			return utils.ErrThunderURLTaken
		default:
			return err
		}
	}
	return err
}

func (r *envThunderURLRepo) Delete(ctx context.Context, ouID, envName string) error {
	return r.db.WithContext(ctx).Where("ou_id = ? AND env_name = ?", ouID, envName).
		Delete(&models.EnvThunderURL{}).Error
}

func (r *envThunderURLRepo) ExistsByHandle(ctx context.Context, handle string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.EnvThunderURL{}).
		Where("thunder_handle = ?", handle).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

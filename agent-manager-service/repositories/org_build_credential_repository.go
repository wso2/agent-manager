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

// OrgBuildCredentialRepository defines the interface for per-org build client data access
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/org_build_credential_repository_mock.go . OrgBuildCredentialRepository:OrgBuildCredentialRepositoryMock
type OrgBuildCredentialRepository interface {
	GetByOrgName(ouID string) (*models.OrgBuildCredential, error)
	Upsert(cred *models.OrgBuildCredential) error
	// WithProvisioningLock runs fn while holding an org-scoped advisory lock, so
	// only one replica provisions an organization's build client at a time. The
	// lock is a transaction-scoped Postgres advisory lock, released when fn returns.
	WithProvisioningLock(ctx context.Context, ouID string, fn func() error) error
}

type orgBuildCredentialRepo struct {
	db *gorm.DB
}

// NewOrgBuildCredentialRepo creates a new OrgBuildCredentialRepository
func NewOrgBuildCredentialRepo(db *gorm.DB) OrgBuildCredentialRepository {
	return &orgBuildCredentialRepo{db: db}
}

// GetByOrgName returns the build credential for the given org.
// Returns (nil, gorm.ErrRecordNotFound) if no record exists; returns (nil, err) on other DB errors.
func (r *orgBuildCredentialRepo) GetByOrgName(ouID string) (*models.OrgBuildCredential, error) {
	var cred models.OrgBuildCredential
	result := r.db.Where("ou_id = ?", ouID).First(&cred)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, result.Error
	}
	return &cred, nil
}

// Upsert atomically creates or updates the build credential for an org.
func (r *orgBuildCredentialRepo) Upsert(cred *models.OrgBuildCredential) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "ou_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"org_uuid", "client_id", "app_id", "secret_ref_name", "updated_at"}),
	}).Create(cred).Error
}

// WithProvisioningLock holds pg_advisory_xact_lock(buildCredentialLockNamespace,
// hashtext(ouID)) for the duration of fn. The lock spans the identity provider
// and secret store calls fn makes: provisioning runs once per organization, and
// without it two replicas can each regenerate the client secret and store a copy
// the other has already invalidated. This is a deliberate exception to "never
// hold a lock across I/O": it is per organization, taken only while no build
// client is stored, and bounded by the caller's timeout; it holds one pool
// connection for that time.
func (r *orgBuildCredentialRepo) WithProvisioningLock(ctx context.Context, ouID string, fn func() error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := advisoryXactLock(tx, buildCredentialLockNamespace, ouID); err != nil {
			return err
		}
		return fn()
	})
}

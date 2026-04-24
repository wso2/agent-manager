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
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// OrgPublisherCredentialRepository defines the interface for per-org publisher credential data access
type OrgPublisherCredentialRepository interface {
	GetByOrgName(orgName string) (*models.OrgPublisherCredential, error)
	Upsert(cred *models.OrgPublisherCredential) error
}

type orgPublisherCredentialRepo struct {
	db *gorm.DB
}

// NewOrgPublisherCredentialRepo creates a new OrgPublisherCredentialRepository
func NewOrgPublisherCredentialRepo(db *gorm.DB) OrgPublisherCredentialRepository {
	return &orgPublisherCredentialRepo{db: db}
}

// GetByOrgName returns the publisher credentials for the given org.
// Returns (nil, gorm.ErrRecordNotFound) if no record exists; returns (nil, err) on other DB errors.
func (r *orgPublisherCredentialRepo) GetByOrgName(orgName string) (*models.OrgPublisherCredential, error) {
	var cred models.OrgPublisherCredential
	result := r.db.Where("org_name = ?", orgName).First(&cred)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, result.Error
	}
	return &cred, nil
}

// Upsert atomically creates or updates publisher credentials for an org.
func (r *orgPublisherCredentialRepo) Upsert(cred *models.OrgPublisherCredential) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "org_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"org_uuid", "client_id", "secret_kv_path", "secret_key", "updated_at"}),
	}).Create(cred).Error
}

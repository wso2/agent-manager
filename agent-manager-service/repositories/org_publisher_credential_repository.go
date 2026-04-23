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
	"gorm.io/gorm"

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

// GetByOrgName returns the publisher credentials for the given org, or nil if not found.
func (r *orgPublisherCredentialRepo) GetByOrgName(orgName string) (*models.OrgPublisherCredential, error) {
	var cred models.OrgPublisherCredential
	result := r.db.Where("org_name = ?", orgName).First(&cred)
	if result.Error != nil {
		return nil, result.Error
	}
	return &cred, nil
}

// Upsert creates or updates publisher credentials for an org.
func (r *orgPublisherCredentialRepo) Upsert(cred *models.OrgPublisherCredential) error {
	return r.db.Save(cred).Error
}

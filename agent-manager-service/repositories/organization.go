// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
	"fmt"

	"github.com/google/uuid"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

type OrganizationRepository interface {
	GetOrganizationsByUserIdpID(ctx context.Context, userIdpID uuid.UUID) ([]models.Organization, error)
	CreateOrganization(ctx context.Context, organization *models.Organization) error
	GetOrganizationByOrgName(ctx context.Context, userIdpID uuid.UUID, orgName string) (*models.Organization, error)
}

type organizationRepository struct{}

func NewOrganizationRepository() OrganizationRepository {
	return &organizationRepository{}
}

func (r *organizationRepository) GetOrganizationsByUserIdpID(ctx context.Context, userIdpID uuid.UUID) ([]models.Organization, error) {
	var orgs []models.Organization
	if err := db.DB(ctx).Where("user_idp_id = ?", userIdpID).Order("created_at DESC").Find(&orgs).Error; err != nil {
		return nil, fmt.Errorf("organizationRepository.GetOrganizationsByUserIdpID: %w", err)
	}
	return orgs, nil
}

func (r *organizationRepository) CreateOrganization(ctx context.Context, organization *models.Organization) error {
	if err := db.DB(ctx).Create(organization).Error; err != nil {
		return fmt.Errorf("organizationRepository.CreateOrganization: %w", err)
	}
	return nil
}

func (r *organizationRepository) GetOrganizationByOrgName(ctx context.Context, userIdpID uuid.UUID, orgName string) (*models.Organization, error) {
	var org models.Organization
	if err := db.DB(ctx).Where("user_idp_id = ? AND org_name = ?", userIdpID, orgName).First(&org).Error; err != nil {
		return nil, fmt.Errorf("organizationRepository.GetOrganizationByOrgName: %w", err)
	}
	return &org, nil
}

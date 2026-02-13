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
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// APIRepository defines the interface for API data operations
type APIRepository interface {
	CreateAPI(api *models.API) error
	GetAPIByUUID(apiUUID, orgUUID string) (*models.API, error)
	GetAPIMetadataByHandle(handle, orgUUID string) (*models.APIMetadata, error)
	GetAPIsByProjectUUID(projectUUID, orgUUID string) ([]*models.API, error)
	GetAPIsByOrganizationUUID(orgUUID string, projectUUID *string) ([]*models.API, error)
	GetAPIsByGatewayUUID(gatewayUUID, orgUUID string) ([]*models.API, error)
	GetDeployedAPIsByGatewayUUID(gatewayUUID, orgUUID string) ([]*models.API, error)
	UpdateAPI(api *models.API) error
	DeleteAPI(apiUUID, orgUUID string) error

	// API-Gateway association methods
	GetAPIGatewaysWithDetails(apiUUID, orgUUID string) ([]*models.APIGatewayWithDetails, error)

	// Unified API association methods (supports both gateways and dev portals)
	CreateAPIAssociation(association *models.APIAssociation) error
	GetAPIAssociations(apiUUID, associationType, orgUUID string) ([]*models.APIAssociation, error)
	UpdateAPIAssociation(apiUUID, resourceId, associationType, orgUUID string) error

	// API name validation methods
	CheckAPIExistsByHandleInOrganization(handle, orgUUID string) (bool, error)
	CheckAPIExistsByNameAndVersionInOrganization(name, version, orgUUID, excludeHandle string) (bool, error)
}

// APIRepo implements APIRepository using GORM
type APIRepo struct {
	db           *gorm.DB
	artifactRepo ArtifactRepository
}

// NewAPIRepo creates a new API repository
func NewAPIRepo(db *gorm.DB) APIRepository {
	return &APIRepo{
		db:           db,
		artifactRepo: NewArtifactRepo(db),
	}
}

// CreateAPI inserts a new API with all its configurations
func (r *APIRepo) CreateAPI(api *models.API) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Always generate a new UUID for the API
		apiUUID := uuid.New()
		api.ID = apiUUID.String()
		api.CreatedAt = time.Now()
		api.UpdatedAt = time.Now()

		// Determine kind
		kind := models.KindRestAPI
		if api.Kind == models.KindWebSubAPI {
			kind = models.KindWebSubAPI
		}

		// Parse organization and project UUIDs
		orgUUID, err := uuid.Parse(api.OrganizationID)
		if err != nil {
			return fmt.Errorf("invalid organization UUID: %w", err)
		}
		projectUUID, err := uuid.Parse(api.ProjectID)
		if err != nil {
			return fmt.Errorf("invalid project UUID: %w", err)
		}

		// Create artifact first
		if err := r.artifactRepo.Create(tx, &models.Artifact{
			UUID:             apiUUID,
			Handle:           api.Handle,
			Name:             api.Name,
			Version:          api.Version,
			Kind:             kind,
			OrganizationUUID: orgUUID,
			CreatedAt:        api.CreatedAt,
			UpdatedAt:        api.UpdatedAt,
		}); err != nil {
			return err
		}

		// Create RestAPI record
		return tx.Create(&models.RestAPI{
			UUID:            apiUUID,
			Description:     api.Description,
			CreatedBy:       api.CreatedBy,
			ProjectUUID:     projectUUID,
			LifecycleStatus: api.LifeCycleStatus,
			Transport:       api.Transport,
			Configuration:   api.Configuration,
		}).Error
	})
}

// GetAPIByUUID retrieves an API by UUID with all its configurations
func (r *APIRepo) GetAPIByUUID(apiUUID, orgUUID string) (*models.API, error) {
	var api models.API
	err := r.db.Table("rest_apis a").
		Select("art.uuid, art.handle, art.name, art.kind, a.description, art.version, a.created_by, "+
			"a.project_uuid, art.organization_uuid, a.lifecycle_status, "+
			"a.transport, a.configuration, art.created_at, art.updated_at").
		Joins("INNER JOIN artifacts art ON a.uuid = art.uuid").
		Where("a.uuid = ? AND art.organization_uuid = ?", apiUUID, orgUUID).
		Scan(&api).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return &api, nil
}

// GetAPIMetadataByHandle retrieves minimal API information by handle and organization ID
func (r *APIRepo) GetAPIMetadataByHandle(handle, orgUUID string) (*models.APIMetadata, error) {
	var metadata models.APIMetadata
	err := r.db.Table("artifacts").
		Select("uuid, handle, name, version, kind, organization_uuid").
		Where("handle = ? AND organization_uuid = ?", handle, orgUUID).
		Scan(&metadata).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return &metadata, nil
}

// GetAPIsByProjectUUID retrieves all APIs for a project
func (r *APIRepo) GetAPIsByProjectUUID(projectUUID, orgUUID string) ([]*models.API, error) {
	var apis []*models.API
	err := r.db.Table("rest_apis a").
		Select("art.uuid, art.handle, art.name, art.kind, a.description, art.version, a.created_by, "+
			"a.project_uuid, art.organization_uuid, a.lifecycle_status, "+
			"a.transport, a.configuration, art.created_at, art.updated_at").
		Joins("INNER JOIN artifacts art ON a.uuid = art.uuid").
		Where("a.project_uuid = ? AND art.organization_uuid = ?", projectUUID, orgUUID).
		Order("art.created_at DESC").
		Scan(&apis).Error
	return apis, err
}

// GetAPIsByOrganizationUUID retrieves all APIs for an organization with optional project filter
func (r *APIRepo) GetAPIsByOrganizationUUID(orgUUID string, projectUUID *string) ([]*models.API, error) {
	var apis []*models.API
	query := r.db.Table("rest_apis a").
		Select("art.uuid, art.handle, art.name, art.kind, a.description, art.version, a.created_by, "+
			"a.project_uuid, art.organization_uuid, a.lifecycle_status, "+
			"a.transport, a.configuration, art.created_at, art.updated_at").
		Joins("INNER JOIN artifacts art ON a.uuid = art.uuid").
		Where("art.organization_uuid = ?", orgUUID)

	if projectUUID != nil && *projectUUID != "" {
		query = query.Where("a.project_uuid = ?", *projectUUID)
	}

	err := query.Order("art.created_at DESC").Scan(&apis).Error
	return apis, err
}

// GetDeployedAPIsByGatewayUUID retrieves all APIs deployed to a specific gateway
func (r *APIRepo) GetDeployedAPIsByGatewayUUID(gatewayUUID, orgUUID string) ([]*models.API, error) {
	var apis []*models.API
	err := r.db.Table("rest_apis a").
		Select("a.uuid, art.name, a.description, art.version, a.created_by, "+
			"a.project_uuid, art.organization_uuid, art.kind, art.created_at, art.updated_at").
		Joins("INNER JOIN artifacts art ON a.uuid = art.uuid").
		Joins("INNER JOIN deployment_status ad ON art.uuid = ad.artifact_uuid").
		Where("ad.gateway_uuid = ? AND art.organization_uuid = ? AND ad.status = ?",
			gatewayUUID, orgUUID, string(models.DeploymentStatusDeployed)).
		Order("art.created_at DESC").
		Scan(&apis).Error
	return apis, err
}

// GetAPIsByGatewayUUID retrieves all APIs associated with a specific gateway
func (r *APIRepo) GetAPIsByGatewayUUID(gatewayUUID, orgUUID string) ([]*models.API, error) {
	var apis []*models.API
	err := r.db.Table("rest_apis a").
		Select("a.uuid, art.name, a.description, art.version, a.created_by, "+
			"a.project_uuid, art.organization_uuid, art.kind, art.created_at, art.updated_at").
		Joins("INNER JOIN artifacts art ON a.uuid = art.uuid").
		Joins("INNER JOIN association_mappings aa ON a.uuid = aa.artifact_uuid").
		Where("aa.resource_uuid = ? AND aa.association_type = ? AND art.organization_uuid = ?",
			gatewayUUID, "gateway", orgUUID).
		Order("art.created_at DESC").
		Scan(&apis).Error
	return apis, err
}

// UpdateAPI modifies an existing API
func (r *APIRepo) UpdateAPI(api *models.API) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		api.UpdatedAt = time.Now()

		// Parse UUIDs
		apiUUID, err := uuid.Parse(api.ID)
		if err != nil {
			return fmt.Errorf("invalid API UUID: %w", err)
		}
		orgUUID, err := uuid.Parse(api.OrganizationID)
		if err != nil {
			return fmt.Errorf("invalid organization UUID: %w", err)
		}

		// Update artifact record
		if err := r.artifactRepo.Update(tx, &models.Artifact{
			UUID:             apiUUID,
			Name:             api.Name,
			Version:          api.Version,
			OrganizationUUID: orgUUID,
			UpdatedAt:        api.UpdatedAt,
		}); err != nil {
			return err
		}

		// Update main API record
		return tx.Model(&models.RestAPI{}).
			Where("uuid = ?", apiUUID).
			Updates(map[string]interface{}{
				"description":      api.Description,
				"created_by":       api.CreatedBy,
				"lifecycle_status": api.LifeCycleStatus,
				"transport":        api.Transport,
				"configuration":    api.Configuration,
			}).Error
	})
}

// DeleteAPI removes an API and all its configurations
func (r *APIRepo) DeleteAPI(apiUUID, orgUUID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Delete in order of dependencies (children first, parent last)

		// Delete API associations first
		if err := tx.Where("artifact_uuid = ? AND organization_uuid = ?", apiUUID, orgUUID).
			Delete(&models.APIAssociation{}).Error; err != nil {
			return err
		}

		// Delete API deployments
		if err := tx.Where("artifact_uuid = ? AND organization_uuid = ?", apiUUID, orgUUID).
			Delete(&models.Deployment{}).Error; err != nil {
			return err
		}

		// Delete from rest_apis table first
		if err := tx.Where("uuid = ?", apiUUID).Delete(&models.RestAPI{}).Error; err != nil {
			return err
		}

		// Delete from artifacts table
		return r.artifactRepo.Delete(tx, apiUUID)
	})
}

// CheckAPIExistsByHandleInOrganization checks if an API with the given handle exists within a specific organization
func (r *APIRepo) CheckAPIExistsByHandleInOrganization(handle, orgUUID string) (bool, error) {
	var count int64
	err := r.db.Model(&models.Artifact{}).
		Where("handle = ? AND organization_uuid = ? AND kind = ?", handle, orgUUID, models.KindRestAPI).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// CheckAPIExistsByNameAndVersionInOrganization checks if an API with the given name and version exists within a specific organization
func (r *APIRepo) CheckAPIExistsByNameAndVersionInOrganization(name, version, orgUUID, excludeHandle string) (bool, error) {
	var count int64
	query := r.db.Model(&models.Artifact{}).
		Where("name = ? AND version = ? AND organization_uuid = ?", name, version, orgUUID)

	if excludeHandle != "" {
		query = query.Where("handle != ?", excludeHandle)
	}

	err := query.Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// CreateAPIAssociation creates an association between an API and resource (e.g., gateway or dev portal)
func (r *APIRepo) CreateAPIAssociation(association *models.APIAssociation) error {
	association.CreatedAt = time.Now()
	association.UpdatedAt = time.Now()
	return r.db.Create(association).Error
}

// UpdateAPIAssociation updates the updated_at timestamp for an existing API resource association
func (r *APIRepo) UpdateAPIAssociation(apiUUID, resourceId, associationType, orgUUID string) error {
	return r.db.Model(&models.APIAssociation{}).
		Where("artifact_uuid = ? AND resource_uuid = ? AND association_type = ? AND organization_uuid = ?",
			apiUUID, resourceId, associationType, orgUUID).
		Update("updated_at", time.Now()).Error
}

// GetAPIAssociations retrieves all resource associations for an API of a specific type
func (r *APIRepo) GetAPIAssociations(apiUUID, associationType, orgUUID string) ([]*models.APIAssociation, error) {
	var associations []*models.APIAssociation
	err := r.db.Where("artifact_uuid = ? AND association_type = ? AND organization_uuid = ?",
		apiUUID, associationType, orgUUID).
		Find(&associations).Error
	return associations, err
}

// GetAPIGatewaysWithDetails retrieves all gateways associated with an API including deployment details
func (r *APIRepo) GetAPIGatewaysWithDetails(apiUUID, orgUUID string) ([]*models.APIGatewayWithDetails, error) {
	var gateways []*models.APIGatewayWithDetails
	err := r.db.Table("gateways g").
		Select("g.uuid as id, g.organization_uuid, g.name, g.display_name, g.description, g.properties, "+
			"g.vhost, g.is_critical, g.gateway_functionality_type as functionality_type, g.is_active, "+
			"g.created_at, g.updated_at, aa.created_at as associated_at, aa.updated_at as association_updated_at, "+
			"CASE WHEN ad.deployment_id IS NOT NULL THEN 1 ELSE 0 END as is_deployed, "+
			"ad.deployment_id, ad.updated_at as deployed_at").
		Joins("INNER JOIN association_mappings aa ON g.uuid = aa.resource_uuid AND aa.association_type = ?", "gateway").
		Joins("LEFT JOIN deployment_status ad ON g.uuid = ad.gateway_uuid AND ad.artifact_uuid = ? AND ad.status = ?",
			apiUUID, string(models.DeploymentStatusDeployed)).
		Where("aa.artifact_uuid = ? AND g.organization_uuid = ?", apiUUID, orgUUID).
		Order("aa.created_at DESC").
		Scan(&gateways).Error
	return gateways, err
}

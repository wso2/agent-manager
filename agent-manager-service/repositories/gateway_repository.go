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
	"time"

	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// GatewayFilterOptions defines filtering options for gateway queries
type GatewayFilterOptions struct {
	OrganizationID    string
	FunctionalityType *string // Filter by gateway_functionality_type (ai, regular, event)
	Status            *bool   // Filter by is_active
	EnvironmentID     *string // Filter by environment (via gateway_environment_mappings)
	Limit             int     // Pagination limit (0 = no limit)
	Offset            int     // Pagination offset
}

// GatewayRepository defines the interface for gateway data access
type GatewayRepository interface {
	// Gateway operations
	Create(gateway *models.Gateway) error
	GetByUUID(gatewayId string) (*models.Gateway, error)
	GetByOrganizationID(orgID string) ([]*models.Gateway, error)
	GetByNameAndOrgID(name, orgID string) (*models.Gateway, error)
	List() ([]*models.Gateway, error)
	ListWithFilters(filters GatewayFilterOptions) ([]*models.Gateway, error)
	CountWithFilters(filters GatewayFilterOptions) (int64, error)
	Delete(gatewayID, organizationID string) error
	UpdateGateway(gateway *models.Gateway) error
	UpdateActiveStatus(gatewayId string, isActive bool) error

	// Gateway association checking operations
	HasGatewayDeployments(gatewayID, organizationID string) (bool, error)
	HasGatewayAssociations(gatewayID, organizationID string) (bool, error)
	HasGatewayAssociationsOrDeployments(gatewayID, organizationID string) (bool, error)

	// Token operations
	CreateToken(token *models.GatewayToken) error
	GetActiveTokensByGatewayUUID(gatewayId string) ([]*models.GatewayToken, error)
	GetTokenByUUID(tokenId string) (*models.GatewayToken, error)
	GetActiveTokenByPrefix(tokenPrefix string) (*models.GatewayToken, error)
	RevokeToken(tokenId string) error
	CountActiveTokens(gatewayId string) (int, error)

	// Gateway-Environment mapping operations
	CreateEnvironmentMapping(mapping *models.GatewayEnvironmentMapping) error
	DeleteEnvironmentMapping(gatewayID, environmentID string) error
	GetEnvironmentMappingsByGatewayID(gatewayID string) ([]models.GatewayEnvironmentMapping, error)
	GetEnvironmentMappingsByGatewayIDs(gatewayIDs []string) (map[string][]models.GatewayEnvironmentMapping, error)
	EnvironmentMappingExists(gatewayID, environmentID string) (bool, error)
}

// GatewayRepo implements GatewayRepository using GORM
type GatewayRepo struct {
	db *gorm.DB
}

// NewGatewayRepo creates a new gateway repository
func NewGatewayRepo(db *gorm.DB) GatewayRepository {
	return &GatewayRepo{db: db}
}

// Create inserts a new gateway
func (r *GatewayRepo) Create(gateway *models.Gateway) error {
	gateway.CreatedAt = time.Now()
	gateway.UpdatedAt = time.Now()
	gateway.IsActive = false // Set default value to false at registration
	return r.db.Create(gateway).Error
}

// GetByUUID retrieves a gateway by ID
func (r *GatewayRepo) GetByUUID(gatewayId string) (*models.Gateway, error) {
	var gateway models.Gateway
	err := r.db.Where("uuid = ?", gatewayId).First(&gateway).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return &gateway, nil
}

// GetByOrganizationID retrieves all gateways for an organization
func (r *GatewayRepo) GetByOrganizationID(orgID string) ([]*models.Gateway, error) {
	var gateways []*models.Gateway
	err := r.db.Where("organization_uuid = ?", orgID).
		Order("created_at DESC").
		Find(&gateways).Error
	return gateways, err
}

// GetByNameAndOrgID checks if a gateway with the given name exists within an organization
func (r *GatewayRepo) GetByNameAndOrgID(name, orgID string) (*models.Gateway, error) {
	var gateway models.Gateway
	err := r.db.Where("name = ? AND organization_uuid = ?", name, orgID).First(&gateway).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrGatewayNotFound
		}
		return nil, err
	}
	return &gateway, nil
}

// List retrieves all gateways
func (r *GatewayRepo) List() ([]*models.Gateway, error) {
	var gateways []*models.Gateway
	err := r.db.Order("created_at DESC").Find(&gateways).Error
	return gateways, err
}

// ListWithFilters retrieves gateways with optional filtering and pagination
func (r *GatewayRepo) ListWithFilters(filters GatewayFilterOptions) ([]*models.Gateway, error) {
	query := r.buildFilterQuery(filters)

	// Apply pagination at database level
	if filters.Limit > 0 {
		query = query.Limit(filters.Limit)
	}
	if filters.Offset > 0 {
		query = query.Offset(filters.Offset)
	}

	var gateways []*models.Gateway
	err := query.Order("gateways.created_at DESC").Find(&gateways).Error
	return gateways, err
}

// CountWithFilters counts gateways matching the filter criteria (excluding pagination)
func (r *GatewayRepo) CountWithFilters(filters GatewayFilterOptions) (int64, error) {
	// Create query without pagination
	filterCopy := filters
	filterCopy.Limit = 0
	filterCopy.Offset = 0
	query := r.buildFilterQuery(filterCopy)

	var count int64
	err := query.Count(&count).Error
	return count, err
}

// buildFilterQuery constructs the base query with all filters applied
func (r *GatewayRepo) buildFilterQuery(filters GatewayFilterOptions) *gorm.DB {
	query := r.db.Model(&models.Gateway{})

	// Filter by organization
	if filters.OrganizationID != "" {
		query = query.Where("organization_uuid = ?", filters.OrganizationID)
	}

	// Filter by functionality type
	if filters.FunctionalityType != nil && *filters.FunctionalityType != "" {
		query = query.Where("gateway_functionality_type = ?", *filters.FunctionalityType)
	}

	// Filter by status (is_active)
	if filters.Status != nil {
		query = query.Where("is_active = ?", *filters.Status)
	}

	// Filter by environment (via gateway_environment_mappings)
	if filters.EnvironmentID != nil && *filters.EnvironmentID != "" {
		query = query.Joins("INNER JOIN gateway_environment_mappings ON gateway_environment_mappings.gateway_uuid = gateways.uuid").
			Where("gateway_environment_mappings.environment_uuid = ?", *filters.EnvironmentID).
			Distinct()
	}

	return query
}

// Delete removes a gateway with organization isolation and cleans up all associations
func (r *GatewayRepo) Delete(gatewayID, organizationID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Delete API associations for this gateway
		if err := tx.Where("resource_uuid = ? AND association_type = ? AND organization_uuid = ?",
			gatewayID, "gateway", organizationID).
			Delete(&models.APIAssociation{}).Error; err != nil {
			return err
		}

		// Delete gateway with organization isolation (gateway_tokens and deployments will be cascade deleted via FK)
		result := tx.Where("uuid = ? AND organization_uuid = ?", gatewayID, organizationID).
			Delete(&models.Gateway{})
		if result.Error != nil {
			return result.Error
		}

		// Check if gateway was actually deleted
		if result.RowsAffected == 0 {
			return utils.ErrGatewayNotFound
		}

		return nil
	})
}

// UpdateGateway updates gateway details
func (r *GatewayRepo) UpdateGateway(gateway *models.Gateway) error {
	gateway.UpdatedAt = time.Now()
	res := r.db.Model(&models.Gateway{}).
		Where("uuid = ?", gateway.UUID).
		Updates(map[string]interface{}{
			"display_name": gateway.DisplayName,
			"description":  gateway.Description,
			"is_critical":  gateway.IsCritical,
			"properties":   gateway.Properties,
			"updated_at":   gateway.UpdatedAt,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateActiveStatus updates the is_active status of a gateway
func (r *GatewayRepo) UpdateActiveStatus(gatewayId string, isActive bool) error {
	res := r.db.Model(&models.Gateway{}).
		Where("uuid = ?", gatewayId).
		Updates(map[string]interface{}{
			"is_active":  isActive,
			"updated_at": time.Now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// CreateToken inserts a new token
func (r *GatewayRepo) CreateToken(token *models.GatewayToken) error {
	token.CreatedAt = time.Now()
	return r.db.Create(token).Error
}

// GetActiveTokensByGatewayUUID retrieves all active tokens for a gateway
func (r *GatewayRepo) GetActiveTokensByGatewayUUID(gatewayId string) ([]*models.GatewayToken, error) {
	var tokens []*models.GatewayToken
	err := r.db.Where("gateway_uuid = ? AND status = ?", gatewayId, "active").
		Order("created_at DESC").
		Find(&tokens).Error
	return tokens, err
}

// GetTokenByUUID retrieves a specific token by UUID
func (r *GatewayRepo) GetTokenByUUID(tokenId string) (*models.GatewayToken, error) {
	var token models.GatewayToken
	err := r.db.Where("uuid = ?", tokenId).First(&token).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return &token, nil
}

// GetActiveTokenByPrefix retrieves an active token by its unique prefix (UUID)
// This enables O(1) indexed lookup instead of O(N) full table scan
func (r *GatewayRepo) GetActiveTokenByPrefix(tokenPrefix string) (*models.GatewayToken, error) {
	var token models.GatewayToken
	err := r.db.Where("token_prefix = ? AND status = ?", tokenPrefix, "active").
		First(&token).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Return nil, nil for invalid token (not an error)
		}
		return nil, err
	}
	return &token, nil
}

// RevokeToken updates token status to revoked
func (r *GatewayRepo) RevokeToken(tokenId string) error {
	now := time.Now()
	result := r.db.Model(&models.GatewayToken{}).
		Where("uuid = ?", tokenId).
		Updates(map[string]interface{}{
			"status":     "revoked",
			"revoked_at": now,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// CountActiveTokens counts the number of active tokens for a gateway
func (r *GatewayRepo) CountActiveTokens(gatewayId string) (int, error) {
	var count int64
	err := r.db.Model(&models.GatewayToken{}).
		Where("gateway_uuid = ? AND status = ?", gatewayId, "active").
		Count(&count).Error
	return int(count), err
}

// HasGatewayDeployments checks if a gateway has any deployments
func (r *GatewayRepo) HasGatewayDeployments(gatewayID, organizationID string) (bool, error) {
	var count int64
	err := r.db.Model(&models.DeploymentStatusRecord{}).
		Where("gateway_uuid = ? AND organization_uuid = ? AND status = ?",
			gatewayID, organizationID, string(models.DeploymentStatusDeployed)).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// HasGatewayAssociations checks if a gateway has any associations
func (r *GatewayRepo) HasGatewayAssociations(gatewayID, organizationID string) (bool, error) {
	var count int64
	err := r.db.Model(&models.APIAssociation{}).
		Where("resource_uuid = ? AND association_type = ? AND organization_uuid = ?",
			gatewayID, "gateway", organizationID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// HasGatewayAssociationsOrDeployments checks if a gateway has any associations (deployments or associations)
func (r *GatewayRepo) HasGatewayAssociationsOrDeployments(gatewayID, organizationID string) (bool, error) {
	// Check deployments first
	hasDeployments, err := r.HasGatewayDeployments(gatewayID, organizationID)
	if err != nil {
		return false, err
	}

	if hasDeployments {
		return true, nil
	}

	// Check associations
	return r.HasGatewayAssociations(gatewayID, organizationID)
}

// CreateEnvironmentMapping creates a mapping between a gateway and an environment
func (r *GatewayRepo) CreateEnvironmentMapping(mapping *models.GatewayEnvironmentMapping) error {
	return r.db.Create(mapping).Error
}

// DeleteEnvironmentMapping deletes a mapping between a gateway and an environment
func (r *GatewayRepo) DeleteEnvironmentMapping(gatewayID, environmentID string) error {
	result := r.db.Where("gateway_uuid = ? AND environment_uuid = ?", gatewayID, environmentID).
		Delete(&models.GatewayEnvironmentMapping{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// GetEnvironmentMappingsByGatewayID retrieves all environment mappings for a gateway
func (r *GatewayRepo) GetEnvironmentMappingsByGatewayID(gatewayID string) ([]models.GatewayEnvironmentMapping, error) {
	var mappings []models.GatewayEnvironmentMapping
	err := r.db.Where("gateway_uuid = ?", gatewayID).Find(&mappings).Error
	return mappings, err
}

// GetEnvironmentMappingsByGatewayIDs retrieves environment mappings for multiple gateways in bulk
// Returns a map of gatewayID -> []GatewayEnvironmentMapping
// This avoids N+1 queries when fetching environments for multiple gateways
func (r *GatewayRepo) GetEnvironmentMappingsByGatewayIDs(gatewayIDs []string) (map[string][]models.GatewayEnvironmentMapping, error) {
	if len(gatewayIDs) == 0 {
		return make(map[string][]models.GatewayEnvironmentMapping), nil
	}

	var mappings []models.GatewayEnvironmentMapping
	err := r.db.Where("gateway_uuid IN ?", gatewayIDs).Find(&mappings).Error
	if err != nil {
		return nil, err
	}

	// Group mappings by gateway UUID
	result := make(map[string][]models.GatewayEnvironmentMapping)
	for _, mapping := range mappings {
		gwID := mapping.GatewayUUID.String()
		result[gwID] = append(result[gwID], mapping)
	}

	return result, nil
}

// EnvironmentMappingExists checks if a mapping exists between a gateway and an environment
func (r *GatewayRepo) EnvironmentMappingExists(gatewayID, environmentID string) (bool, error) {
	var count int64
	err := r.db.Model(&models.GatewayEnvironmentMapping{}).
		Where("gateway_uuid = ? AND environment_uuid = ?", gatewayID, environmentID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

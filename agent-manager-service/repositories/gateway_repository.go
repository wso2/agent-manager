/*
 *  Copyright (c) 2025, WSO2 LLC. (http://www.wso2.org) All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package repositories

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// GatewayRepository defines the interface for gateway data access
type GatewayRepository interface {
	// Gateway operations
	Create(gateway *models.Gateway) error
	GetByUUID(gatewayId string) (*models.Gateway, error)
	GetByOrganizationID(orgID string) ([]*models.Gateway, error)
	GetByNameAndOrgID(name, orgID string) (*models.Gateway, error)
	List() ([]*models.Gateway, error)
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
	RevokeToken(tokenId string) error
	CountActiveTokens(gatewayId string) (int, error)
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
			return nil, nil
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
			return nil, nil
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
			return errors.New("gateway not found")
		}

		return nil
	})
}

// UpdateGateway updates gateway details
func (r *GatewayRepo) UpdateGateway(gateway *models.Gateway) error {
	gateway.UpdatedAt = time.Now()
	return r.db.Model(&models.Gateway{}).
		Where("uuid = ?", gateway.UUID).
		Updates(map[string]interface{}{
			"display_name": gateway.DisplayName,
			"description":  gateway.Description,
			"is_critical":  gateway.IsCritical,
			"properties":   gateway.Properties,
			"updated_at":   gateway.UpdatedAt,
		}).Error
}

// UpdateActiveStatus updates the is_active status of a gateway
func (r *GatewayRepo) UpdateActiveStatus(gatewayId string, isActive bool) error {
	return r.db.Model(&models.Gateway{}).
		Where("uuid = ?", gatewayId).
		Updates(map[string]interface{}{
			"is_active":  isActive,
			"updated_at": time.Now(),
		}).Error
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
			return nil, nil
		}
		return nil, err
	}
	return &token, nil
}

// RevokeToken updates token status to revoked
func (r *GatewayRepo) RevokeToken(tokenId string) error {
	now := time.Now()
	return r.db.Model(&models.GatewayToken{}).
		Where("uuid = ?", tokenId).
		Updates(map[string]interface{}{
			"status":     "revoked",
			"revoked_at": now,
		}).Error
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

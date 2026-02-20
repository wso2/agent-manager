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
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// LLMProxyRepository defines the interface for LLM proxy persistence
type LLMProxyRepository interface {
	Create(p *models.LLMProxy, handle, name, version string, orgName string) error
	GetByID(proxyID, orgName string) (*models.LLMProxy, error)
	List(orgName string, limit, offset int) ([]*models.LLMProxy, error)
	ListByProject(orgName, projectUUID string, limit, offset int) ([]*models.LLMProxy, error)
	ListByProvider(orgName, providerUUID string, limit, offset int) ([]*models.LLMProxy, error)
	Count(orgName string) (int, error)
	CountByProject(orgName, projectUUID string) (int, error)
	CountByProvider(orgName, providerUUID string) (int, error)
	Update(p *models.LLMProxy, handle string, orgName string) error
	Delete(proxyID, orgName string) error
	Exists(proxyID, orgName string) (bool, error)
}

// LLMProxyRepo implements LLMProxyRepository using GORM
type LLMProxyRepo struct {
	db           *gorm.DB
	artifactRepo ArtifactRepository
}

// NewLLMProxyRepo creates a new LLM proxy repository
func NewLLMProxyRepo(db *gorm.DB) LLMProxyRepository {
	return &LLMProxyRepo{
		db:           db,
		artifactRepo: NewArtifactRepo(db),
	}
}

// proxyWithArtifact is a helper struct for joining LLM proxies with artifact data
type proxyWithArtifact struct {
	models.LLMProxy
	ArtifactOrgName   string    `gorm:"column:artifact_org_name"`
	ArtifactHandle    string    `gorm:"column:artifact_handle"`
	ArtifactName      string    `gorm:"column:artifact_name"`
	ArtifactVersion   string    `gorm:"column:artifact_version"`
	ArtifactCreatedAt time.Time `gorm:"column:artifact_created_at"`
	ArtifactUpdatedAt time.Time `gorm:"column:artifact_updated_at"`
}

// populateProxyArtifactFields populates the artifact-derived fields in an LLMProxy
func populateProxyArtifactFields(proxy *models.LLMProxy, result proxyWithArtifact) {
	proxy.OrganizationName = result.ArtifactOrgName
	proxy.ID = result.ArtifactHandle
	proxy.Name = result.ArtifactName
	proxy.Version = result.ArtifactVersion
	proxy.CreatedAt = result.ArtifactCreatedAt
	proxy.UpdatedAt = result.ArtifactUpdatedAt
	proxy.Handle = result.ArtifactHandle
}

// convertProxyResults converts proxyWithArtifact results to LLMProxy slice
func convertProxyResults(results []proxyWithArtifact) []*models.LLMProxy {
	proxies := make([]*models.LLMProxy, len(results))
	for i, result := range results {
		proxy := result.LLMProxy
		populateProxyArtifactFields(&proxy, result)
		proxies[i] = &proxy
	}
	return proxies
}

// getProxyUUIDByHandle retrieves the proxy UUID from a handle
func (r *LLMProxyRepo) getProxyUUIDByHandle(tx *gorm.DB, handle string, orgName string) (uuid.UUID, error) {
	var artifact struct{ UUID uuid.UUID }
	result := tx.Table("artifacts").
		Select("uuid").
		Where("handle = ? AND organization_name = ? AND kind = ?", handle, orgName, models.KindLLMProxy).
		Scan(&artifact)
	if result.Error != nil {
		return uuid.Nil, result.Error
	}
	if result.RowsAffected == 0 {
		return uuid.Nil, gorm.ErrRecordNotFound
	}
	return artifact.UUID, nil
}

// Create inserts a new LLM proxy
func (r *LLMProxyRepo) Create(p *models.LLMProxy, handle, name, version string, orgName string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if p.UUID == uuid.Nil {
			p.UUID = uuid.New()
		}
		now := time.Now()

		// Insert into artifacts table first
		if err := r.artifactRepo.Create(tx, &models.Artifact{
			UUID:             p.UUID,
			Handle:           handle,
			Name:             name,
			Version:          version,
			Kind:             models.KindLLMProxy,
			OrganizationName: orgName,
			CreatedAt:        now,
			UpdatedAt:        now,
		}); err != nil {
			return fmt.Errorf("failed to create artifact: %w", err)
		}

		// Insert into llm_proxies table
		return tx.Create(p).Error
	})
}

// GetByID retrieves an LLM proxy by ID (handle)
func (r *LLMProxyRepo) GetByID(proxyID, orgName string) (*models.LLMProxy, error) {
	var result proxyWithArtifact

	err := r.db.
		Table("llm_proxies").
		Select("llm_proxies.*, a.organization_name as artifact_org_uuid, a.handle as artifact_handle, a.name as artifact_name, a.version as artifact_version, a.created_at as artifact_created_at, a.updated_at as artifact_updated_at").
		Joins("JOIN artifacts a ON llm_proxies.uuid = a.uuid").
		Where("a.handle = ? AND a.organization_name = ? AND a.kind = ?", proxyID, orgName, models.KindLLMProxy).
		Take(&result).Error
	if err != nil {
		return nil, err
	}

	// Scan does not return ErrRecordNotFound when no rows match, so check for zero UUID
	if result.UUID == uuid.Nil {
		return nil, gorm.ErrRecordNotFound
	}

	proxy := result.LLMProxy
	populateProxyArtifactFields(&proxy, result)
	return &proxy, nil
}

// List retrieves LLM proxies with pagination
func (r *LLMProxyRepo) List(orgName string, limit, offset int) ([]*models.LLMProxy, error) {
	var results []proxyWithArtifact
	err := r.db.
		Table("llm_proxies").
		Select("llm_proxies.*, a.organization_name as artifact_org_uuid, a.handle as artifact_handle, a.name as artifact_name, a.version as artifact_version, a.created_at as artifact_created_at, a.updated_at as artifact_updated_at").
		Joins("JOIN artifacts a ON llm_proxies.uuid = a.uuid").
		Where("a.organization_name = ? AND a.kind = ?", orgName, models.KindLLMProxy).
		Order("a.created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return convertProxyResults(results), nil
}

// ListByProject retrieves LLM proxies for a specific project with pagination
func (r *LLMProxyRepo) ListByProject(orgName, projectUUID string, limit, offset int) ([]*models.LLMProxy, error) {
	var results []proxyWithArtifact
	err := r.db.
		Table("llm_proxies").
		Select("llm_proxies.*, a.organization_name as artifact_org_uuid, a.handle as artifact_handle, a.name as artifact_name, a.version as artifact_version, a.created_at as artifact_created_at, a.updated_at as artifact_updated_at").
		Joins("JOIN artifacts a ON llm_proxies.uuid = a.uuid").
		Where("a.organization_name = ? AND llm_proxies.project_uuid = ? AND a.kind = ?", orgName, projectUUID, models.KindLLMProxy).
		Order("a.created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return convertProxyResults(results), nil
}

// ListByProvider retrieves LLM proxies for a specific provider with pagination
func (r *LLMProxyRepo) ListByProvider(orgName, providerUUID string, limit, offset int) ([]*models.LLMProxy, error) {
	var results []proxyWithArtifact
	err := r.db.
		Table("llm_proxies").
		Select("llm_proxies.*, a.organization_name as artifact_org_uuid, a.handle as artifact_handle, a.name as artifact_name, a.version as artifact_version, a.created_at as artifact_created_at, a.updated_at as artifact_updated_at").
		Joins("JOIN artifacts a ON llm_proxies.uuid = a.uuid").
		Where("a.organization_name = ? AND llm_proxies.provider_uuid = ? AND a.kind = ?", orgName, providerUUID, models.KindLLMProxy).
		Order("a.created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return convertProxyResults(results), nil
}

// Count counts LLM proxies for an organization
func (r *LLMProxyRepo) Count(orgName string) (int, error) {
	return r.artifactRepo.CountByKindAndOrg(models.KindLLMProxy, orgName)
}

// CountByProject counts LLM proxies for a specific project
func (r *LLMProxyRepo) CountByProject(orgName, projectUUID string) (int, error) {
	var count int64
	err := r.db.Table("artifacts a").
		Joins("JOIN llm_proxies p ON a.uuid = p.uuid").
		Where("a.organization_name = ? AND p.project_uuid = ? AND a.kind = ?", orgName, projectUUID, models.KindLLMProxy).
		Count(&count).Error
	return int(count), err
}

// CountByProvider counts LLM proxies for a specific provider
func (r *LLMProxyRepo) CountByProvider(orgName, providerUUID string) (int, error) {
	var count int64
	err := r.db.Table("artifacts a").
		Joins("JOIN llm_proxies p ON a.uuid = p.uuid").
		Where("a.organization_name = ? AND p.provider_uuid = ? AND a.kind = ?", orgName, providerUUID, models.KindLLMProxy).
		Count(&count).Error
	return int(count), err
}

// Update modifies an existing LLM proxy
func (r *LLMProxyRepo) Update(p *models.LLMProxy, handle string, orgName string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()

		// Get the proxy UUID from handle
		proxyUUID, err := r.getProxyUUIDByHandle(tx, handle, orgName)
		if err != nil {
			return err
		}

		// Update artifacts table
		if err := r.artifactRepo.Update(tx, &models.Artifact{
			UUID:             proxyUUID,
			OrganizationName: orgName,
			UpdatedAt:        now,
		}); err != nil {
			return fmt.Errorf("failed to update artifact: %w", err)
		}

		// Update llm_proxies table
		updateResult := tx.Model(&models.LLMProxy{}).
			Where("uuid = ?", proxyUUID).
			Updates(map[string]any{
				"description":   p.Description,
				"provider_uuid": p.ProviderUUID,
				"openapi_spec":  p.OpenAPISpec,
				"status":        p.Status,
				"configuration": p.Configuration,
			})

		if updateResult.Error != nil {
			return fmt.Errorf("failed to update proxy: %w", updateResult.Error)
		}
		if updateResult.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// Delete removes an LLM proxy
func (r *LLMProxyRepo) Delete(proxyID, orgName string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		proxyUUID, err := r.getProxyUUIDByHandle(tx, proxyID, orgName)
		if err != nil {
			return err
		}

		// Delete from llm_proxies first
		if err := tx.Where("uuid = ?", proxyUUID).Delete(&models.LLMProxy{}).Error; err != nil {
			return err
		}

		// Delete from artifacts
		return r.artifactRepo.Delete(tx, proxyUUID.String())
	})
}

// Exists checks if an LLM proxy exists
func (r *LLMProxyRepo) Exists(proxyID, orgName string) (bool, error) {
	return r.artifactRepo.Exists(models.KindLLMProxy, proxyID, orgName)
}

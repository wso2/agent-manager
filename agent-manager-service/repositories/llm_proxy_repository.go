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
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// LLMProxyRepository defines the interface for LLM proxy persistence
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/llm_proxy_repository_mock.go . LLMProxyRepository:LLMProxyRepositoryMock
type LLMProxyRepository interface {
	Create(ctx context.Context, p *models.LLMProxy, handle, name, version string, ouID string) error
	GetByID(proxyID, ouID string) (*models.LLMProxy, error)
	// GetByIDCtx is GetByID with context propagation, for call paths (e.g.
	// UndeployLLMProxyDeployment) that need cancellation to reach the query.
	// GetByID itself is left as-is to avoid forcing ctx onto its many other
	// existing callers.
	GetByIDCtx(ctx context.Context, proxyID, ouID string) (*models.LLMProxy, error)
	GetByIDAndProject(proxyID, ouID, projectUUID string) (*models.LLMProxy, error)
	List(ouID string, limit, offset int) ([]*models.LLMProxy, error)
	ListByProject(ouID, projectUUID string, limit, offset int) ([]*models.LLMProxy, error)
	ListByProvider(ouID, providerUUID string, limit, offset int) ([]*models.LLMProxy, error)
	Count(ouID string) (int, error)
	CountByProject(ouID, projectUUID string) (int, error)
	CountByProvider(ouID, providerUUID string) (int, error)
	Update(p *models.LLMProxy, handle string, ouID string) error
	Delete(proxyID, ouID string) error
	DeleteInProject(ctx context.Context, proxyID, ouID, projectUUID string) error
	Exists(proxyID, ouID string) (bool, error)
}

// LLMProxyRepo implements LLMProxyRepository using GORM
type LLMProxyRepo struct {
	db           *gorm.DB
	artifactRepo ArtifactRepository
	providerRepo LLMProviderRepository
}

// NewLLMProxyRepo creates a new LLM proxy repository
func NewLLMProxyRepo(db *gorm.DB) LLMProxyRepository {
	return &LLMProxyRepo{
		db:           db,
		artifactRepo: NewArtifactRepo(db),
		providerRepo: NewLLMProviderRepo(db),
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
func (r *LLMProxyRepo) getProxyUUIDByHandle(tx *gorm.DB, handle string, ouID string) (uuid.UUID, error) {
	var artifact struct{ UUID uuid.UUID }
	result := tx.Table("artifacts").
		Select("uuid").
		Where("handle = ? AND ou_id = ? AND kind = ?", handle, ouID, models.KindLLMProxy).
		Scan(&artifact)
	if result.Error != nil {
		return uuid.Nil, result.Error
	}
	if result.RowsAffected == 0 {
		return uuid.Nil, gorm.ErrRecordNotFound
	}
	return artifact.UUID, nil
}

// Create inserts a new LLM proxy. The provider's deleting status is locked and
// rechecked inside this same transaction (see IsDeletingForUpdate) so it is
// consistent with LLMProviderService.Delete's MarkDeleting row-locking UPDATE: a
// concurrent Delete either commits its claim before this transaction starts (and
// this insert is rejected) or starts after this transaction commits (and is then
// caught by Delete's own HasAssociatedProxies check). There is no interleaving
// where the proxy is inserted without either side observing the other.
func (r *LLMProxyRepo) Create(ctx context.Context, p *models.LLMProxy, handle, name, version string, ouID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		deleting, err := r.providerRepo.IsDeletingForUpdate(ctx, tx, p.ProviderUUID)
		if err != nil {
			return fmt.Errorf("failed to check provider deleting status: %w", err)
		}
		if deleting {
			return utils.ErrLLMProviderBeingDeleted
		}

		if p.UUID == uuid.Nil {
			p.UUID = uuid.New()
		}
		now := time.Now()

		// Insert into artifacts table first
		if err := r.artifactRepo.Create(tx, &models.Artifact{
			UUID:      p.UUID,
			Handle:    handle,
			Name:      name,
			Version:   version,
			Kind:      models.KindLLMProxy,
			OUID:      ouID,
			CreatedAt: now,
			UpdatedAt: now,
		}); err != nil {
			return fmt.Errorf("failed to create artifact: %w", err)
		}

		// Insert into llm_proxies table
		return tx.Omit("Status").Create(p).Error
	})
}

// GetByID retrieves an LLM proxy by ID (handle)
func (r *LLMProxyRepo) GetByID(proxyID, ouID string) (*models.LLMProxy, error) {
	return r.GetByIDCtx(context.Background(), proxyID, ouID)
}

// GetByIDCtx is GetByID with context propagation — see the interface doc comment.
func (r *LLMProxyRepo) GetByIDCtx(ctx context.Context, proxyID, ouID string) (*models.LLMProxy, error) {
	var result proxyWithArtifact

	err := r.db.WithContext(ctx).
		Table("llm_proxies").
		Select("llm_proxies.*, a.ou_id as artifact_org_uuid, a.handle as artifact_handle, a.name as artifact_name, a.version as artifact_version, a.created_at as artifact_created_at, a.updated_at as artifact_updated_at").
		Joins("JOIN artifacts a ON llm_proxies.uuid = a.uuid").
		Where("a.handle = ? AND a.ou_id = ? AND a.kind = ?", proxyID, ouID, models.KindLLMProxy).
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

// GetByIDAndProject retrieves an LLM proxy by ID (handle) scoped to a specific
// project. The proxy is only returned when it belongs to the given org AND
// project, so a proxy owned by a different project is treated as not found.
func (r *LLMProxyRepo) GetByIDAndProject(proxyID, ouID, projectUUID string) (*models.LLMProxy, error) {
	var result proxyWithArtifact

	err := r.db.
		Table("llm_proxies").
		Select("llm_proxies.*, a.ou_id as artifact_org_uuid, a.handle as artifact_handle, a.name as artifact_name, a.version as artifact_version, a.created_at as artifact_created_at, a.updated_at as artifact_updated_at").
		Joins("JOIN artifacts a ON llm_proxies.uuid = a.uuid").
		Where("a.handle = ? AND a.ou_id = ? AND llm_proxies.project_uuid = ? AND a.kind = ?", proxyID, ouID, projectUUID, models.KindLLMProxy).
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
func (r *LLMProxyRepo) List(ouID string, limit, offset int) ([]*models.LLMProxy, error) {
	var results []proxyWithArtifact
	err := r.db.
		Table("llm_proxies").
		Select("llm_proxies.*, a.ou_id as artifact_org_uuid, a.handle as artifact_handle, a.name as artifact_name, a.version as artifact_version, a.created_at as artifact_created_at, a.updated_at as artifact_updated_at").
		Joins("JOIN artifacts a ON llm_proxies.uuid = a.uuid").
		Where("a.ou_id = ? AND a.kind = ?", ouID, models.KindLLMProxy).
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
func (r *LLMProxyRepo) ListByProject(ouID, projectUUID string, limit, offset int) ([]*models.LLMProxy, error) {
	var results []proxyWithArtifact
	err := r.db.
		Table("llm_proxies").
		Select("llm_proxies.*, a.ou_id as artifact_org_uuid, a.handle as artifact_handle, a.name as artifact_name, a.version as artifact_version, a.created_at as artifact_created_at, a.updated_at as artifact_updated_at").
		Joins("JOIN artifacts a ON llm_proxies.uuid = a.uuid").
		Where("a.ou_id = ? AND llm_proxies.project_uuid = ? AND a.kind = ?", ouID, projectUUID, models.KindLLMProxy).
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
func (r *LLMProxyRepo) ListByProvider(ouID, providerUUID string, limit, offset int) ([]*models.LLMProxy, error) {
	var results []proxyWithArtifact
	err := r.db.
		Table("llm_proxies").
		Select("llm_proxies.*, a.ou_id as artifact_org_uuid, a.handle as artifact_handle, a.name as artifact_name, a.version as artifact_version, a.created_at as artifact_created_at, a.updated_at as artifact_updated_at").
		Joins("JOIN artifacts a ON llm_proxies.uuid = a.uuid").
		Where("a.ou_id = ? AND llm_proxies.provider_uuid = ? AND a.kind = ?", ouID, providerUUID, models.KindLLMProxy).
		// uuid breaks ties on created_at: proxies provisioned together share a
		// timestamp, and without a unique second key offset paging can repeat one
		// row and skip another — a skipped proxy is one left permanently stale by
		// any caller that pages through to reconcile.
		Order("a.created_at DESC, llm_proxies.uuid").
		Limit(limit).
		Offset(offset).
		Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return convertProxyResults(results), nil
}

// Count counts LLM proxies for an organization
func (r *LLMProxyRepo) Count(ouID string) (int, error) {
	return r.artifactRepo.CountByKindAndOrg(models.KindLLMProxy, ouID)
}

// CountByProject counts LLM proxies for a specific project
func (r *LLMProxyRepo) CountByProject(ouID, projectUUID string) (int, error) {
	var count int64
	err := r.db.Table("artifacts a").
		Joins("JOIN llm_proxies p ON a.uuid = p.uuid").
		Where("a.ou_id = ? AND p.project_uuid = ? AND a.kind = ?", ouID, projectUUID, models.KindLLMProxy).
		Count(&count).Error
	return int(count), err
}

// CountByProvider counts LLM proxies for a specific provider
func (r *LLMProxyRepo) CountByProvider(ouID, providerUUID string) (int, error) {
	var count int64
	err := r.db.Table("artifacts a").
		Joins("JOIN llm_proxies p ON a.uuid = p.uuid").
		Where("a.ou_id = ? AND p.provider_uuid = ? AND a.kind = ?", ouID, providerUUID, models.KindLLMProxy).
		Count(&count).Error
	return int(count), err
}

// Update modifies an existing LLM proxy
func (r *LLMProxyRepo) Update(p *models.LLMProxy, handle string, ouID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Get the proxy UUID from handle
		proxyUUID, err := r.getProxyUUIDByHandle(tx, handle, ouID)
		if err != nil {
			return err
		}

		// Update llm_proxies table
		updateResult := tx.Model(&models.LLMProxy{}).
			Where("uuid = ?", proxyUUID).
			Updates(map[string]any{
				"description":   p.Description,
				"provider_uuid": p.ProviderUUID,
				"openapi_spec":  p.OpenAPISpec,
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
func (r *LLMProxyRepo) Delete(proxyID, ouID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		proxyUUID, err := r.getProxyUUIDByHandle(tx, proxyID, ouID)
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
func (r *LLMProxyRepo) Exists(proxyID, ouID string) (bool, error) {
	return r.artifactRepo.Exists(models.KindLLMProxy, proxyID, ouID)
}

// DeleteInProject removes a proxy only within the resolved organization and project.
func (r *LLMProxyRepo) DeleteInProject(ctx context.Context, proxyID, ouID, projectUUID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var proxy models.LLMProxy
		err := tx.Table("llm_proxies").Select("llm_proxies.*").
			Joins("JOIN artifacts a ON llm_proxies.uuid = a.uuid").
			Where("a.handle = ? AND a.ou_id = ? AND llm_proxies.project_uuid = ? AND a.kind = ?", proxyID, ouID, projectUUID, models.KindLLMProxy).
			Take(&proxy).Error
		if err != nil {
			return fmt.Errorf("resolve proxy for deletion: %w", err)
		}
		result := tx.Where("uuid = ? AND project_uuid = ?", proxy.UUID, projectUUID).Delete(&models.LLMProxy{})
		if result.Error != nil {
			return fmt.Errorf("delete proxy: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return r.artifactRepo.Delete(tx, proxy.UUID.String())
	})
}

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
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// MCPProxyRepository defines the interface for MCP proxy persistence.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/mcp_proxy_repository_mock.go . MCPProxyRepository:MCPProxyRepositoryMock
type MCPProxyRepository interface {
	// Create inserts the proxy. projectUUID and projectName are both nil for an
	// organization-level proxy (listed in the catalog) and both set for a
	// project-scoped proxy (not in the catalog).
	Create(ctx context.Context, tx *gorm.DB, p *models.MCPProxy, handle, name, version, orgUUID string, projectUUID, projectName *string) error
	Update(ctx context.Context, tx *gorm.DB, p *models.MCPProxy, orgUUID string) error
	GetByUUID(ctx context.Context, proxyUUID, orgUUID string) (*models.MCPProxy, error)
	GetByHandle(ctx context.Context, handle, orgUUID string) (*models.MCPProxy, error)
	GetByHandleForUpdate(ctx context.Context, tx *gorm.DB, handle, orgUUID string) (*models.MCPProxy, error)
	// List returns the proxies matching the filter and the total matching count
	// ignoring Limit/Offset. The zero filter returns organization-level proxies only.
	List(ctx context.Context, orgUUID string, f MCPProxyListFilter) ([]*models.MCPProxy, int64, error)
	// GetInProject returns the proxy only when it belongs to the project;
	// utils.ErrMCPProxyNotFound when it is in another project or is org-owned.
	GetInProject(ctx context.Context, handle, orgUUID, projectUUID string) (*models.MCPProxy, error)
	// GetInProjectByName returns the project proxy stored under projectName, whatever project
	// UUID it carries: the lookup for a project that no longer exists, whose
	// proxies keep the UUID of the deleted project. utils.ErrMCPProxyNotFound when there is
	// none, or the handle is an org proxy or another project's.
	GetInProjectByName(ctx context.Context, handle, orgUUID, projectName string) (*models.MCPProxy, error)
	// ReassignProject moves a project proxy stored under projectName from fromProjectUUID to
	// toProjectUUID: a re-created project (same name, new UUID) adopting the proxy its
	// deleted predecessor left. A proxy already on toProjectUUID is left as is.
	// utils.ErrMCPProxyNotFound when no such proxy is in the organization.
	ReassignProject(ctx context.Context, tx *gorm.DB, proxyUUID uuid.UUID, orgUUID, projectName, fromProjectUUID, toProjectUUID string) error
	Delete(ctx context.Context, handle, orgUUID string) error
	Count(ctx context.Context, orgUUID string) (int, error)
	Exists(ctx context.Context, handle, orgUUID string) (bool, error)
}

// MCPProxyListFilter narrows List. The zero value lists organization-level proxies
// only, which is what the org-scoped API has always returned.
type MCPProxyListFilter struct {
	// AllProjects lists every proxy in the organization, org-owned and every project's,
	// with no project predicate. ProjectUUID and IncludeOrgOwned are ignored. For
	// internal sweeps (environment cleanup, reconcile, scope listing), never user lists.
	AllProjects bool
	// ProjectUUID, when set, lists that project's proxies.
	ProjectUUID *string
	// IncludeOrgOwned additionally lists organization-level proxies. It has no effect
	// without ProjectUUID.
	IncludeOrgOwned bool
	// ProjectName, when set and ProjectUUID is not, lists the project proxies stored under
	// that project name whatever project UUID they carry: the listing for a project that no
	// longer exists. Org proxies are never included.
	ProjectName *string
	// Component and Endpoint, when non-empty, match configuration.source.
	Component, Endpoint string
	// Limit <= 0 means no limit.
	Limit, Offset int
}

// apply adds the org, kind, project and source predicates. The query must already join
// artifacts as a.
func (f MCPProxyListFilter) apply(q *gorm.DB, orgUUID string) *gorm.DB {
	q = q.Where("a.ou_id = ? AND a.kind = ?", orgUUID, models.KindMCPProxy)
	switch {
	case f.AllProjects:
	case f.ProjectUUID == nil && f.ProjectName != nil:
		q = q.Where("mcp_proxies.project_uuid IS NOT NULL AND mcp_proxies.project_name = ?", *f.ProjectName)
	case f.ProjectUUID == nil:
		q = q.Where("mcp_proxies.project_uuid IS NULL")
	case f.IncludeOrgOwned:
		q = q.Where("(mcp_proxies.project_uuid = ? OR mcp_proxies.project_uuid IS NULL)", *f.ProjectUUID)
	default:
		q = q.Where("mcp_proxies.project_uuid = ?", *f.ProjectUUID)
	}
	if f.Component != "" {
		q = q.Where("mcp_proxies.configuration->'source'->>'componentName' = ?", f.Component)
	}
	if f.Endpoint != "" {
		q = q.Where("mcp_proxies.configuration->'source'->>'endpointName' = ?", f.Endpoint)
	}
	return q
}

// MCPProxyRepo implements MCPProxyRepository using GORM.
type MCPProxyRepo struct {
	db           *gorm.DB
	artifactRepo ArtifactRepository
}

// NewMCPProxyRepo creates a new MCP proxy repository.
func NewMCPProxyRepo(db *gorm.DB) MCPProxyRepository {
	return &MCPProxyRepo{
		db:           db,
		artifactRepo: NewArtifactRepo(db),
	}
}

// Create inserts a new MCP proxy and its artifact row.
func (r *MCPProxyRepo) Create(ctx context.Context, tx *gorm.DB, p *models.MCPProxy, handle, name, version, orgUUID string, projectUUID, projectName *string) error {
	if p.UUID == uuid.Nil {
		p.UUID = uuid.New()
	}
	if p.Status == "" {
		p.Status = models.StatusCreated
	}
	p.ProjectUUID, p.ProjectName = projectUUID, projectName
	now := time.Now()

	if err := r.artifactRepo.Create(tx.WithContext(ctx), &models.Artifact{
		UUID:      p.UUID,
		Handle:    handle,
		Name:      name,
		Version:   version,
		Kind:      models.KindMCPProxy,
		OUID:      orgUUID,
		CreatedAt: now,
		UpdatedAt: now,
		InCatalog: projectUUID == nil,
	}); err != nil {
		return fmt.Errorf("failed to create artifact: %w", err)
	}

	if err := tx.WithContext(ctx).Create(p).Error; err != nil {
		return err
	}
	return nil
}

// Update modifies an existing MCP proxy and bumps the backing artifact timestamp.
func (r *MCPProxyRepo) Update(ctx context.Context, tx *gorm.DB, p *models.MCPProxy, orgUUID string) error {
	if p == nil || p.UUID == uuid.Nil {
		return gorm.ErrRecordNotFound
	}

	result := tx.WithContext(ctx).Model(&models.MCPProxy{}).
		Where("uuid = ?", p.UUID).
		Updates(map[string]interface{}{
			"description":   p.Description,
			"configuration": p.Configuration,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}

	// A project proxy's artifact name is its handle (see models.MCPProxyArtifactName), so its
	// display name can repeat across the org.
	handle := p.Handle
	if p.Artifact != nil && p.Artifact.Handle != "" {
		handle = p.Artifact.Handle
	}
	return r.artifactRepo.Update(tx.WithContext(ctx), &models.Artifact{
		UUID:    p.UUID,
		Name:    models.MCPProxyArtifactName(p.ProjectName, handle, p.Configuration.Name),
		Version: p.Configuration.Version,
		OUID:    orgUUID,
	})
}

// GetByUUID retrieves an MCP proxy by artifact UUID.
func (r *MCPProxyRepo) GetByUUID(ctx context.Context, proxyUUID, orgUUID string) (*models.MCPProxy, error) {
	var proxy models.MCPProxy
	err := r.db.WithContext(ctx).
		Preload("Artifact").
		Preload("Endpoints").
		Preload("Endpoints.Environments").
		Joins("JOIN artifacts a ON mcp_proxies.uuid = a.uuid").
		Where("mcp_proxies.uuid = ? AND a.ou_id = ? AND a.kind = ?", proxyUUID, orgUUID, models.KindMCPProxy).
		First(&proxy).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return &proxy, nil
}

// GetByHandle retrieves an MCP proxy by artifact handle.
func (r *MCPProxyRepo) GetByHandle(ctx context.Context, handle, orgUUID string) (*models.MCPProxy, error) {
	var proxy models.MCPProxy
	err := r.db.WithContext(ctx).
		Preload("Artifact").
		Preload("Endpoints").
		Preload("Endpoints.Environments").
		Joins("JOIN artifacts a ON mcp_proxies.uuid = a.uuid").
		Where("a.handle = ? AND a.ou_id = ? AND a.kind = ?", handle, orgUUID, models.KindMCPProxy).
		First(&proxy).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return &proxy, nil
}

// GetByHandleForUpdate retrieves an MCP proxy by handle and locks the
// underlying mcp_proxies and artifacts rows for the duration of the
// transaction. Use this to serialize read-modify-write flows against
// concurrent updates.
func (r *MCPProxyRepo) GetByHandleForUpdate(ctx context.Context, tx *gorm.DB, handle, orgUUID string) (*models.MCPProxy, error) {
	var proxy models.MCPProxy
	err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "mcp_proxies"}}).
		Preload("Artifact").
		Preload("Endpoints").
		Preload("Endpoints.Environments").
		Joins("JOIN artifacts a ON mcp_proxies.uuid = a.uuid").
		Where("a.handle = ? AND a.ou_id = ? AND a.kind = ?", handle, orgUUID, models.KindMCPProxy).
		First(&proxy).Error
	if err != nil {
		return nil, err
	}
	return &proxy, nil
}

// List retrieves MCP proxies matching the filter, newest first, with the total count.
func (r *MCPProxyRepo) List(ctx context.Context, orgUUID string, f MCPProxyListFilter) ([]*models.MCPProxy, int64, error) {
	base := func() *gorm.DB {
		return f.apply(r.db.WithContext(ctx).Model(&models.MCPProxy{}).
			Joins("JOIN artifacts a ON mcp_proxies.uuid = a.uuid"), orgUUID)
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	q := base().
		Preload("Artifact").
		Preload("Endpoints").
		Preload("Endpoints.Environments").
		Order("a.created_at DESC")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Offset > 0 {
		q = q.Offset(f.Offset)
	}
	var proxies []*models.MCPProxy
	if err := q.Find(&proxies).Error; err != nil {
		return nil, 0, err
	}
	return proxies, total, nil
}

// GetInProject retrieves an MCP proxy by handle only when it belongs to the project.
func (r *MCPProxyRepo) GetInProject(ctx context.Context, handle, orgUUID, projectUUID string) (*models.MCPProxy, error) {
	var proxy models.MCPProxy
	err := r.db.WithContext(ctx).
		Preload("Artifact").
		Preload("Endpoints").
		Preload("Endpoints.Environments").
		Joins("JOIN artifacts a ON mcp_proxies.uuid = a.uuid").
		Where("a.handle = ? AND a.ou_id = ? AND a.kind = ? AND mcp_proxies.project_uuid = ?",
			handle, orgUUID, models.KindMCPProxy, projectUUID).
		First(&proxy).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMCPProxyNotFound
		}
		return nil, err
	}
	return &proxy, nil
}

// GetInProjectByName retrieves a project proxy by handle and stored project name, whatever its project UUID.
func (r *MCPProxyRepo) GetInProjectByName(ctx context.Context, handle, orgUUID, projectName string) (*models.MCPProxy, error) {
	var proxy models.MCPProxy
	err := r.db.WithContext(ctx).
		Preload("Artifact").
		Preload("Endpoints").
		Preload("Endpoints.Environments").
		Joins("JOIN artifacts a ON mcp_proxies.uuid = a.uuid").
		Where("a.handle = ? AND a.ou_id = ? AND a.kind = ? AND mcp_proxies.project_uuid IS NOT NULL AND mcp_proxies.project_name = ?",
			handle, orgUUID, models.KindMCPProxy, projectName).
		First(&proxy).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMCPProxyNotFound
		}
		return nil, err
	}
	return &proxy, nil
}

// ReassignProject moves a project proxy from fromProjectUUID to toProjectUUID, keeping its project name.
func (r *MCPProxyRepo) ReassignProject(
	ctx context.Context, tx *gorm.DB, proxyUUID uuid.UUID, orgUUID, projectName, fromProjectUUID, toProjectUUID string,
) error {
	db := tx.WithContext(ctx)
	ownedByOrg := db.Session(&gorm.Session{NewDB: true}).Model(&models.Artifact{}).Select("uuid").
		Where("ou_id = ? AND kind = ?", orgUUID, models.KindMCPProxy)
	result := db.Model(&models.MCPProxy{}).
		Where("uuid = ? AND project_name = ? AND project_uuid IN (?, ?)", proxyUUID, projectName, fromProjectUUID, toProjectUUID).
		Where("uuid IN (?)", ownedByOrg).
		Update("project_uuid", toProjectUUID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return utils.ErrMCPProxyNotFound
	}
	return nil
}

// Delete removes an MCP proxy and its artifact row.
func (r *MCPProxyRepo) Delete(ctx context.Context, handle, orgUUID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var proxy models.MCPProxy
		err := tx.
			Joins("JOIN artifacts a ON mcp_proxies.uuid = a.uuid").
			Where("a.handle = ? AND a.ou_id = ? AND a.kind = ?", handle, orgUUID, models.KindMCPProxy).
			First(&proxy).Error
		if err != nil {
			return err
		}

		if err := tx.Where("artifact_uuid = ? AND ou_id = ?", proxy.UUID, orgUUID).
			Delete(&models.DeploymentStatusRecord{}).Error; err != nil {
			return err
		}
		if err := tx.Where("artifact_uuid = ? AND ou_id = ?", proxy.UUID, orgUUID).
			Delete(&models.Deployment{}).Error; err != nil {
			return err
		}
		if err := tx.Where("uuid = ?", proxy.UUID).Delete(&models.MCPProxy{}).Error; err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				return utils.ErrMCPProxyHasMappings
			}
			return err
		}
		return r.artifactRepo.Delete(tx, proxy.UUID.String())
	})
}

// Count counts organization-level MCP proxies (project-scoped proxies are excluded).
func (r *MCPProxyRepo) Count(ctx context.Context, orgUUID string) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&models.MCPProxy{}).
		Joins("JOIN artifacts a ON mcp_proxies.uuid = a.uuid").
		Where("a.ou_id = ? AND a.kind = ? AND mcp_proxies.project_uuid IS NULL", orgUUID, models.KindMCPProxy).
		Count(&count).Error
	return int(count), err
}

// Exists checks whether an MCP proxy exists by handle and organization.
func (r *MCPProxyRepo) Exists(ctx context.Context, handle, orgUUID string) (bool, error) {
	return r.artifactRepo.Exists(models.KindMCPProxy, handle, orgUUID)
}

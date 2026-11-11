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

	"gorm.io/gorm"

	"github.com/google/uuid"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// MCPProxyScopeRepository persists per-proxy MCP scopes.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/mcp_proxy_scope_repository_mock.go . MCPProxyScopeRepository:MCPProxyScopeRepositoryMock
type MCPProxyScopeRepository interface {
	// ListByProxy returns the proxy's scopes, action-ascending.
	ListByProxy(ctx context.Context, proxyUUID uuid.UUID) ([]models.MCPProxyScope, error)
	// ListByProxyUUIDs returns scopes for all given proxies (aggregate endpoint).
	ListByProxyUUIDs(ctx context.Context, proxyUUIDs []uuid.UUID) ([]models.MCPProxyScope, error)
	// Get returns gorm.ErrRecordNotFound (unwrapped) when absent.
	Get(ctx context.Context, proxyUUID uuid.UUID, action string) (*models.MCPProxyScope, error)
	Create(ctx context.Context, scope *models.MCPProxyScope) error
	Update(ctx context.Context, scope *models.MCPProxyScope) error
	Delete(ctx context.Context, proxyUUID uuid.UUID, action string) error
}

type mcpProxyScopeRepository struct{ db *gorm.DB }

// NewMCPProxyScopeRepository creates a new MCP proxy scope repository.
func NewMCPProxyScopeRepository(db *gorm.DB) MCPProxyScopeRepository {
	return &mcpProxyScopeRepository{db: db}
}

// scopeTool is one tool a scope covers.
type scopeTool struct {
	ScopeID  uuid.UUID `gorm:"column:scope_id;primaryKey"`
	ToolName string    `gorm:"column:tool_name;primaryKey"`
	OrgID    string    `gorm:"column:org_id"`
}

func (scopeTool) TableName() string { return "mcp_proxy_scope_tools" }

// withTools reads each scope's tools, in name order.
func (r *mcpProxyScopeRepository) withTools(ctx context.Context, scopes []models.MCPProxyScope) ([]models.MCPProxyScope, error) {
	if len(scopes) == 0 {
		return scopes, nil
	}
	ids := make([]uuid.UUID, len(scopes))
	for i := range scopes {
		ids[i] = scopes[i].UUID
		scopes[i].Tools = []string{}
	}
	byScope := make(map[uuid.UUID][]string, len(scopes))
	for _, chunk := range dbx.Chunks(ids) {
		var tools []scopeTool
		if err := r.db.WithContext(ctx).Where("scope_id IN ?", chunk).Order("tool_name").Find(&tools).Error; err != nil {
			return nil, fmt.Errorf("failed to list mcp proxy scope tools: %w", err)
		}
		for _, t := range tools {
			byScope[t.ScopeID] = append(byScope[t.ScopeID], t.ToolName)
		}
	}
	for i := range scopes {
		if tools, ok := byScope[scopes[i].UUID]; ok {
			scopes[i].Tools = tools
		}
	}
	return scopes, nil
}

// replaceTools makes the scope's stored tools exactly tools.
func replaceTools(db *gorm.DB, scope *models.MCPProxyScope) error {
	if err := db.Where("scope_id = ?", scope.UUID).Delete(&scopeTool{}).Error; err != nil {
		return err
	}
	seen := make(map[string]bool, len(scope.Tools))
	for _, name := range scope.Tools {
		if seen[name] {
			continue
		}
		seen[name] = true
		if err := db.Create(&scopeTool{ScopeID: scope.UUID, ToolName: name, OrgID: scope.OrgID}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *mcpProxyScopeRepository) ListByProxy(ctx context.Context, proxyUUID uuid.UUID) ([]models.MCPProxyScope, error) {
	var scopes []models.MCPProxyScope
	if err := r.db.WithContext(ctx).Where("mcp_proxy_id = ?", proxyUUID).
		Order("action asc").Find(&scopes).Error; err != nil {
		return nil, fmt.Errorf("failed to list mcp proxy scopes: %w", err)
	}
	return r.withTools(ctx, scopes)
}

func (r *mcpProxyScopeRepository) ListByProxyUUIDs(ctx context.Context, proxyUUIDs []uuid.UUID) ([]models.MCPProxyScope, error) {
	if len(proxyUUIDs) == 0 {
		return nil, nil
	}
	var scopes []models.MCPProxyScope
	for _, chunk := range dbx.Chunks(proxyUUIDs) {
		var part []models.MCPProxyScope
		if err := r.db.WithContext(ctx).Where("mcp_proxy_id IN ?", chunk).Find(&part).Error; err != nil {
			return nil, fmt.Errorf("failed to list mcp proxy scopes: %w", err)
		}
		scopes = append(scopes, part...)
	}
	return r.withTools(ctx, scopes)
}

func (r *mcpProxyScopeRepository) Get(ctx context.Context, proxyUUID uuid.UUID, action string) (*models.MCPProxyScope, error) {
	var scope models.MCPProxyScope
	if err := r.db.WithContext(ctx).Where("mcp_proxy_id = ? AND action = ?", proxyUUID, action).
		First(&scope).Error; err != nil {
		return nil, err
	}
	scopes, err := r.withTools(ctx, []models.MCPProxyScope{scope})
	if err != nil {
		return nil, err
	}
	return &scopes[0], nil
}

func (r *mcpProxyScopeRepository) Create(ctx context.Context, scope *models.MCPProxyScope) error {
	if scope.UUID == uuid.Nil {
		scope.UUID = uuid.Must(uuid.NewV7())
	}
	now := dbx.Now()
	scope.CreatedAt, scope.UpdatedAt = now, now
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if scope.OrgID == "" {
			var proxy models.MCPProxy
			if err := tx.Select("org_id").Where("id = ?", scope.MCPProxyUUID).Take(&proxy).Error; err != nil {
				return err
			}
			scope.OrgID = proxy.OrgID
		}
		if err := tx.Create(scope).Error; err != nil {
			return err
		}
		return replaceTools(tx, scope)
	})
	if err != nil {
		return fmt.Errorf("failed to create mcp proxy scope: %w", err)
	}
	return nil
}

func (r *mcpProxyScopeRepository) Update(ctx context.Context, scope *models.MCPProxyScope) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.MCPProxyScope
		err := tx.Where("mcp_proxy_id = ? AND action = ?", scope.MCPProxyUUID, scope.Action).Take(&current).Error
		if err != nil {
			return err
		}
		if err := tx.Model(&models.MCPProxyScope{}).Where("id = ?", current.UUID).
			Updates(map[string]any{"description": scope.Description, "updated_at": dbx.Now()}).Error; err != nil {
			return fmt.Errorf("failed to update mcp proxy scope: %w", err)
		}
		current.Tools = scope.Tools
		return replaceTools(tx, &current)
	})
}

func (r *mcpProxyScopeRepository) Delete(ctx context.Context, proxyUUID uuid.UUID, action string) error {
	res := r.db.WithContext(ctx).Where("mcp_proxy_id = ? AND action = ?", proxyUUID, action).
		Delete(&models.MCPProxyScope{})
	if res.Error != nil {
		return fmt.Errorf("failed to delete mcp proxy scope: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

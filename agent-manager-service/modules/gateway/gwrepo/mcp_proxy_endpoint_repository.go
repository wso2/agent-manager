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

package gwrepo

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// MCPProxyEndpointRepository persists MCP proxy endpoints and their
// endpoint→environment deployment bindings.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg gwrepomocks -out gwrepomocks/mcp_proxy_endpoint_repository_mock.go . MCPProxyEndpointRepository:MCPProxyEndpointRepositoryMock
type MCPProxyEndpointRepository interface {
	// CreateEndpoint inserts the endpoint. An encrypted upstream credential in its configuration (Upstream.Auth.SecretRef)
	// moves into a new secret; with none, a set UpstreamSecretID takes over that secret, which keeps an omitted
	// credential across a replacement of the endpoint rows.
	CreateEndpoint(ctx context.Context, tx *gorm.DB, endpoint *models.MCPProxyEndpoint) error
	// UpdateEndpoint rewrites the endpoint. A new credential rewrites its secret, auth with no credential keeps it, and
	// no auth releases it.
	UpdateEndpoint(ctx context.Context, tx *gorm.DB, endpoint *models.MCPProxyEndpoint) error
	// DeleteEndpoint deletes the endpoint and releases its upstream secret, unless that secret is in keepSecrets: an
	// endpoint created next in the same transaction takes it over.
	DeleteEndpoint(ctx context.Context, tx *gorm.DB, endpointUUID uuid.UUID, keepSecrets ...uuid.UUID) error
	GetEndpoint(ctx context.Context, endpointUUID uuid.UUID) (*models.MCPProxyEndpoint, error)
	// ListEndpointsByProxy returns every endpoint of the proxy with its
	// environment bindings preloaded.
	ListEndpointsByProxy(ctx context.Context, proxyUUID uuid.UUID) ([]models.MCPProxyEndpoint, error)
	AddEndpointEnvironment(ctx context.Context, tx *gorm.DB, ee *models.MCPProxyEndpointEnvironment) error
	RemoveEndpointEnvironment(ctx context.Context, tx *gorm.DB, endpointUUID, envUUID uuid.UUID) error
	ListEndpointEnvironments(ctx context.Context, endpointUUID uuid.UUID) ([]models.MCPProxyEndpointEnvironment, error)
	// GetEndpointEnvByProxyAndEnv is the agent-binding resolver: it returns the
	// single (endpoint, environment) row for the proxy, guaranteed unique by
	// uq_proxy_env_single, or gorm.ErrRecordNotFound when the proxy has no
	// endpoint bound to that environment.
	GetEndpointEnvByProxyAndEnv(ctx context.Context, proxyUUID, envUUID uuid.UUID) (*models.MCPProxyEndpointEnvironment, error)
	// ListEndpointEnvironmentsByProxy returns every endpoint→environment binding
	// for the proxy. Used by the delete teardown to collect all artifact UUIDs.
	ListEndpointEnvironmentsByProxy(ctx context.Context, proxyUUID uuid.UUID) ([]models.MCPProxyEndpointEnvironment, error)
}

type mcpProxyEndpointRepository struct {
	db      *gorm.DB
	secrets *repositories.SecretStore
}

// NewMCPProxyEndpointRepository creates a new MCP proxy endpoint repository. secrets keeps the upstream credentials;
// a repository built without one fails to save or resolve a credential.
func NewMCPProxyEndpointRepository(db *gorm.DB, secrets *repositories.SecretStore) MCPProxyEndpointRepository {
	return &mcpProxyEndpointRepository{db: db, secrets: secrets}
}

// ownerKindMCPEndpoint is secrets.owner_kind for an MCP proxy endpoint's upstream credential.
const ownerKindMCPEndpoint = "mcp_proxy_endpoint"

func endpointAuth(endpoint *models.MCPProxyEndpoint) *models.UpstreamAuth {
	if endpoint.Configuration.Upstream == nil {
		return nil
	}
	return endpoint.Configuration.Upstream.Auth
}

// saveUpstreamCredential moves the encrypted credential out of the endpoint's configuration into a secret and returns
// the id the row keeps. current is the secret the row refers to: a new credential rewrites it, auth without a
// credential keeps it, and no auth drops it (the caller releases it). restore puts the credential back into the
// endpoint once the row is written.
func (r *mcpProxyEndpointRepository) saveUpstreamCredential(
	tx *gorm.DB, endpoint *models.MCPProxyEndpoint, orgID string, current *uuid.UUID,
) (id *uuid.UUID, restore func(), err error) {
	restore = func() {}
	auth := endpointAuth(endpoint)
	if auth == nil {
		return nil, restore, nil
	}
	if auth.SecretRef == nil || *auth.SecretRef == "" {
		if auth.Value != nil {
			return nil, restore, nil
		}
		return current, restore, nil
	}
	if r.secrets == nil {
		return nil, restore, repositories.ErrNoSecretStore
	}
	id, err = r.secrets.Put(tx, orgID, ownerKindMCPEndpoint, endpoint.UUID, current, *auth.SecretRef)
	if err != nil {
		return nil, restore, fmt.Errorf("failed to store MCP upstream credential: %w", err)
	}
	ref := auth.SecretRef
	auth.SecretRef = nil
	return id, func() { auth.SecretRef = ref }, nil
}

func (r *mcpProxyEndpointRepository) CreateEndpoint(ctx context.Context, tx *gorm.DB, endpoint *models.MCPProxyEndpoint) error {
	if endpoint == nil {
		return fmt.Errorf("endpoint is required")
	}
	if endpoint.UUID == uuid.Nil {
		endpoint.UUID = uuid.New()
	}
	db := tx.WithContext(ctx)
	if err := dbx.LockRow(db, models.MCPProxy{}.TableName(), "status", "id = ?", endpoint.MCPProxyUUID); err != nil {
		return fmt.Errorf("failed to lock the parent MCP proxy: %w", err)
	}
	if endpoint.OrgID == "" {
		var proxy struct{ OrgID string }
		if err := db.Session(&gorm.Session{NewDB: true}).Table(models.MCPProxy{}.TableName()).Select("org_id").
			Where("id = ?", endpoint.MCPProxyUUID).Take(&proxy).Error; err != nil {
			return fmt.Errorf("failed to resolve MCP proxy endpoint organization: %w", err)
		}
		endpoint.OrgID = proxy.OrgID
	}
	// A set UpstreamSecretID is one the caller hands over from an endpoint this one replaces.
	adopted := endpoint.UpstreamSecretID
	secretID, restore, err := r.saveUpstreamCredential(db, endpoint, endpoint.OrgID, nil)
	if err != nil {
		return err
	}
	defer restore()
	if secretID == nil && endpointAuth(endpoint) != nil {
		secretID = adopted
	}
	endpoint.UpstreamSecretID = secretID
	if err := db.Create(endpoint).Error; err != nil {
		return fmt.Errorf("failed to create MCP proxy endpoint: %w", err)
	}
	if secretID != nil && adopted != nil && *secretID == *adopted {
		if err := repositories.AdoptSecret(db, *secretID, endpoint.UUID); err != nil {
			return err
		}
	} else if adopted != nil {
		// The caller kept the secret for this endpoint, which no longer refers to it.
		if err := repositories.ReleaseSecret(db, adopted); err != nil {
			return fmt.Errorf("failed to release MCP upstream credential: %w", err)
		}
	}
	return nil
}

func (r *mcpProxyEndpointRepository) UpdateEndpoint(ctx context.Context, tx *gorm.DB, endpoint *models.MCPProxyEndpoint) error {
	if endpoint == nil || endpoint.UUID == uuid.Nil {
		return gorm.ErrRecordNotFound
	}
	db := tx.WithContext(ctx)
	// Lock the endpoint before reading its secret id, so a concurrent update or delete reads this one's result.
	if err := lockEndpoint(db, endpoint.UUID); err != nil {
		return err
	}
	var stored models.MCPProxyEndpoint
	if err := db.Select("id", "org_id", "upstream_secret_id").Where("id = ?", endpoint.UUID).Take(&stored).Error; err != nil {
		return fmt.Errorf("failed to read MCP proxy endpoint: %w", err)
	}
	secretID, restore, err := r.saveUpstreamCredential(db, endpoint, stored.OrgID, stored.UpstreamSecretID)
	if err != nil {
		return err
	}
	defer restore()
	result := db.Model(&models.MCPProxyEndpoint{}).
		Where("id = ?", endpoint.UUID).
		Updates(map[string]interface{}{
			"handle":             endpoint.Handle,
			"name":               endpoint.Name,
			"status":             endpoint.Status,
			"configuration":      dbx.JSON(endpoint.Configuration),
			"upstream_secret_id": secretID,
		})
	if result.Error != nil {
		return fmt.Errorf("failed to update MCP proxy endpoint: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	endpoint.UpstreamSecretID = secretID
	if stored.UpstreamSecretID != nil && (secretID == nil || *secretID != *stored.UpstreamSecretID) {
		if err := repositories.ReleaseSecret(db, stored.UpstreamSecretID); err != nil {
			return fmt.Errorf("failed to release MCP upstream credential: %w", err)
		}
	}
	return nil
}

func (r *mcpProxyEndpointRepository) DeleteEndpoint(ctx context.Context, tx *gorm.DB, endpointUUID uuid.UUID, keepSecrets ...uuid.UUID) error {
	db := tx.WithContext(ctx)
	// Lock the endpoint before reading its secret id, so the release below sees what a concurrent update committed.
	if err := lockEndpoint(db, endpointUUID); err != nil {
		return err
	}
	var stored models.MCPProxyEndpoint
	if err := db.Select("id", "upstream_secret_id").Where("id = ?", endpointUUID).Take(&stored).Error; err != nil {
		return fmt.Errorf("failed to read MCP proxy endpoint: %w", err)
	}
	result := db.Where("id = ?", endpointUUID).Delete(&models.MCPProxyEndpoint{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete MCP proxy endpoint: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	if stored.UpstreamSecretID != nil && !slices.Contains(keepSecrets, *stored.UpstreamSecretID) {
		if err := repositories.ReleaseSecret(db, stored.UpstreamSecretID); err != nil {
			return fmt.Errorf("failed to release MCP upstream credential: %w", err)
		}
	}
	return nil
}

// lockEndpoint locks an endpoint row for the rest of tx; a missing endpoint is gorm.ErrRecordNotFound.
func lockEndpoint(db *gorm.DB, id uuid.UUID) error {
	err := dbx.LockRow(db, models.MCPProxyEndpoint{}.TableName(), "status", "id = ?", id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to lock MCP proxy endpoint %s: %w", id, err)
	}
	return err
}

func (r *mcpProxyEndpointRepository) GetEndpoint(ctx context.Context, endpointUUID uuid.UUID) (*models.MCPProxyEndpoint, error) {
	var endpoint models.MCPProxyEndpoint
	err := r.db.WithContext(ctx).
		Preload("Environments").
		Where("id = ?", endpointUUID).
		First(&endpoint).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get MCP proxy endpoint: %w", err)
	}
	return &endpoint, nil
}

func (r *mcpProxyEndpointRepository) ListEndpointsByProxy(ctx context.Context, proxyUUID uuid.UUID) ([]models.MCPProxyEndpoint, error) {
	var endpoints []models.MCPProxyEndpoint
	err := r.db.WithContext(ctx).
		Preload("Environments").
		Where("mcp_proxy_id = ?", proxyUUID).
		Order("created_at ASC").
		Find(&endpoints).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP proxy endpoints: %w", err)
	}
	return endpoints, nil
}

func (r *mcpProxyEndpointRepository) AddEndpointEnvironment(ctx context.Context, tx *gorm.DB, ee *models.MCPProxyEndpointEnvironment) error {
	if ee == nil {
		return fmt.Errorf("endpoint environment is required")
	}
	if err := tx.WithContext(ctx).Create(ee).Error; err != nil {
		return fmt.Errorf("failed to create MCP proxy endpoint environment: %w", err)
	}
	return nil
}

func (r *mcpProxyEndpointRepository) RemoveEndpointEnvironment(ctx context.Context, tx *gorm.DB, endpointUUID, envUUID uuid.UUID) error {
	result := tx.WithContext(ctx).
		Where("endpoint_id = ? AND environment_id = ?", endpointUUID, envUUID).
		Delete(&models.MCPProxyEndpointEnvironment{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete MCP proxy endpoint environment: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *mcpProxyEndpointRepository) ListEndpointEnvironments(ctx context.Context, endpointUUID uuid.UUID) ([]models.MCPProxyEndpointEnvironment, error) {
	var envs []models.MCPProxyEndpointEnvironment
	err := r.db.WithContext(ctx).
		Where("endpoint_id = ?", endpointUUID).
		Find(&envs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP proxy endpoint environments: %w", err)
	}
	return envs, nil
}

func (r *mcpProxyEndpointRepository) GetEndpointEnvByProxyAndEnv(ctx context.Context, proxyUUID, envUUID uuid.UUID) (*models.MCPProxyEndpointEnvironment, error) {
	var ee models.MCPProxyEndpointEnvironment
	err := r.db.WithContext(ctx).
		Where("mcp_proxy_id = ? AND environment_id = ?", proxyUUID, envUUID).
		First(&ee).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to resolve MCP proxy endpoint environment: %w", err)
	}
	return &ee, nil
}

func (r *mcpProxyEndpointRepository) ListEndpointEnvironmentsByProxy(ctx context.Context, proxyUUID uuid.UUID) ([]models.MCPProxyEndpointEnvironment, error) {
	var envs []models.MCPProxyEndpointEnvironment
	err := r.db.WithContext(ctx).
		Where("mcp_proxy_id = ?", proxyUUID).
		Find(&envs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP proxy endpoint environments by proxy: %w", err)
	}
	return envs, nil
}

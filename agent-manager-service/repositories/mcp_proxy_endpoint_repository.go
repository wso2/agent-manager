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
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// MCPProxyEndpointRepository persists MCP proxy endpoints and their
// endpoint→environment deployment bindings.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/mcp_proxy_endpoint_repository_mock.go . MCPProxyEndpointRepository:MCPProxyEndpointRepositoryMock
type MCPProxyEndpointRepository interface {
	CreateEndpoint(ctx context.Context, tx *gorm.DB, endpoint *models.MCPProxyEndpoint) error
	UpdateEndpoint(ctx context.Context, tx *gorm.DB, endpoint *models.MCPProxyEndpoint) error
	// ClearIdentityCleanupID drops the endpoint's identityCleanupId once the removal it names succeeded, if it is still
	// that ID.
	ClearIdentityCleanupID(ctx context.Context, endpointUUID uuid.UUID, cleanupID string) error
	// MarkIdentityCleanupID sets the endpoint's identityCleanupId to cleanupID while it is off identity security with
	// none set, and reports whether it did.
	MarkIdentityCleanupID(ctx context.Context, endpointUUID uuid.UUID, cleanupID string) (bool, error)
	DeleteEndpoint(ctx context.Context, tx *gorm.DB, endpointUUID uuid.UUID) error
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
	// GetEndpointEnvByArtifact returns the (endpoint, environment) row whose gateway artifact is artifactUUID, or
	// gorm.ErrRecordNotFound.
	GetEndpointEnvByArtifact(ctx context.Context, artifactUUID uuid.UUID) (*models.MCPProxyEndpointEnvironment, error)
	// ListEndpointEnvironmentsByProxy returns every endpoint→environment binding
	// for the proxy. Used by the delete teardown to collect all artifact UUIDs.
	ListEndpointEnvironmentsByProxy(ctx context.Context, proxyUUID uuid.UUID) ([]models.MCPProxyEndpointEnvironment, error)
}

type mcpProxyEndpointRepository struct {
	db *gorm.DB
}

// NewMCPProxyEndpointRepository creates a new MCP proxy endpoint repository.
func NewMCPProxyEndpointRepository(db *gorm.DB) MCPProxyEndpointRepository {
	return &mcpProxyEndpointRepository{db: db}
}

func (r *mcpProxyEndpointRepository) CreateEndpoint(ctx context.Context, tx *gorm.DB, endpoint *models.MCPProxyEndpoint) error {
	if endpoint == nil {
		return fmt.Errorf("endpoint is required")
	}
	if endpoint.UUID == uuid.Nil {
		endpoint.UUID = uuid.New()
	}
	if err := tx.WithContext(ctx).Create(endpoint).Error; err != nil {
		return fmt.Errorf("failed to create MCP proxy endpoint: %w", err)
	}
	return nil
}

// UpdateEndpoint writes the endpoint, keeping the stored configuration's capabilityDiscovery key whatever
// endpoint.Configuration carries: that key is the capability discovery's bookkeeping, written only by its own
// conditional statements (MCPCapabilityDiscoveryRepository) without the proxy lock a binding write holds. Taking it
// from the row in the same statement means a discovery queued, claimed or recorded between the caller's read and
// this write is never overwritten.
func (r *mcpProxyEndpointRepository) UpdateEndpoint(ctx context.Context, tx *gorm.DB, endpoint *models.MCPProxyEndpoint) error {
	if endpoint == nil || endpoint.UUID == uuid.Nil {
		return gorm.ErrRecordNotFound
	}
	configuration, err := json.Marshal(endpoint.Configuration)
	if err != nil {
		return fmt.Errorf("failed to encode MCP proxy endpoint configuration: %w", err)
	}
	result := tx.WithContext(ctx).Exec(`UPDATE mcp_proxy_endpoints SET handle = ?, name = ?, status = ?, updated_at = ?,
		configuration = CASE WHEN configuration->'capabilityDiscovery' IS NULL THEN ?::jsonb - 'capabilityDiscovery'
			ELSE jsonb_set(?::jsonb, '{capabilityDiscovery}', configuration->'capabilityDiscovery') END
		WHERE uuid = ?`,
		endpoint.Handle, endpoint.Name, endpoint.Status, time.Now(), string(configuration), string(configuration), endpoint.UUID)
	if result.Error != nil {
		return fmt.Errorf("failed to update MCP proxy endpoint: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ClearIdentityCleanupID removes the identityCleanupId key from the endpoint's configuration in place, so nothing else
// a binding write stored since is touched, and only while it is cleanupID: a write that left identity again since set
// a new ID, whose removal is still to run. A missing endpoint, or a different ID, is not an error.
func (r *mcpProxyEndpointRepository) ClearIdentityCleanupID(ctx context.Context, endpointUUID uuid.UUID, cleanupID string) error {
	if err := r.db.WithContext(ctx).Exec(
		`UPDATE mcp_proxy_endpoints SET configuration = configuration - 'identityCleanupId'
		WHERE uuid = ? AND configuration->>'identityCleanupId' = ?`,
		endpointUUID, cleanupID).Error; err != nil {
		return fmt.Errorf("failed to clear the MCP proxy endpoint's identity cleanup ID: %w", err)
	}
	return nil
}

// MarkIdentityCleanupID sets identityCleanupId to cleanupID in place, only while the endpoint is off identity security
// (security and its identity variant not both enabled, as mcpIdentityEnabled reads them) and has no identityCleanupId:
// a registration that finished after the binding left identity, and after that write's own removal, leaves a resource
// server nothing else would remove. It reports whether the endpoint was marked.
func (r *mcpProxyEndpointRepository) MarkIdentityCleanupID(ctx context.Context, endpointUUID uuid.UUID, cleanupID string) (bool, error) {
	result := r.db.WithContext(ctx).Exec(
		`UPDATE mcp_proxy_endpoints SET configuration = jsonb_set(configuration, '{identityCleanupId}', to_jsonb(?::text))
		WHERE uuid = ? AND configuration->'identityCleanupId' IS NULL
			AND NOT (COALESCE(configuration#>>'{security,enabled}', '') = 'true'
				AND COALESCE(configuration#>>'{security,identity,enabled}', '') = 'true')`,
		cleanupID, endpointUUID)
	if result.Error != nil {
		return false, fmt.Errorf("failed to mark the MCP proxy endpoint for identity cleanup: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *mcpProxyEndpointRepository) DeleteEndpoint(ctx context.Context, tx *gorm.DB, endpointUUID uuid.UUID) error {
	result := tx.WithContext(ctx).Where("uuid = ?", endpointUUID).Delete(&models.MCPProxyEndpoint{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete MCP proxy endpoint: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *mcpProxyEndpointRepository) GetEndpoint(ctx context.Context, endpointUUID uuid.UUID) (*models.MCPProxyEndpoint, error) {
	var endpoint models.MCPProxyEndpoint
	err := r.db.WithContext(ctx).
		Preload("Environments").
		Where("uuid = ?", endpointUUID).
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
		Where("mcp_proxy_uuid = ?", proxyUUID).
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
		Where("endpoint_uuid = ? AND environment_uuid = ?", endpointUUID, envUUID).
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
		Where("endpoint_uuid = ?", endpointUUID).
		Find(&envs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP proxy endpoint environments: %w", err)
	}
	return envs, nil
}

func (r *mcpProxyEndpointRepository) GetEndpointEnvByProxyAndEnv(ctx context.Context, proxyUUID, envUUID uuid.UUID) (*models.MCPProxyEndpointEnvironment, error) {
	var ee models.MCPProxyEndpointEnvironment
	err := r.db.WithContext(ctx).
		Where("mcp_proxy_uuid = ? AND environment_uuid = ?", proxyUUID, envUUID).
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
		Where("mcp_proxy_uuid = ?", proxyUUID).
		Find(&envs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list MCP proxy endpoint environments by proxy: %w", err)
	}
	return envs, nil
}

func (r *mcpProxyEndpointRepository) GetEndpointEnvByArtifact(ctx context.Context, artifactUUID uuid.UUID) (*models.MCPProxyEndpointEnvironment, error) {
	var ee models.MCPProxyEndpointEnvironment
	err := r.db.WithContext(ctx).Where("artifact_uuid = ?", artifactUUID).Take(&ee).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to resolve MCP proxy endpoint environment: %w", err)
	}
	return &ee, nil
}

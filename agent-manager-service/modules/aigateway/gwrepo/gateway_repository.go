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
	"time"

	"gorm.io/gorm"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// IdentityProviderWithContext is an identity provider mirror row enriched with the
// gateway and environment it is registered to, for the org-wide listing.
// EnvironmentUUID is the environment id from gateway_environments.
type IdentityProviderWithContext struct {
	gwmodels.GatewayIdentityProvider
	GatewayName     string `gorm:"column:gateway_name"`
	EnvironmentUUID string `gorm:"column:environment_uuid"`
}

// GatewayFilterOptions defines filtering options for gateway queries
type GatewayFilterOptions struct {
	OrganizationID string
	// FunctionalityType filters on one exact role (ingress, egress, both).
	FunctionalityType *string
	// FunctionalityTypeIn filters on a role set, e.g. gwmodels.IngressGatewayRoles.
	// Applied as IN (?) alongside, not instead of, FunctionalityType.
	FunctionalityTypeIn []string
	Status              *bool   // Filter by is_active
	Hosting             string  // Filter by hosting (managed, self_hosted); empty for any
	EnvironmentID       *string // Filter by environment (via gateway_environments)
	Limit               int     // Pagination limit (0 = no limit)
	Offset              int     // Pagination offset
}

// GatewayFailureWindow bounds which disconnected gateways count as failed.
//
// Half-open, [NotOlderThan, StaleBefore): a gateway's updated_at must be before
// StaleBefore and at or after NotOlderThan. Both bounds are needed because
// updated_at is when liveness last changed, which conflates two very different
// situations — a gateway that dropped five minutes ago, and one nobody has
// touched in months.
type GatewayFailureWindow struct {
	// StaleBefore excludes gateways that only just disconnected: down, but not
	// yet down long enough to be worth reporting.
	StaleBefore time.Time
	// NotOlderThan excludes gateways nothing has touched since before it —
	// decommissioned or abandoned rows, which would otherwise be reported as
	// failing forever and drown out the ones that just broke.
	NotOlderThan time.Time
}

// GatewayFailureCounts is the result of one cross-organization health sweep:
// how many gateways exist and how many of them have been disconnected past the
// caller's threshold. Returned together because they come from a single query —
// two separate counts could be taken either side of a status change and report
// a fleet state that never existed.
type GatewayFailureCounts struct {
	// Total is every gateway in the deployment, abandoned ones included.
	Total int64
	// Failed is the gateways inside the failure window.
	Failed int64
	// Abandoned is the disconnected gateways older than the window's far edge.
	// Reported separately because they are excluded from the denominator the
	// failure percentage is taken over — see GatewayFailureWindow.
	Abandoned int64
}

// GatewayRepository defines the interface for gateway data access
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg gwrepomocks -out gwrepomocks/gateway_repository_mock.go . GatewayRepository:GatewayRepositoryMock
type GatewayRepository interface {
	// Gateway operations
	Create(gateway *gwmodels.Gateway) error
	GetByUUID(gatewayId string) (*gwmodels.Gateway, error)
	GetByOrganizationID(ctx context.Context, ouID string) ([]*gwmodels.Gateway, error)
	GetByNameAndOrgID(name, ouID string) (*gwmodels.Gateway, error)
	List() ([]*gwmodels.Gateway, error)
	ListWithFilters(filters GatewayFilterOptions) ([]*gwmodels.Gateway, error)
	ListWithFiltersCtx(ctx context.Context, filters GatewayFilterOptions) ([]*gwmodels.Gateway, error)
	CountWithFilters(filters GatewayFilterOptions) (int64, error)
	Delete(gatewayID, ouID string) error
	UpdateGateway(gateway *gwmodels.Gateway) error
	UpdateActiveStatus(gatewayId string, isActive bool) error

	// Cross-organization fleet health. Both deliberately carry no ou_id
	// predicate — they back the platform-admin failure summary, which reports
	// on every tenant at once. Nothing else may call them.
	CountFailureSummaryAllOrgs(ctx context.Context, window GatewayFailureWindow) (GatewayFailureCounts, error)
	ListFailedGatewaysAllOrgs(ctx context.Context, window GatewayFailureWindow, limit int) ([]*gwmodels.Gateway, error)

	// Transaction-scoped operations. The ingress cap is enforced in the service under
	// pg_advisory_xact_lock, so the service needs to drive a transaction it can also
	// insert the gateway row and its mappings into.
	Transaction(fn func(tx *gorm.DB) error) error
	AcquireEnvironmentLock(tx *gorm.DB, environmentID string) error
	AcquireGatewayLock(tx *gorm.DB, gatewayID string) error
	CreateTx(tx *gorm.DB, gateway *gwmodels.Gateway) error
	CreateEnvironmentMappingTx(tx *gorm.DB, mapping *gwmodels.GatewayEnvironmentMapping) error
	CountIngressCapableInEnvironment(tx *gorm.DB, environmentID string) (int64, error)

	// Gateway association checking operations
	HasGatewayDeployments(gatewayID, ouID string) (bool, error)

	// Token operations
	CreateToken(token *gwmodels.GatewayToken) error
	GetActiveTokensByGatewayUUID(gatewayId string) ([]*gwmodels.GatewayToken, error)
	GetTokenByUUID(tokenId string) (*gwmodels.GatewayToken, error)
	GetActiveTokenByPrefix(ctx context.Context, tokenPrefix string) (*gwmodels.GatewayToken, error)
	RevokeToken(tokenId string) error
	CountActiveTokens(gatewayId string) (int, error)

	// Gateway-Environment mapping operations
	CreateEnvironmentMapping(mapping *gwmodels.GatewayEnvironmentMapping) error
	DeleteEnvironmentMapping(gatewayID, environmentID string) error
	// DeleteEnvironmentMappingsByEnvironmentID removes every gateway↔env mapping for this env.
	// Returns the number of rows deleted. Used during env deletion to cascade clean the link table.
	DeleteEnvironmentMappingsByEnvironmentID(environmentID string) (int64, error)
	GetEnvironmentMappingsByGatewayID(gatewayID string) ([]gwmodels.GatewayEnvironmentMapping, error)
	GetEnvironmentMappingsByGatewayIDs(gatewayIDs []string) (map[string][]gwmodels.GatewayEnvironmentMapping, error)
	GetEnvironmentMappingsByEnvironmentID(environmentID string) ([]gwmodels.GatewayEnvironmentMapping, error)
	EnvironmentMappingExists(gatewayID, environmentID string) (bool, error)

	// Identity provider mirror operations
	UpsertIdentityProvider(provider *gwmodels.GatewayIdentityProvider) error
	DeleteIdentityProvider(gatewayID, name string) error
	ListIdentityProvidersByGateway(gatewayID string) ([]gwmodels.GatewayIdentityProvider, error)
	ListIdentityProvidersByEnvironment(environmentID string) ([]gwmodels.GatewayIdentityProvider, error)
	ListIdentityProvidersByOrg(ouID string) ([]IdentityProviderWithContext, error)

	// WithTx returns the repository bound to tx.
	WithTx(tx *gorm.DB) GatewayRepository
}

// GatewayRepo implements GatewayRepository using GORM
type GatewayRepo struct {
	db *gorm.DB
}

// NewGatewayRepo creates a new gateway repository
func NewGatewayRepo(db *gorm.DB) GatewayRepository {
	return &GatewayRepo{db: db}
}

// WithTx returns the repository bound to tx.
func (r *GatewayRepo) WithTx(tx *gorm.DB) GatewayRepository {
	return &GatewayRepo{db: tx}
}

// Create inserts a new gateway
func (r *GatewayRepo) Create(gateway *gwmodels.Gateway) error {
	return r.CreateTx(r.db, gateway)
}

// GetByUUID retrieves a gateway by ID
func (r *GatewayRepo) GetByUUID(gatewayId string) (*gwmodels.Gateway, error) {
	var gateway gwmodels.Gateway
	err := r.db.Where("id = ?", gatewayId).First(&gateway).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return &gateway, nil
}

// GetByOrganizationID retrieves all gateways for an organization
func (r *GatewayRepo) GetByOrganizationID(ctx context.Context, ouID string) ([]*gwmodels.Gateway, error) {
	var gateways []*gwmodels.Gateway
	err := r.db.WithContext(ctx).Where("org_id = ?", ouID).
		Order("created_at DESC").
		Find(&gateways).Error
	return gateways, err
}

// GetByNameAndOrgID checks if a gateway with the given name exists within an organization
func (r *GatewayRepo) GetByNameAndOrgID(name, ouID string) (*gwmodels.Gateway, error) {
	var gateway gwmodels.Gateway
	err := r.db.Where("name = ? AND org_id = ?", name, ouID).First(&gateway).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrGatewayNotFound
		}
		return nil, err
	}
	return &gateway, nil
}

// List retrieves all gateways
func (r *GatewayRepo) List() ([]*gwmodels.Gateway, error) {
	var gateways []*gwmodels.Gateway
	err := r.db.Order("created_at DESC").Find(&gateways).Error
	return gateways, err
}

// ListWithFilters retrieves gateways with optional filtering and pagination
func (r *GatewayRepo) ListWithFilters(filters GatewayFilterOptions) ([]*gwmodels.Gateway, error) {
	return r.ListWithFiltersCtx(context.Background(), filters)
}

func (r *GatewayRepo) ListWithFiltersCtx(ctx context.Context, filters GatewayFilterOptions) ([]*gwmodels.Gateway, error) {
	query := r.buildFilterQuery(filters).WithContext(ctx)

	// Apply pagination at database level
	if filters.Limit > 0 {
		query = query.Limit(filters.Limit)
	}
	if filters.Offset > 0 {
		query = query.Offset(filters.Offset)
	}

	var gateways []*gwmodels.Gateway
	err := query.Order("mod_ai_gateway_gateways.created_at DESC, mod_ai_gateway_gateways.id").Find(&gateways).Error
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
	query := r.db.Model(&gwmodels.Gateway{})

	// Filter by organization
	if filters.OrganizationID != "" {
		query = query.Where("mod_ai_gateway_gateways.org_id = ?", filters.OrganizationID)
	}

	// Filter by functionality type
	if filters.FunctionalityType != nil && *filters.FunctionalityType != "" {
		query = query.Where("functionality_type = ?", *filters.FunctionalityType)
	}

	// Filter by a set of functionality types
	if len(filters.FunctionalityTypeIn) > 0 {
		query = query.Where("functionality_type IN ?", filters.FunctionalityTypeIn)
	}

	if filters.Hosting != "" {
		query = query.Where("hosting = ?", filters.Hosting)
	}

	// Filter by status (is_active)
	if filters.Status != nil {
		query = query.Where("is_active = ?", *filters.Status)
	}

	// Filter by environment (via gateway_environments); one row per gateway and environment, so no duplicates.
	if filters.EnvironmentID != nil && *filters.EnvironmentID != "" {
		query = query.Joins("INNER JOIN mod_ai_gateway_gateway_environments ON mod_ai_gateway_gateway_environments.gateway_id = mod_ai_gateway_gateways.id").
			Where("mod_ai_gateway_gateway_environments.environment_id = ?", *filters.EnvironmentID)
	}

	return query
}

// Delete removes a gateway with organization isolation. Its environment links, tokens, identity providers and event
// state cascade; its deployment status rows and deployments are removed here first, because their foreign keys to
// the gateway take no action (schema rule 7).
func (r *GatewayRepo) Delete(gatewayID, ouID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("gateway_id = ? AND org_id = ?", gatewayID, ouID).
			Delete(&gwmodels.DeploymentStatusRecord{}).Error; err != nil {
			return err
		}
		// A deployment can be another's base; clear those links before deleting.
		if err := tx.Model(&gwmodels.Deployment{}).Where("gateway_id = ? AND org_id = ?", gatewayID, ouID).
			Update("base_deployment_id", nil).Error; err != nil {
			return err
		}
		if err := tx.Where("gateway_id = ? AND org_id = ?", gatewayID, ouID).
			Delete(&gwmodels.Deployment{}).Error; err != nil {
			return err
		}
		result := tx.Where("id = ? AND org_id = ?", gatewayID, ouID).Delete(&gwmodels.Gateway{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return utils.ErrGatewayNotFound
		}
		return nil
	})
}

// UpdateGateway updates gateway details. The manifest column is deliberately not
// written here: gateway-reported manifests are large and re-pushed on every gateway
// (re)connect, so they live in the manifest cache instead. The column is left in place
// (and still read as a fallback) for rows written before that move.
func (r *GatewayRepo) UpdateGateway(gateway *gwmodels.Gateway) error {
	gateway.UpdatedAt = dbx.Now()
	res := r.db.Model(&gwmodels.Gateway{}).
		Where("id = ?", gateway.UUID).
		Updates(map[string]interface{}{
			"display_name": gateway.DisplayName,
			"description":  gateway.Description,
			"is_critical":  gateway.IsCritical,
			"properties":   dbx.JSON(gateway.Properties),
			"runtime_url":  gateway.RuntimeURL,
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
	res := r.db.Model(&gwmodels.Gateway{}).
		Where("id = ?", gatewayId).
		Updates(map[string]interface{}{
			"is_active":  isActive,
			"updated_at": dbx.Now(),
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
func (r *GatewayRepo) CreateToken(token *gwmodels.GatewayToken) error {
	token.CreatedAt = dbx.Now()
	if token.OrgID == uuid.Nil {
		orgID, err := r.gatewayOrgID(r.db, token.GatewayUUID)
		if err != nil {
			return err
		}
		token.OrgID = orgID
	}
	return r.db.Create(token).Error
}

// gatewayOrgID is the organization a gateway belongs to, for child rows that carry org_id.
func (r *GatewayRepo) gatewayOrgID(db *gorm.DB, gatewayID uuid.UUID) (uuid.UUID, error) {
	var g gwmodels.Gateway
	if err := db.Select("org_id").Where("id = ?", gatewayID).First(&g).Error; err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(g.OUID)
}

// GetActiveTokensByGatewayUUID retrieves all active tokens for a gateway
func (r *GatewayRepo) GetActiveTokensByGatewayUUID(gatewayId string) ([]*gwmodels.GatewayToken, error) {
	var tokens []*gwmodels.GatewayToken
	err := r.db.Where("gateway_id = ? AND status = ?", gatewayId, "active").
		Order("created_at DESC").
		Find(&tokens).Error
	return tokens, err
}

// GetTokenByUUID retrieves a specific token by UUID
func (r *GatewayRepo) GetTokenByUUID(tokenId string) (*gwmodels.GatewayToken, error) {
	var token gwmodels.GatewayToken
	err := r.db.Where("id = ?", tokenId).First(&token).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return &token, nil
}

// GetActiveTokenByPrefix retrieves an active token by its unique prefix (UUID)
// This enables O(1) indexed lookup instead of O(N) full table scan.
// Takes ctx so a cancelled/timed-out request actually stops the query instead
// of continuing to hold a DB connection after the caller has given up.
func (r *GatewayRepo) GetActiveTokenByPrefix(ctx context.Context, tokenPrefix string) (*gwmodels.GatewayToken, error) {
	var token gwmodels.GatewayToken
	err := r.db.WithContext(ctx).Where("token_prefix = ? AND status = ?", tokenPrefix, "active").
		First(&token).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return &token, nil
}

// RevokeToken updates token status to revoked
func (r *GatewayRepo) RevokeToken(tokenId string) error {
	now := dbx.Now()
	result := r.db.Model(&gwmodels.GatewayToken{}).
		Where("id = ?", tokenId).
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
	err := r.db.Model(&gwmodels.GatewayToken{}).
		Where("gateway_id = ? AND status = ?", gatewayId, "active").
		Count(&count).Error
	return int(count), err
}

// HasGatewayDeployments reports whether the gateway holds any artifact: a row that names a deployment and isn't
// UNDEPLOYED. PENDING rows were never sent, and UNDEPLOYED rows are kept for good.
func (r *GatewayRepo) HasGatewayDeployments(gatewayID, ouID string) (bool, error) {
	var count int64
	err := r.db.Model(&gwmodels.DeploymentStatusRecord{}).
		Where("gateway_id = ? AND org_id = ? AND deployment_id IS NOT NULL AND status <> ?",
			gatewayID, ouID, gwmodels.DeploymentStatusUndeployed).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// CreateEnvironmentMapping creates a mapping between a gateway and an environment
func (r *GatewayRepo) CreateEnvironmentMapping(mapping *gwmodels.GatewayEnvironmentMapping) error {
	return r.CreateEnvironmentMappingTx(r.db, mapping)
}

// DeleteEnvironmentMapping deletes a mapping between a gateway and an environment
func (r *GatewayRepo) DeleteEnvironmentMapping(gatewayID, environmentID string) error {
	result := r.db.Where("gateway_id = ? AND environment_id = ?", gatewayID, environmentID).
		Delete(&gwmodels.GatewayEnvironmentMapping{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// GetEnvironmentMappingsByGatewayID retrieves all environment mappings for a gateway
func (r *GatewayRepo) GetEnvironmentMappingsByGatewayID(gatewayID string) ([]gwmodels.GatewayEnvironmentMapping, error) {
	var mappings []gwmodels.GatewayEnvironmentMapping
	err := r.db.Where("gateway_id = ?", gatewayID).Find(&mappings).Error
	return mappings, err
}

// GetEnvironmentMappingsByGatewayIDs retrieves environment mappings for multiple gateways in bulk
// Returns a map of gatewayID -> []GatewayEnvironmentMapping
// This avoids N+1 queries when fetching environments for multiple gateways
func (r *GatewayRepo) GetEnvironmentMappingsByGatewayIDs(gatewayIDs []string) (map[string][]gwmodels.GatewayEnvironmentMapping, error) {
	if len(gatewayIDs) == 0 {
		return make(map[string][]gwmodels.GatewayEnvironmentMapping), nil
	}

	var mappings []gwmodels.GatewayEnvironmentMapping
	for _, chunk := range dbx.Chunks(gatewayIDs) {
		var part []gwmodels.GatewayEnvironmentMapping
		if err := r.db.Where("gateway_id IN ?", chunk).Find(&part).Error; err != nil {
			return nil, err
		}
		mappings = append(mappings, part...)
	}

	// Group mappings by gateway UUID
	result := make(map[string][]gwmodels.GatewayEnvironmentMapping)
	for _, mapping := range mappings {
		gwID := mapping.GatewayUUID.String()
		result[gwID] = append(result[gwID], mapping)
	}

	return result, nil
}

// GetEnvironmentMappingsByEnvironmentID retrieves all gateway mappings for an environment
func (r *GatewayRepo) GetEnvironmentMappingsByEnvironmentID(environmentID string) ([]gwmodels.GatewayEnvironmentMapping, error) {
	var mappings []gwmodels.GatewayEnvironmentMapping
	err := r.db.Where("environment_id = ?", environmentID).Find(&mappings).Error
	return mappings, err
}

// DeleteEnvironmentMappingsByEnvironmentID removes every mapping pointing at the given env.
func (r *GatewayRepo) DeleteEnvironmentMappingsByEnvironmentID(environmentID string) (int64, error) {
	result := r.db.Where("environment_id = ?", environmentID).
		Delete(&gwmodels.GatewayEnvironmentMapping{})
	return result.RowsAffected, result.Error
}

// EnvironmentMappingExists checks if a mapping exists between a gateway and an environment
func (r *GatewayRepo) EnvironmentMappingExists(gatewayID, environmentID string) (bool, error) {
	var count int64
	err := r.db.Model(&gwmodels.GatewayEnvironmentMapping{}).
		Where("gateway_id = ? AND environment_id = ?", gatewayID, environmentID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// UpsertIdentityProvider creates or updates an identity provider mirror row,
// keyed by (gateway_id, name). The reserved internal provider is never mirrored.
func (r *GatewayRepo) UpsertIdentityProvider(provider *gwmodels.GatewayIdentityProvider) error {
	if provider.Name == models.ReservedIdentityProviderName {
		return httpx.ErrInvalidInput
	}
	if provider.OrgID == uuid.Nil {
		orgID, err := r.gatewayOrgID(r.db, provider.GatewayUUID)
		if err != nil {
			return err
		}
		provider.OrgID = orgID
	}
	_, err := dbx.Upsert(r.db, provider, func(db *gorm.DB) error {
		return db.Model(&gwmodels.GatewayIdentityProvider{}).
			Where("gateway_id = ? AND name = ?", provider.GatewayUUID, provider.Name).
			Updates(map[string]any{
				"issuer": provider.Issuer, "jwks_uri": provider.JWKSUri, "description": provider.Description,
				"type": provider.Type, "jwks_skip_tls_verify": provider.JWKSSkipTLSVerify, "updated_at": provider.UpdatedAt,
			}).Error
	})
	return err
}

// DeleteIdentityProvider removes an identity provider mirror row by gateway and
// name. System providers (and the reserved internal provider) cannot be deleted.
func (r *GatewayRepo) DeleteIdentityProvider(gatewayID, name string) error {
	if name == models.ReservedIdentityProviderName || models.IsSystemIdentityProvider(name) {
		return httpx.ErrInvalidInput
	}
	var existing gwmodels.GatewayIdentityProvider
	err := r.db.Where("gateway_id = ? AND name = ?", gatewayID, name).First(&existing).Error
	if err != nil {
		return err
	}
	if existing.Type == gwmodels.IdentityProviderTypeSystem {
		return httpx.ErrInvalidInput
	}
	return r.db.Where("gateway_id = ? AND name = ?", gatewayID, name).
		Delete(&gwmodels.GatewayIdentityProvider{}).Error
}

// ListIdentityProvidersByGateway returns the identity providers mirrored for a
// gateway, excluding the reserved internal provider.
func (r *GatewayRepo) ListIdentityProvidersByGateway(gatewayID string) ([]gwmodels.GatewayIdentityProvider, error) {
	var providers []gwmodels.GatewayIdentityProvider
	err := r.db.Where("gateway_id = ? AND name <> ?", gatewayID, models.ReservedIdentityProviderName).
		Order("name ASC").
		Find(&providers).Error
	return providers, err
}

// ListIdentityProvidersByEnvironment returns the union (deduped by name) of
// identity providers configured on the gateways mapped to an environment,
// excluding the reserved internal provider.
func (r *GatewayRepo) ListIdentityProvidersByEnvironment(environmentID string) ([]gwmodels.GatewayIdentityProvider, error) {
	var rows []gwmodels.GatewayIdentityProvider
	err := r.db.
		Joins("JOIN mod_ai_gateway_gateway_environments ON mod_ai_gateway_gateway_environments.gateway_id = mod_ai_gateway_gateway_identity_providers.gateway_id").
		Where("mod_ai_gateway_gateway_environments.environment_id = ? AND mod_ai_gateway_gateway_identity_providers.name <> ?", environmentID, models.ReservedIdentityProviderName).
		Order("mod_ai_gateway_gateway_identity_providers.name ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	// Dedup by name — the same provider may exist on multiple gateways in the env.
	seen := make(map[string]bool)
	deduped := make([]gwmodels.GatewayIdentityProvider, 0, len(rows))
	for _, row := range rows {
		if seen[row.Name] {
			continue
		}
		seen[row.Name] = true
		deduped = append(deduped, row)
	}
	return deduped, nil
}

// ListIdentityProvidersByOrg returns every identity provider mirrored across the
// organization's gateways, enriched with gateway + environment context. The
// reserved internal provider is excluded. A provider whose gateway maps to
// multiple environments appears once per environment.
func (r *GatewayRepo) ListIdentityProvidersByOrg(ouID string) ([]IdentityProviderWithContext, error) {
	var results []IdentityProviderWithContext
	err := r.db.Table("mod_ai_gateway_gateway_identity_providers").
		Select("mod_ai_gateway_gateway_identity_providers.*, mod_ai_gateway_gateways.name AS gateway_name, mod_ai_gateway_gateway_environments.environment_id AS environment_uuid").
		Joins("JOIN mod_ai_gateway_gateways ON mod_ai_gateway_gateways.id = mod_ai_gateway_gateway_identity_providers.gateway_id").
		Joins("LEFT JOIN mod_ai_gateway_gateway_environments ON mod_ai_gateway_gateway_environments.gateway_id = mod_ai_gateway_gateway_identity_providers.gateway_id").
		Where("mod_ai_gateway_gateways.org_id = ? AND mod_ai_gateway_gateway_identity_providers.name <> ?", ouID, models.ReservedIdentityProviderName).
		Order("mod_ai_gateway_gateway_identity_providers.name ASC").
		Scan(&results).Error
	return results, err
}

// Transaction runs fn inside a database transaction.
func (r *GatewayRepo) Transaction(fn func(tx *gorm.DB) error) error {
	return r.db.Transaction(fn)
}

// AcquireEnvironmentLock locks the environment row for the rest of tx, serialising concurrent ingress-cap checks for
// that environment.
func (r *GatewayRepo) AcquireEnvironmentLock(tx *gorm.DB, environmentID string) error {
	return dbx.LockRow(tx, "environments", "id", "id = ?", environmentID)
}

// AcquireGatewayLock locks the gateway row for the rest of tx, serialising concurrent environment assignments of one
// gateway.
func (r *GatewayRepo) AcquireGatewayLock(tx *gorm.DB, gatewayID string) error {
	return dbx.LockRow(tx, "mod_ai_gateway_gateways", "id", "id = ?", gatewayID)
}

// CreateTx inserts a gateway inside an existing transaction. Mirrors Create,
// including the is_active=false default at registration.
func (r *GatewayRepo) CreateTx(tx *gorm.DB, gateway *gwmodels.Gateway) error {
	if _, err := repositories.EnsureOrg(tx.Statement.Context, tx, gateway.OUID); err != nil {
		return err
	}
	now := dbx.Now()
	gateway.CreatedAt, gateway.UpdatedAt = now, now
	gateway.IsActive = false
	if gateway.Hosting == "" {
		gateway.Hosting = gwmodels.GatewayHostingManaged
	}
	if gateway.Properties == nil {
		gateway.Properties = map[string]interface{}{}
	}
	return tx.Create(gateway).Error
}

// CreateEnvironmentMappingTx inserts a gateway↔environment mapping inside an
// existing transaction.
func (r *GatewayRepo) CreateEnvironmentMappingTx(tx *gorm.DB, mapping *gwmodels.GatewayEnvironmentMapping) error {
	if mapping.OrgID == uuid.Nil {
		orgID, err := r.gatewayOrgID(tx, mapping.GatewayUUID)
		if err != nil {
			return err
		}
		mapping.OrgID = orgID
	}
	if mapping.CreatedAt.IsZero() {
		mapping.CreatedAt = dbx.Now()
	}
	return tx.Create(mapping).Error
}

// CountIngressCapableInEnvironment counts non-deleted ingress-capable gateways mapped
// to the environment. Deliberately omits is_active: the cap is a membership rule, and
// is_active is WebSocket liveness that flaps.
func (r *GatewayRepo) CountIngressCapableInEnvironment(tx *gorm.DB, environmentID string) (int64, error) {
	var count int64
	err := tx.Table("mod_ai_gateway_gateway_environments AS m").
		Joins("JOIN mod_ai_gateway_gateways g ON g.id = m.gateway_id").
		Where("m.environment_id = ?", environmentID).
		Where("g.functionality_type IN ?", gwmodels.IngressGatewayRoles).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("failed to count ingress-capable gateways in environment %s: %w", environmentID, err)
	}
	return count, nil
}

// failedGatewayCondition is the SQL predicate for "failed": disconnected, down
// long enough to matter, and not so long that it is no longer news.
//
// is_active is WebSocket liveness and updated_at is when that liveness last
// changed (UpdateActiveStatus writes both together), so the pair reads as "has
// been down for a while" rather than "is down right now".
//
// Two consequences are inherent to the definition and not bugs here: a gateway
// whose row was edited while disconnected has a fresher updated_at and drops out
// until the staleness threshold passes again, and a gateway whose holding pod
// died without cleanup stays is_active=true and is never counted.
const failedGatewayCondition = managedGatewayCondition + " AND is_active = ? AND updated_at < ? AND updated_at >= ?"

// abandonedGatewayCondition matches the rows past the window's far edge:
// disconnected, and untouched for longer than the caller is willing to call a
// failure. Decommissioned or forgotten, not broken.
//
// is_active = false is load-bearing, not incidental. updated_at is equally old
// on a gateway that has been connected without interruption for months, because
// nothing writes the column while a connection holds. Matching on age alone
// would classify the healthiest gateways in the fleet as abandoned and pull them
// out of the denominator, inflating the failure percentage instead of sharpening
// it. Only a gateway that is both old and down is abandoned.
const abandonedGatewayCondition = managedGatewayCondition + " AND is_active = ? AND updated_at < ?"

// managedGatewayCondition limits the failure summary to the gateways the platform runs.
const managedGatewayCondition = "hosting = '" + gwmodels.GatewayHostingManaged + "'"

// CountFailureSummaryAllOrgs counts every managed gateway in the deployment, how many
// are failed, and how many are abandoned, in one round trip.
//
// Conditional aggregation rather than three queries: all three counts must
// describe the same instant, or the arithmetic between them (total - abandoned)
// describes a fleet that never existed. It is also one scan instead of three.
//
// For a disconnected gateway the three buckets partition cleanly: too fresh to
// be failed, inside the window, or past it and abandoned.
func (r *GatewayRepo) CountFailureSummaryAllOrgs(
	ctx context.Context, window GatewayFailureWindow,
) (GatewayFailureCounts, error) {
	var counts GatewayFailureCounts
	err := r.db.WithContext(ctx).
		Table("mod_ai_gateway_gateways").
		Select("COUNT(*) AS total"+
			", COALESCE(SUM(CASE WHEN "+failedGatewayCondition+" THEN 1 ELSE 0 END), 0) AS failed"+
			", COALESCE(SUM(CASE WHEN "+abandonedGatewayCondition+" THEN 1 ELSE 0 END), 0) AS abandoned",
			false, window.StaleBefore, window.NotOlderThan, false, window.NotOlderThan).
		Where(managedGatewayCondition).
		Scan(&counts).Error
	if err != nil {
		return GatewayFailureCounts{}, fmt.Errorf("failed to count gateway failure summary: %w", err)
	}
	return counts, nil
}

// ListFailedGatewaysAllOrgs returns the failed gateways across every
// organization, oldest failure first, capped at limit rows.
//
// Oldest first because a truncated list should keep the longest-running
// failures, which are the ones worth naming. The cap is required, not
// defensive: this list is unbounded in the number of tenants and is served to a
// polling caller.
func (r *GatewayRepo) ListFailedGatewaysAllOrgs(
	ctx context.Context, window GatewayFailureWindow, limit int,
) ([]*gwmodels.Gateway, error) {
	if limit <= 0 {
		return []*gwmodels.Gateway{}, nil
	}
	var gateways []*gwmodels.Gateway
	err := r.db.WithContext(ctx).
		Model(&gwmodels.Gateway{}).
		Where(failedGatewayCondition, false, window.StaleBefore, window.NotOlderThan).
		// uuid breaks ties. Without it the "oldest N" page is not a stable set:
		// gateways that dropped in the same write batch share an updated_at, and
		// Postgres is free to order them differently between calls — so a capped
		// list could show a different subset each poll.
		Order("updated_at ASC, id ASC").
		Limit(limit).
		Find(&gateways).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list failed gateways: %w", err)
	}
	return gateways, nil
}

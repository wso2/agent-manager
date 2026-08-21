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

package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// PlatformGatewayService handles gateway business logic for API Platform integration
type PlatformGatewayService struct {
	gatewayRepo    repositories.GatewayRepository
	tokenCache     *TokenCache
	gatewayApplier GatewayConfigApplier
}

// NewPlatformGatewayService creates a new platform gateway service. gatewayApplier is
// nil in open-source deployments (gateway config applied out of band by
// manage-identity-provider.sh) and non-nil in cloud deployments, which patch the
// gateway runtime config in the same request that writes the mirror.
func NewPlatformGatewayService(
	gatewayRepo repositories.GatewayRepository,
	gatewayApplier GatewayConfigApplier,
) *PlatformGatewayService {
	// Initialize token cache with 5 minute TTL
	tokenCache := NewTokenCache(5 * time.Minute)

	return &PlatformGatewayService{
		gatewayRepo:    gatewayRepo,
		tokenCache:     tokenCache,
		gatewayApplier: gatewayApplier,
	}
}

// GatewayResponse represents the gateway DTO
type GatewayResponse struct {
	ID                string                 `json:"id"`
	OrganizationID    string                 `json:"organizationId"`
	Token             string                 `json:"token,omitempty"`
	Name              string                 `json:"name"`
	DisplayName       string                 `json:"displayName"`
	Description       string                 `json:"description"`
	Properties        map[string]interface{} `json:"properties,omitempty"`
	Vhost             string                 `json:"vhost"`
	RuntimeURL        string                 `json:"runtimeUrl,omitempty"`
	IsCritical        bool                   `json:"isCritical"`
	FunctionalityType string                 `json:"functionalityType"`
	IsActive          bool                   `json:"isActive"`
	CreatedAt         time.Time              `json:"createdAt"`
	UpdatedAt         time.Time              `json:"updatedAt"`
}

// GatewayListResponse represents a list of gateways
type GatewayListResponse struct {
	Count      int               `json:"count"`
	List       []GatewayResponse `json:"list"`
	Pagination Pagination        `json:"pagination"`
}

// TokenRotationResponse represents the response for token rotation
type TokenRotationResponse struct {
	ID        string    `json:"id"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"createdAt"`
	Message   string    `json:"message"`
}

// GatewayTokenInfo represents a token's metadata (no secret values exposed)
type GatewayTokenInfo struct {
	ID        string     `json:"id"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"createdAt"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
}

// GatewayTokenListResponse represents a list of token metadata
type GatewayTokenListResponse struct {
	Count int                `json:"count"`
	List  []GatewayTokenInfo `json:"list"`
}

// GatewayStatusResponse represents lightweight gateway status
type GatewayStatusResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IsActive   bool   `json:"isActive"`
	IsCritical bool   `json:"isCritical"`
}

// GatewayStatusListResponse represents a list of gateway statuses
type GatewayStatusListResponse struct {
	Count      int                     `json:"count"`
	List       []GatewayStatusResponse `json:"list"`
	Pagination Pagination              `json:"pagination"`
}

// GatewayArtifact represents an artifact deployed to a gateway
type GatewayArtifact struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// GatewayArtifactListResponse represents a list of gateway artifacts
type GatewayArtifactListResponse struct {
	Count      int               `json:"count"`
	List       []GatewayArtifact `json:"list"`
	Pagination Pagination        `json:"pagination"`
}

// Pagination represents pagination metadata
type Pagination struct {
	Total  int `json:"total"`
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

// RegisterGateway registers a new gateway with organization validation
func (s *PlatformGatewayService) RegisterGateway(
	ouID, name, displayName, description, vhost, runtimeURL string,
	isCritical bool, functionalityType string,
	properties map[string]interface{},
	environmentIDs []string,
) (*GatewayResponse, error) {
	// 1. Validate inputs
	if err := s.validateGatewayInput(ouID, name, displayName, vhost); err != nil {
		return nil, err
	}
	if err := validateGatewayRuntimeURL(runtimeURL); err != nil {
		return nil, err
	}

	role, err := normalizeGatewayRole(functionalityType)
	if err != nil {
		return nil, err
	}

	// 3. Check gateway name uniqueness within organization
	existing, err := s.gatewayRepo.GetByNameAndOrgID(name, ouID)
	if err != nil && !errors.Is(err, utils.ErrGatewayNotFound) {
		return nil, fmt.Errorf("failed to check gateway name uniqueness: %w", err)
	}
	if existing != nil {
		return nil, utils.ErrGatewayAlreadyExists
	}

	// 4. Generate UUID for gateway
	gatewayID := uuid.New().String()

	// 5. Parse and create Gateway model
	gatewayUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("invalid gateway UUID: %w", err)
	}

	// Initialize properties as empty map if nil (database column is NOT NULL)
	if properties == nil {
		properties = make(map[string]interface{})
	}

	gateway := &models.Gateway{
		UUID:                     gatewayUUID,
		OUID:                     ouID,
		Name:                     name,
		DisplayName:              displayName,
		Description:              description,
		Properties:               properties,
		Vhost:                    vhost,
		RuntimeURL:               strings.TrimSpace(runtimeURL),
		IsCritical:               isCritical,
		GatewayFunctionalityType: role,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}

	// Dedupe: a repeated environment ID would re-enter assignGatewayToEnvironmentTx in the
	// same tx, where the pooled-connection existence check cannot see the first insert.
	seenEnvIDs := make(map[string]struct{}, len(environmentIDs))
	uniqueEnvIDs := make([]string, 0, len(environmentIDs))
	for _, envID := range environmentIDs {
		if _, ok := seenEnvIDs[envID]; ok {
			continue
		}
		seenEnvIDs[envID] = struct{}{}
		uniqueEnvIDs = append(uniqueEnvIDs, envID)
	}

	if err := s.gatewayRepo.Transaction(func(tx *gorm.DB) error {
		if err := s.gatewayRepo.CreateTx(tx, gateway); err != nil {
			return fmt.Errorf("error while registering gateway: %w", err)
		}
		for _, envID := range uniqueEnvIDs {
			if err := s.assignGatewayToEnvironmentTx(tx, gateway, envID); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	response := &GatewayResponse{
		ID:                gateway.UUID.String(),
		OrganizationID:    gateway.OUID,
		Name:              gateway.Name,
		DisplayName:       gateway.DisplayName,
		Description:       gateway.Description,
		Properties:        gateway.Properties,
		Vhost:             gateway.Vhost,
		RuntimeURL:        gateway.RuntimeURL,
		IsCritical:        gateway.IsCritical,
		FunctionalityType: gateway.GatewayFunctionalityType,
		IsActive:          gateway.IsActive,
		CreatedAt:         gateway.CreatedAt,
		UpdatedAt:         gateway.UpdatedAt,
	}

	return response, nil
}

// GatewayListFilters contains optional filters for listing gateways
type GatewayListFilters struct {
	FunctionalityType *string // Filter by gateway role (ingress, egress, both)
	Status            *bool   // Filter by is_active status
	EnvironmentID     *string // Filter by environment UUID
}

// ListGateways retrieves gateways with constitution-compliant envelope structure and DB-level pagination
func (s *PlatformGatewayService) ListGateways(ouID *string, filters *GatewayListFilters, limit, offset int) (*GatewayListResponse, error) {
	// Build filter options
	filterOpts := repositories.GatewayFilterOptions{
		Limit:  limit,
		Offset: offset,
	}

	if ouID != nil && *ouID != "" {
		filterOpts.OrganizationID = *ouID
	}

	if filters != nil {
		if filters.FunctionalityType != nil {
			// Route the raw query value through the same alias normalization used at
			// registration (REGULAR -> both, AI -> egress) so the filter matches the
			// canonical roles actually stored in the database.
			normalized, err := normalizeGatewayRole(*filters.FunctionalityType)
			if err != nil {
				return nil, err
			}
			filterOpts.FunctionalityType = &normalized
		}
		filterOpts.Status = filters.Status
		filterOpts.EnvironmentID = filters.EnvironmentID
	}

	// Get total count (without pagination)
	total, err := s.gatewayRepo.CountWithFilters(filterOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to count gateways: %w", err)
	}

	// Get paginated results
	gateways, err := s.gatewayRepo.ListWithFilters(filterOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list gateways: %w", err)
	}

	// Convert to DTOs
	responses := make([]GatewayResponse, 0, len(gateways))
	for _, gw := range gateways {
		responses = append(responses, GatewayResponse{
			ID:                gw.UUID.String(),
			OrganizationID:    gw.OUID,
			Name:              gw.Name,
			DisplayName:       gw.DisplayName,
			Description:       gw.Description,
			Properties:        gw.Properties,
			Vhost:             gw.Vhost,
			RuntimeURL:        gw.RuntimeURL,
			IsCritical:        gw.IsCritical,
			FunctionalityType: gw.GatewayFunctionalityType,
			IsActive:          gw.IsActive,
			CreatedAt:         gw.CreatedAt,
			UpdatedAt:         gw.UpdatedAt,
		})
	}

	// Build constitution-compliant list response with pagination metadata
	listResponse := &GatewayListResponse{
		Count: len(responses),
		List:  responses,
		Pagination: Pagination{
			Total:  int(total),
			Offset: offset,
			Limit:  limit,
		},
	}

	return listResponse, nil
}

// resolveGateway looks the identifier up as a UUID, falling back to a name lookup
// within the org. A non-UUID identifier used to be rejected with a bare error that
// matched no case in handleGatewayErrors and surfaced as HTTP 500.
func (s *PlatformGatewayService) resolveGateway(identifier, ouID string) (*models.Gateway, error) {
	if _, err := uuid.Parse(identifier); err != nil {
		return normalizeGatewayLookup(s.gatewayRepo.GetByNameAndOrgID(identifier, ouID))
	}
	return normalizeGatewayLookup(s.gatewayRepo.GetByUUID(identifier))
}

// normalizeGatewayLookup reduces either repository lookup's "no such row" shapes — a
// gorm not-found error, the repository's own sentinel, or a nil gateway with no error
// — to ErrGatewayNotFound, and wraps anything else with context. Both branches of
// resolveGateway need this: a raw repository error matches no case in
// handleGatewayErrors and surfaces as a 500, and a nil gateway returned without an
// error is dereferenced by GetGateway.
func normalizeGatewayLookup(gateway *models.Gateway, err error) (*models.Gateway, error) {
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, utils.ErrGatewayNotFound) {
			return nil, utils.ErrGatewayNotFound
		}
		return nil, fmt.Errorf("failed to get gateway: %w", err)
	}
	if gateway == nil {
		return nil, utils.ErrGatewayNotFound
	}
	return gateway, nil
}

// GetGateway retrieves a gateway by UUID or by name, matching what the spec
// advertises for the path parameter ("Gateway UUID or name").
func (s *PlatformGatewayService) GetGateway(gatewayID, ouID string) (*GatewayResponse, error) {
	gateway, err := s.resolveGateway(gatewayID, ouID)
	if err != nil {
		return nil, err
	}

	if gateway.OUID != ouID {
		return nil, utils.ErrGatewayNotFound
	}

	response := &GatewayResponse{
		ID:                gateway.UUID.String(),
		OrganizationID:    gateway.OUID,
		Name:              gateway.Name,
		DisplayName:       gateway.DisplayName,
		Description:       gateway.Description,
		Properties:        gateway.Properties,
		Vhost:             gateway.Vhost,
		RuntimeURL:        gateway.RuntimeURL,
		IsCritical:        gateway.IsCritical,
		FunctionalityType: gateway.GatewayFunctionalityType,
		IsActive:          gateway.IsActive,
		CreatedAt:         gateway.CreatedAt,
		UpdatedAt:         gateway.UpdatedAt,
	}

	return response, nil
}

// SaveGatewayPolicyManifest stores the latest gateway-reported policy manifest in the
// in-process cache. Gateways re-push their full manifest on every heartbeat, so writing
// it to the jsonb column cost a large row update per push for data that is regenerated
// on the next push anyway. Every gateway runs the same policy bundle, so the cache keeps
// a single newest copy shared by all of them.
func (s *PlatformGatewayService) SaveGatewayPolicyManifest(ctx context.Context, gatewayID string, manifest map[string]interface{}) error {
	gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
	if err != nil {
		return err
	}
	if gateway == nil {
		return utils.ErrGatewayNotFound
	}
	logger.GetLogger(ctx).Info("Saving gateway policy manifest for gateway", "gateway_id", gatewayID)

	return gatewayManifestCache.Set(ctx, manifest)
}

// UpdateGateway updates gateway details
func (s *PlatformGatewayService) UpdateGateway(
	gatewayID, ouID string,
	description, displayName *string,
	isCritical *bool,
	properties *map[string]interface{},
	runtimeURL *string,
) (*GatewayResponse, error) {
	// Get existing gateway
	gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
	if err != nil {
		return nil, err
	}
	if gateway == nil {
		return nil, utils.ErrGatewayNotFound
	}
	if gateway.OUID != ouID {
		return nil, utils.ErrGatewayNotFound
	}

	if description != nil {
		gateway.Description = *description
	}
	if displayName != nil {
		gateway.DisplayName = *displayName
	}
	if isCritical != nil {
		gateway.IsCritical = *isCritical
	}
	if properties != nil {
		gateway.Properties = *properties
	}
	// The address is mutable — unlike the role it is a fact about placement, so a stale
	// one is simply wrong. Validated on every write for the same reason as at registration.
	if runtimeURL != nil {
		if err := validateGatewayRuntimeURL(*runtimeURL); err != nil {
			return nil, err
		}
		gateway.RuntimeURL = strings.TrimSpace(*runtimeURL)
	}
	gateway.UpdatedAt = time.Now()

	err = s.gatewayRepo.UpdateGateway(gateway)
	if err != nil {
		return nil, err
	}

	updatedGateway := &GatewayResponse{
		ID:                gateway.UUID.String(),
		OrganizationID:    gateway.OUID,
		Name:              gateway.Name,
		DisplayName:       gateway.DisplayName,
		Description:       gateway.Description,
		Properties:        gateway.Properties,
		Vhost:             gateway.Vhost,
		RuntimeURL:        gateway.RuntimeURL,
		IsCritical:        gateway.IsCritical,
		FunctionalityType: gateway.GatewayFunctionalityType,
		IsActive:          gateway.IsActive,
		CreatedAt:         gateway.CreatedAt,
		UpdatedAt:         gateway.UpdatedAt,
	}
	return updatedGateway, nil
}

// DeleteGateway deletes a gateway after verifying no active deployments exist
func (s *PlatformGatewayService) DeleteGateway(ctx context.Context, gatewayID, ouID string) error {
	// Validate UUID format
	if _, err := uuid.Parse(gatewayID); err != nil {
		return errors.New("invalid UUID format")
	}

	// Verify gateway exists and belongs to organization
	gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
	if err != nil {
		return err
	}
	if gateway == nil {
		return utils.ErrGatewayNotFound
	}
	if gateway.OUID != ouID {
		return utils.ErrGatewayNotFound
	}

	// Reject deletion if the gateway has active deployments (LLM providers/proxies)
	hasDeployments, err := s.gatewayRepo.HasGatewayDeployments(gatewayID, ouID)
	if err != nil {
		return fmt.Errorf("failed to check gateway deployments: %w", err)
	}
	if hasDeployments {
		return utils.ErrGatewayHasDeployments
	}

	err = s.gatewayRepo.Delete(gatewayID, ouID)
	if err != nil {
		return err
	}

	// Invalidate all cached tokens for this gateway. The cached policy manifest is not
	// touched: it is one shared copy describing every gateway's policy bundle, so the
	// gateways that remain still need it.
	s.tokenCache.InvalidateGateway(gateway.UUID)
	logger.GetLogger(ctx).Info("gateway deleted and cache invalidated", "gateway_id", gatewayID)

	return nil
}

// VerifyToken verifies a plain-text token and returns the associated gateway
// Optimized O(1) approach using UUID prefix:
// 1. Extract UUID prefix from token (format: {UUID}-{random})
// 2. Single indexed DB lookup by prefix (WHERE token_prefix = ? AND status = 'active')
// 3. Verify token hash with constant-time comparison
// 4. Return gateway or cache result
func (s *PlatformGatewayService) VerifyToken(ctx context.Context, plainToken string) (*models.PlatformGateway, error) {
	start := time.Now()
	defer func() {
		logger.GetLogger(ctx).Debug("token verification completed", "duration_ms", time.Since(start).Milliseconds())
	}()

	if plainToken == "" {
		logger.GetLogger(ctx).Warn("token verification failed: empty token")
		return nil, errors.New("token is required")
	}

	// Step 1: Extract UUID prefix from token (format: UUID-random)
	// Example: "550e8400-e29b-41d4-a716-446655440000-kQpL8vK9..."
	parts := strings.SplitN(plainToken, "-", 6) // UUID has 5 dashes, so split into 6 parts
	if len(parts) < 6 {
		logger.GetLogger(ctx).Warn("token verification failed: invalid token format", "token_prefix", plainToken[:min(16, len(plainToken))])
		return nil, errors.New("invalid token")
	}

	// Reconstruct UUID prefix (first 5 parts joined with dashes)
	tokenPrefix := strings.Join(parts[:5], "-")

	// Validate UUID format
	if _, err := uuid.Parse(tokenPrefix); err != nil {
		logger.GetLogger(ctx).Warn("token verification failed: invalid UUID prefix", "token_prefix", tokenPrefix)
		return nil, errors.New("invalid token")
	}

	// Step 2: Check cache first using prefix as key
	// This is O(1) lookup without any hashing required
	if entry, found := s.tokenCache.Get(tokenPrefix); found {
		// Verify token hash with constant-time comparison (cache stores hash+salt)
		if verifyToken(plainToken, entry.TokenHash, entry.Salt) {
			// Cache hit with valid hash - return cached gateway directly
			logger.GetLogger(ctx).Debug("token verified from cache", "token_prefix", tokenPrefix, "gateway_uuid", entry.GatewayUUID)
			return entry.Gateway, nil
		}
		// Hash mismatch - token was rotated/revoked, invalidate stale cache
		logger.GetLogger(ctx).Warn("cached token hash mismatch, invalidating cache", "token_prefix", tokenPrefix)
		s.tokenCache.Invalidate(tokenPrefix)
	}

	// Step 3: Cache miss - single indexed DB lookup by UUID prefix
	token, err := s.gatewayRepo.GetActiveTokenByPrefix(tokenPrefix)
	if err != nil {
		logger.GetLogger(ctx).Warn("failed to lookup token by prefix", "token_prefix", tokenPrefix, "error", err)
		return nil, fmt.Errorf("failed to verify token: %w", err)
	}

	if token == nil {
		logger.GetLogger(ctx).Warn("token verification failed: no active token with prefix", "token_prefix", tokenPrefix)
		return nil, errors.New("invalid token")
	}

	// Step 4: Verify token hash with constant-time comparison
	if !verifyToken(plainToken, token.TokenHash, token.Salt) {
		logger.GetLogger(ctx).Warn("token verification failed: hash mismatch", "token_prefix", tokenPrefix)
		return nil, errors.New("invalid token")
	}

	// Step 5: Get gateway (only on cache miss)
	gateway, err := s.gatewayRepo.GetByUUID(token.GatewayUUID.String())
	if err != nil {
		logger.GetLogger(ctx).Warn("failed to get gateway for valid token", "gateway_uuid", token.GatewayUUID, "error", err)
		return nil, fmt.Errorf("failed to get gateway: %w", err)
	}

	if gateway == nil {
		logger.GetLogger(ctx).Warn("gateway not found for valid token", "gateway_uuid", token.GatewayUUID)
		return nil, utils.ErrGatewayNotFound
	}

	// Step 6: Cache the valid token using prefix as key (stores full gateway + hash + salt)
	s.tokenCache.Set(tokenPrefix, token.GatewayUUID, gateway, token.TokenHash, token.Salt)
	logger.GetLogger(ctx).Info("token verified successfully and cached", "token_prefix", tokenPrefix, "gateway_uuid", gateway.UUID)

	return gateway, nil
}

// RotateToken generates a new token for a gateway (max 2 active tokens)
func (s *PlatformGatewayService) RotateToken(gatewayID, ouID string) (*TokenRotationResponse, error) {
	// 1. Validate gateway exists
	gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to query gateway: %w", err)
	}
	if gateway == nil {
		return nil, utils.ErrGatewayNotFound
	}
	if gateway.OUID != ouID {
		return nil, utils.ErrGatewayNotFound
	}

	// 2. Count active tokens
	activeCount, err := s.gatewayRepo.CountActiveTokens(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to count active tokens: %w", err)
	}

	// 3. Check max 2 active tokens limit
	if activeCount >= 2 {
		return nil, errors.New("maximum 2 active tokens allowed. Revoke old tokens before rotating")
	}

	// 4. Generate new plain-text token with unique prefix and salt
	plainToken, tokenPrefix, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	saltBytes, err := generateSalt()
	if err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// 5. Hash new token
	tokenHash := hashToken(plainToken, saltBytes)
	saltHex := hex.EncodeToString(saltBytes)

	// 6. Create new GatewayToken model with prefix for fast lookup
	tokenID := uuid.New()
	gatewayToken := &models.GatewayToken{
		UUID:        tokenID,
		GatewayUUID: uuid.MustParse(gatewayID),
		TokenPrefix: tokenPrefix, // UUID prefix for indexed lookup
		TokenHash:   tokenHash,
		Salt:        saltHex,
		Status:      "active",
		CreatedAt:   time.Now(),
		RevokedAt:   nil,
	}

	// 7. Insert token using repository
	if err := s.gatewayRepo.CreateToken(gatewayToken); err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	// 8. Return TokenRotationResponse
	response := &TokenRotationResponse{
		ID:        tokenID.String(),
		Token:     plainToken,
		CreatedAt: gatewayToken.CreatedAt,
		Message:   "New token generated successfully. Old token remains active until revoked.",
	}

	return response, nil
}

// ListTokens retrieves all active tokens for a gateway (metadata only - no secret values)
func (s *PlatformGatewayService) ListTokens(gatewayID, ouID string) (*GatewayTokenListResponse, error) {
	// 1. Validate gateway exists and belongs to organization
	gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to query gateway: %w", err)
	}
	if gateway == nil {
		return nil, utils.ErrGatewayNotFound
	}
	if gateway.OUID != ouID {
		return nil, utils.ErrGatewayNotFound
	}

	// 2. Fetch all active tokens for the gateway
	activeTokens, err := s.gatewayRepo.GetActiveTokensByGatewayUUID(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tokens: %w", err)
	}

	// 3. Map to metadata DTOs (never expose hash/salt/prefix)
	tokens := make([]GatewayTokenInfo, 0, len(activeTokens))
	for _, t := range activeTokens {
		tokens = append(tokens, GatewayTokenInfo{
			ID:        t.UUID.String(),
			Status:    t.Status,
			CreatedAt: t.CreatedAt,
			RevokedAt: t.RevokedAt,
		})
	}

	return &GatewayTokenListResponse{
		Count: len(tokens),
		List:  tokens,
	}, nil
}

// RevokeTokenByID revokes a token and invalidates it from cache
func (s *PlatformGatewayService) RevokeTokenByID(ctx context.Context, tokenID, gatewayID, ouID string) error {
	// 1. Validate gateway exists and belongs to organization
	gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
	if err != nil {
		return fmt.Errorf("failed to query gateway: %w", err)
	}
	if gateway == nil {
		return utils.ErrGatewayNotFound
	}
	if gateway.OUID != ouID {
		return utils.ErrGatewayNotFound
	}

	// 2. Get token details before revocation (for cache invalidation)
	token, err := s.gatewayRepo.GetTokenByUUID(tokenID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("token not found")
		}
		return fmt.Errorf("failed to get token: %w", err)
	}

	// Verify token belongs to the specified gateway
	if token.GatewayUUID.String() != gatewayID {
		return errors.New("token does not belong to this gateway")
	}

	// 3. Revoke the token in database
	if err := s.gatewayRepo.RevokeToken(tokenID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("token not found")
		}
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	// 4. Invalidate from cache using token prefix
	s.tokenCache.Invalidate(token.TokenPrefix)
	logger.GetLogger(ctx).Info("token revoked and cache invalidated", "token_id", tokenID, "token_prefix", token.TokenPrefix, "gateway_id", gatewayID)

	return nil
}

// InvalidateGatewayTokensCache invalidates all cached tokens for a gateway
// Useful when a gateway is deleted or its security context changes
func (s *PlatformGatewayService) InvalidateGatewayTokensCache(gatewayUUID uuid.UUID) {
	s.tokenCache.InvalidateGateway(gatewayUUID)
}

// GetGatewayStatus retrieves gateway status information for polling
func (s *PlatformGatewayService) GetGatewayStatus(ctx context.Context, ouID string, gatewayID *string) (*GatewayStatusListResponse, error) {
	// Validate organizationId is provided and valid
	if strings.TrimSpace(ouID) == "" {
		return nil, errors.New("organization name is required")
	}

	var gateways []*models.Gateway
	var err error

	// If gatewayId is provided, get specific gateway
	if gatewayID != nil && *gatewayID != "" {
		gateway, err := s.gatewayRepo.GetByUUID(*gatewayID)
		if err != nil {
			return nil, fmt.Errorf("failed to get gateway: %w", err)
		}
		if gateway == nil {
			return nil, utils.ErrGatewayNotFound
		}
		// Check organization access
		if gateway.OUID != ouID {
			return nil, utils.ErrGatewayNotFound
		}
		gateways = []*models.Gateway{gateway}
	} else {
		// Get all gateways for organization
		gateways, err = s.gatewayRepo.GetByOrganizationID(ctx, ouID)
		if err != nil {
			return nil, fmt.Errorf("failed to list gateways: %w", err)
		}
	}

	// Convert to lightweight status DTOs
	statusResponses := make([]GatewayStatusResponse, 0, len(gateways))
	for _, gw := range gateways {
		statusResponses = append(statusResponses, GatewayStatusResponse{
			ID:         gw.UUID.String(),
			Name:       gw.Name,
			IsActive:   gw.IsActive,
			IsCritical: gw.IsCritical,
		})
	}

	// Build constitution-compliant list response
	listResponse := &GatewayStatusListResponse{
		Count: len(statusResponses),
		List:  statusResponses,
		Pagination: Pagination{
			Total:  len(statusResponses),
			Offset: 0,
			Limit:  len(statusResponses),
		},
	}

	return listResponse, nil
}

// UpdateGatewayActiveStatus updates the active status of a gateway
func (s *PlatformGatewayService) UpdateGatewayActiveStatus(gatewayID string, isActive bool) error {
	return s.gatewayRepo.UpdateActiveStatus(gatewayID, isActive)
}

// resolveGatewayUUID resolves a gateway identifier (UUID or name) to its UUID,
// scoped to the organization.
func (s *PlatformGatewayService) resolveGatewayUUID(gatewayID, ouID string) (string, error) {
	if gw, err := s.gatewayRepo.GetByNameAndOrgID(gatewayID, ouID); err == nil {
		return gw.UUID.String(), nil
	}
	gw, err := s.gatewayRepo.GetByUUID(gatewayID)
	if err != nil {
		return "", utils.ErrGatewayNotFound
	}
	if gw.OUID != ouID {
		return "", utils.ErrGatewayNotFound
	}
	return gw.UUID.String(), nil
}

// UpsertIdentityProvider creates or updates an identity provider mirror row for a
// gateway. System provenance is derived from the well-known provider names.
//
// When a gateway config applier is configured (cloud deployments), the gateway runtime
// config is patched before the mirror is written, so the two never diverge; if the
// apply fails the mirror is left untouched. System providers are bootstrapped into the
// gateway out of band, so only custom providers are applied here. Open-source builds
// (nil applier) write the mirror only and rely on manage-identity-provider.sh.
func (s *PlatformGatewayService) UpsertIdentityProvider(ctx context.Context, gatewayID, ouID, name, issuer, jwksURI, description string, skipTLS bool) (*models.GatewayIdentityProvider, error) {
	if name == models.ReservedIdentityProviderName {
		return nil, utils.ErrInvalidInput
	}
	gwUUIDStr, err := s.resolveGatewayUUID(gatewayID, ouID)
	if err != nil {
		return nil, err
	}
	gwUUID, err := uuid.Parse(gwUUIDStr)
	if err != nil {
		return nil, utils.ErrGatewayNotFound
	}
	providerType := models.IdentityProviderTypeCustom
	if models.IsSystemIdentityProvider(name) {
		providerType = models.IdentityProviderTypeSystem
	}
	now := time.Now()
	provider := &models.GatewayIdentityProvider{
		UUID:              uuid.New(),
		GatewayUUID:       gwUUID,
		Name:              name,
		Issuer:            issuer,
		JWKSUri:           jwksURI,
		Description:       description,
		Type:              providerType,
		JWKSSkipTLSVerify: skipTLS,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	// Patch the gateway runtime config first so a failed apply never leaves a mirror
	// row the gateway does not honor.
	applied := false
	if s.gatewayApplier != nil && providerType == models.IdentityProviderTypeCustom {
		if err := s.gatewayApplier.ApplyIdentityProvider(ctx, gatewayID, ouID, *provider); err != nil {
			return nil, err
		}
		applied = true
	}
	if err := s.gatewayRepo.UpsertIdentityProvider(provider); err != nil {
		if applied {
			// Gateway was patched but the mirror write failed: the two are diverged
			// until the client retries (both operations are idempotent).
			logger.GetLogger(ctx).Warn("gateway identity provider applied but mirror upsert failed; retry to reconcile",
				"gateway_id", gatewayID, "provider", name, "error", err)
		}
		return nil, err
	}
	return provider, nil
}

// DeleteIdentityProvider removes an identity provider mirror row for a gateway.
// System providers cannot be deleted (enforced in the repository).
//
// When a gateway config applier is configured (cloud deployments), the provider is
// removed from the gateway runtime config before the mirror row, keeping the two in
// sync; if the apply fails the mirror is left untouched. System providers are skipped
// (the repository rejects their deletion), so only custom providers are applied here.
func (s *PlatformGatewayService) DeleteIdentityProvider(ctx context.Context, gatewayID, ouID, name string) error {
	gwUUIDStr, err := s.resolveGatewayUUID(gatewayID, ouID)
	if err != nil {
		return err
	}
	removed := false
	if s.gatewayApplier != nil && !models.IsSystemIdentityProvider(name) {
		if err := s.gatewayApplier.DeleteIdentityProvider(ctx, gatewayID, ouID, name); err != nil {
			return err
		}
		removed = true
	}
	if err := s.gatewayRepo.DeleteIdentityProvider(gwUUIDStr, name); err != nil {
		if removed {
			// Gateway entry was removed but the mirror delete failed: the two are
			// diverged until the client retries (both operations are idempotent).
			logger.GetLogger(ctx).Warn("gateway identity provider removed but mirror delete failed; retry to reconcile",
				"gateway_id", gatewayID, "provider", name, "error", err)
		}
		return err
	}
	return nil
}

// ListIdentityProvidersByGateway lists the identity providers mirrored for a gateway.
func (s *PlatformGatewayService) ListIdentityProvidersByGateway(gatewayID, ouID string) ([]models.GatewayIdentityProvider, error) {
	gwUUIDStr, err := s.resolveGatewayUUID(gatewayID, ouID)
	if err != nil {
		return nil, err
	}
	return s.gatewayRepo.ListIdentityProvidersByGateway(gwUUIDStr)
}

// ListIdentityProvidersByEnvironment lists the identity providers available in an
// environment (union over the environment's gateways).
func (s *PlatformGatewayService) ListIdentityProvidersByEnvironment(environmentID string) ([]models.GatewayIdentityProvider, error) {
	return s.gatewayRepo.ListIdentityProvidersByEnvironment(environmentID)
}

// ListIdentityProvidersByOrg lists every identity provider across the org's
// gateways, enriched with gateway + environment context.
func (s *PlatformGatewayService) ListIdentityProvidersByOrg(ouID string) ([]repositories.IdentityProviderWithContext, error) {
	return s.gatewayRepo.ListIdentityProvidersByOrg(ouID)
}

// AssignGatewayToEnvironment maps a gateway to an environment, enforcing the
// one-ingress-capable-gateway-per-environment cap. This is the single membership choke
// point — RegisterGateway drives the same transactional helper.
func (s *PlatformGatewayService) AssignGatewayToEnvironment(gatewayID, environmentID string) error {
	if _, err := uuid.Parse(gatewayID); err != nil {
		return fmt.Errorf("%w: invalid gateway UUID %q", utils.ErrBadRequest, gatewayID)
	}
	gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
	if err != nil {
		return err
	}
	if gateway == nil {
		return utils.ErrGatewayNotFound
	}
	return s.gatewayRepo.Transaction(func(tx *gorm.DB) error {
		return s.assignGatewayToEnvironmentTx(tx, gateway, environmentID)
	})
}

// assignGatewayToEnvironmentTx performs the capped, locked mapping insert inside an
// existing transaction.
//
// The advisory lock is taken before the existence check so the check-then-insert window
// is serialised per environment. The existence check sits inside the lock so an
// idempotent re-assign of an already-mapped gateway returns before the count runs and
// can never count itself.
//
// Egress-capable gateways are never counted or rejected: egress is uncapped.
func (s *PlatformGatewayService) assignGatewayToEnvironmentTx(
	tx *gorm.DB, gateway *models.Gateway, environmentID string,
) error {
	envUUID, err := uuid.Parse(environmentID)
	if err != nil {
		return fmt.Errorf("%w: invalid environment UUID %q", utils.ErrBadRequest, environmentID)
	}
	envIDStr := envUUID.String()

	if err := s.gatewayRepo.AcquireEnvironmentLock(tx, envIDStr); err != nil {
		return fmt.Errorf("failed to lock environment %s: %w", envIDStr, err)
	}

	// Reads on the pooled connection (not tx) are safe here: the advisory lock is
	// held until commit, so a competing writer has already committed by the time
	// this lock grant returns, and READ COMMITTED guarantees that commit is visible.
	exists, err := s.gatewayRepo.EnvironmentMappingExists(gateway.UUID.String(), envIDStr)
	if err != nil {
		return fmt.Errorf("failed to check existing mapping: %w", err)
	}
	if exists {
		return nil // already assigned
	}

	if gateway.IsIngressCapable() {
		count, err := s.gatewayRepo.CountIngressCapableInEnvironment(tx, envIDStr)
		if err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("%w: environment %s already has an ingress gateway; "+
				"register this gateway with gatewayType EGRESS instead", utils.ErrGatewayIngressCapExceeded, envIDStr)
		}
	}

	mapping := &models.GatewayEnvironmentMapping{
		GatewayUUID:     gateway.UUID,
		EnvironmentUUID: envUUID,
	}
	if err := s.gatewayRepo.CreateEnvironmentMappingTx(tx, mapping); err != nil {
		return fmt.Errorf("failed to create gateway-environment mapping: %w", err)
	}
	return nil
}

// RemoveGatewayFromEnvironment deletes a mapping between a gateway and an environment
func (s *PlatformGatewayService) RemoveGatewayFromEnvironment(gatewayID, environmentID string) error {
	// Validate UUIDs
	if _, err := uuid.Parse(gatewayID); err != nil {
		return fmt.Errorf("invalid gateway UUID: %w", err)
	}

	if _, err := uuid.Parse(environmentID); err != nil {
		return fmt.Errorf("invalid environment UUID: %w", err)
	}

	// Delete mapping
	if err := s.gatewayRepo.DeleteEnvironmentMapping(gatewayID, environmentID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("gateway-environment mapping not found")
		}
		return fmt.Errorf("failed to remove gateway from environment: %w", err)
	}

	return nil
}

// GetGatewayEnvironmentMappings retrieves all environment mappings for a gateway
func (s *PlatformGatewayService) GetGatewayEnvironmentMappings(gatewayID string) ([]models.GatewayEnvironmentMapping, error) {
	// Validate UUID
	if _, err := uuid.Parse(gatewayID); err != nil {
		return nil, fmt.Errorf("invalid gateway UUID: %w", err)
	}

	mappings, err := s.gatewayRepo.GetEnvironmentMappingsByGatewayID(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway environment mappings: %w", err)
	}

	return mappings, nil
}

// GetGatewayEnvironmentMappingsBulk retrieves environment mappings for multiple gateways in bulk
// This avoids N+1 queries when fetching environments for a list of gateways
func (s *PlatformGatewayService) GetGatewayEnvironmentMappingsBulk(gatewayIDs []string) (map[string][]models.GatewayEnvironmentMapping, error) {
	// Validate UUIDs
	for _, id := range gatewayIDs {
		if _, err := uuid.Parse(id); err != nil {
			return nil, fmt.Errorf("invalid gateway UUID: %s: %w", id, err)
		}
	}

	mappings, err := s.gatewayRepo.GetEnvironmentMappingsByGatewayIDs(gatewayIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway environment mappings in bulk: %w", err)
	}

	return mappings, nil
}

// DeleteGatewayEnvironmentMappings deletes all environment mappings for a gateway
func (s *PlatformGatewayService) DeleteGatewayEnvironmentMappings(ctx context.Context, gatewayID string) error {
	// Validate UUID
	gwUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return fmt.Errorf("invalid gateway UUID: %w", err)
	}

	// Get all mappings
	mappings, err := s.gatewayRepo.GetEnvironmentMappingsByGatewayID(gatewayID)
	if err != nil {
		return fmt.Errorf("failed to get gateway environment mappings: %w", err)
	}

	// Delete each mapping
	for _, mapping := range mappings {
		if err := s.gatewayRepo.DeleteEnvironmentMapping(gwUUID.String(), mapping.EnvironmentUUID.String()); err != nil {
			logger.GetLogger(ctx).Warn("failed to delete gateway-environment mapping", "gateway_id", gatewayID, "environment_id", mapping.EnvironmentUUID, "error", err)
			// Continue with other mappings
		}
	}

	return nil
}

// validateGatewayInput validates gateway registration inputs
func (s *PlatformGatewayService) validateGatewayInput(ouID, name, displayName, vhost string) error {
	// Organization ID validation
	if strings.TrimSpace(ouID) == "" {
		return errors.New("organization name is required")
	}

	// Gateway name validation
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("gateway name is required")
	}
	if len(name) < 3 {
		return errors.New("gateway name must be at least 3 characters")
	}
	if len(name) > 64 {
		return errors.New("gateway name must not exceed 64 characters")
	}

	// Check pattern: ^[a-z0-9-]+$
	namePattern := regexp.MustCompile(`^[a-z0-9-]+$`)
	if !namePattern.MatchString(name) {
		return errors.New("gateway name must contain only lowercase letters, numbers, and hyphens")
	}

	// No leading/trailing hyphens
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return errors.New("gateway name cannot start or end with a hyphen")
	}

	// Display name validation
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return errors.New("display name is required")
	}
	if len(displayName) > 128 {
		return errors.New("display name must not exceed 128 characters")
	}

	// VHost validation
	vhost = strings.TrimSpace(vhost)
	if vhost == "" {
		return errors.New("vhost is required")
	}

	return nil
}

var dnsLabelPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// validateGatewayRuntimeURL enforces the shape of the stored in-cluster gateway address.
// Empty is legal and means "no internal address; use vhost".
//
// Validated rather than treated as opaque because the value is materialized into sandboxed
// agent pod env vars alongside the minted gateway API key. The port rule is the control that
// matters: the sandbox NetworkPolicy permits arbitrary destinations on exactly three ports —
// 80 and 443 via its public ipBlock rule, and 53 TCP+UDP via its DNS rule, which carries no
// `to:` selector and so reaches every address — so all three are refused. Host shape is
// defence in depth only, since a two-label host is indistinguishable from a public domain.
func validateGatewayRuntimeURL(runtimeURL string) error {
	runtimeURL = strings.TrimSpace(runtimeURL)
	if runtimeURL == "" {
		return nil
	}
	parsed, err := url.Parse(runtimeURL)
	if err != nil {
		return fmt.Errorf("%w: runtimeUrl is not a valid URL: %w", utils.ErrBadRequest, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: runtimeUrl scheme must be http or https", utils.ErrBadRequest)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%w: runtimeUrl must not carry userinfo, a query or a fragment", utils.ErrBadRequest)
	}
	// Any path, "/" included, would concatenate into a double slash Envoy does not merge.
	if parsed.Path != "" {
		return fmt.Errorf("%w: runtimeUrl must be a base URL without a path", utils.ErrBadRequest)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("%w: runtimeUrl must specify an explicit numeric port", utils.ErrBadRequest)
	}
	if port == 80 || port == 443 || port == 53 {
		return fmt.Errorf("%w: runtimeUrl must not use port 80, 443 or 53; the agent sandbox "+
			"egress policy permits arbitrary destinations on those ports", utils.ErrBadRequest)
	}
	if !isClusterLocalHost(parsed.Hostname()) {
		return fmt.Errorf("%w: runtimeUrl host must be cluster-local: a dotless Service name, "+
			"name.namespace[.svc[.cluster.local]], or a private-range IP", utils.ErrBadRequest)
	}
	return nil
}

// isClusterLocalHost matches in-cluster address shapes. Shape only — a two-label host is
// indistinguishable from a public domain, which is why the port rule above carries the weight.
func isClusterLocalHost(host string) bool {
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		// Link-local is deliberately absent: it would admit the 169.254.169.254 metadata address.
		return ip.IsLoopback() || ip.IsPrivate()
	}
	labels := strings.Split(strings.TrimSuffix(host, "."), ".")
	switch len(labels) {
	case 1, 2: // "runtime" or "runtime.namespace"
	case 3:
		if labels[2] != "svc" {
			return false
		}
	case 5:
		if labels[2] != "svc" || labels[3] != "cluster" || labels[4] != "local" {
			return false
		}
	default:
		return false
	}
	for _, label := range labels {
		if !dnsLabelPattern.MatchString(label) {
			return false
		}
	}
	return true
}

// normalizeGatewayRole maps wire input to the canonical stored role.
//
// REGULAR and AI are accepted as deprecated input-only aliases: the gateway bootstrap
// job is the only thing that writes a role and it ships in a separately-versioned OCI
// chart, so old-chart-against-new-AMS is a routine combination. Responses always emit
// canonical values. REGULAR maps to "both" (not "ingress") because 100% of registrations
// in existence send REGULAR, and an ingress-only environment has no legal LLM/MCP target.
//
// "event" is no longer accepted. It never worked: the CHECK constraint from migration 002
// was IN ('regular','ai'), so an event gateway passed validation and then failed at INSERT.
func normalizeGatewayRole(functionalityType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(functionalityType)) {
	case "":
		return "", fmt.Errorf("%w: gateway functionality type is required", utils.ErrBadRequest)
	case models.GatewayRoleIngress:
		return models.GatewayRoleIngress, nil
	case models.GatewayRoleEgress, "ai":
		return models.GatewayRoleEgress, nil
	case models.GatewayRoleBoth, "regular":
		return models.GatewayRoleBoth, nil
	default:
		return "", fmt.Errorf("%w: gateway type must be one of: INGRESS, EGRESS, BOTH", utils.ErrBadRequest)
	}
}

// Token Generation and Hashing Utilities

// generateToken generates a cryptographically secure token with guaranteed uniqueness
// Format: {UUID}-{32-random-bytes-base64}
// The UUID prefix ensures uniqueness and enables fast indexed lookups
// The random suffix provides 256 bits of entropy for security
func generateToken() (token string, prefix string, err error) {
	// Generate UUID for uniqueness
	tokenUUID := uuid.New()
	prefix = tokenUUID.String()

	// Generate 32 random bytes for entropy
	randomBytes := make([]byte, 32)
	_, err = rand.Read(randomBytes)
	if err != nil {
		return "", "", errors.New("failed to generate secure random bytes")
	}

	// Encode random bytes as base64 (URL-safe, no padding)
	randomSuffix := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(randomBytes)

	// Combine: UUID-randomBytes
	token = fmt.Sprintf("%s-%s", prefix, randomSuffix)

	return token, prefix, nil
}

// generateSalt generates a cryptographically secure 32-byte random salt
func generateSalt() ([]byte, error) {
	salt := make([]byte, 32)
	_, err := rand.Read(salt)
	if err != nil {
		return nil, errors.New("failed to generate secure random salt")
	}
	return salt, nil
}

// hashToken computes SHA-256 hash of (token + salt) and returns hex-encoded string
func hashToken(plainToken string, salt []byte) string {
	h := sha256.New()
	h.Write([]byte(plainToken))
	h.Write(salt)
	tokenHash := h.Sum(nil)
	return hex.EncodeToString(tokenHash)
}

// verifyToken performs constant-time comparison of plain token against stored hash+salt
func verifyToken(plainToken string, storedHashHex string, storedSaltHex string) bool {
	storedSalt, err := hex.DecodeString(storedSaltHex)
	if err != nil {
		return false
	}
	storedHash, err := hex.DecodeString(storedHashHex)
	if err != nil {
		return false
	}
	h := sha256.New()
	h.Write([]byte(plainToken))
	h.Write(storedSalt)
	computedHash := h.Sum(nil)
	return subtle.ConstantTimeCompare(computedHash, storedHash) == 1
}

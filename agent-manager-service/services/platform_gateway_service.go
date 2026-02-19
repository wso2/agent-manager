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
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// PlatformGatewayService handles gateway business logic for API Platform integration
type PlatformGatewayService struct {
	gatewayRepo repositories.GatewayRepository
	tokenCache  *TokenCache
}

// NewPlatformGatewayService creates a new platform gateway service
func NewPlatformGatewayService(
	gatewayRepo repositories.GatewayRepository,
) *PlatformGatewayService {
	// Initialize token cache with 5 minute TTL
	tokenCache := NewTokenCache(5 * time.Minute)

	return &PlatformGatewayService{
		gatewayRepo: gatewayRepo,
		tokenCache:  tokenCache,
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
	orgName, name, displayName, description, vhost string,
	isCritical bool, functionalityType string,
	properties map[string]interface{},
) (*GatewayResponse, error) {
	// 1. Validate inputs
	if err := s.validateGatewayInput(orgName, name, displayName, vhost, functionalityType); err != nil {
		return nil, err
	}

	// 3. Check gateway name uniqueness within organization
	existing, err := s.gatewayRepo.GetByNameAndOrgID(name, orgName)
	if err != nil && !errors.Is(err, utils.ErrGatewayNotFound) {
		return nil, fmt.Errorf("failed to check gateway name uniqueness: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("gateway with name '%s' already exists in this organization", name)
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
		OrganizationName:         orgName,
		Name:                     name,
		DisplayName:              displayName,
		Description:              description,
		Properties:               properties,
		Vhost:                    vhost,
		IsCritical:               isCritical,
		GatewayFunctionalityType: strings.ToLower(functionalityType),
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}

	err = s.gatewayRepo.Create(gateway)
	if err != nil {
		return nil, fmt.Errorf("error while registering gateway: %w", err)
	}

	response := &GatewayResponse{
		ID:                gateway.UUID.String(),
		OrganizationID:    gateway.OrganizationName,
		Name:              gateway.Name,
		DisplayName:       gateway.DisplayName,
		Description:       gateway.Description,
		Properties:        gateway.Properties,
		Vhost:             gateway.Vhost,
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
	FunctionalityType *string // Filter by gateway type (ai, regular, event)
	Status            *bool   // Filter by is_active status
	EnvironmentID     *string // Filter by environment UUID
}

// ListGateways retrieves gateways with constitution-compliant envelope structure and DB-level pagination
func (s *PlatformGatewayService) ListGateways(orgName *string, filters *GatewayListFilters, limit, offset int) (*GatewayListResponse, error) {
	// Build filter options
	filterOpts := repositories.GatewayFilterOptions{
		Limit:  limit,
		Offset: offset,
	}

	if orgName != nil && *orgName != "" {
		filterOpts.OrganizationID = *orgName
	}

	if filters != nil {
		filterOpts.FunctionalityType = filters.FunctionalityType
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
			OrganizationID:    gw.OrganizationName,
			Name:              gw.Name,
			DisplayName:       gw.DisplayName,
			Description:       gw.Description,
			Properties:        gw.Properties,
			Vhost:             gw.Vhost,
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

// GetGateway retrieves a gateway by ID
func (s *PlatformGatewayService) GetGateway(gatewayID, orgName string) (*GatewayResponse, error) {
	// Validate UUID format
	if _, err := uuid.Parse(gatewayID); err != nil {
		return nil, errors.New("invalid UUID format")
	}

	gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway: %w", err)
	}

	if gateway == nil {
		return nil, utils.ErrGatewayNotFound
	}

	if gateway.OrganizationName != orgName {
		return nil, utils.ErrGatewayNotFound
	}

	response := &GatewayResponse{
		ID:                gateway.UUID.String(),
		OrganizationID:    gateway.OrganizationName,
		Name:              gateway.Name,
		DisplayName:       gateway.DisplayName,
		Description:       gateway.Description,
		Properties:        gateway.Properties,
		Vhost:             gateway.Vhost,
		IsCritical:        gateway.IsCritical,
		FunctionalityType: gateway.GatewayFunctionalityType,
		IsActive:          gateway.IsActive,
		CreatedAt:         gateway.CreatedAt,
		UpdatedAt:         gateway.UpdatedAt,
	}

	return response, nil
}

// UpdateGateway updates gateway details
func (s *PlatformGatewayService) UpdateGateway(
	gatewayID, orgName string,
	description, displayName *string,
	isCritical *bool,
	properties *map[string]interface{},
) (*GatewayResponse, error) {
	// Get existing gateway
	gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
	if err != nil {
		return nil, err
	}
	if gateway == nil {
		return nil, utils.ErrGatewayNotFound
	}
	if gateway.OrganizationName != orgName {
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
	gateway.UpdatedAt = time.Now()

	err = s.gatewayRepo.UpdateGateway(gateway)
	if err != nil {
		return nil, err
	}

	updatedGateway := &GatewayResponse{
		ID:                gateway.UUID.String(),
		OrganizationID:    gateway.OrganizationName,
		Name:              gateway.Name,
		DisplayName:       gateway.DisplayName,
		Description:       gateway.Description,
		Properties:        gateway.Properties,
		Vhost:             gateway.Vhost,
		IsCritical:        gateway.IsCritical,
		FunctionalityType: gateway.GatewayFunctionalityType,
		IsActive:          gateway.IsActive,
		CreatedAt:         gateway.CreatedAt,
		UpdatedAt:         gateway.UpdatedAt,
	}
	return updatedGateway, nil
}

// DeleteGateway deletes a gateway and all associated tokens (CASCADE)
func (s *PlatformGatewayService) DeleteGateway(gatewayID, orgName string) error {
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
	if gateway.OrganizationName != orgName {
		return utils.ErrGatewayNotFound
	}

	// Delete gateway (FK CASCADE will automatically remove tokens and deployments)
	err = s.gatewayRepo.Delete(gatewayID, orgName)
	if err != nil {
		return err
	}

	// Invalidate all cached tokens for this gateway
	s.tokenCache.InvalidateGateway(gateway.UUID)
	slog.Info("gateway deleted and cache invalidated", "gatewayID", gatewayID)

	return nil
}

// VerifyToken verifies a plain-text token and returns the associated gateway
// Optimized O(1) approach using UUID prefix:
// 1. Extract UUID prefix from token (format: {UUID}-{random})
// 2. Single indexed DB lookup by prefix (WHERE token_prefix = ? AND status = 'active')
// 3. Verify token hash with constant-time comparison
// 4. Return gateway or cache result
func (s *PlatformGatewayService) VerifyToken(plainToken string) (*models.PlatformGateway, error) {
	start := time.Now()
	defer func() {
		slog.Debug("token verification completed", "duration_ms", time.Since(start).Milliseconds())
	}()

	if plainToken == "" {
		slog.Warn("token verification failed: empty token")
		return nil, errors.New("token is required")
	}

	// Step 1: Extract UUID prefix from token (format: UUID-random)
	// Example: "550e8400-e29b-41d4-a716-446655440000-kQpL8vK9..."
	parts := strings.SplitN(plainToken, "-", 6) // UUID has 5 dashes, so split into 6 parts
	if len(parts) < 6 {
		slog.Warn("token verification failed: invalid token format", "tokenPrefix", plainToken[:min(16, len(plainToken))])
		return nil, errors.New("invalid token")
	}

	// Reconstruct UUID prefix (first 5 parts joined with dashes)
	tokenPrefix := strings.Join(parts[:5], "-")

	// Validate UUID format
	if _, err := uuid.Parse(tokenPrefix); err != nil {
		slog.Warn("token verification failed: invalid UUID prefix", "tokenPrefix", tokenPrefix)
		return nil, errors.New("invalid token")
	}

	// Step 2: Check cache first using prefix as key
	// This is O(1) lookup without any hashing required
	if entry, found := s.tokenCache.Get(tokenPrefix); found {
		// Verify token hash with constant-time comparison (cache stores hash+salt)
		if verifyToken(plainToken, entry.TokenHash, entry.Salt) {
			// Cache hit with valid hash - return cached gateway directly
			slog.Debug("token verified from cache", "tokenPrefix", tokenPrefix, "gatewayUUID", entry.GatewayUUID)
			return entry.Gateway, nil
		}
		// Hash mismatch - token was rotated/revoked, invalidate stale cache
		slog.Warn("cached token hash mismatch, invalidating cache", "tokenPrefix", tokenPrefix)
		s.tokenCache.Invalidate(tokenPrefix)
	}

	// Step 3: Cache miss - single indexed DB lookup by UUID prefix
	token, err := s.gatewayRepo.GetActiveTokenByPrefix(tokenPrefix)
	if err != nil {
		slog.Error("failed to lookup token by prefix", "tokenPrefix", tokenPrefix, "error", err)
		return nil, fmt.Errorf("failed to verify token: %w", err)
	}

	if token == nil {
		slog.Warn("token verification failed: no active token with prefix", "tokenPrefix", tokenPrefix)
		return nil, errors.New("invalid token")
	}

	// Step 4: Verify token hash with constant-time comparison
	if !verifyToken(plainToken, token.TokenHash, token.Salt) {
		slog.Warn("token verification failed: hash mismatch", "tokenPrefix", tokenPrefix)
		return nil, errors.New("invalid token")
	}

	// Step 5: Get gateway (only on cache miss)
	gateway, err := s.gatewayRepo.GetByUUID(token.GatewayUUID.String())
	if err != nil {
		slog.Error("failed to get gateway for valid token", "gatewayUUID", token.GatewayUUID, "error", err)
		return nil, fmt.Errorf("failed to get gateway: %w", err)
	}

	if gateway == nil {
		slog.Warn("gateway not found for valid token", "gatewayUUID", token.GatewayUUID)
		return nil, utils.ErrGatewayNotFound
	}

	// Step 6: Cache the valid token using prefix as key (stores full gateway + hash + salt)
	s.tokenCache.Set(tokenPrefix, token.GatewayUUID, gateway, token.TokenHash, token.Salt)
	slog.Info("token verified successfully and cached", "tokenPrefix", tokenPrefix, "gatewayUUID", gateway.UUID)

	return gateway, nil
}

// RotateToken generates a new token for a gateway (max 2 active tokens)
func (s *PlatformGatewayService) RotateToken(gatewayID, orgName string) (*TokenRotationResponse, error) {
	// 1. Validate gateway exists
	gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to query gateway: %w", err)
	}
	if gateway == nil {
		return nil, utils.ErrGatewayNotFound
	}
	if gateway.OrganizationName != orgName {
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

// RevokeTokenByID revokes a token and invalidates it from cache
func (s *PlatformGatewayService) RevokeTokenByID(tokenID, gatewayID, orgName string) error {
	// 1. Validate gateway exists and belongs to organization
	gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
	if err != nil {
		return fmt.Errorf("failed to query gateway: %w", err)
	}
	if gateway == nil {
		return utils.ErrGatewayNotFound
	}
	if gateway.OrganizationName != orgName {
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
	slog.Info("token revoked and cache invalidated", "tokenID", tokenID, "tokenPrefix", token.TokenPrefix, "gatewayID", gatewayID)

	return nil
}

// InvalidateGatewayTokensCache invalidates all cached tokens for a gateway
// Useful when a gateway is deleted or its security context changes
func (s *PlatformGatewayService) InvalidateGatewayTokensCache(gatewayUUID uuid.UUID) {
	s.tokenCache.InvalidateGateway(gatewayUUID)
}

// GetGatewayStatus retrieves gateway status information for polling
func (s *PlatformGatewayService) GetGatewayStatus(orgName string, gatewayID *string) (*GatewayStatusListResponse, error) {
	// Validate organizationId is provided and valid
	if strings.TrimSpace(orgName) == "" {
		return nil, errors.New("organization ID is required")
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
		if gateway.OrganizationName != orgName {
			return nil, utils.ErrGatewayNotFound
		}
		gateways = []*models.Gateway{gateway}
	} else {
		// Get all gateways for organization
		gateways, err = s.gatewayRepo.GetByOrganizationID(orgName)
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

// AssignGatewayToEnvironment creates a mapping between a gateway and an environment
func (s *PlatformGatewayService) AssignGatewayToEnvironment(gatewayID, environmentID string) error {
	// Parse UUIDs
	gwUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return fmt.Errorf("invalid gateway UUID: %w", err)
	}

	envUUID, err := uuid.Parse(environmentID)
	if err != nil {
		return fmt.Errorf("invalid environment UUID: %w", err)
	}

	// Check if mapping already exists
	exists, err := s.gatewayRepo.EnvironmentMappingExists(gatewayID, environmentID)
	if err != nil {
		return fmt.Errorf("failed to check existing mapping: %w", err)
	}

	if exists {
		// Already assigned, treat as success
		return nil
	}

	// Create mapping
	mapping := &models.GatewayEnvironmentMapping{
		GatewayUUID:     gwUUID,
		EnvironmentUUID: envUUID,
	}

	if err := s.gatewayRepo.CreateEnvironmentMapping(mapping); err != nil {
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
func (s *PlatformGatewayService) DeleteGatewayEnvironmentMappings(gatewayID string) error {
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
			slog.Warn("failed to delete gateway-environment mapping", "gatewayID", gatewayID, "environmentID", mapping.EnvironmentUUID, "error", err)
			// Continue with other mappings
		}
	}

	return nil
}

// validateGatewayInput validates gateway registration inputs
func (s *PlatformGatewayService) validateGatewayInput(orgName, name, displayName, vhost, functionalityType string) error {
	// Organization ID validation
	if strings.TrimSpace(orgName) == "" {
		return errors.New("organization ID is required")
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

	// Gateway type validation
	functionalityType = strings.TrimSpace(functionalityType)
	if functionalityType == "" {
		return errors.New("gateway functionality type is required")
	}
	// Normalize to lowercase for consistent validation and storage
	normalized := strings.ToLower(functionalityType)
	validTypes := map[string]bool{
		"regular": true,
		"ai":      true,
		"event":   true,
	}
	if !validTypes[normalized] {
		return fmt.Errorf("gateway type must be one of: Regular, AI, Event")
	}

	return nil
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

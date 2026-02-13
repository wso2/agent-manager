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
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// PlatformGatewayService handles gateway business logic for API Platform integration
type PlatformGatewayService struct {
	gatewayRepo repositories.GatewayRepository
	orgRepo     repositories.OrganizationRepository
	apiRepo     repositories.APIRepository
}

// NewPlatformGatewayService creates a new platform gateway service
func NewPlatformGatewayService(
	gatewayRepo repositories.GatewayRepository,
	orgRepo repositories.OrganizationRepository,
	apiRepo repositories.APIRepository,
) *PlatformGatewayService {
	return &PlatformGatewayService{
		gatewayRepo: gatewayRepo,
		orgRepo:     orgRepo,
		apiRepo:     apiRepo,
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
	orgID, name, displayName, description, vhost string,
	isCritical bool, functionalityType string,
	properties map[string]interface{},
) (*GatewayResponse, error) {
	// 1. Validate inputs
	if err := s.validateGatewayInput(orgID, name, displayName, vhost, functionalityType); err != nil {
		return nil, err
	}

	// 3. Check gateway name uniqueness within organization
	existing, err := s.gatewayRepo.GetByNameAndOrgID(name, orgID)
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
	orgUUID, err := uuid.Parse(orgID)
	if err != nil {
		return nil, fmt.Errorf("invalid organization UUID: %w", err)
	}

	// Initialize properties as empty map if nil (database column is NOT NULL)
	if properties == nil {
		properties = make(map[string]interface{})
	}

	gateway := &models.Gateway{
		UUID:                     gatewayUUID,
		OrganizationUUID:         orgUUID,
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

	// 6. Generate plain-text token and salt
	plainToken, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	saltBytes, err := generateSalt()
	if err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// 7. Hash token with salt
	tokenHash := hashToken(plainToken, saltBytes)
	saltHex := hex.EncodeToString(saltBytes)

	// 8. Create GatewayToken model
	tokenID := uuid.New()
	gatewayToken := &models.GatewayToken{
		UUID:        tokenID,
		GatewayUUID: uuid.MustParse(gatewayID),
		TokenHash:   tokenHash,
		Salt:        saltHex,
		Status:      "active",
		CreatedAt:   time.Now(),
		RevokedAt:   nil,
	}

	// 9. Insert gateway and token
	if err := s.gatewayRepo.Create(gateway); err != nil {
		return nil, fmt.Errorf("failed to create gateway: %w", err)
	}

	if err := s.gatewayRepo.CreateToken(gatewayToken); err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	// 10. Return GatewayResponse
	response := &GatewayResponse{
		ID:                gateway.UUID.String(),
		OrganizationID:    gateway.OrganizationUUID.String(),
		Token:             plainToken,
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

// ListGateways retrieves all gateways with constitution-compliant envelope structure
func (s *PlatformGatewayService) ListGateways(orgID *string) (*GatewayListResponse, error) {
	var gateways []*models.Gateway
	var err error

	// If orgID provided and non-empty, filter by organization
	if orgID != nil && *orgID != "" {
		gateways, err = s.gatewayRepo.GetByOrganizationID(*orgID)
	} else {
		gateways, err = s.gatewayRepo.List()
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list gateways: %w", err)
	}

	// Convert to DTOs
	responses := make([]GatewayResponse, 0, len(gateways))
	for _, gw := range gateways {
		responses = append(responses, GatewayResponse{
			ID:                gw.UUID.String(),
			OrganizationID:    gw.OrganizationUUID.String(),
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
			Total:  len(responses),
			Offset: 0,
			Limit:  len(responses),
		},
	}

	return listResponse, nil
}

// GetGateway retrieves a gateway by ID
func (s *PlatformGatewayService) GetGateway(gatewayID, orgID string) (*GatewayResponse, error) {
	// Validate UUID format
	if _, err := uuid.Parse(gatewayID); err != nil {
		return nil, errors.New("invalid UUID format")
	}

	gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway: %w", err)
	}

	if gateway == nil {
		return nil, errors.New("gateway not found")
	}

	if gateway.OrganizationUUID.String() != orgID {
		return nil, errors.New("gateway not found")
	}

	response := &GatewayResponse{
		ID:                gateway.UUID.String(),
		OrganizationID:    gateway.OrganizationUUID.String(),
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
	gatewayID, orgID string,
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
		return nil, errors.New("gateway not found")
	}
	if gateway.OrganizationUUID.String() != orgID {
		return nil, errors.New("gateway not found")
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
		OrganizationID:    gateway.OrganizationUUID.String(),
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
func (s *PlatformGatewayService) DeleteGateway(gatewayID, orgID string) error {
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
		return errors.New("gateway not found")
	}
	if gateway.OrganizationUUID.String() != orgID {
		return errors.New("gateway not found")
	}

	// Delete gateway (FK CASCADE will automatically remove tokens and deployments)
	err = s.gatewayRepo.Delete(gatewayID, orgID)
	if err != nil {
		return err
	}

	return nil
}

// VerifyToken verifies a plain-text token and returns the associated gateway
func (s *PlatformGatewayService) VerifyToken(plainToken string) (*models.PlatformGateway, error) {
	if plainToken == "" {
		return nil, errors.New("token is required")
	}

	// Get all gateways to check their active tokens
	gateways, err := s.gatewayRepo.List()
	if err != nil {
		return nil, fmt.Errorf("failed to query gateways: %w", err)
	}

	// For each gateway, check if the token matches any active token
	for _, gateway := range gateways {
		activeTokens, err := s.gatewayRepo.GetActiveTokensByGatewayUUID(gateway.UUID.String())
		if err != nil {
			continue // Skip this gateway on error
		}

		for _, token := range activeTokens {
			if verifyToken(plainToken, token.TokenHash, token.Salt) {
				// Token matches - return gateway
				return gateway, nil
			}
		}
	}

	return nil, errors.New("invalid token")
}

// RotateToken generates a new token for a gateway (max 2 active tokens)
func (s *PlatformGatewayService) RotateToken(gatewayID, orgID string) (*TokenRotationResponse, error) {
	// 1. Validate gateway exists
	gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to query gateway: %w", err)
	}
	if gateway == nil {
		return nil, errors.New("gateway not found")
	}
	if gateway.OrganizationUUID.String() != orgID {
		return nil, errors.New("gateway not found")
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

	// 4. Generate new plain-text token and salt
	plainToken, err := generateToken()
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

	// 6. Create new GatewayToken model with status='active'
	tokenID := uuid.New()
	gatewayToken := &models.GatewayToken{
		UUID:        tokenID,
		GatewayUUID: uuid.MustParse(gatewayID),
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

// GetGatewayStatus retrieves gateway status information for polling
func (s *PlatformGatewayService) GetGatewayStatus(orgID string, gatewayID *string) (*GatewayStatusListResponse, error) {
	// Validate organizationId is provided and valid
	if strings.TrimSpace(orgID) == "" {
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
			return nil, errors.New("gateway not found")
		}
		// Check organization access
		if gateway.OrganizationUUID.String() != orgID {
			return nil, errors.New("gateway not found")
		}
		gateways = []*models.Gateway{gateway}
	} else {
		// Get all gateways for organization
		gateways, err = s.gatewayRepo.GetByOrganizationID(orgID)
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

// GetGatewayArtifacts retrieves all artifacts (APIs) deployed to a specific gateway
func (s *PlatformGatewayService) GetGatewayArtifacts(gatewayID, orgID, artifactType string) (*GatewayArtifactListResponse, error) {
	// First validate that the gateway exists and belongs to the organization
	gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
	if err != nil {
		return nil, err
	}
	if gateway == nil {
		return nil, errors.New("gateway not found")
	}
	if gateway.OrganizationUUID.String() != orgID {
		return nil, errors.New("gateway not found")
	}

	// Get all APIs deployed to this gateway
	apis, err := s.apiRepo.GetDeployedAPIsByGatewayUUID(gatewayID, orgID)
	if err != nil {
		return nil, err
	}

	// Apply type filtering before iterating
	allArtifacts := make([]GatewayArtifact, 0)
	// Short-circuit if artifactType filter doesn't match "API", "all", or empty
	if artifactType != "" && artifactType != "all" && artifactType != "API" {
		// Return empty list for non-API artifact types
		listResponse := &GatewayArtifactListResponse{
			Count: 0,
			List:  allArtifacts,
			Pagination: Pagination{
				Total:  0,
				Offset: 0,
				Limit:  0,
			},
		}
		return listResponse, nil
	}

	// Convert APIs to GatewayArtifact DTOs
	for _, api := range apis {
		artifact := GatewayArtifact{
			ID:        api.Handle,
			Name:      api.Name,
			Kind:      "RestAPI",
			CreatedAt: api.CreatedAt,
			UpdatedAt: api.UpdatedAt,
		}
		allArtifacts = append(allArtifacts, artifact)
	}

	listResponse := &GatewayArtifactListResponse{
		Count: len(allArtifacts),
		List:  allArtifacts,
		Pagination: Pagination{
			Total:  len(allArtifacts),
			Offset: 0,
			Limit:  len(allArtifacts),
		},
	}

	return listResponse, nil
}

// validateGatewayInput validates gateway registration inputs
func (s *PlatformGatewayService) validateGatewayInput(orgID, name, displayName, vhost, functionalityType string) error {
	// Organization ID validation
	if strings.TrimSpace(orgID) == "" {
		return errors.New("organization ID is required")
	}
	if _, err := uuid.Parse(orgID); err != nil {
		return errors.New("invalid organization ID format")
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

// generateToken generates a cryptographically secure 32-byte random token, base64-encoded
func generateToken() (string, error) {
	tokenBytes := make([]byte, 32)
	_, err := rand.Read(tokenBytes)
	if err != nil {
		return "", errors.New("failed to generate secure random token")
	}
	token := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(tokenBytes)
	return token, nil
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

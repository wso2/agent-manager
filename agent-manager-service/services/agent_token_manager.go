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
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// AgentTokenManagerService defines the interface for agent token operations
type AgentTokenManagerService interface {
	// GenerateToken creates a signed JWT token for an agent
	GenerateToken(ctx context.Context, req GenerateTokenRequest) (*models.TokenResponse, error)
	// GetJWKS returns the JSON Web Key Set for token verification
	GetJWKS(ctx context.Context) (*models.JWKS, error)
}

// GenerateTokenRequest contains the parameters for token generation
type GenerateTokenRequest struct {
	OrgName     string
	ProjectName string
	AgentName   string
	Environment string // Optional, defaults to config default if not provided
	ExpiresIn   string // Optional, Go duration format (e.g., "720h")
}

// AgentTokenClaims represents the custom claims for agent tokens
type AgentTokenClaims struct {
	jwt.RegisteredClaims
	ComponentUid    string `json:"component_uid"`
	EnvironmentUid  string `json:"environment_uid"`
	OrganizationUid string `json:"organization_uid,omitempty"`
	ProjectUid      string `json:"project_uid,omitempty"`
}

// KeyPair holds a private/public RSA key pair with its metadata
type KeyPair struct {
	KeyID      string
	PrivateKey *rsa.PrivateKey
	PublicKey  *rsa.PublicKey
}

type agentTokenManagerService struct {
	openChoreoClient openchoreosvc.OpenChoreoSvcClient
	config           config.JWTSigningConfig
	logger           *slog.Logger

	// Key management
	keyPairs     map[string]*KeyPair
	activeKeyID  string
	keyPairMutex sync.RWMutex
}

// NewAgentTokenManagerService creates a new AgentTokenManagerService instance
func NewAgentTokenManagerService(
	openChoreoClient openchoreosvc.OpenChoreoSvcClient,
	cfg config.JWTSigningConfig,
	logger *slog.Logger,
) (AgentTokenManagerService, error) {
	service := &agentTokenManagerService{
		openChoreoClient: openChoreoClient,
		config:           cfg,
		logger:           logger,
		keyPairs:         make(map[string]*KeyPair),
		activeKeyID:      cfg.ActiveKeyID,
	}

	// Load keys on initialization
	if err := service.loadKeys(); err != nil {
		return nil, fmt.Errorf("failed to load signing keys: %w", err)
	}

	return service, nil
}

// loadKeys loads RSA key pairs from configured paths
func (s *agentTokenManagerService) loadKeys() error {
	s.keyPairMutex.Lock()
	defer s.keyPairMutex.Unlock()

	// Load private key for signing
	privateKeyPEM, err := os.ReadFile(s.config.PrivateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key file: %w", err)
	}

	privateKeyBlock, _ := pem.Decode(privateKeyPEM)
	if privateKeyBlock == nil {
		return fmt.Errorf("failed to decode private key PEM")
	}

	var privateKey *rsa.PrivateKey
	// Try PKCS#1 format first
	privateKey, err = x509.ParsePKCS1PrivateKey(privateKeyBlock.Bytes)
	if err != nil {
		// Try PKCS#8 format
		key, err := x509.ParsePKCS8PrivateKey(privateKeyBlock.Bytes)
		if err != nil {
			return fmt.Errorf("failed to parse private key: %w", err)
		}
		var ok bool
		privateKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return fmt.Errorf("private key is not RSA")
		}
	}

	// Load public keys from JSON configuration
	if err := s.loadPublicKeysFromJSON(); err != nil {
		return fmt.Errorf("failed to load public keys from JSON: %w", err)
	}

	// Set the active key's private key
	if keyPair, ok := s.keyPairs[s.activeKeyID]; ok {
		keyPair.PrivateKey = privateKey
	} else {
		return fmt.Errorf("active key ID %s not found in loaded public keys", s.activeKeyID)
	}

	s.logger.Info("Successfully loaded JWT signing keys", "activeKeyID", s.activeKeyID, "totalKeys", len(s.keyPairs))
	return nil
}

// loadPublicKeysFromJSON loads multiple public keys from a JSON configuration file
func (s *agentTokenManagerService) loadPublicKeysFromJSON() error {
	configData, err := os.ReadFile(s.config.PublicKeysConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read public keys config file: %w", err)
	}

	var keysConfig config.PublicKeysConfig
	if err := json.Unmarshal(configData, &keysConfig); err != nil {
		return fmt.Errorf("failed to parse public keys config JSON: %w", err)
	}

	if len(keysConfig.Keys) == 0 {
		return fmt.Errorf("no keys found in public keys configuration")
	}

	// Load each public key
	for _, keyConfig := range keysConfig.Keys {
		publicKey, err := s.loadPublicKey(keyConfig.PublicKeyPath)
		if err != nil {
			s.logger.Warn("Failed to load public key, skipping",
				"kid", keyConfig.Kid,
				"path", keyConfig.PublicKeyPath,
				"error", err)
			continue
		}

		keyPair := &KeyPair{
			KeyID:      keyConfig.Kid,
			PublicKey:  publicKey,
			PrivateKey: nil, // Will be set for active key in loadKeys()
		}
		s.keyPairs[keyConfig.Kid] = keyPair
		s.logger.Info("Loaded public key", "kid", keyConfig.Kid, "description", keyConfig.Description)
	}

	if len(s.keyPairs) == 0 {
		return fmt.Errorf("failed to load any public keys from configuration")
	}

	return nil
}

// loadPublicKey loads a single RSA public key from a PEM file
func (s *agentTokenManagerService) loadPublicKey(path string) (*rsa.PublicKey, error) {
	publicKeyPEM, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read public key file: %w", err)
	}

	publicKeyBlock, _ := pem.Decode(publicKeyPEM)
	if publicKeyBlock == nil {
		return nil, fmt.Errorf("failed to decode public key PEM")
	}

	var publicKey *rsa.PublicKey
	// Try parsing as PKIX/SubjectPublicKeyInfo format first
	pubKey, err := x509.ParsePKIXPublicKey(publicKeyBlock.Bytes)
	if err != nil {
		// Try PKCS#1 format
		publicKey, err = x509.ParsePKCS1PublicKey(publicKeyBlock.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse public key: %w", err)
		}
	} else {
		var ok bool
		publicKey, ok = pubKey.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("public key is not RSA")
		}
	}

	return publicKey, nil
}

// GenerateToken creates a signed JWT token for an agent
func (s *agentTokenManagerService) GenerateToken(ctx context.Context, req GenerateTokenRequest) (*models.TokenResponse, error) {
	s.logger.Info("Generating token for agent",
		"agentName", req.AgentName,
		"orgName", req.OrgName,
		"projectName", req.ProjectName,
	)

	// Fetch component UID from OpenChoreo
	component, err := s.openChoreoClient.GetAgentComponent(ctx, req.OrgName, req.ProjectName, req.AgentName)
	if err != nil {
		s.logger.Error("Failed to get agent component", "agentName", req.AgentName, "error", err)
		return nil, fmt.Errorf("failed to get agent component: %w", err)
	}

	// Determine which environment to use
	environmentName := req.Environment
	if environmentName == "" {
		environmentName = s.config.DefaultEnvironment
	}

	// Fetch environment UID from OpenChoreo
	environment, err := s.openChoreoClient.GetEnvironment(ctx, req.OrgName, environmentName)
	if err != nil {
		s.logger.Error("Failed to get environment", "environment", environmentName, "error", err)
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	// // Fetch organization UID
	// organization, err := s.openChoreoClient.GetOrganization(ctx, req.OrgName)
	// if err != nil {
	// 	s.logger.Error("Failed to get organization", "orgName", req.OrgName, "error", err)
	// 	return nil, fmt.Errorf("failed to get organization: %w", err)
	// }

	// Fetch project UID
	project, err := s.openChoreoClient.GetProject(ctx, req.ProjectName, req.OrgName)
	if err != nil {
		s.logger.Error("Failed to get project", "projectName", req.ProjectName, "error", err)
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	// Determine expiry duration
	expiryDuration, err := s.parseExpiryDuration(req.ExpiresIn)
	if err != nil {
		return nil, fmt.Errorf("invalid expiry duration: %w", err)
	}

	now := time.Now()
	expiresAt := now.Add(expiryDuration)

	// Create claims
	claims := AgentTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.config.Issuer,
			Subject:   req.AgentName,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			NotBefore: jwt.NewNumericDate(now),
		},
		ComponentUid:   component.UUID,
		EnvironmentUid: environment.UUID,
		// OrganizationUid: organization.UUID,
		ProjectUid: project.UUID,
	}

	// Get the active signing key
	s.keyPairMutex.RLock()
	keyPair, exists := s.keyPairs[s.activeKeyID]
	s.keyPairMutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("active signing key not found: %s", s.activeKeyID)
	}

	// Create and sign the token
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyPair.KeyID

	signedToken, err := token.SignedString(keyPair.PrivateKey)
	if err != nil {
		s.logger.Error("Failed to sign token", "error", err)
		return nil, fmt.Errorf("failed to sign token: %w", err)
	}

	s.logger.Info("Token generated successfully",
		"agentName", req.AgentName,
		"expiresAt", expiresAt,
		"keyID", keyPair.KeyID,
	)

	return &models.TokenResponse{
		Token:     signedToken,
		ExpiresAt: expiresAt.Unix(),
		IssuedAt:  now.Unix(),
		TokenType: "Bearer",
	}, nil
}

// parseExpiryDuration parses the expiry duration string and validates it
func (s *agentTokenManagerService) parseExpiryDuration(expiresIn string) (time.Duration, error) {
	if expiresIn == "" {
		// Use default expiry duration from config
		duration, err := time.ParseDuration(s.config.DefaultExpiryDuration)
		if err != nil {
			// Fallback to 1 year if parsing fails
			return 365 * 24 * time.Hour, nil
		}
		return duration, nil
	}

	duration, err := time.ParseDuration(expiresIn)
	if err != nil {
		return 0, fmt.Errorf("invalid duration format: %w", err)
	}

	// Validate duration is positive and not zero
	if duration <= 0 {
		return 0, fmt.Errorf("expiry duration must be positive")
	}

	// Set a maximum expiry (e.g., 10 years)
	maxExpiry := 10 * 365 * 24 * time.Hour
	if duration > maxExpiry {
		return 0, fmt.Errorf("expiry duration cannot exceed 10 years")
	}

	return duration, nil
}

// GetJWKS returns the JSON Web Key Set containing all public keys
func (s *agentTokenManagerService) GetJWKS(ctx context.Context) (*models.JWKS, error) {
	s.keyPairMutex.RLock()
	defer s.keyPairMutex.RUnlock()

	keys := make([]models.JWK, 0, len(s.keyPairs))

	for keyID, keyPair := range s.keyPairs {
		jwk := models.JWK{
			Kty: "RSA",
			Alg: "RS256",
			Use: "sig",
			Kid: keyID,
			N:   base64.RawURLEncoding.EncodeToString(keyPair.PublicKey.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(keyPair.PublicKey.E)).Bytes()),
		}
		keys = append(keys, jwk)
	}

	return &models.JWKS{
		Keys: keys,
	}, nil
}

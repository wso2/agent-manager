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
	"fmt"
	"log/slog"
	"path"

	"github.com/wso2/ai-agent-management-platform/secret-management-service/providers"
)

// SecretService provides secret management operations
type SecretService interface {
	CreateSecret(ctx context.Context, orgName string, req CreateSecretRequest) (*SecretResponse, error)
	GetSecret(ctx context.Context, orgName, secretPath string, version *int) (*SecretResponse, error)
	UpdateSecret(ctx context.Context, orgName, secretPath string, req UpdateSecretRequest) (*SecretResponse, error)
	DeleteSecret(ctx context.Context, orgName, secretPath string) error
	ListSecrets(ctx context.Context, orgName, pathPrefix string) ([]SecretMetadataResponse, error)
	GetSecretVersions(ctx context.Context, orgName, secretPath string) ([]SecretVersionResponse, error)
}

// CreateSecretRequest represents a request to create a secret
type CreateSecretRequest struct {
	Path string            `json:"path"`
	Data map[string]string `json:"data"`
}

// UpdateSecretRequest represents a request to update a secret
type UpdateSecretRequest struct {
	Data map[string]string `json:"data"`
}

// SecretResponse represents a secret in responses
type SecretResponse struct {
	Path     string                   `json:"path"`
	Data     map[string]string        `json:"data"`
	Version  int                      `json:"version"`
	Metadata SecretMetadataResponse   `json:"metadata"`
}

// SecretMetadataResponse represents secret metadata in responses
type SecretMetadataResponse struct {
	Path        string `json:"path"`
	Version     int    `json:"version"`
	CreatedTime string `json:"createdTime"`
	UpdatedTime string `json:"updatedTime"`
}

// SecretVersionResponse represents a secret version in responses
type SecretVersionResponse struct {
	Version     int    `json:"version"`
	CreatedTime string `json:"createdTime"`
	Deleted     bool   `json:"deleted"`
}

type secretService struct {
	kvProvider providers.Provider
	logger     *slog.Logger
}

// NewSecretService creates a new secret service
func NewSecretService(kvProvider providers.Provider, logger *slog.Logger) SecretService {
	return &secretService{
		kvProvider: kvProvider,
		logger:     logger,
	}
}

// buildOrgPath creates a path scoped to the organization
func buildOrgPath(orgName, secretPath string) string {
	return path.Join(orgName, secretPath)
}

func (s *secretService) CreateSecret(ctx context.Context, orgName string, req CreateSecretRequest) (*SecretResponse, error) {
	fullPath := buildOrgPath(orgName, req.Path)

	s.logger.Info("Creating secret", "org", orgName, "path", req.Path)

	secret, err := s.kvProvider.CreateSecret(ctx, providers.CreateSecretRequest{
		Path:           fullPath,
		Data:           req.Data,
		OrganizationID: orgName,
	})
	if err != nil {
		s.logger.Error("Failed to create secret", "error", err, "org", orgName, "path", req.Path)
		return nil, fmt.Errorf("failed to create secret: %w", err)
	}

	return toSecretResponse(secret), nil
}

func (s *secretService) GetSecret(ctx context.Context, orgName, secretPath string, version *int) (*SecretResponse, error) {
	fullPath := buildOrgPath(orgName, secretPath)

	s.logger.Info("Getting secret", "org", orgName, "path", secretPath)

	secret, err := s.kvProvider.GetSecret(ctx, fullPath, version)
	if err != nil {
		s.logger.Error("Failed to get secret", "error", err, "org", orgName, "path", secretPath)
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	return toSecretResponse(secret), nil
}

func (s *secretService) UpdateSecret(ctx context.Context, orgName, secretPath string, req UpdateSecretRequest) (*SecretResponse, error) {
	fullPath := buildOrgPath(orgName, secretPath)

	s.logger.Info("Updating secret", "org", orgName, "path", secretPath)

	secret, err := s.kvProvider.UpdateSecret(ctx, fullPath, providers.UpdateSecretRequest{
		Data: req.Data,
	})
	if err != nil {
		s.logger.Error("Failed to update secret", "error", err, "org", orgName, "path", secretPath)
		return nil, fmt.Errorf("failed to update secret: %w", err)
	}

	return toSecretResponse(secret), nil
}

func (s *secretService) DeleteSecret(ctx context.Context, orgName, secretPath string) error {
	fullPath := buildOrgPath(orgName, secretPath)

	s.logger.Info("Deleting secret", "org", orgName, "path", secretPath)

	if err := s.kvProvider.DeleteSecret(ctx, fullPath); err != nil {
		s.logger.Error("Failed to delete secret", "error", err, "org", orgName, "path", secretPath)
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	return nil
}

func (s *secretService) ListSecrets(ctx context.Context, orgName, pathPrefix string) ([]SecretMetadataResponse, error) {
	fullPath := buildOrgPath(orgName, pathPrefix)

	s.logger.Info("Listing secrets", "org", orgName, "pathPrefix", pathPrefix)

	secrets, err := s.kvProvider.ListSecrets(ctx, fullPath)
	if err != nil {
		s.logger.Error("Failed to list secrets", "error", err, "org", orgName, "pathPrefix", pathPrefix)
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	var result []SecretMetadataResponse
	for _, secret := range secrets {
		result = append(result, SecretMetadataResponse{
			Path:    secret.Path,
			Version: secret.Version,
		})
	}

	return result, nil
}

func (s *secretService) GetSecretVersions(ctx context.Context, orgName, secretPath string) ([]SecretVersionResponse, error) {
	fullPath := buildOrgPath(orgName, secretPath)

	s.logger.Info("Getting secret versions", "org", orgName, "path", secretPath)

	versions, err := s.kvProvider.GetSecretVersions(ctx, fullPath)
	if err != nil {
		s.logger.Error("Failed to get secret versions", "error", err, "org", orgName, "path", secretPath)
		return nil, fmt.Errorf("failed to get secret versions: %w", err)
	}

	var result []SecretVersionResponse
	for _, v := range versions {
		result = append(result, SecretVersionResponse{
			Version:     v.Version,
			CreatedTime: v.CreatedTime.Format("2006-01-02T15:04:05Z07:00"),
			Deleted:     v.Deleted,
		})
	}

	return result, nil
}

func toSecretResponse(secret *providers.Secret) *SecretResponse {
	return &SecretResponse{
		Path:    secret.Path,
		Data:    secret.Data,
		Version: secret.Metadata.Version,
		Metadata: SecretMetadataResponse{
			Path:        secret.Metadata.Path,
			Version:     secret.Metadata.Version,
			CreatedTime: secret.Metadata.CreatedTime.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedTime: secret.Metadata.UpdatedTime.Format("2006-01-02T15:04:05Z07:00"),
		},
	}
}

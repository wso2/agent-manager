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
	"errors"
	"log/slog"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

const (
	// Git secret type values (used in API)
	GitSecretTypeBasic = "basic-auth"
)

// GitSecretInfo contains metadata about a git secret (without credentials)
type GitSecretInfo struct {
	Name string
}

// GitSecretService handles git secret business logic
type GitSecretService struct {
	ocClient client.OpenChoreoClient
}

// NewGitSecretService creates a new git secret service
func NewGitSecretService(
	ocClient client.OpenChoreoClient,
) *GitSecretService {
	return &GitSecretService{
		ocClient: ocClient,
	}
}

// Create creates a new git secret
func (s *GitSecretService) Create(ctx context.Context, orgName string, req *spec.CreateGitSecretRequest) (*GitSecretInfo, error) {
	slog.Info("GitSecretService.Create: starting", "orgName", orgName, "name", req.Name, "type", req.Type)

	// Validate input
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Map API type to OpenChoreo type
	secretType := mapAPITypeToOCType(req.Type)

	// Build the OpenChoreo request
	ocReq := client.CreateGitSecretRequest{
		Name:       req.Name,
		SecretType: secretType,
		Username:   req.Credentials.Username,
		Token:      req.Credentials.Password,
	}

	// Create git secret via OpenChoreo
	result, err := s.ocClient.CreateGitSecret(ctx, orgName, ocReq)
	if err != nil {
		if errors.Is(err, utils.ErrConflict) {
			slog.Warn("GitSecretService.Create: git secret already exists", "orgName", orgName, "name", req.Name)
			return nil, utils.ErrGitSecretAlreadyExists
		}
		slog.Error("GitSecretService.Create: failed to create git secret", "orgName", orgName, "name", req.Name, "error", err)
		return nil, err
	}

	slog.Info("GitSecretService.Create: git secret created successfully", "orgName", orgName, "name", result.Name)

	return &GitSecretInfo{
		Name: result.Name,
	}, nil
}

// List lists all git secrets for an organization
func (s *GitSecretService) List(ctx context.Context, orgName string, limit, offset int) ([]*GitSecretInfo, int, error) {
	slog.Info("GitSecretService.List: starting", "orgName", orgName, "limit", limit, "offset", offset)

	// List git secrets via OpenChoreo
	secrets, err := s.ocClient.ListGitSecrets(ctx, orgName)
	if err != nil {
		slog.Error("GitSecretService.List: failed to list git secrets", "orgName", orgName, "error", err)
		return nil, 0, err
	}

	// Convert to GitSecretInfo
	gitSecrets := make([]*GitSecretInfo, len(secrets))
	for i, secret := range secrets {
		gitSecrets[i] = &GitSecretInfo{
			Name: secret.Name,
		}
	}

	totalCount := len(gitSecrets)

	// Apply pagination
	if offset >= totalCount {
		return []*GitSecretInfo{}, totalCount, nil
	}
	end := offset + limit
	if end > totalCount {
		end = totalCount
	}

	slog.Info("GitSecretService.List: completed", "orgName", orgName, "count", len(gitSecrets[offset:end]), "total", totalCount)
	return gitSecrets[offset:end], totalCount, nil
}

// Delete deletes a git secret
func (s *GitSecretService) Delete(ctx context.Context, orgName, secretName string) error {
	slog.Info("GitSecretService.Delete: starting", "orgName", orgName, "secretName", secretName)

	if secretName == "" {
		return utils.ErrInvalidInput
	}

	// Delete git secret via OpenChoreo
	if err := s.ocClient.DeleteGitSecret(ctx, orgName, secretName); err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			return utils.ErrGitSecretNotFound
		}
		slog.Error("GitSecretService.Delete: failed to delete git secret", "orgName", orgName, "secretName", secretName, "error", err)
		return err
	}

	slog.Info("GitSecretService.Delete: completed", "orgName", orgName, "secretName", secretName)
	return nil
}

// validateCreateRequest validates the create git secret request
func (s *GitSecretService) validateCreateRequest(req *spec.CreateGitSecretRequest) error {
	if req.Name == "" {
		return utils.ErrInvalidInput
	}

	// Validate name format (alphanumeric, hyphens allowed)
	if !isValidSecretName(req.Name) {
		return utils.ErrInvalidInput
	}

	if req.Type != GitSecretTypeBasic {
		return utils.ErrGitSecretInvalidType
	}

	if req.Credentials.Password == "" {
		return utils.ErrInvalidInput
	}

	return nil
}

// mapAPITypeToOCType maps API git secret type to OpenChoreo type
func mapAPITypeToOCType(_ string) client.GitSecretType {
	return client.GitSecretTypeBasicAuth
}

// isValidSecretName validates that a secret name contains only valid characters
func isValidSecretName(name string) bool {
	if len(name) == 0 || len(name) > 63 {
		return false
	}
	// Must start and end with alphanumeric, can contain hyphens
	for i, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			continue
		}
		if c == '-' && i > 0 && i < len(name)-1 {
			continue
		}
		return false
	}
	return true
}

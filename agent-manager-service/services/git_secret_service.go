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

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
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
func (s *GitSecretService) Create(ctx context.Context, ouID string, req *spec.CreateGitSecretRequest) (*GitSecretInfo, error) {
	logger.GetLogger(ctx).Info("GitSecretService.Create: starting", "ou_id", ouID, "name", req.Name, "type", req.Type)

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

	// The credential lands in an external secret store, so the change and its
	// audit record cannot commit together. Record the intent first and refuse
	// the operation if that fails: a credential written with no trace of who
	// wrote it is worse than a failed request. Only the name, type and username
	// are recorded — the password never leaves this function.
	attempt, err := audit.Begin(
		ctx, audit.ActionGitSecretCreate,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceGitSecret, req.Name, req.Name),
		audit.Detail("secretType", string(secretType)),
		audit.Detail("username", req.Credentials.Username),
	)
	if err != nil {
		logger.GetLogger(ctx).Warn("GitSecretService.Create: refusing, audit record could not be written",
			"ou_id", ouID, "name", req.Name, "error", err)
		return nil, err
	}

	// Create git secret via OpenChoreo
	result, err := s.ocClient.CreateGitSecret(ctx, ouID, ocReq)
	if err != nil {
		attempt.Complete(ctx, err)
		if errors.Is(err, utils.ErrConflict) {
			logger.GetLogger(ctx).Warn("GitSecretService.Create: git secret already exists", "ou_id", ouID, "name", req.Name)
			return nil, utils.ErrGitSecretAlreadyExists
		}
		logger.GetLogger(ctx).Warn("GitSecretService.Create: failed to create git secret", "ou_id", ouID, "name", req.Name, "error", err)
		return nil, err
	}
	attempt.Complete(ctx, nil)

	logger.GetLogger(ctx).Info("GitSecretService.Create: git secret created successfully", "ou_id", ouID, "name", result.Name)

	return &GitSecretInfo{
		Name: result.Name,
	}, nil
}

// List lists all git secrets for an organization
func (s *GitSecretService) List(ctx context.Context, ouID string, limit, offset int) ([]*GitSecretInfo, int, error) {
	logger.GetLogger(ctx).Info("GitSecretService.List: starting", "ou_id", ouID, "limit", limit, "offset", offset)

	// List git secrets via OpenChoreo
	secrets, err := s.ocClient.ListGitSecrets(ctx, ouID)
	if err != nil {
		logger.GetLogger(ctx).Warn("GitSecretService.List: failed to list git secrets", "ou_id", ouID, "error", err)
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

	logger.GetLogger(ctx).Info("GitSecretService.List: completed", "ou_id", ouID, "count", len(gitSecrets[offset:end]), "total", totalCount)
	return gitSecrets[offset:end], totalCount, nil
}

// Delete deletes a git secret
func (s *GitSecretService) Delete(ctx context.Context, ouID, secretName string) error {
	logger.GetLogger(ctx).Info("GitSecretService.Delete: starting", "ou_id", ouID, "secret_name", secretName)

	if secretName == "" {
		return utils.ErrInvalidInput
	}

	attempt, err := audit.Begin(
		ctx, audit.ActionGitSecretDelete,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceGitSecret, secretName, secretName),
	)
	if err != nil {
		logger.GetLogger(ctx).Warn("GitSecretService.Delete: refusing, audit record could not be written",
			"ou_id", ouID, "secret_name", secretName, "error", err)
		return err
	}

	// Delete git secret via OpenChoreo
	if err := s.ocClient.DeleteGitSecret(ctx, ouID, secretName); err != nil {
		attempt.Complete(ctx, err)
		if errors.Is(err, utils.ErrNotFound) {
			return utils.ErrGitSecretNotFound
		}
		logger.GetLogger(ctx).Warn("GitSecretService.Delete: failed to delete git secret", "ou_id", ouID, "secret_name", secretName, "error", err)
		return err
	}
	attempt.Complete(ctx, nil)

	logger.GetLogger(ctx).Info("GitSecretService.Delete: completed", "ou_id", ouID, "secret_name", secretName)
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

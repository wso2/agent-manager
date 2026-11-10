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
	"log/slog"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
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
	store GitSecretStore
}

// NewGitSecretService creates a new git secret service
func NewGitSecretService(
	store GitSecretStore,
) *GitSecretService {
	return &GitSecretService{
		store: store,
	}
}

// Create creates a new git secret
func (s *GitSecretService) Create(ctx context.Context, ouID string, req *spec.CreateGitSecretRequest) (*GitSecretInfo, error) {
	slog.Info("GitSecretService.Create: starting", "ouID", ouID, "name", req.Name, "type", req.Type)

	// Validate input
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Record intent before writing the credential and publishing its workflow
	// reference. The audit event contains only the name, type and username.
	attempt, err := audit.Begin(
		ctx, audit.ActionGitSecretCreate,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceGitSecret, req.Name, req.Name),
		audit.Detail("secretType", req.Type),
		audit.Detail("username", req.Credentials.Username),
	)
	if err != nil {
		slog.Error("GitSecretService.Create: refusing, audit record could not be written",
			"ouID", ouID, "name", req.Name, "error", err)
		return nil, err
	}

	err = s.store.CreateGitSecret(ctx, ouID, req.Name, req.Credentials.Username, req.Credentials.Password)
	attempt.Complete(ctx, err)
	if err != nil {
		return nil, err
	}
	return &GitSecretInfo{Name: req.Name}, nil
}

// List lists all git secrets for an organization
func (s *GitSecretService) List(ctx context.Context, ouID string, limit, offset int) ([]*GitSecretInfo, int, error) {
	slog.Info("GitSecretService.List: starting", "ouID", ouID, "limit", limit, "offset", offset)

	names, total, err := s.store.ListGitSecretNames(ctx, ouID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	gitSecrets := make([]*GitSecretInfo, len(names))
	for i, name := range names {
		gitSecrets[i] = &GitSecretInfo{Name: name}
	}
	return gitSecrets, total, nil
}

// Delete deletes a git secret
func (s *GitSecretService) Delete(ctx context.Context, ouID, secretName string) error {
	slog.Info("GitSecretService.Delete: starting", "ouID", ouID, "secretName", secretName)

	if !isValidSecretName(secretName) {
		return utils.ErrInvalidInput
	}

	attempt, err := audit.Begin(
		ctx, audit.ActionGitSecretDelete,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceGitSecret, secretName, secretName),
	)
	if err != nil {
		slog.Error("GitSecretService.Delete: refusing, audit record could not be written",
			"ouID", ouID, "secretName", secretName, "error", err)
		return err
	}

	err = s.store.DeleteGitSecret(ctx, ouID, secretName)
	attempt.Complete(ctx, err)
	return err
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

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

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/gitprovider"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// RepositoryService defines the interface for repository operations
type RepositoryService interface {
	// ListBranches returns branches for a repository
	ListBranches(ctx context.Context, req spec.ListBranchesRequest, providerType gitprovider.ProviderType, limit, offset int) (*spec.ListBranchesResponse, error)
	// ListCommits returns commits for a repository
	ListCommits(ctx context.Context, req spec.ListCommitsRequest, providerType gitprovider.ProviderType, limit, offset int) (*spec.ListCommitsResponse, error)
	// GetLatestCommit returns the latest commit SHA for a given branch
	GetLatestCommit(ctx context.Context, owner, repo, branch string) (string, error)
}

type repositoryService struct {
	gitCredentialsService GitCredentialsService
	logger                *slog.Logger
}

// NewRepositoryService creates a new repository service
func NewRepositoryService(gitCredentialsService GitCredentialsService, logger *slog.Logger) RepositoryService {
	return &repositoryService{
		gitCredentialsService: gitCredentialsService,
		logger:                logger,
	}
}

// getGitProviderConfig returns the git provider configuration with token from server config
func getGitProviderConfig() gitprovider.Config {
	cfg := config.GetConfig()
	return gitprovider.Config{
		Token: cfg.GitHub.Token,
	}
}

// getGitProviderConfigWithCredentials returns the git provider configuration with credentials
// Returns error if credentials are invalid or missing
func getGitProviderConfigWithCredentials(creds *GitCredentials) (gitprovider.Config, error) {
	if creds == nil {
		return gitprovider.Config{}, utils.ErrGitSecretInvalidType
	}
	// Only basic-auth with a valid password is supported
	if creds.Type != "basic-auth" || creds.Password == "" {
		return gitprovider.Config{}, utils.ErrGitSecretInvalidType
	}
	return gitprovider.Config{
		Token: creds.Password,
	}, nil
}

// ListBranches returns branches for a repository
func (s *repositoryService) ListBranches(ctx context.Context, req spec.ListBranchesRequest, providerType gitprovider.ProviderType, limit, offset int) (*spec.ListBranchesResponse, error) {
	// Determine git provider configuration
	providerConfig := getGitProviderConfig()

	// If secretRef is provided, fetch git credentials from workflow plane OpenBao
	if req.HasSecretRef() && req.HasOrgName() {
		creds, err := s.gitCredentialsService.GetGitCredentials(ctx, req.GetOrgName(), req.GetSecretRef())
		if err != nil {
			s.logger.Error("failed to get git credentials", "error", err, "secretRef", req.GetSecretRef(), "orgName", req.GetOrgName())
			return nil, err
		}
		providerConfig, err = getGitProviderConfigWithCredentials(creds)
		if err != nil {
			s.logger.Error("invalid git credentials", "error", err, "secretRef", req.GetSecretRef())
			return nil, err
		}
		s.logger.Debug("using git credentials for private repository", "secretRef", req.GetSecretRef())
	}

	// Create provider with configuration
	provider, err := gitprovider.NewProvider(providerType, providerConfig)
	if err != nil {
		return nil, err
	}

	// List branches
	includeDefault := false
	if req.IncludeDefault != nil {
		includeDefault = *req.IncludeDefault
	}
	result, err := provider.ListBranches(ctx, req.Owner, req.Repository, gitprovider.ListBranchesOptions{
		IncludeDefault: includeDefault,
	})
	if err != nil {
		return nil, err
	}

	// Convert to response model
	branches := make([]spec.Branch, len(result.Branches))
	for i, b := range result.Branches {
		branches[i] = spec.Branch{
			Name:      b.Name,
			CommitSha: b.CommitSHA,
			IsDefault: b.IsDefault,
		}
	}

	response := &spec.ListBranchesResponse{
		Branches: branches,
		Limit:    int32(limit),
		Offset:   int32(offset),
	}
	if result.HasMore {
		nextOffset := int32(offset + limit)
		response.NextOffset = &nextOffset
	}
	return response, nil
}

// ListCommits returns commits for a repository
func (s *repositoryService) ListCommits(ctx context.Context, req spec.ListCommitsRequest, providerType gitprovider.ProviderType, limit, offset int) (*spec.ListCommitsResponse, error) {
	// Determine git provider configuration
	providerConfig := getGitProviderConfig()

	// If secretRef is provided, fetch git credentials from workflow plane OpenBao
	if req.HasSecretRef() && req.HasOrgName() {
		creds, err := s.gitCredentialsService.GetGitCredentials(ctx, req.GetOrgName(), req.GetSecretRef())
		if err != nil {
			s.logger.Error("failed to get git credentials", "error", err, "secretRef", req.GetSecretRef(), "orgName", req.GetOrgName())
			return nil, err
		}
		providerConfig, err = getGitProviderConfigWithCredentials(creds)
		if err != nil {
			s.logger.Error("invalid git credentials", "error", err, "secretRef", req.GetSecretRef())
			return nil, err
		}
		s.logger.Debug("using git credentials for private repository", "secretRef", req.GetSecretRef())
	}

	// Create provider with configuration
	provider, err := gitprovider.NewProvider(providerType, providerConfig)
	if err != nil {
		return nil, err
	}
	// Build options
	opts := gitprovider.ListCommitsOptions{
		SHA:    req.GetBranch(),
		Path:   req.GetPath(),
		Author: req.GetAuthor(),
		Since:  req.Since,
		Until:  req.Until,
	}

	// List commits
	result, err := provider.ListCommits(ctx, req.Owner, req.Repo, opts)
	if err != nil {
		return nil, err
	}

	// Convert to response model
	commits := make([]spec.Commit, len(result.Commits))
	for i, c := range result.Commits {
		shortSHA := c.SHA
		if len(shortSHA) > 7 {
			shortSHA = shortSHA[:7]
		}

		author := spec.CommitAuthor{
			Name:  c.Author.Name,
			Email: c.Author.Email,
		}
		if c.Author.AvatarURL != "" {
			author.AvatarUrl = &c.Author.AvatarURL
		}

		commits[i] = spec.Commit{
			Sha:       c.SHA,
			ShortSha:  shortSHA,
			Message:   c.Message,
			Author:    author,
			Timestamp: c.Timestamp,
			IsLatest:  c.IsLatest,
		}
	}

	response := &spec.ListCommitsResponse{
		Commits: commits,
		Limit:   int32(limit),
		Offset:  int32(offset),
	}
	if result.HasMore {
		nextOffset := int32(offset + limit)
		response.NextOffset = &nextOffset
	}
	return response, nil
}

// GetLatestCommit returns the latest commit SHA for a given branch
func (s *repositoryService) GetLatestCommit(ctx context.Context, owner, repo, branch string) (string, error) {
	// Create provider with server-side token configuration
	provider, err := gitprovider.NewProvider(gitprovider.ProviderGitHub, getGitProviderConfig())
	if err != nil {
		return "", err
	}

	// Get only the first commit (latest)
	result, err := provider.ListCommits(ctx, owner, repo, gitprovider.ListCommitsOptions{
		SHA:     branch,
		Page:    1,
		PerPage: 1,
	})
	if err != nil {
		return "", err
	}

	if len(result.Commits) == 0 {
		return "", gitprovider.ErrNotFound
	}

	return result.Commits[0].SHA, nil
}

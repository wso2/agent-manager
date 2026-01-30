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

package gitprovider

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/requests"
)

const (
	// GitHubAPIBaseURL is the base URL for GitHub REST API
	GitHubAPIBaseURL = "https://api.github.com"

	// GitHubAPIVersion is the API version to use
	// Reference: https://docs.github.com/en/rest/about-the-rest-api/api-versions
	GitHubAPIVersion = "2022-11-28"

	// DefaultPerPage is the default number of results per page
	DefaultPerPage = 30

	// MaxPerPage is the maximum number of results per page allowed by GitHub
	MaxPerPage = 100

	// Rate limit retry configuration
	// Reference: https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api#handle-rate-limit-errors-appropriately
	gitHubRetryWaitMin     = 60 * time.Second // GitHub recommends minimum 1 minute wait
	gitHubRetryWaitMax     = 5 * time.Minute  // Maximum wait between retries
	gitHubRetryAttemptsMax = 3                // Maximum retry attempts
	gitHubAttemptTimeout   = 30 * time.Second // Timeout for individual requests
)

// gitHubRetryConfig returns the retry configuration for GitHub API requests
func gitHubRetryConfig() requests.RequestRetryConfig {
	return requests.RequestRetryConfig{
		RetryWaitMin:     gitHubRetryWaitMin,
		RetryWaitMax:     gitHubRetryWaitMax,
		RetryAttemptsMax: gitHubRetryAttemptsMax,
		AttemptTimeout:   gitHubAttemptTimeout,
	}
}

// GitHubProvider implements the Provider interface for GitHub
type GitHubProvider struct {
	token   string
	baseURL string
}

// NewGitHubProvider creates a new GitHub provider
func NewGitHubProvider(cfg Config) (*GitHubProvider, error) {
	return &GitHubProvider{
		token:   cfg.Token,
		baseURL: GitHubAPIBaseURL,
	}, nil
}

// GetProviderType returns the provider type
func (g *GitHubProvider) GetProviderType() ProviderType {
	return ProviderGitHub
}

// ListBranches returns available branches for a repository
// Reference: https://docs.github.com/en/rest/branches/branches
func (g *GitHubProvider) ListBranches(ctx context.Context, owner, repo string, opts ListBranchesOptions) (*ListBranchesResponse, error) {
	perPage, page := normalizePagination(opts.PerPage, opts.Page)

	// Get default branch from repository info
	defaultBranch, err := g.getDefaultBranch(ctx, owner, repo)
	if err != nil {
		return nil, err
	}

	req := (&requests.HttpRequest{
		Name:   "github.ListBranches",
		URL:    fmt.Sprintf("%s/repos/%s/%s/branches", g.baseURL, owner, repo),
		Method: http.MethodGet,
	}).
		SetHeader("Accept", "application/vnd.github+json").
		SetHeader("X-GitHub-Api-Version", GitHubAPIVersion).
		SetQuery("per_page", strconv.Itoa(perPage)).
		SetQuery("page", strconv.Itoa(page))

	if g.token != "" {
		req.SetHeader("Authorization", "Bearer "+g.token)
	}

	var ghBranches []githubBranch
	result := requests.SendRequest(ctx, http.DefaultClient, req, gitHubRetryConfig())
	if err := result.ScanResponse(&ghBranches, http.StatusOK); err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}

	branches := make([]Branch, len(ghBranches))
	for i, b := range ghBranches {
		branches[i] = Branch{
			Name:      b.Name,
			CommitSHA: b.Commit.SHA,
			IsDefault: b.Name == defaultBranch,
		}
	}

	return &ListBranchesResponse{
		Branches: branches,
		Page:     page,
		PerPage:  perPage,
		HasMore:  hasNextPage(result.GetHeader("Link")),
	}, nil
}

// getDefaultBranch fetches the repository's default branch name
func (g *GitHubProvider) getDefaultBranch(ctx context.Context, owner, repo string) (string, error) {
	req := (&requests.HttpRequest{
		Name:   "github.GetRepository",
		URL:    fmt.Sprintf("%s/repos/%s/%s", g.baseURL, owner, repo),
		Method: http.MethodGet,
	}).
		SetHeader("Accept", "application/vnd.github+json").
		SetHeader("X-GitHub-Api-Version", GitHubAPIVersion)

	if g.token != "" {
		req.SetHeader("Authorization", "Bearer "+g.token)
	}

	var repoInfo struct {
		DefaultBranch string `json:"default_branch"`
	}
	result := requests.SendRequest(ctx, http.DefaultClient, req, gitHubRetryConfig())
	if err := result.ScanResponse(&repoInfo, http.StatusOK); err != nil {
		return "", fmt.Errorf("failed to get repository info: %w", err)
	}

	return repoInfo.DefaultBranch, nil
}

// ListCommits returns commits for a repository
// Reference: https://docs.github.com/en/rest/commits/commits
func (g *GitHubProvider) ListCommits(ctx context.Context, owner, repo string, opts ListCommitsOptions) (*ListCommitsResponse, error) {
	perPage, page := normalizePagination(opts.PerPage, opts.Page)

	req := (&requests.HttpRequest{
		Name:   "github.ListCommits",
		URL:    fmt.Sprintf("%s/repos/%s/%s/commits", g.baseURL, owner, repo),
		Method: http.MethodGet,
	}).
		SetHeader("Accept", "application/vnd.github+json").
		SetHeader("X-GitHub-Api-Version", GitHubAPIVersion).
		SetQuery("per_page", strconv.Itoa(perPage)).
		SetQuery("page", strconv.Itoa(page))

	if g.token != "" {
		req.SetHeader("Authorization", "Bearer "+g.token)
	}
	if opts.SHA != "" {
		req.SetQuery("sha", opts.SHA)
	}
	if opts.Path != "" {
		req.SetQuery("path", opts.Path)
	}
	if opts.Author != "" {
		req.SetQuery("author", opts.Author)
	}
	if opts.Since != nil {
		req.SetQuery("since", opts.Since.Format(time.RFC3339))
	}
	if opts.Until != nil {
		req.SetQuery("until", opts.Until.Format(time.RFC3339))
	}

	var ghCommits []githubCommit
	result := requests.SendRequest(ctx, http.DefaultClient, req, gitHubRetryConfig())
	if err := result.ScanResponse(&ghCommits, http.StatusOK); err != nil {
		return nil, fmt.Errorf("failed to list commits: %w", err)
	}

	commits := make([]Commit, len(ghCommits))
	for i, c := range ghCommits {
		commits[i] = Commit{
			SHA:     c.SHA,
			Message: c.Commit.Message,
			Author: Author{
				Name:      c.Commit.Author.Name,
				Email:     c.Commit.Author.Email,
				AvatarURL: c.Author.AvatarURL,
			},
			Timestamp: c.Commit.Author.Date,
			IsLatest:  i == 0 && page == 1,
		}
	}

	return &ListCommitsResponse{
		Commits: commits,
		Page:    page,
		PerPage: perPage,
		HasMore: hasNextPage(result.GetHeader("Link")),
	}, nil
}

// normalizePagination applies defaults and limits to pagination parameters
func normalizePagination(perPage, page int) (int, int) {
	if perPage <= 0 {
		perPage = DefaultPerPage
	}
	if perPage > MaxPerPage {
		perPage = MaxPerPage
	}
	if page <= 0 {
		page = 1
	}
	return perPage, page
}

// hasNextPage checks if there are more pages by parsing the Link header
// Reference: https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api#use-link-headers
func hasNextPage(linkHeader string) bool {
	return strings.Contains(linkHeader, `rel="next"`)
}

// GitHubError represents an error from the GitHub API
type GitHubError struct {
	StatusCode int
	Message    string
	Response   string
}

func (e *GitHubError) Error() string {
	return fmt.Sprintf("GitHub API error (status %d): %s", e.StatusCode, e.Message)
}

// GitHub API response types

type githubBranch struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
		URL string `json:"url"`
	} `json:"commit"`
	Protected bool `json:"protected"`
}

type githubCommit struct {
	SHA    string `json:"sha"`
	NodeID string `json:"node_id"`
	Commit struct {
		Author struct {
			Name  string    `json:"name"`
			Email string    `json:"email"`
			Date  time.Time `json:"date"`
		} `json:"author"`
		Message string `json:"message"`
	} `json:"commit"`
	Author struct {
		AvatarURL string `json:"avatar_url"`
	} `json:"author"`
}

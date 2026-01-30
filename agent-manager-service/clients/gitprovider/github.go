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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
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

	// requestTimeout is the timeout for individual API requests
	requestTimeout = 30 * time.Second
)

// GitHubProvider implements the Provider interface for GitHub
type GitHubProvider struct {
	token      string
	httpClient *http.Client
	baseURL    string
}

// NewGitHubProvider creates a new GitHub provider
func NewGitHubProvider(cfg Config) (*GitHubProvider, error) {
	return &GitHubProvider{
		token:      cfg.Token,
		httpClient: &http.Client{Timeout: requestTimeout},
		baseURL:    GitHubAPIBaseURL,
	}, nil
}

// GetProviderType returns the provider type
func (g *GitHubProvider) GetProviderType() ProviderType {
	return ProviderGitHub
}

// ListBranches returns available branches for a repository
// Reference: https://docs.github.com/en/rest/branches/branches
func (g *GitHubProvider) ListBranches(ctx context.Context, owner, repo string, opts ListBranchesOptions) (*ListBranchesResponse, error) {
	// Apply defaults
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = DefaultPerPage
	}
	if perPage > MaxPerPage {
		perPage = MaxPerPage
	}

	page := opts.Page
	if page <= 0 {
		page = 1
	}

	// Get default branch from repository info
	defaultBranch, err := g.getDefaultBranch(ctx, owner, repo)
	if err != nil {
		return nil, err
	}

	// Build URL
	url := fmt.Sprintf("%s/repos/%s/%s/branches?per_page=%d&page=%d",
		g.baseURL, owner, repo, perPage, page)

	// Make request
	req, err := g.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, err
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := g.checkResponse(resp); err != nil {
		return nil, err
	}

	// Parse response
	var ghBranches []githubBranch
	if err := json.NewDecoder(resp.Body).Decode(&ghBranches); err != nil {
		return nil, fmt.Errorf("failed to decode branches response: %w", err)
	}

	// Convert to our model
	branches := make([]Branch, len(ghBranches))
	for i, b := range ghBranches {
		branches[i] = Branch{
			Name:      b.Name,
			CommitSHA: b.Commit.SHA,
			IsDefault: b.Name == defaultBranch,
		}
	}

	// Check if there are more pages using Link header
	hasMore := g.hasNextPage(resp)

	return &ListBranchesResponse{
		Branches: branches,
		Page:     page,
		PerPage:  perPage,
		HasMore:  hasMore,
	}, nil
}

// getDefaultBranch fetches the repository's default branch name
func (g *GitHubProvider) getDefaultBranch(ctx context.Context, owner, repo string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", g.baseURL, owner, repo)

	req, err := g.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return "", err
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get repository info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := g.checkResponse(resp); err != nil {
		return "", err
	}

	var repoInfo struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&repoInfo); err != nil {
		return "", fmt.Errorf("failed to decode repository response: %w", err)
	}

	return repoInfo.DefaultBranch, nil
}

// ListCommits returns commits for a repository
// Reference: https://docs.github.com/en/rest/commits/commits
func (g *GitHubProvider) ListCommits(ctx context.Context, owner, repo string, opts ListCommitsOptions) (*ListCommitsResponse, error) {
	// Apply defaults
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = DefaultPerPage
	}
	if perPage > MaxPerPage {
		perPage = MaxPerPage
	}

	page := opts.Page
	if page <= 0 {
		page = 1
	}

	// Build URL
	url := fmt.Sprintf("%s/repos/%s/%s/commits?per_page=%d&page=%d",
		g.baseURL, owner, repo, perPage, page)

	if opts.SHA != "" {
		url += fmt.Sprintf("&sha=%s", opts.SHA)
	}
	if opts.Path != "" {
		url += fmt.Sprintf("&path=%s", opts.Path)
	}
	if opts.Author != "" {
		url += fmt.Sprintf("&author=%s", opts.Author)
	}
	if opts.Since != nil {
		url += fmt.Sprintf("&since=%s", opts.Since.Format(time.RFC3339))
	}
	if opts.Until != nil {
		url += fmt.Sprintf("&until=%s", opts.Until.Format(time.RFC3339))
	}

	// Make request
	req, err := g.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, err
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list commits: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := g.checkResponse(resp); err != nil {
		return nil, err
	}

	// Parse response
	var ghCommits []githubCommit
	if err := json.NewDecoder(resp.Body).Decode(&ghCommits); err != nil {
		return nil, fmt.Errorf("failed to decode commits response: %w", err)
	}

	// Convert to our model
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
			IsLatest:  i == 0 && page == 1, // First commit on first page is latest
		}
	}

	// Check if there are more pages using Link header
	hasMore := g.hasNextPage(resp)

	return &ListCommitsResponse{
		Commits: commits,
		Page:    page,
		PerPage: perPage,
		HasMore: hasMore,
	}, nil
}

// newRequest creates a new HTTP request with appropriate headers
func (g *GitHubProvider) newRequest(ctx context.Context, method, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set required headers
	// Reference: https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", GitHubAPIVersion)

	// Set authorization if token is provided
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}

	return req, nil
}

// checkResponse checks the response for errors
func (g *GitHubProvider) checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	var ghError githubErrorResponse
	if err := json.Unmarshal(body, &ghError); err == nil && ghError.Message != "" {
		return &GitHubError{
			StatusCode: resp.StatusCode,
			Message:    ghError.Message,
			Response:   string(body),
		}
	}

	return &GitHubError{
		StatusCode: resp.StatusCode,
		Message:    fmt.Sprintf("GitHub API error: %d", resp.StatusCode),
		Response:   string(body),
	}
}

// hasNextPage checks if there are more pages by parsing the Link header
// Reference: https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api#use-link-headers
func (g *GitHubProvider) hasNextPage(resp *http.Response) bool {
	linkHeader := resp.Header.Get("Link")
	if linkHeader == "" {
		return false
	}
	return strings.Contains(linkHeader, `rel="next"`)
}

// getRateLimitInfo extracts rate limit information from response headers
func (g *GitHubProvider) getRateLimitInfo(resp *http.Response) *RateLimitInfo {
	remaining := resp.Header.Get("X-RateLimit-Remaining")
	reset := resp.Header.Get("X-RateLimit-Reset")
	limit := resp.Header.Get("X-RateLimit-Limit")

	info := &RateLimitInfo{}
	if remaining != "" {
		if v, err := strconv.Atoi(remaining); err == nil {
			info.Remaining = v
		}
	}
	if reset != "" {
		if v, err := strconv.ParseInt(reset, 10, 64); err == nil {
			info.ResetAt = time.Unix(v, 0)
		}
	}
	if limit != "" {
		if v, err := strconv.Atoi(limit); err == nil {
			info.Limit = v
		}
	}
	return info
}

// RateLimitInfo contains rate limit information from GitHub API
type RateLimitInfo struct {
	Limit     int
	Remaining int
	ResetAt   time.Time
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

type githubErrorResponse struct {
	Message          string `json:"message"`
	DocumentationURL string `json:"documentation_url"`
}

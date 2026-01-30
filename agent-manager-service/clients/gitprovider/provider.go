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
	"time"
)

// ProviderType represents the type of git provider
type ProviderType string

const (
	ProviderGitHub ProviderType = "github"
)

// Provider defines the interface for git providers
type Provider interface {
	// ListBranches returns available branches for a repository
	ListBranches(ctx context.Context, owner, repo string, opts ListBranchesOptions) (*ListBranchesResponse, error)

	// ListCommits returns commits for a repository
	ListCommits(ctx context.Context, owner, repo string, opts ListCommitsOptions) (*ListCommitsResponse, error)

	// GetProviderType returns the provider type
	GetProviderType() ProviderType
}

// ListBranchesOptions contains options for listing branches
type ListBranchesOptions struct {
	// PerPage is the number of results per page (max 100)
	PerPage int
	// Page is the page number to fetch
	Page int
}

// ListCommitsOptions contains options for listing commits
type ListCommitsOptions struct {
	// SHA is the SHA or branch to start listing commits from
	SHA string
	// Path filters commits to those affecting the specified file path
	Path string
	// Author filters commits by author (GitHub username or email)
	Author string
	// Since filters commits after this date (ISO 8601 format)
	Since *time.Time
	// Until filters commits before this date (ISO 8601 format)
	Until *time.Time
	// PerPage is the number of results per page (max 100)
	PerPage int
	// Page is the page number to fetch
	Page int
}

// Branch represents a git branch
type Branch struct {
	Name      string `json:"name"`
	CommitSHA string `json:"commitSha"`
	IsDefault bool   `json:"isDefault"`
}

// Commit represents a git commit
type Commit struct {
	SHA       string    `json:"sha"`
	Message   string    `json:"message"`
	Author    Author    `json:"author"`
	Timestamp time.Time `json:"timestamp"`
	IsLatest  bool      `json:"isLatest"`
}

// Author represents a commit author
type Author struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatarUrl,omitempty"`
}

// ListBranchesResponse contains the response for listing branches
type ListBranchesResponse struct {
	Branches   []Branch `json:"branches"`
	TotalCount int      `json:"totalCount,omitempty"`
	Page       int      `json:"page"`
	PerPage    int      `json:"perPage"`
	HasMore    bool     `json:"hasMore"`
}

// ListCommitsResponse contains the response for listing commits
type ListCommitsResponse struct {
	Commits    []Commit `json:"commits"`
	TotalCount int      `json:"totalCount,omitempty"`
	Page       int      `json:"page"`
	PerPage    int      `json:"perPage"`
	HasMore    bool     `json:"hasMore"`
}

// Config holds provider authentication details
type Config struct {
	// Token is the personal access token for authentication
	Token string
}

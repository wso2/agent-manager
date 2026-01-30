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
	"fmt"
	"net/url"
	"strings"
)

// NewProvider creates the appropriate git provider based on the provider type
func NewProvider(providerType ProviderType, cfg Config) (Provider, error) {
	switch providerType {
	case ProviderGitHub:
		return NewGitHubProvider(cfg)
	default:
		return nil, fmt.Errorf("unsupported git provider: %s", providerType)
	}
}

// NewProviderFromURL creates the appropriate git provider based on the repository URL
func NewProviderFromURL(repoURL string, cfg Config) (Provider, error) {
	providerType, err := DetectProvider(repoURL)
	if err != nil {
		return nil, err
	}
	return NewProvider(providerType, cfg)
}

// DetectProvider determines the provider type from a repository URL
func DetectProvider(repoURL string) (ProviderType, error) {
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return "", fmt.Errorf("invalid repository URL: %w", err)
	}

	host := strings.ToLower(parsed.Host)

	switch {
	case strings.Contains(host, "github.com"):
		return ProviderGitHub, nil
	default:
		return "", fmt.Errorf("unknown git provider for host: %s", host)
	}
}

// ParseRepoURL extracts owner and repo name from a repository URL
// Supports formats:
//   - https://github.com/owner/repo
//   - https://github.com/owner/repo.git
//   - git@github.com:owner/repo.git
func ParseRepoURL(repoURL string) (owner, repo string, err error) {
	// Handle SSH format: git@github.com:owner/repo.git
	if strings.HasPrefix(repoURL, "git@") {
		parts := strings.SplitN(repoURL, ":", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid SSH repository URL format: %s", repoURL)
		}
		path := strings.TrimSuffix(parts[1], ".git")
		pathParts := strings.Split(path, "/")
		if len(pathParts) < 2 {
			return "", "", fmt.Errorf("invalid repository path: %s", path)
		}
		return pathParts[0], pathParts[1], nil
	}

	// Handle HTTPS format
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid repository URL: %w", err)
	}

	path := strings.TrimPrefix(parsed.Path, "/")
	path = strings.TrimSuffix(path, ".git")

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid repository path: %s", path)
	}

	return parts[0], parts[1], nil
}

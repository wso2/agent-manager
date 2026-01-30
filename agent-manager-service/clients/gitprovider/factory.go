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
	// Handle SSH format: git@github.com:owner/repo.git
	if strings.HasPrefix(repoURL, "git@") {
		hostPart := strings.TrimPrefix(repoURL, "git@")
		if idx := strings.Index(hostPart, ":"); idx > 0 {
			host := strings.ToLower(hostPart[:idx])
			if strings.Contains(host, "github.com") {
				return ProviderGitHub, nil
			}
			return "", fmt.Errorf("unknown git provider for host: %s", host)
		}
		return "", fmt.Errorf("invalid SSH repository URL format: %s", repoURL)
	}

	// Handle HTTPS format
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return "", fmt.Errorf("invalid repository URL: %w", err)
	}

	host := strings.ToLower(parsed.Host)
	if host == "" {
		return "", fmt.Errorf("invalid repository URL: no host found in %s", repoURL)
	}

	switch {
	case strings.Contains(host, "github.com"):
		return ProviderGitHub, nil
	default:
		return "", fmt.Errorf("unknown git provider for host: %s", host)
	}
}

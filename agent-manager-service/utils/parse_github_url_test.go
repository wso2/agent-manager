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

package utils

import "testing"

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		url, owner, repo string
	}{
		{"https://github.com/owner/repo", "owner", "repo"},
		{"https://github.com/owner/repo.git", "owner", "repo"},
		{"http://github.com/owner/repo/", "owner", "repo"},
		{"git@github.com:owner/repo.git", "owner", "repo"},
		{"HTTPS://GITHUB.COM/Owner/Repo", "Owner", "Repo"},
		{"Https://GitHub.com/Owner/my.repo.git", "Owner", "my.repo"},
		{"git@GitHub.com:owner/repo.git", "owner", "repo"},
		{"ssh://git@github.com/owner/repo.git", "owner", "repo"},
		{"ssh://git@github.com/owner/repo", "owner", "repo"},
		{"SSH://git@GITHUB.COM/Owner/Repo.git", "Owner", "Repo"},
		{"", "", ""},
		{"https://gitlab.com/owner/repo", "", ""},
		{"https://github.com.evil.com/owner/repo", "", ""},
		{"ssh://git@github.com:22/owner/repo.git", "", ""},
		{"https://github.com/owner", "", ""},
		{"https://github.com/owner/repo/tree/main", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			owner, repo := ParseGitHubURL(tt.url)
			if owner != tt.owner || repo != tt.repo {
				t.Errorf("ParseGitHubURL(%q) = (%q, %q), want (%q, %q)", tt.url, owner, repo, tt.owner, tt.repo)
			}
		})
	}
}

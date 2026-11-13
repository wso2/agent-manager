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

// UNIT tests for the repository and build inputs that reach the build workflow.
// No `//go:build integration` tag, so these run in the fast unit tier
// (`make test-unit`) and must NOT touch the network.
//
// These inputs become arguments to git and to the container build running inside
// the build pod. The pod holds the git credential secret and the registry push
// secret, so the character sets below are deliberately narrower than what git or
// GitHub would accept on their own: the build templates deliver every value as an
// environment variable so the shell never parses it, and these checks are the
// second half of that, keeping out values that are hostile to git's own option
// parser or simply cannot be a real repository.

package utils

import (
	"strings"
	"testing"
)

// TestIsValidGitHubBranch pins both halves of the check: the character allowlist,
// and the git refname rules a character class cannot express.
func TestIsValidGitHubBranch(t *testing.T) {
	tests := []struct {
		name   string
		branch string
		want   bool
	}{
		{"simple", "main", true},
		{"namespaced", "feature/add-thing", true},
		{"version-like", "release/1.2.3", true},
		{"underscores", "my_branch", true},
		{"at sign is valid in a ref", "user@work", true},
		{"plus is valid in a ref", "a+b", true},

		{"empty", "", false},
		{"command separator", "main;id", false},
		{"pipe", "main|id", false},
		{"ampersand", "main&id", false},
		{"dollar", "main$X", false},
		{"backtick", "main`id`", false},
		{"parentheses", "main(id)", false},
		{"space", "my branch", false},
		{"tab", "my\tbranch", false},
		{"newline", "main\nid", false},
		{"null byte", "main\x00", false},
		{"tilde", "main~1", false},
		{"caret", "main^", false},
		{"colon", "origin:main", false},
		{"question mark", "main?", false},
		{"asterisk", "main*", false},
		{"open bracket", "main[1]", false},
		{"backslash", "main\\x", false},
		{"leading dash", "-main", false},
		{"leading slash", "/main", false},
		{"trailing slash", "main/", false},
		{"double slash", "a//b", false},
		{"traversal", "a/../b", false},
		{"reflog syntax", "main@{1}", false},
		{"trailing dot", "main.", false},
		{"lock suffix", "main.lock", false},
		{"over-long", strings.Repeat("a", MaxGitBranchLength+1), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidGitHubBranch(tt.branch); got != tt.want {
				t.Errorf("isValidGitHubBranch(%q) = %v, want %v", tt.branch, got, tt.want)
			}
		})
	}
}

// TestValidateGitCommitID pins the accepted abbreviation range at both ends, an
// empty value meaning "latest on the branch", and rejection of anything non-hex.
func TestValidateGitCommitID(t *testing.T) {
	tests := []struct {
		name    string
		commit  string
		wantErr bool
	}{
		// Empty means "latest commit on the branch".
		{"empty", "", false},
		{"abbreviated", "ee4f70e4", false},
		{"rev-parse --short length", "ee4f70e", false},
		// git's own minimum abbreviation, and what existing callers may send.
		{"shortest accepted", "abc1", false},
		{"six characters", "abc123", false},
		{"full sha", "ee4f70e4cba1234567890abcdef1234567890abc", false},
		{"upper case", "EE4F70E4", false},

		{"too short", "abc", true},
		{"too long", strings.Repeat("a", 41), true},
		{"non-hexadecimal", "zzzzzzz", true},
		// The first eight characters of the commit also become part of the image
		// tag, so a separator here would reach a second script as well.
		{"command separator", "ee4f70e4;id", true},
		{"command substitution", "$(id)abc", true},
		{"leading dash", "-ee4f70e4", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGitCommitID(tt.commit)
			if tt.wantErr && err == nil {
				t.Fatalf("expected rejection of commitId %q", tt.commit)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected rejection of commitId %q: %v", tt.commit, err)
			}
			if err != nil && IsValidationError(err) == nil {
				t.Errorf("error should be a *ValidationError so it maps to 400, got %T: %v", err, err)
			}
		})
	}
}

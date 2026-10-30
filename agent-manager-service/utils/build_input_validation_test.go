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

	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// repoConfigForTest builds the minimal RepositoryConfig the validator reads:
// secretRef is left unset, which means a public repository.
func repoConfigForTest(url, branch, appPath string) *spec.RepositoryConfig {
	return &spec.RepositoryConfig{
		Url:     url,
		Branch:  branch,
		AppPath: appPath,
	}
}

// TestValidateRepoDetails covers the repository fields together, since the
// validator short-circuits: a case has to keep the other two fields valid for its
// own field to be the one under test.
func TestValidateRepoDetails(t *testing.T) {
	const (
		okURL     = "https://github.com/wso2/agent-manager"
		okBranch  = "main"
		okAppPath = "/samples/customer-support-agent"
	)

	tests := []struct {
		name    string
		url     string
		branch  string
		appPath string
		wantErr bool
	}{
		// Accepted: the shapes real repositories take.
		{"plain repository", okURL, okBranch, okAppPath, false},
		{"repository root as app path", okURL, okBranch, "/", false},
		{".git suffix", "https://github.com/wso2/agent-manager.git", okBranch, okAppPath, false},
		{"trailing slash", "https://github.com/wso2/agent-manager/", okBranch, okAppPath, false},
		{"dots in the repository name", "https://github.com/wso2/my.repo", okBranch, okAppPath, false},
		{"underscores and hyphens", "https://github.com/my-org/my_repo", okBranch, okAppPath, false},
		{"branch with a slash", okURL, "feature/add-thing", okAppPath, false},
		{"branch with dots", okURL, "release/1.2.3", okAppPath, false},

		// A ";" needs no quote to break out of a shell assignment, so it is the
		// cheapest way in and the first thing to keep out.
		{
			"command separator in the URL",
			`https://github.com/wso2/agent-manager;echo injected;`,
			okBranch, okAppPath, true,
		},
		{
			"command separator in the branch",
			okURL, `main;echo injected;`, okAppPath, true,
		},

		// The same class through the other metacharacters.
		{"command substitution in the URL", "https://github.com/wso2/agent-$(id)", okBranch, okAppPath, true},
		{"backtick in the repository name", "https://github.com/wso2/agent-`id`", okBranch, okAppPath, true},
		{"double quote in the branch", okURL, `main";id;"`, okAppPath, true},
		{"single quote in the branch", okURL, "main'id'", okAppPath, true},
		{"newline in the branch", okURL, "main\nid", okAppPath, true},
		{"double quote in the app path", okURL, okBranch, `/x";echo pwned;"`, true},
		{"command substitution in the app path", okURL, okBranch, "/$(id)", true},

		// git's option parser, not the shell.
		{"branch opening with a dash", okURL, "--upload-pack=id", okAppPath, true},

		// Traversal and shape.
		{"traversal in the app path", okURL, okBranch, "/../../etc", true},
		{"relative app path", okURL, okBranch, "samples/agent", true},
		{"traversal in the branch", okURL, "feature/../main", okAppPath, true},
		{"empty branch", okURL, "", okAppPath, true},
		{"empty app path", okURL, okBranch, "", true},
		{"empty URL", "", okBranch, okAppPath, true},
		// Non-GitHub HTTPS Git hosts: now accepted.
		{"gitlab.com owner/repo", "https://gitlab.com/myorg/myrepo", okBranch, okAppPath, false},
		{"gitlab.com with group path", "https://gitlab.com/myorg/mygroup/myrepo", okBranch, okAppPath, false},
		{"gitlab.com with deep group path", "https://gitlab.com/myorg/group-a/group-b/myrepo", okBranch, okAppPath, false},
		{"self-hosted git", "https://git.example.com/myorg/myrepo", okBranch, okAppPath, false},
		{"self-hosted git deep path", "https://git.example.com/myorg/mygroup/myrepo", okBranch, okAppPath, false},
		{"bitbucket.org", "https://bitbucket.org/myorg/myrepo", okBranch, okAppPath, false},
		// Self-hosted GitLab: the gitlab.<domain> subdomain pattern is the most common
		// naming convention for self-hosted instances and is the shape validated in
		// production. A group-nested path is also expected and must be accepted.
		{"self-hosted gitlab hostname", "https://gitlab.example.com/myorg/myrepo", okBranch, okAppPath, false},
		{"self-hosted gitlab with group path", "https://gitlab.example.com/myorg/group/myrepo", okBranch, okAppPath, false},
		{"self-hosted gitlab .git suffix", "https://gitlab.example.com/myorg/myrepo.git", okBranch, okAppPath, false},
		// The non-GitHub path must be equally hostile to injection; these exercise
		// the gitRepoURLRe path-segment allowlist through a self-hosted host.
		{
			"command separator in non-GitHub URL path",
			"https://gitlab.example.com/myorg/myrepo;echo injected",
			okBranch, okAppPath, true,
		},
		{
			"path traversal in non-GitHub URL",
			"https://gitlab.example.com/myorg/../etc/myrepo",
			okBranch, okAppPath, true,
		},
		// Non-HTTPS and non-domain URLs must still be rejected.
		{"plain http", "http://github.com/wso2/agent-manager", okBranch, okAppPath, true},
		{"localhost rejected", "https://localhost/myorg/myrepo", okBranch, okAppPath, true},
		{"IP address rejected", "https://192.168.1.1/myorg/myrepo", okBranch, okAppPath, true},
		{"bare hostname rejected", "https://internal/myorg/myrepo", okBranch, okAppPath, true},
		// Lone dot in path segment must be rejected (isValidGitHubIdentifier regression).
		{"lone dot segment rejected", "https://gitlab.example.com/./myrepo", okBranch, okAppPath, true},
		{"lone dot group segment rejected", "https://gitlab.example.com/myorg/./myrepo", okBranch, okAppPath, true},
		// Cluster-internal hostnames must be rejected as a defence-in-depth measure.
		{"svc.cluster.local rejected", "https://openbao.openbao.svc.cluster.local/x/y", okBranch, okAppPath, true},
		{"cluster.local rejected", "https://myservice.cluster.local/group/repo", okBranch, okAppPath, true},
		{"dot-local rejected", "https://myserver.local/group/repo", okBranch, okAppPath, true},
		{"dot-internal rejected", "https://myserver.internal/group/repo", okBranch, okAppPath, true},
		// nip.io-style DNS-rebinding names are NOT blocked by format validation alone;
		// egress policy on the build pod is the primary guard for those.
		{"over-long URL", okURL + strings.Repeat("a", MaxRepositoryURLLength), okBranch, okAppPath, true},
		{"over-long branch", okURL, strings.Repeat("b", MaxGitBranchLength+1), okAppPath, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRepoDetails(repoConfigForTest(tt.url, tt.branch, tt.appPath))
			if tt.wantErr && err == nil {
				t.Fatalf("expected rejection of url=%q branch=%q appPath=%q", tt.url, tt.branch, tt.appPath)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected rejection of url=%q branch=%q appPath=%q: %v",
					tt.url, tt.branch, tt.appPath, err)
			}
			// A rejection has to reach the caller as a 400, not a 500.
			if err != nil && IsValidationError(err) == nil {
				t.Errorf("error should be a *ValidationError so it maps to 400, got %T: %v", err, err)
			}
		})
	}
}

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

// TestValidateBuildConfigurationDockerfilePath exercises the shared path rules
// through the docker build branch, which is the other caller of validateSafePath.
func TestValidateBuildConfigurationDockerfilePath(t *testing.T) {
	tests := []struct {
		name           string
		dockerfilePath string
		wantErr        bool
	}{
		{"repository root", "/Dockerfile", false},
		{"nested", "/docker/Dockerfile", false},
		{"dotted name", "/build/Dockerfile.prod", false},

		{"empty", "", true},
		{"relative", "Dockerfile", true},
		{"traversal", "/../Dockerfile", true},
		{"double quote", `/x";echo pwned;"`, true},
		{"command substitution", "/$(id)/Dockerfile", true},
		{"command separator", "/Dockerfile;id", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			build := spec.DockerBuildAsBuild(&spec.DockerBuild{
				Docker: spec.DockerConfig{DockerfilePath: tt.dockerfilePath},
			})
			err := validateBuildConfiguration(&build)
			if tt.wantErr && err == nil {
				t.Fatalf("expected rejection of dockerfilePath %q", tt.dockerfilePath)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected rejection of dockerfilePath %q: %v", tt.dockerfilePath, err)
			}
		})
	}
}

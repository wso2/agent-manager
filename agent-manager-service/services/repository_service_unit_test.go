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

// UNIT tests for repositoryService. No `//go:build integration` tag, so these
// run in the fast unit tier (`make test-unit`) and must NOT touch the network.
//
// repositoryService's public methods (ListBranches/ListCommits/GetLatestCommit)
// ultimately call a real git provider (HTTP to GitHub). We therefore cannot
// drive their happy paths in a unit test. What we CAN exercise without any
// network is everything that runs BEFORE the provider call:
//
//   - getGitProviderConfigWithCredentials: a pure helper (nil/invalid/valid creds).
//   - credential resolution: when the request carries a secretRef + ouID the
//     service calls GitCredentialsService.GetGitCredentials; we assert that a
//     fetch error and an invalid-credentials result are propagated verbatim,
//     before any provider/network work happens.
//   - provider construction: an unsupported ProviderType makes NewProvider fail
//     locally (no network), so we can assert that error surfaces.
//
// GitCredentialsService has no generated mock, so we hand-write a func-field stub
// (gitCredsStub) following the same pattern the generated moq mocks use: an
// unset func field would panic, making an unexpected code path fail loudly.
//
// Parts that genuinely need a live git remote (the successful branch/commit
// listing and SHA transformation, and GetLatestCommit end-to-end) are left to
// integration-level tests and are noted inline below.
package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/gitprovider"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// gitCredsStub is a hand-written stub for the in-package GitCredentialsService
// interface (no moq mock exists for it). A nil GetGitCredentialsFunc panics, so
// a test that reaches this method unexpectedly fails loudly.
type gitCredsStub struct {
	GetGitCredentialsFunc func(ctx context.Context, ouID, secretRef string) (*GitCredentials, error)
}

type commitProviderStub struct {
	ListCommitsFunc func(ctx context.Context, projectName, componentName string, opts gitprovider.ListCommitsOptions) (*gitprovider.ListCommitsResponse, bool, error)
}

func (s *commitProviderStub) ListCommits(ctx context.Context, projectName, componentName string, opts gitprovider.ListCommitsOptions) (*gitprovider.ListCommitsResponse, bool, error) {
	return s.ListCommitsFunc(ctx, projectName, componentName, opts)
}

func (s *gitCredsStub) GetGitCredentials(ctx context.Context, ouID, secretRef string) (*GitCredentials, error) {
	return s.GetGitCredentialsFunc(ctx, ouID, secretRef)
}

// newRepoService wires repositoryService with the credential stub and a discard
// logger (discardLogger lives in evaluator_manager_unit_test.go in this package).
func newRepoService(creds GitCredentialsService) RepositoryService {
	return NewRepositoryService(creds, discardLogger())
}

// branchReq builds a ListBranchesRequest carrying a secretRef, which together
// with a non-empty org triggers the credential-resolution branch. The org now
// comes from the caller's token, so it is passed to the service separately.
func branchReq(secretRef string) spec.ListBranchesRequest {
	return spec.ListBranchesRequest{
		Owner:      "acme",
		Repository: "widgets",
		SecretRef:  strPtr(secretRef),
	}
}

// commitReq mirrors branchReq for ListCommits.
func commitReq(secretRef string) spec.ListCommitsRequest {
	return spec.ListCommitsRequest{
		Owner:     "acme",
		Repo:      "widgets",
		SecretRef: strPtr(secretRef),
	}
}

// -----------------------------------------------------------------------------
// getGitProviderConfigWithCredentials — pure helper, no dependencies. Covers the
// validation gate that decides whether supplied credentials are usable.
// -----------------------------------------------------------------------------

func TestRepositoryService_getGitProviderConfigWithCredentials(t *testing.T) {
	t.Run("nil credentials -> ErrGitSecretInvalidType", func(t *testing.T) {
		_, err := getGitProviderConfigWithCredentials(nil)
		assert.ErrorIs(t, err, utils.ErrGitSecretInvalidType)
	})

	t.Run("non basic-auth type -> ErrGitSecretInvalidType", func(t *testing.T) {
		_, err := getGitProviderConfigWithCredentials(&GitCredentials{Type: "ssh", Password: "pw"})
		assert.ErrorIs(t, err, utils.ErrGitSecretInvalidType)
	})

	t.Run("basic-auth with empty password -> ErrGitSecretInvalidType", func(t *testing.T) {
		_, err := getGitProviderConfigWithCredentials(&GitCredentials{Type: "basic-auth", Password: ""})
		assert.ErrorIs(t, err, utils.ErrGitSecretInvalidType)
	})

	t.Run("valid basic-auth -> token wired into provider config", func(t *testing.T) {
		cfg, err := getGitProviderConfigWithCredentials(&GitCredentials{Type: "basic-auth", Password: "ghp_secret"})
		require.NoError(t, err)
		assert.Equal(t, "ghp_secret", cfg.Token)
	})
}

// -----------------------------------------------------------------------------
// ListBranches — pre-network branches: credential fetch failure, invalid creds,
// and unsupported provider type. The successful listing path requires a live
// git remote and is covered by integration tests.
// -----------------------------------------------------------------------------

func TestRepositoryService_ListBranches(t *testing.T) {
	t.Run("propagates credential-fetch error", func(t *testing.T) {
		boom := errors.New("openbao unreachable")
		creds := &gitCredsStub{
			GetGitCredentialsFunc: func(_ context.Context, _, _ string) (*GitCredentials, error) {
				return nil, boom
			},
		}
		svc := newRepoService(creds)

		_, err := svc.ListBranches(context.Background(), branchReq("git-secret"), "acme", gitprovider.ProviderGitHub, 10, 0)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		// A real fetch error must not be masked as "not found".
		assert.NotErrorIs(t, err, gitprovider.ErrNotFound)
	})

	t.Run("propagates invalid-credentials error before any provider work", func(t *testing.T) {
		creds := &gitCredsStub{
			GetGitCredentialsFunc: func(_ context.Context, _, _ string) (*GitCredentials, error) {
				// Wrong type makes getGitProviderConfigWithCredentials reject it.
				return &GitCredentials{Type: "ssh"}, nil
			},
		}
		svc := newRepoService(creds)

		_, err := svc.ListBranches(context.Background(), branchReq("git-secret"), "acme", gitprovider.ProviderGitHub, 10, 0)

		assert.ErrorIs(t, err, utils.ErrGitSecretInvalidType)
		assert.NotErrorIs(t, err, gitprovider.ErrNotFound)
	})

	t.Run("secretRef without an org identity is rejected, not silently downgraded", func(t *testing.T) {
		// Stub func left nil: a missing org must fail before any credential fetch,
		// rather than falling back to the platform's default (public-repo) config.
		svc := newRepoService(&gitCredsStub{})

		_, err := svc.ListBranches(context.Background(), branchReq("git-secret"), "", gitprovider.ProviderGitHub, 10, 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "organization identity is required")
	})

	t.Run("unsupported provider type fails locally without network", func(t *testing.T) {
		// No secretRef/ouID => credential branch is skipped (stub func left nil
		// to assert it is never called). NewProvider rejects the unknown type
		// before any HTTP call is made.
		svc := newRepoService(&gitCredsStub{})

		req := spec.ListBranchesRequest{Owner: "acme", Repository: "widgets"}
		_, err := svc.ListBranches(context.Background(), req, "acme", gitprovider.ProviderType("bitbucket"), 10, 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported git provider")
	})
}

// -----------------------------------------------------------------------------
// ListCommits — deployment-provider selection and mapping, followed by the same
// pre-network credential/provider branches as ListBranches.
// -----------------------------------------------------------------------------

func TestRepositoryService_ListCommits(t *testing.T) {
	t.Run("uses deployment commit provider for a bound component", func(t *testing.T) {
		timestamp := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
		provider := &commitProviderStub{
			ListCommitsFunc: func(_ context.Context, projectName, componentName string, opts gitprovider.ListCommitsOptions) (*gitprovider.ListCommitsResponse, bool, error) {
				assert.Equal(t, "default", projectName)
				assert.Equal(t, "private-agent", componentName)
				assert.Equal(t, "main", opts.SHA)
				assert.Equal(t, 10, opts.PerPage)
				assert.Equal(t, 2, opts.Page)
				return &gitprovider.ListCommitsResponse{
					Commits: []gitprovider.Commit{{
						SHA:       "0123456789abcdef",
						Message:   "private commit",
						Timestamp: timestamp,
						IsLatest:  true,
						Author: gitprovider.Author{
							Name:      "Octo Cat",
							Email:     "octo@example.com",
							AvatarURL: "https://example.com/avatar.png",
						},
					}},
				}, true, nil
			},
		}
		svc := newRepoService(&gitCredsStub{})
		svc.SetCommitProvider(provider)
		req := spec.ListCommitsRequest{
			Owner:         "acme",
			Repo:          "private-repo",
			Branch:        strPtr("main"),
			ProjectName:   strPtr("default"),
			ComponentName: strPtr("private-agent"),
		}

		response, err := svc.ListCommits(context.Background(), req, "acme", gitprovider.ProviderGitHub, 10, 10)

		require.NoError(t, err)
		require.Len(t, response.Commits, 1)
		assert.Equal(t, "0123456", response.Commits[0].ShortSha)
		assert.Equal(t, "private commit", response.Commits[0].Message)
		assert.Equal(t, timestamp, response.Commits[0].Timestamp)
		assert.Equal(t, "https://example.com/avatar.png", response.Commits[0].Author.GetAvatarUrl())
	})

	t.Run("falls back to the standard provider when component has no binding", func(t *testing.T) {
		provider := &commitProviderStub{
			ListCommitsFunc: func(_ context.Context, _, _ string, _ gitprovider.ListCommitsOptions) (*gitprovider.ListCommitsResponse, bool, error) {
				return nil, false, nil
			},
		}
		svc := newRepoService(&gitCredsStub{})
		svc.SetCommitProvider(provider)
		req := spec.ListCommitsRequest{
			Owner:         "acme",
			Repo:          "public-repo",
			ProjectName:   strPtr("default"),
			ComponentName: strPtr("public-agent"),
		}

		_, err := svc.ListCommits(context.Background(), req, "acme", gitprovider.ProviderType("gitlab"), 10, 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported git provider")
	})

	t.Run("propagates deployment commit provider errors", func(t *testing.T) {
		boom := errors.New("git app unavailable")
		provider := &commitProviderStub{
			ListCommitsFunc: func(_ context.Context, _, _ string, _ gitprovider.ListCommitsOptions) (*gitprovider.ListCommitsResponse, bool, error) {
				return nil, false, boom
			},
		}
		svc := newRepoService(&gitCredsStub{})
		svc.SetCommitProvider(provider)
		req := spec.ListCommitsRequest{
			Owner:         "acme",
			Repo:          "private-repo",
			ProjectName:   strPtr("default"),
			ComponentName: strPtr("private-agent"),
		}

		_, err := svc.ListCommits(context.Background(), req, "acme", gitprovider.ProviderGitHub, 10, 0)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		assert.Contains(t, err.Error(), "component repository binding")
	})

	t.Run("propagates credential-fetch error", func(t *testing.T) {
		boom := errors.New("openbao unreachable")
		creds := &gitCredsStub{
			GetGitCredentialsFunc: func(_ context.Context, _, _ string) (*GitCredentials, error) {
				return nil, boom
			},
		}
		svc := newRepoService(creds)

		_, err := svc.ListCommits(context.Background(), commitReq("git-secret"), "acme", gitprovider.ProviderGitHub, 10, 0)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		// A real fetch error must not be masked as "not found".
		assert.NotErrorIs(t, err, gitprovider.ErrNotFound)
	})

	t.Run("propagates invalid-credentials error before any provider work", func(t *testing.T) {
		creds := &gitCredsStub{
			GetGitCredentialsFunc: func(_ context.Context, _, _ string) (*GitCredentials, error) {
				return &GitCredentials{Type: "basic-auth", Password: ""}, nil
			},
		}
		svc := newRepoService(creds)

		_, err := svc.ListCommits(context.Background(), commitReq("git-secret"), "acme", gitprovider.ProviderGitHub, 10, 0)

		assert.ErrorIs(t, err, utils.ErrGitSecretInvalidType)
		assert.NotErrorIs(t, err, gitprovider.ErrNotFound)
	})

	t.Run("secretRef without an org identity is rejected, not silently downgraded", func(t *testing.T) {
		svc := newRepoService(&gitCredsStub{})

		_, err := svc.ListCommits(context.Background(), commitReq("git-secret"), "", gitprovider.ProviderGitHub, 10, 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "organization identity is required")
	})

	t.Run("unsupported provider type fails locally without network", func(t *testing.T) {
		svc := newRepoService(&gitCredsStub{})

		req := spec.ListCommitsRequest{Owner: "acme", Repo: "widgets"}
		_, err := svc.ListCommits(context.Background(), req, "acme", gitprovider.ProviderType("gitlab"), 10, 0)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported git provider")
	})
}

// NOTE: GetLatestCommit is intentionally not unit-tested. It hard-codes
// gitprovider.ProviderGitHub and immediately calls provider.ListCommits, which
// performs a real HTTP request to GitHub — there is no pre-network branch to
// exercise without a live remote (or an injectable provider). Its behaviour
// (latest-SHA extraction and the empty-result -> gitprovider.ErrNotFound mapping)
// belongs in integration tests.

// pagedCommitSource serves numbered commits c1..cTotal in pages of perPage, the way a
// page-based provider does, and records which pages were requested.
func pagedCommitSource(total int) (*commitProviderStub, *[]int) {
	var pages []int
	return &commitProviderStub{
		ListCommitsFunc: func(_ context.Context, _, _ string, opts gitprovider.ListCommitsOptions) (*gitprovider.ListCommitsResponse, bool, error) {
			pages = append(pages, opts.Page)
			start := (opts.Page - 1) * opts.PerPage
			end := min(start+opts.PerPage, total)
			var commits []gitprovider.Commit
			for i := start; i < end; i++ {
				commits = append(commits, gitprovider.Commit{SHA: fmt.Sprintf("c%d", i+1)})
			}
			return &gitprovider.ListCommitsResponse{Commits: commits, HasMore: end < total}, true, nil
		},
	}, &pages
}

func commitSHAs(response *spec.ListCommitsResponse) []string {
	shas := make([]string, 0, len(response.Commits))
	for _, c := range response.Commits {
		shas = append(shas, c.Sha)
	}
	return shas
}

func componentCommitReq() spec.ListCommitsRequest {
	return spec.ListCommitsRequest{
		Owner:         "acme",
		Repo:          "private-repo",
		ProjectName:   strPtr("default"),
		ComponentName: strPtr("private-agent"),
	}
}

func TestRepositoryService_ListCommitsProviderHonoursUnalignedOffset(t *testing.T) {
	t.Run("a window spanning two pages fetches both and slices it", func(t *testing.T) {
		provider, pages := pagedCommitSource(25)
		svc := newRepoService(&gitCredsStub{})
		svc.SetCommitProvider(provider)

		response, err := svc.ListCommits(context.Background(), componentCommitReq(), "acme", gitprovider.ProviderGitHub, 10, 5)

		require.NoError(t, err)
		assert.Equal(t, []int{1, 2}, *pages)
		assert.Equal(t, []string{"c6", "c7", "c8", "c9", "c10", "c11", "c12", "c13", "c14", "c15"}, commitSHAs(response))
		assert.Equal(t, int32(5), response.Offset)
		require.NotNil(t, response.NextOffset)
		assert.Equal(t, int32(15), *response.NextOffset)
	})

	t.Run("a window starting in the last page fetches it once and reports no more", func(t *testing.T) {
		provider, pages := pagedCommitSource(8)
		svc := newRepoService(&gitCredsStub{})
		svc.SetCommitProvider(provider)

		response, err := svc.ListCommits(context.Background(), componentCommitReq(), "acme", gitprovider.ProviderGitHub, 10, 5)

		require.NoError(t, err)
		assert.Equal(t, []int{1}, *pages)
		assert.Equal(t, []string{"c6", "c7", "c8"}, commitSHAs(response))
		assert.Nil(t, response.NextOffset)
	})

	t.Run("an aligned offset still fetches a single page", func(t *testing.T) {
		provider, pages := pagedCommitSource(25)
		svc := newRepoService(&gitCredsStub{})
		svc.SetCommitProvider(provider)

		response, err := svc.ListCommits(context.Background(), componentCommitReq(), "acme", gitprovider.ProviderGitHub, 10, 20)

		require.NoError(t, err)
		assert.Equal(t, []int{3}, *pages)
		assert.Equal(t, []string{"c21", "c22", "c23", "c24", "c25"}, commitSHAs(response))
		assert.Nil(t, response.NextOffset)
	})
}

func TestListCommitsWindow(t *testing.T) {
	t.Run("an error on the second page is returned", func(t *testing.T) {
		boom := errors.New("boom")
		_, _, err := listCommitsWindow(10, 5, func(page int) (*gitprovider.ListCommitsResponse, bool, error) {
			if page == 2 {
				return nil, false, boom
			}
			return &gitprovider.ListCommitsResponse{
				Commits: make([]gitprovider.Commit, 10),
				HasMore: true,
			}, true, nil
		})
		assert.ErrorIs(t, err, boom)
	})

	t.Run("an unbound component is passed through as unhandled", func(t *testing.T) {
		calls := 0
		result, handled, err := listCommitsWindow(10, 5, func(int) (*gitprovider.ListCommitsResponse, bool, error) {
			calls++
			return nil, false, nil
		})
		require.NoError(t, err)
		assert.False(t, handled)
		assert.Nil(t, result)
		assert.Equal(t, 1, calls)
	})

	t.Run("a non-positive limit is rejected rather than dividing by zero", func(t *testing.T) {
		_, _, err := listCommitsWindow(0, 0, func(int) (*gitprovider.ListCommitsResponse, bool, error) {
			t.Fatal("must not fetch")
			return nil, false, nil
		})
		assert.Error(t, err)
	})
}

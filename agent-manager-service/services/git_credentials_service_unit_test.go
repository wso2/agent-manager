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

// Unit tests for gitCredentialsService. Like agent_kind_service_unit_test.go
// (the reference), this file carries NO `//go:build integration` tag, so it runs
// in the fast unit tier with its collaborators faked.
//
// gitCredentialsService.GetGitCredentials checks that OpenChoreo still lists
// the git secret (ocClient.ListGitSecrets), derives its workflow-plane OpenBao
// path from the namespace (ocClient.NamespaceFor) and the secret name, then
// reads it through kvReader. The tests fake kvReader to check the path and the
// parsing, and leave the OpenChoreo mock's GetSecretReferenceFunc nil so that
// reading a SecretReference panics.
package services

import (
	"context"
	"errors"
	"strings"
	"testing"

	vault "github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// gitTestConfig returns a minimal config that lets NewGitCredentialsService succeed.
// The service only reads cfg.WorkflowPlaneOpenBao.{URL,Token}.
func gitTestConfig() config.Config {
	return config.Config{
		WorkflowPlaneOpenBao: config.OpenBaoConfig{
			URL:   "http://localhost:8200",
			Token: "test-token",
		},
	}
}

// fakeKVReader records the paths it is asked to read and returns a canned result.
type fakeKVReader struct {
	secret *vault.Secret
	err    error
	paths  []string
}

func (f *fakeKVReader) ReadWithContext(_ context.Context, path string) (*vault.Secret, error) {
	f.paths = append(f.paths, path)
	return f.secret, f.err
}

// kvSecret wraps data the way a KV v2 read returns it.
func kvSecret(data map[string]interface{}) *vault.Secret {
	return &vault.Secret{Data: map[string]interface{}{"data": data}}
}

const (
	gitTestOrg       = "acme"
	gitTestNamespace = "acme-ns"
)

// newGitCredsServiceWithKV builds the service with the given KV reader and an
// OpenChoreo mock that resolves gitTestOrg to gitTestNamespace and lists the
// given git secrets for it. Any other org fails the test, so both calls must
// receive the caller's org.
func newGitCredsServiceWithKV(t *testing.T, kv kvReader, gitSecrets ...string) *gitCredentialsService {
	t.Helper()
	return &gitCredentialsService{
		ocClient: &clientmocks.OpenChoreoClientMock{
			NamespaceForFunc: func(ouID string) string {
				assert.Equal(t, gitTestOrg, ouID)
				return gitTestNamespace
			},
			ListGitSecretsFunc: func(_ context.Context, ouID string) ([]*occlient.GitSecretInfo, error) {
				assert.Equal(t, gitTestOrg, ouID)
				infos := make([]*occlient.GitSecretInfo, 0, len(gitSecrets))
				for _, name := range gitSecrets {
					infos = append(infos, &occlient.GitSecretInfo{Name: name, Namespace: gitTestNamespace})
				}
				return infos, nil
			},
		},
		kv: kv,
	}
}

// -----------------------------------------------------------------------------
// NewGitCredentialsService — constructor wiring.
// -----------------------------------------------------------------------------

func TestNewGitCredentialsService(t *testing.T) {
	t.Run("succeeds with a valid workflow-plane OpenBao config", func(t *testing.T) {
		svc, err := NewGitCredentialsService(&clientmocks.OpenChoreoClientMock{}, gitTestConfig())
		require.NoError(t, err)
		assert.NotNil(t, svc)
	})
}

// -----------------------------------------------------------------------------
// GetGitCredentials — name validation, the derived OpenBao path, and parsing.
// -----------------------------------------------------------------------------

func TestGitCredentialsService_GetGitCredentials(t *testing.T) {
	const org = gitTestOrg

	t.Run("reads the secret at the path derived from the namespace and name", func(t *testing.T) {
		kv := &fakeKVReader{secret: kvSecret(map[string]interface{}{
			"username": "octocat",
			"password": "ghp-token",
		})}
		svc := newGitCredsServiceWithKV(t, kv, "private-repo")

		creds, err := svc.GetGitCredentials(context.Background(), org, "private-repo")

		require.NoError(t, err)
		assert.Equal(t, []string{"secret/data/acme-ns/git/private-repo"}, kv.paths)
		assert.Equal(t, &GitCredentials{Type: "basic-auth", Username: "octocat", Password: "ghp-token"}, creds)
	})

	t.Run("leaves the username empty when the secret has none", func(t *testing.T) {
		kv := &fakeKVReader{secret: kvSecret(map[string]interface{}{"password": "ghp-token"})}
		svc := newGitCredsServiceWithKV(t, kv, "private-repo")

		creds, err := svc.GetGitCredentials(context.Background(), org, "private-repo")

		require.NoError(t, err)
		assert.Equal(t, &GitCredentials{Type: "basic-auth", Password: "ghp-token"}, creds)
	})

	t.Run("rejects an empty secretRef before reading anything", func(t *testing.T) {
		kv := &fakeKVReader{}
		svc := newGitCredsServiceWithKV(t, kv)

		creds, err := svc.GetGitCredentials(context.Background(), org, "")

		require.Error(t, err)
		assert.Nil(t, creds)
		assert.Contains(t, err.Error(), "secretRef is required")
		assert.Empty(t, kv.paths)
	})

	t.Run("accepts git secret names OpenChoreo allows", func(t *testing.T) {
		// Kubernetes object names may contain dots and run to 253 characters.
		for _, name := range []string{"team.git", "Private-Repo", strings.Repeat("a", 64)} {
			t.Run(name, func(t *testing.T) {
				kv := &fakeKVReader{secret: kvSecret(map[string]interface{}{"password": "ghp-token"})}
				svc := newGitCredsServiceWithKV(t, kv, name)

				creds, err := svc.GetGitCredentials(context.Background(), org, name)

				require.NoError(t, err)
				assert.Equal(t, "ghp-token", creds.Password)
				assert.Equal(t, []string{"secret/data/acme-ns/git/" + name}, kv.paths)
			})
		}
	})

	t.Run("rejects names that could leave the namespace's git secrets", func(t *testing.T) {
		for _, name := range []string{
			"../other-ns/git/secret",
			"other/secret",
			"..",
			".",
			"team..git",
			".hidden",
			"trailing.",
			"-leading-hyphen",
			"trailing-hyphen-",
			"has space",
			strings.Repeat("a", 254),
		} {
			t.Run(name, func(t *testing.T) {
				kv := &fakeKVReader{}
				svc := newGitCredsServiceWithKV(t, kv)

				creds, err := svc.GetGitCredentials(context.Background(), org, name)

				require.Error(t, err)
				assert.Nil(t, creds)
				assert.Contains(t, err.Error(), "invalid secretRef")
				assert.Empty(t, kv.paths)
			})
		}
	})

	t.Run("propagates the OpenBao read error", func(t *testing.T) {
		boom := errors.New("openbao unreachable")
		svc := newGitCredsServiceWithKV(t, &fakeKVReader{err: boom}, "private-repo")

		creds, err := svc.GetGitCredentials(context.Background(), org, "private-repo")

		require.Error(t, err)
		assert.Nil(t, creds)
		assert.ErrorIs(t, err, boom)
		assert.Contains(t, err.Error(), "failed to read secret from OpenBao")
	})

	t.Run("does not read a git secret OpenChoreo no longer lists", func(t *testing.T) {
		// Deleting a git secret can leave its value in OpenBao; it must not be used.
		kv := &fakeKVReader{secret: kvSecret(map[string]interface{}{"password": "leaked-token"})}
		svc := newGitCredsServiceWithKV(t, kv, "other-secret")

		creds, err := svc.GetGitCredentials(context.Background(), org, "deleted-secret")

		require.Error(t, err)
		assert.Nil(t, creds)
		assert.ErrorIs(t, err, utils.ErrNotFound)
		assert.Contains(t, err.Error(), `git secret "deleted-secret"`)
		assert.Empty(t, kv.paths)
	})

	t.Run("propagates the git secret list error without reading", func(t *testing.T) {
		boom := errors.New("openchoreo unreachable")
		kv := &fakeKVReader{}
		svc := &gitCredentialsService{
			ocClient: &clientmocks.OpenChoreoClientMock{
				ListGitSecretsFunc: func(_ context.Context, _ string) ([]*occlient.GitSecretInfo, error) {
					return nil, boom
				},
			},
			kv: kv,
		}

		creds, err := svc.GetGitCredentials(context.Background(), org, "private-repo")

		require.Error(t, err)
		assert.Nil(t, creds)
		assert.ErrorIs(t, err, boom)
		assert.Contains(t, err.Error(), "failed to list git secrets")
		assert.Empty(t, kv.paths)
	})

	t.Run("fails when the listed secret has no value in OpenBao", func(t *testing.T) {
		for name, secret := range map[string]*vault.Secret{
			"nil secret":      nil,
			"secret, no data": {},
		} {
			t.Run(name, func(t *testing.T) {
				svc := newGitCredsServiceWithKV(t, &fakeKVReader{secret: secret}, "missing")

				creds, err := svc.GetGitCredentials(context.Background(), org, "missing")

				require.Error(t, err)
				assert.Nil(t, creds)
				assert.Contains(t, err.Error(), "secret not found in OpenBao at path: secret/data/acme-ns/git/missing")
			})
		}
	})

	t.Run("fails when the data is not in KV v2 form", func(t *testing.T) {
		kv := &fakeKVReader{secret: &vault.Secret{Data: map[string]interface{}{"password": "ghp-token"}}}
		svc := newGitCredsServiceWithKV(t, kv, "private-repo")

		creds, err := svc.GetGitCredentials(context.Background(), org, "private-repo")

		require.Error(t, err)
		assert.Nil(t, creds)
		assert.Contains(t, err.Error(), "invalid secret data format")
	})

	t.Run("fails when the secret has no password", func(t *testing.T) {
		for name, data := range map[string]map[string]interface{}{
			"missing": {"username": "octocat"},
			"empty":   {"username": "octocat", "password": ""},
		} {
			t.Run(name, func(t *testing.T) {
				svc := newGitCredsServiceWithKV(t, &fakeKVReader{secret: kvSecret(data)}, "private-repo")

				creds, err := svc.GetGitCredentials(context.Background(), org, "private-repo")

				require.Error(t, err)
				assert.Nil(t, creds)
				assert.Contains(t, err.Error(), "no valid credentials found in secret")
			})
		}
	})
}

// -----------------------------------------------------------------------------
// gitSecretKVPath — the remote key OpenChoreo's git secret API writes to.
// -----------------------------------------------------------------------------

func TestGitSecretKVPath(t *testing.T) {
	assert.Equal(t, "secret/default/git/private", gitSecretKVPath("default", "private"))
}

// -----------------------------------------------------------------------------
// convertToKV2ReadPath — pure helper, no dependencies. Verifies the KV v1 -> v2
// read-path rewrite that determines where the Vault read happens.
// -----------------------------------------------------------------------------

func TestConvertToKV2ReadPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "inserts /data/ after the mount for a nested path",
			in:   "secret/default/git/private",
			want: "secret/data/default/git/private",
		},
		{
			name: "inserts /data/ for a single-segment path",
			in:   "secret/private",
			want: "secret/data/private",
		},
		{
			name: "returns a path with no separator unchanged",
			in:   "secret",
			want: "secret",
		},
		{
			name: "returns empty input unchanged",
			in:   "",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, convertToKV2ReadPath(tc.in))
		})
	}
}

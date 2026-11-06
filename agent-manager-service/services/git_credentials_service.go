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

package services

import (
	"context"
	"fmt"
	"strings"

	vault "github.com/hashicorp/vault/api"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// GitCredentials holds the credentials for git authentication
type GitCredentials struct {
	// Type is the type of authentication: "basic-auth"
	Type string
	// Username for basic-auth (optional, defaults to "git" if not provided)
	Username string
	// Password/Token for basic-auth
	Password string
}

// GitCredentialsService provides methods to fetch git credentials from OpenBao
type GitCredentialsService interface {
	// GetGitCredentials fetches git credentials for a given secret reference
	GetGitCredentials(ctx context.Context, ouID, secretRef string) (*GitCredentials, error)
}

// gitSecretKVMount is the workflow-plane OpenBao mount that holds git secrets.
// OpenChoreo's git secret API pushes each secret to the remote key
// "secret/{namespace}/git/{name}" (fixed in its gitsecret service), with the
// token under "password" and the optional username under "username".
const gitSecretKVMount = "secret"

// kvReader reads a secret from the workflow-plane OpenBao. *vault.Logical
// satisfies it; tests substitute a fake.
type kvReader interface {
	ReadWithContext(ctx context.Context, path string) (*vault.Secret, error)
}

type gitCredentialsService struct {
	ocClient client.OpenChoreoClient
	kv       kvReader
}

// NewGitCredentialsService creates a new git credentials service
func NewGitCredentialsService(ocClient client.OpenChoreoClient, cfg config.Config) (GitCredentialsService, error) {
	// Create vault client for workflow plane OpenBao
	vaultCfg := vault.DefaultConfig()
	vaultCfg.Address = cfg.WorkflowPlaneOpenBao.URL

	vaultClient, err := vault.NewClient(vaultCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client for workflow plane: %w", err)
	}

	vaultClient.SetToken(cfg.WorkflowPlaneOpenBao.Token)

	return &gitCredentialsService{
		ocClient: ocClient,
		kv:       vaultClient.Logical(),
	}, nil
}

// convertToKV2ReadPath converts a KV path to the KV v2 read path format.
// Input: "{mount}/{path}" (e.g., "secret/default/git/private")
// Output: "{mount}/data/{path}" (e.g., "secret/data/default/git/private")
func convertToKV2ReadPath(kvPath string) string {
	parts := strings.SplitN(kvPath, "/", 2)
	if len(parts) < 2 {
		// Invalid path format - return as-is and let Vault return an error
		return kvPath
	}
	return parts[0] + "/data/" + parts[1]
}

// requireGitSecret returns an error unless OpenChoreo lists a git secret named
// secretName in the namespace.
func (s *gitCredentialsService) requireGitSecret(ctx context.Context, ouID, secretName string) error {
	secrets, err := s.ocClient.ListGitSecrets(ctx, ouID)
	if err != nil {
		return fmt.Errorf("failed to list git secrets: %w", err)
	}
	for _, secret := range secrets {
		if secret != nil && secret.Name == secretName {
			return nil
		}
	}
	return fmt.Errorf("git secret %q: %w", secretName, utils.ErrNotFound)
}

// gitSecretKVPath returns the workflow-plane OpenBao path of a git secret,
// "{mount}/{namespace}/git/{name}", the remote key OpenChoreo's git secret API
// pushes the secret to.
func gitSecretKVPath(namespace, secretName string) string {
	return gitSecretKVMount + "/" + namespace + "/git/" + secretName
}

// GetGitCredentials fetches the credentials of the git secret named secretRef.
// The secret's OpenBao path is derived from its name and the namespace, so its
// SecretReference is never read.
func (s *gitCredentialsService) GetGitCredentials(ctx context.Context, ouID, secretRef string) (*GitCredentials, error) {
	if secretRef == "" {
		return nil, fmt.Errorf("secretRef is required")
	}
	// The name becomes a path segment, so only accept the characters a git
	// secret name may contain. This keeps the read inside the namespace's git
	// secrets.
	if !isValidSecretName(secretRef) {
		return nil, fmt.Errorf("invalid secretRef %q: must be 1-63 alphanumeric characters or hyphens", secretRef)
	}

	// Deleting a git secret can leave its value in OpenBao (OpenChoreo's push
	// does not remove it), so only read secrets that still exist.
	if err := s.requireGitSecret(ctx, ouID, secretRef); err != nil {
		return nil, err
	}

	// KV v2 reads go to "{mount}/data/{path}".
	secretPath := convertToKV2ReadPath(gitSecretKVPath(s.ocClient.NamespaceFor(ouID), secretRef))
	secret, err := s.kv.ReadWithContext(ctx, secretPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read secret from OpenBao: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("secret not found in OpenBao at path: %s", secretPath)
	}

	// KV v2: data is nested under "data" key
	dataMap, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid secret data format")
	}

	// Check for basic auth (password/token)
	password, ok := dataMap["password"].(string)
	if !ok || password == "" {
		return nil, fmt.Errorf("no valid credentials found in secret")
	}

	creds := &GitCredentials{
		Type:     "basic-auth",
		Password: password,
	}

	if username, ok := dataMap["username"].(string); ok {
		creds.Username = username
	}

	return creds, nil
}

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
	"encoding/json"
	"fmt"
	"strings"

	vault "github.com/hashicorp/vault/api"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
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
	GetGitCredentials(ctx context.Context, orgName, secretRef string) (*GitCredentials, error)
}

type gitCredentialsService struct {
	ocClient    client.OpenChoreoClient
	vaultClient *vault.Client
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
		ocClient:    ocClient,
		vaultClient: vaultClient,
	}, nil
}

// convertToKV2ReadPath converts a KV path to the KV v2 read path format.
// Input: "{mount}/{path}" (e.g., "secret/default/git/private")
// Output: "{mount}/data/{path}" (e.g., "secret/data/default/git/private")
func convertToKV2ReadPath(kvPath string) string {
	parts := strings.SplitN(kvPath, "/", 2)
	if len(parts) < 2 {
		return kvPath + "/data"
	}
	return parts[0] + "/data/" + parts[1]
}

// GetGitCredentials fetches git credentials for a given secret reference
func (s *gitCredentialsService) GetGitCredentials(ctx context.Context, orgName, secretRef string) (*GitCredentials, error) {
	if secretRef == "" {
		return nil, fmt.Errorf("secretRef is required")
	}

	// Get the SecretReference from OpenChoreo to find the OpenBao path
	secretRefInfo, err := s.ocClient.GetSecretReference(ctx, orgName, secretRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret reference: %w", err)
	}

	if secretRefInfo == nil || len(secretRefInfo.Data) == 0 {
		return nil, fmt.Errorf("secret reference not found or has no data")
	}

	// Get the OpenBao path from the first data source
	// The remoteRef.key format is "{mount}/{path}" (e.g., "secret/default/git/private")
	// For KV v2, we need to convert it to "{mount}/data/{path}"
	kvPath := secretRefInfo.Data[0].RemoteRef.Key
	if kvPath == "" {
		return nil, fmt.Errorf("secret reference has no KV path")
	}

	// Insert "/data/" after the mount path (first component)
	secretPath := convertToKV2ReadPath(kvPath)
	secret, err := s.vaultClient.Logical().ReadWithContext(ctx, secretPath)
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

	// Determine the credential type and extract values
	creds := &GitCredentials{}

	// Check for basic auth (password/token)
	if password, ok := dataMap["password"].(string); ok && password != "" {
		creds.Type = "basic-auth"
		creds.Password = password
		if username, ok := dataMap["username"].(string); ok {
			creds.Username = username
		}
		return creds, nil
	}

	// Try to unmarshal as JSON in case it's stored as a single value
	if value, ok := secret.Data["data"]; ok {
		jsonBytes, err := json.Marshal(value)
		if err == nil {
			var credMap map[string]string
			if json.Unmarshal(jsonBytes, &credMap) == nil {
				if password, ok := credMap["password"]; ok && password != "" {
					creds.Type = "basic-auth"
					creds.Password = password
					creds.Username = credMap["username"]
					return creds, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("no valid credentials found in secret")
}

// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package secretmanagersvc

import (
	"context"
	"encoding/json"
	"fmt"
)

const (
	// DefaultManagedBy is the default ownership tag used by the secret management client.
	DefaultManagedBy = "amp-agent-manager"
)

// CreateSecretRequest represents a request to create a secret.
type CreateSecretRequest struct {
	// Path is the KV path where the secret will be stored (e.g., "project/component").
	Path string `json:"path"`
	// Data contains the secret key-value pairs to store.
	Data map[string]string `json:"data"`
}

// UpdateSecretRequest represents a request to update a secret.
type UpdateSecretRequest struct {
	// Data contains the secret key-value pairs to update.
	Data map[string]string `json:"data"`
}

// SecretResponse represents the response from secret operations.
type SecretResponse struct {
	// Path is the KV path of the secret.
	Path string `json:"path"`
	// Data contains the secret key-value pairs (only for GetSecret).
	Data map[string]string `json:"data,omitempty"`
}

// SecretManagementClient defines the interface for secret management operations.
//
//go:generate moq -out ../clientmocks/secret_mgmt_client_fake.go -pkg clientmocks . SecretManagementClient
type SecretManagementClient interface {
	// CreateSecret creates a new secret at the specified path.
	CreateSecret(ctx context.Context, req CreateSecretRequest) (*SecretResponse, error)

	// UpdateSecret updates an existing secret.
	UpdateSecret(ctx context.Context, secretPath string, req UpdateSecretRequest) (*SecretResponse, error)
	// GetSecret retrieves a secret by path.
	GetSecret(ctx context.Context, secretPath string) (*SecretResponse, error)

	// DeleteSecret deletes a secret by path.
	DeleteSecret(ctx context.Context, secretPath string) error
}

// secretManagementClient implements SecretManagementClient using the low-level SecretsClient.
type secretManagementClient struct {
	lowLevelClient SecretsClient
	managedBy      string
}

// NewSecretManagementClient creates a new SecretManagementClient.
func NewSecretManagementClient(cfg *StoreConfig) (SecretManagementClient, error) {
	if cfg == nil || cfg.Provider == "" {
		return nil, fmt.Errorf("failed to create secret management client")
	}

	// Get the provider
	provider, ok := GetProvider(cfg.Provider)
	if !ok {
		return nil, fmt.Errorf("provider %q not registered", cfg.Provider)
	}

	// Create the low-level client
	lowLevelClient, err := provider.NewClient(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create secrets client: %w", err)
	}

	return &secretManagementClient{
		lowLevelClient: lowLevelClient,
		managedBy:      DefaultManagedBy,
	}, nil
}

// CreateSecret creates a new secret at the specified path.
func (c *secretManagementClient) CreateSecret(ctx context.Context, req CreateSecretRequest) (*SecretResponse, error) {
	// Convert map to JSON bytes
	data, err := json.Marshal(req.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal secret data: %w", err)
	}

	// Push the secret
	metadata := &SecretMetadata{
		ManagedBy: c.managedBy,
	}
	if err := c.lowLevelClient.PushSecret(ctx, req.Path, data, metadata); err != nil {
		return nil, fmt.Errorf("failed to create secret: %w", err)
	}

	return &SecretResponse{
		Path: req.Path,
	}, nil
}

// UpdateSecret updates an existing secret.
func (c *secretManagementClient) UpdateSecret(ctx context.Context, secretPath string, req UpdateSecretRequest) (*SecretResponse, error) {
	// Convert map to JSON bytes
	data, err := json.Marshal(req.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal secret data: %w", err)
	}

	// Push the secret (PushSecret handles both create and update)
	metadata := &SecretMetadata{
		ManagedBy: c.managedBy,
	}
	if err := c.lowLevelClient.PushSecret(ctx, secretPath, data, metadata); err != nil {
		return nil, fmt.Errorf("failed to update secret: %w", err)
	}

	return &SecretResponse{
		Path: secretPath,
	}, nil
}

// GetSecret retrieves a secret by path.
func (c *secretManagementClient) GetSecret(ctx context.Context, secretPath string) (*SecretResponse, error) {
	data, err := c.lowLevelClient.GetSecret(ctx, secretPath)
	if err != nil {
		return nil, err
	}

	// Parse JSON data back to map
	var secretData map[string]string
	if err := json.Unmarshal(data, &secretData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal secret data: %w", err)
	}

	return &SecretResponse{
		Path: secretPath,
		Data: secretData,
	}, nil
}

// DeleteSecret deletes a secret by path.
func (c *secretManagementClient) DeleteSecret(ctx context.Context, secretPath string) error {
	metadata := &SecretMetadata{
		ManagedBy: c.managedBy,
	}
	return c.lowLevelClient.DeleteSecret(ctx, secretPath, metadata)
}

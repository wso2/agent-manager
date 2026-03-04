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

// SecretLocation identifies where a secret is stored in the KV hierarchy.
type SecretLocation struct {
	OrgName         string
	ProjectName     string
	EnvironmentName string
	ComponentName   string
}

// KVPath constructs the path for storing secrets in the KV store.
// The path format is: {orgName}/{projectName}/{environmentName}/{componentName}
func (l SecretLocation) KVPath() string {
	return fmt.Sprintf("%s/%s/%s/%s", l.OrgName, l.ProjectName, l.EnvironmentName, l.ComponentName)
}

// SecretManagementClient defines the interface for secret management operations.
//
//go:generate moq -out ../clientmocks/secret_mgmt_client_fake.go -pkg clientmocks . SecretManagementClient
type SecretManagementClient interface {
	// CreateSecret creates a new secret at the location derived from SecretLocation.
	CreateSecret(ctx context.Context, location SecretLocation, data map[string]string) (string, error)

	// UpdateSecret updates an existing secret at the location derived from SecretLocation.
	UpdateSecret(ctx context.Context, location SecretLocation, data map[string]string) (string, error)

	// DeleteSecret deletes a secret at the location derived from SecretLocation.
	DeleteSecret(ctx context.Context, location SecretLocation) error

	// DeleteSecretByPath deletes a secret by its KV path.
	// Use this when the path is retrieved from a stored reference.
	DeleteSecretByPath(ctx context.Context, secretPath string) error
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
	lowLevelClient, err := provider.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create secrets client: %w", err)
	}

	return &secretManagementClient{
		lowLevelClient: lowLevelClient,
		managedBy:      DefaultManagedBy,
	}, nil
}

// CreateSecret creates a new secret at the location derived from SecretLocation.
// Returns the KV path where the secret was stored.
func (c *secretManagementClient) CreateSecret(ctx context.Context, location SecretLocation, secretData map[string]string) (string, error) {
	kvPath := location.KVPath()

	// Convert map to JSON bytes
	data, err := json.Marshal(secretData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal secret data: %w", err)
	}

	// Push the secret
	metadata := &SecretMetadata{
		ManagedBy: c.managedBy,
	}
	if err := c.lowLevelClient.PushSecret(ctx, kvPath, data, metadata); err != nil {
		return "", fmt.Errorf("failed to create secret: %w", err)
	}

	return kvPath, nil
}

// UpdateSecret updates an existing secret at the location derived from SecretLocation.
// Returns the KV path where the secret was stored.
func (c *secretManagementClient) UpdateSecret(ctx context.Context, location SecretLocation, secretData map[string]string) (string, error) {
	kvPath := location.KVPath()

	// Convert map to JSON bytes
	data, err := json.Marshal(secretData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal secret data: %w", err)
	}

	// Push the secret (PushSecret handles both create and update)
	metadata := &SecretMetadata{
		ManagedBy: c.managedBy,
	}
	if err := c.lowLevelClient.PushSecret(ctx, kvPath, data, metadata); err != nil {
		return "", fmt.Errorf("failed to update secret: %w", err)
	}

	return kvPath, nil
}

// DeleteSecret deletes a secret at the location derived from SecretLocation.
func (c *secretManagementClient) DeleteSecret(ctx context.Context, location SecretLocation) error {
	return c.DeleteSecretByPath(ctx, location.KVPath())
}

// DeleteSecretByPath deletes a secret by its KV path.
func (c *secretManagementClient) DeleteSecretByPath(ctx context.Context, secretPath string) error {
	metadata := &SecretMetadata{
		ManagedBy: c.managedBy,
	}
	return c.lowLevelClient.DeleteSecret(ctx, secretPath, metadata)
}

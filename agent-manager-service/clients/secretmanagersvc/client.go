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
	"strings"
)

const (
	// DefaultManagedBy is the default ownership tag used by the secret management client.
	DefaultManagedBy = "amp-agent-manager"

	// SecretKeyAPIKey is the key name used when storing and retrieving API keys in the KV store.
	SecretKeyAPIKey = "api-key"
)

// SecretLocation identifies where a secret is stored in the KV hierarchy.
type SecretLocation struct {
	OrgName         string
	ProjectName     string // optional — empty for org-level secrets
	AgentName       string // optional — for agent-scoped secrets
	EnvironmentName string // optional — empty for org-level secrets
	EntityName      string // e.g., provider-handle or proxy-handle
	ConfigName      string // optional — e.g., "config-name"
	SecretKey       string // optional — e.g., "api-key"
}

// sanitizeSegment trims whitespace and validates the segment for use in a KV path.
// Returns an error if the segment contains '/' to prevent path traversal and path collisions
// (e.g., org "a/b" and org "a_b" would otherwise both produce segment "a_b").
func sanitizeSegment(s string) (string, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		return "", fmt.Errorf("secret path segment %q contains invalid character '/'", s)
	}
	return s, nil
}

// KVPath constructs the path from non-empty segments.
// Returns an error if the required fields OrgName or ComponentName are empty,
// or if any segment contains invalid characters (e.g., '/').
// Examples:
//
//	org/env/provider-handle/api-key               (org-level provider)
//	org/project/env/agent/config-name/provider-handle/api-key  (agent-scoped)
//
// org/project/env/agent/config-name/proxy-handle/api-key  (agent-scoped)
func (l SecretLocation) KVPath() (string, error) {
	if strings.TrimSpace(l.OrgName) == "" {
		return "", fmt.Errorf("SecretLocation.OrgName is required")
	}
	if strings.TrimSpace(l.EntityName) == "" {
		return "", fmt.Errorf("SecretLocation.ComponentName is required")
	}

	orgSeg, err := sanitizeSegment(l.OrgName)
	if err != nil {
		return "", fmt.Errorf("invalid OrgName: %w", err)
	}
	parts := []string{orgSeg}

	if l.ProjectName != "" {
		seg, err := sanitizeSegment(l.ProjectName)
		if err != nil {
			return "", fmt.Errorf("invalid ProjectName: %w", err)
		}
		if seg != "" {
			parts = append(parts, seg)
		}
	}
	if l.EnvironmentName != "" {
		seg, err := sanitizeSegment(l.EnvironmentName)
		if err != nil {
			return "", fmt.Errorf("invalid EnvironmentName: %w", err)
		}
		if seg != "" {
			parts = append(parts, seg)
		}
	}
	if l.AgentName != "" {
		seg, err := sanitizeSegment(l.AgentName)
		if err != nil {
			return "", fmt.Errorf("invalid AgentName: %w", err)
		}
		if seg != "" {
			parts = append(parts, seg)
		}
	}
	if l.ConfigName != "" {
		seg, err := sanitizeSegment(l.ConfigName)
		if err != nil {
			return "", fmt.Errorf("invalid Config name: %w", err)
		}
		if seg != "" {
			parts = append(parts, seg)
		}
	}
	if l.EntityName != "" {
		seg, err := sanitizeSegment(l.EntityName)
		if err != nil {
			return "", fmt.Errorf("invalid Entity name: %w", err)
		}
		if seg != "" {
			parts = append(parts, seg)
		}
	}

	if l.SecretKey != "" {
		seg, err := sanitizeSegment(l.SecretKey)
		if err != nil {
			return "", fmt.Errorf("invalid SecretKey: %w", err)
		}
		if seg != "" {
			parts = append(parts, seg)
		}
	}
	return strings.Join(parts, "/"), nil
}

// SecretManagementClient defines the interface for secret management operations.
//
//go:generate moq -out ../clientmocks/secret_mgmt_client_fake.go -pkg clientmocks . SecretManagementClient
type SecretManagementClient interface {
	// CreateSecret creates or updates a secret at the location derived from SecretLocation.
	// This REPLACES all secret data at the location.
	CreateSecret(ctx context.Context, location SecretLocation, data map[string]string) (string, error)

	// DeleteSecret deletes a secret at the location derived from SecretLocation.
	DeleteSecret(ctx context.Context, location SecretLocation) error

	// DeleteSecretByPath deletes a secret by its KV path.
	// Use this when the path is retrieved from a stored reference.
	DeleteSecretByPath(ctx context.Context, secretPath string) error

	// GetSecret retrieves a secret by its full KV path.
	// Returns the secret data as a key-value map.
	GetSecret(ctx context.Context, kvPath string) (map[string]string, error)
}

// secretManagementClient implements SecretManagementClient using the low-level SecretsClient.
type secretManagementClient struct {
	lowLevelClient SecretsClient
	managedBy      string
}

// NewSecretManagementClient creates a new SecretManagementClient with the given provider.
func NewSecretManagementClient(cfg *StoreConfig, provider Provider) (SecretManagementClient, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if provider == nil {
		return nil, fmt.Errorf("provider is required")
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
	kvPath, err := location.KVPath()
	if err != nil {
		return "", fmt.Errorf("invalid secret location: %w", err)
	}

	// Convert map to JSON bytes
	data, err := json.Marshal(secretData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal secret data: %w", err)
	}

	// Push the secret (handles both create and update)
	metadata := &SecretMetadata{
		ManagedBy: c.managedBy,
	}
	if err := c.lowLevelClient.PushSecret(ctx, kvPath, data, metadata); err != nil {
		return "", fmt.Errorf("failed to upsert secret: %w", err)
	}

	return kvPath, nil
}

// DeleteSecret deletes a secret at the location derived from SecretLocation.
func (c *secretManagementClient) DeleteSecret(ctx context.Context, location SecretLocation) error {
	kvPath, err := location.KVPath()
	if err != nil {
		return fmt.Errorf("invalid secret location: %w", err)
	}
	return c.DeleteSecretByPath(ctx, kvPath)
}

// DeleteSecretByPath deletes a secret by its KV path.
func (c *secretManagementClient) DeleteSecretByPath(ctx context.Context, secretPath string) error {
	metadata := &SecretMetadata{
		ManagedBy: c.managedBy,
	}
	return c.lowLevelClient.DeleteSecret(ctx, secretPath, metadata)
}

// GetSecret retrieves a secret by its KV path.
func (c *secretManagementClient) GetSecret(ctx context.Context, kvPath string) (map[string]string, error) {
	raw, err := c.lowLevelClient.GetSecret(ctx, kvPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret at path %q: %w", kvPath, err)
	}

	var data map[string]string
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal secret data: %w", err)
	}

	return data, nil
}

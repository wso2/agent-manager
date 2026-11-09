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
	"fmt"
	"strings"
)

const (
	// DefaultManagedBy is the default ownership tag used by the secret management client.
	DefaultManagedBy = "amp-agent-manager"

	// LabelKeyManagedBy is the label/metadata key providers use to record the
	// ownership tag on stored secrets.
	LabelKeyManagedBy = "managed-by"

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

// sanitizeSegment trims whitespace and validates one segment of a KV path.
// Rejects '/' (would collide two different segments, e.g. org "a/b" and org
// "a_b" would otherwise both produce "a_b") and the reserved traversal tokens
// "." and "..". Every SecretLocation field funnels through here via KVPath(),
// making this the one choke point for every KV path built in the codebase.
func sanitizeSegment(s string) (string, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		return "", fmt.Errorf("secret path segment %q contains invalid character '/'", s)
	}
	if s == "." || s == ".." {
		return "", fmt.Errorf("secret path segment %q is a reserved path traversal token", s)
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

// SecretRefName builds the SecretReference name from location fields.
// When EnvironmentName is set, it is always included to ensure each environment
// gets its own SecretReference CR (e.g. my-agent-default-secrets, my-agent-staging-secrets).
// When ConfigName is also set, it is prepended (e.g. config-staging-my-agent-secrets).
// The name is sanitized for Kubernetes naming (lowercase, max 63 chars).
func (l SecretLocation) SecretRefName() string {
	var name string
	if l.ConfigName != "" && l.EnvironmentName != "" {
		name = fmt.Sprintf("%s-%s-%s-secrets", sanitizeForK8sName(l.ConfigName), sanitizeForK8sName(l.EnvironmentName), sanitizeForK8sName(l.EntityName))
	} else if l.EnvironmentName != "" {
		name = fmt.Sprintf("%s-%s-secrets", sanitizeForK8sName(l.EntityName), sanitizeForK8sName(l.EnvironmentName))
	} else {
		name = fmt.Sprintf("%s-secrets", sanitizeForK8sName(l.EntityName))
	}
	if len(name) > 63 {
		name = strings.TrimRight(name[:63], "-")
	}
	return name
}

// sanitizeForK8sName converts s to a lowercase DNS-label-safe string.
func sanitizeForK8sName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var result strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		} else {
			result.WriteRune('-')
		}
	}
	return strings.Trim(result.String(), "-")
}

// ParseKVPath parses a KV path string back into a SecretLocation.
// Supports paths matching the shapes produced by SecretLocation.KVPath():
//   - 2 segments: org/entity (org-level secret)
//   - 3 segments: org/entity/key (org-level secret with key)
//   - 4 segments: org/project/env/entity (agent secret without agent/config)
//   - 5 segments: org/project/env/agent/entity
//   - 6 segments: org/project/env/agent/config/entity
//   - 7 segments: org/project/env/agent/config/entity/key
//
// Returns error if the path format is not recognized.
func ParseKVPath(kvPath string) (SecretLocation, error) {
	parts := strings.Split(kvPath, "/")
	switch len(parts) {
	case 2:
		// org/entity
		return SecretLocation{
			OrgName:    parts[0],
			EntityName: parts[1],
		}, nil
	case 3:
		// org/entity/key
		return SecretLocation{
			OrgName:    parts[0],
			EntityName: parts[1],
			SecretKey:  parts[2],
		}, nil
	case 4:
		// org/project/env/entity
		return SecretLocation{
			OrgName:         parts[0],
			ProjectName:     parts[1],
			EnvironmentName: parts[2],
			EntityName:      parts[3],
		}, nil
	case 5:
		// org/project/env/agent/entity
		return SecretLocation{
			OrgName:         parts[0],
			ProjectName:     parts[1],
			EnvironmentName: parts[2],
			AgentName:       parts[3],
			EntityName:      parts[4],
		}, nil
	case 6:
		// org/project/env/agent/config/entity
		return SecretLocation{
			OrgName:         parts[0],
			ProjectName:     parts[1],
			EnvironmentName: parts[2],
			AgentName:       parts[3],
			ConfigName:      parts[4],
			EntityName:      parts[5],
		}, nil
	case 7:
		// org/project/env/agent/config/entity/key
		return SecretLocation{
			OrgName:         parts[0],
			ProjectName:     parts[1],
			EnvironmentName: parts[2],
			AgentName:       parts[3],
			ConfigName:      parts[4],
			EntityName:      parts[5],
			SecretKey:       parts[6],
		}, nil
	default:
		return SecretLocation{}, fmt.Errorf("unrecognized KV path format: %s (expected 2-7 segments, got %d)", kvPath, len(parts))
	}
}

// SecretManagementClient defines the interface for secret management operations.
//
//go:generate moq -out ../clientmocks/secret_mgmt_client_fake.go -pkg clientmocks . SecretManagementClient
type SecretManagementClient interface {
	// CreateSecret creates or updates a secret at the location derived from SecretLocation.
	// This REPLACES all secret data at the location.
	// The secret name is derived from location using SecretRefName().
	// Returns the openchoreo secretRefName
	CreateSecret(ctx context.Context, location SecretLocation, data map[string]string) (string, error)

	// PatchSecret merges data with an existing secret (server-side merge).
	// Keys in data are added/updated, keys in keysToDelete are removed.
	// The secret name is derived from location using SecretRefName().
	// Returns the openchoreo secretRefName
	PatchSecret(ctx context.Context, location SecretLocation, data map[string]string, keysToDelete []string) (string, error)

	// DeleteSecret deletes a secret and its associated SecretReference.
	// secretRefName is retained for interface compatibility; providers derive
	// the secret name from location and manage SecretReferences internally.
	DeleteSecret(ctx context.Context, location SecretLocation, secretRefName string) error

	// DeleteSecretByName deletes a secret and its SecretReference by the
	// SecretReference name returned when it was created. Use it when that name
	// was persisted: location derives the name from current values (such as a
	// configuration name), so it misses a secret created before a rename.
	DeleteSecretByName(ctx context.Context, orgName, secretRefName string) error

	// GetSecret retrieves secret metadata without values.
	// Returns SecretInfo containing ID, keys list, and labels.
	GetSecret(ctx context.Context, kvPath string) (*SecretInfo, error)

	// GetSecretData returns the values of the secret named secretRefName, for
	// copying a secret (e.g. into another environment on promotion).
	GetSecretData(ctx context.Context, orgName, secretRefName string) (map[string]string, error)
}

// secretManagementClient implements SecretManagementClient using the low-level SecretsClient.
type secretManagementClient struct {
	lowLevelClient SecretsClient
	managedBy      string
}

// SecretManagementClientConfig holds configuration for creating a SecretManagementClient.
type SecretManagementClientConfig struct {
	// StoreConfig is the secret store configuration.
	StoreConfig *StoreConfig
	// Provider is the secrets provider (e.g., the OpenChoreo secret API).
	Provider Provider
}

// NewSecretManagementClient creates a new SecretManagementClient with the given provider.
func NewSecretManagementClient(cfg *StoreConfig, provider Provider) (SecretManagementClient, error) {
	return NewSecretManagementClientWithConfig(SecretManagementClientConfig{
		StoreConfig: cfg,
		Provider:    provider,
	})
}

// NewSecretManagementClientWithConfig creates a new SecretManagementClient with full configuration.
func NewSecretManagementClientWithConfig(cfg SecretManagementClientConfig) (SecretManagementClient, error) {
	if cfg.StoreConfig == nil {
		return nil, fmt.Errorf("config is required")
	}
	if cfg.Provider == nil {
		return nil, fmt.Errorf("provider is required")
	}

	// Create the low-level client
	lowLevelClient, err := cfg.Provider.NewClient(cfg.StoreConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create secrets client: %w", err)
	}

	return &secretManagementClient{
		lowLevelClient: lowLevelClient,
		managedBy:      DefaultManagedBy,
	}, nil
}

// CreateSecret creates a new secret at the location derived from SecretLocation.
// Returns the secret reference identifier from the provider (the name of the
// secret, which is also the name of its provider-managed SecretReference).
func (c *secretManagementClient) CreateSecret(ctx context.Context, location SecretLocation, secretData map[string]string) (string, error) {
	// Push the secret - provider derives name/labels from location
	metadata := &SecretMetadata{
		ManagedBy: c.managedBy,
	}
	secretRef, err := c.lowLevelClient.PushSecret(ctx, location, secretData, metadata)
	if err != nil {
		return "", fmt.Errorf("failed to upsert secret: %w", err)
	}

	return secretRef, nil
}

// PatchSecret merges data with an existing secret (server-side merge).
// Keys in data are added/updated, keys in keysToDelete are removed.
// Returns the secret reference identifier (same semantics as CreateSecret).
func (c *secretManagementClient) PatchSecret(ctx context.Context, location SecretLocation, secretData map[string]string, keysToDelete []string) (string, error) {
	metadata := &SecretMetadata{
		ManagedBy: c.managedBy,
	}
	secretRef, err := c.lowLevelClient.PatchSecret(ctx, location, secretData, keysToDelete, metadata)
	if err != nil {
		return "", fmt.Errorf("failed to patch secret: %w", err)
	}

	return secretRef, nil
}

// DeleteSecret deletes a secret and its provider-managed SecretReference.
// secretRefName is retained for interface compatibility; the provider derives
// the secret name from location.
func (c *secretManagementClient) DeleteSecret(ctx context.Context, location SecretLocation, _ string) error {
	metadata := &SecretMetadata{
		ManagedBy: c.managedBy,
	}
	if err := c.lowLevelClient.DeleteSecret(ctx, location, metadata); err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	return nil
}

// DeleteSecretByName deletes a secret and its provider-managed SecretReference
// by the SecretReference name.
func (c *secretManagementClient) DeleteSecretByName(ctx context.Context, orgName, secretRefName string) error {
	metadata := &SecretMetadata{
		ManagedBy: c.managedBy,
	}
	if err := c.lowLevelClient.DeleteSecretByName(ctx, orgName, secretRefName, metadata); err != nil {
		return fmt.Errorf("failed to delete secret %q: %w", secretRefName, err)
	}

	return nil
}

// GetSecret retrieves secret metadata without values.
// The kvPath is parsed back to a SecretLocation for the provider.
func (c *secretManagementClient) GetSecret(ctx context.Context, kvPath string) (*SecretInfo, error) {
	location, err := ParseKVPath(kvPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse KV path %q: %w", kvPath, err)
	}
	info, err := c.lowLevelClient.GetSecret(ctx, location)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret info at path %q: %w", kvPath, err)
	}
	return info, nil
}

// GetSecretData returns the values of the secret named secretRefName.
func (c *secretManagementClient) GetSecretData(ctx context.Context, orgName, secretRefName string) (map[string]string, error) {
	data, err := c.lowLevelClient.GetSecretData(ctx, orgName, secretRefName)
	if err != nil {
		return nil, fmt.Errorf("failed to read secret %q: %w", secretRefName, err)
	}
	return data, nil
}

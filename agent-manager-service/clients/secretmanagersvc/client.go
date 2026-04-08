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
	"errors"
	"fmt"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
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

// SecretRefName builds the SecretReference name from location fields.
// Uses ConfigName-EnvironmentName-secrets if ConfigName is set,
// otherwise uses EntityName-secrets.
// The name is sanitized for Kubernetes naming (lowercase, max 63 chars).
func (l SecretLocation) SecretRefName() string {
	var name string
	if l.ConfigName != "" && l.EnvironmentName != "" {
		name = fmt.Sprintf("%s-%s-secrets", sanitizeForK8sName(l.ConfigName), sanitizeForK8sName(l.EnvironmentName))
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
// Supports paths in the format: org/project/env/entity (4 segments for agent secrets)
// or org/entity (2 segments for org-level secrets).
// Returns error if the path format is not recognized.
func ParseKVPath(kvPath string) (SecretLocation, error) {
	parts := strings.Split(kvPath, "/")
	switch len(parts) {
	case 2:
		// org/entity (org-level secret)
		return SecretLocation{
			OrgName:    parts[0],
			EntityName: parts[1],
		}, nil
	case 4:
		// org/project/env/entity (agent secret)
		return SecretLocation{
			OrgName:         parts[0],
			ProjectName:     parts[1],
			EnvironmentName: parts[2],
			EntityName:      parts[3],
		}, nil
	default:
		return SecretLocation{}, fmt.Errorf("unrecognized KV path format: %s (expected 2 or 4 segments, got %d)", kvPath, len(parts))
	}
}

// SecretManagementClient defines the interface for secret management operations.
//
//go:generate moq -out ../clientmocks/secret_mgmt_client_fake.go -pkg clientmocks . SecretManagementClient
type SecretManagementClient interface {
	// CreateSecret creates or updates a secret at the location derived from SecretLocation.
	// This REPLACES all secret data at the location.
	// The SecretReference name is derived from location using SecretRefName().
	// Returns the openchoreo secretRefName
	CreateSecret(ctx context.Context, location SecretLocation, data map[string]string) (string, error)

	// PatchSecret merges data with an existing secret (server-side merge).
	// Keys in data are added/updated, keys in keysToDelete are removed.
	// The SecretReference name is derived from location using SecretRefName().
	// Returns the openchoreo secretRefName
	PatchSecret(ctx context.Context, location SecretLocation, data map[string]string, keysToDelete []string) (string, error)

	// DeleteSecret deletes a secret and its associated SecretReference CRD.
	// secretRefName is the name of the SecretReference CR to delete.
	// When ocClient is configured (OpenBao provider), also deletes the SecretReference.
	DeleteSecret(ctx context.Context, location SecretLocation, secretRefName string) error

	// GetSecret retrieves secret metadata without values.
	// Returns SecretInfo containing ID, keys list, and labels.
	GetSecret(ctx context.Context, kvPath string) (*SecretInfo, error)

	// GetSecretWithValue retrieves a secret by its full KV path including actual values.
	// Returns the secret data as a key-value map.
	// Returns ErrNotSupported if the provider doesn't support value retrieval.
	GetSecretWithValue(ctx context.Context, kvPath string) (map[string]string, error)
}

// secretManagementClient implements SecretManagementClient using the low-level SecretsClient.
type secretManagementClient struct {
	lowLevelClient  SecretsClient
	managedBy       string
	ocClient        client.OpenChoreoClient // Optional: for SecretReference operations (nil for Secret Manager API)
	refreshInterval string                  // SecretReference refresh interval (e.g., "1h")
}

// SecretManagementClientConfig holds configuration for creating a SecretManagementClient.
type SecretManagementClientConfig struct {
	// StoreConfig is the secret store configuration.
	StoreConfig *StoreConfig
	// Provider is the secrets provider (e.g., OpenBao, Secret Manager API).
	Provider Provider
	// OCClient is the OpenChoreo client for SecretReference operations.
	// Set to nil for Secret Manager API (which handles SecretReferences internally).
	OCClient client.OpenChoreoClient
	// RefreshInterval is how often SecretReferences should refresh from KV (e.g., "1h").
	RefreshInterval string
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
		lowLevelClient:  lowLevelClient,
		managedBy:       DefaultManagedBy,
		ocClient:        cfg.OCClient,
		refreshInterval: cfg.RefreshInterval,
	}, nil
}

// upsertSecretReference creates or updates a SecretReference CRD for the given location.
// The secretRefName is derived from location using SecretRefName().
// Returns the secretRefName on success.
func (c *secretManagementClient) upsertSecretReference(ctx context.Context, location SecretLocation, kvPath string, secretKeys []string) (string, error) {
	secretRefName := location.SecretRefName()
	secretRefReq := client.CreateSecretReferenceRequest{
		Namespace:       location.OrgName,
		Name:            secretRefName,
		ProjectName:     location.ProjectName,
		ComponentName:   location.EntityName,
		KVPath:          kvPath,
		SecretKeys:      secretKeys,
		RefreshInterval: c.refreshInterval,
	}

	// Check if SecretReference already exists
	_, getErr := c.ocClient.GetSecretReference(ctx, location.OrgName, secretRefName)
	if getErr != nil {
		// Only create if SecretReference doesn't exist (NotFound); other errors should be surfaced
		if !errors.Is(getErr, utils.ErrNotFound) {
			return "", fmt.Errorf("failed to check SecretReference existence: %w", getErr)
		}
		// SecretReference doesn't exist, create it
		if _, createErr := c.ocClient.CreateSecretReference(ctx, location.OrgName, secretRefReq); createErr != nil {
			return "", fmt.Errorf("failed to create SecretReference: %w", createErr)
		}
	} else {
		// SecretReference exists, update it
		if _, updateErr := c.ocClient.UpdateSecretReference(ctx, location.OrgName, secretRefName, secretRefReq); updateErr != nil {
			return "", fmt.Errorf("failed to update SecretReference: %w", updateErr)
		}
	}

	return secretRefName, nil
}

// CreateSecret creates a new secret at the location derived from SecretLocation.
// The SecretReference name is derived from location using SecretRefName().
// Returns the KV path where the secret was stored.
// When ocClient is configured (OpenBao provider), also creates/updates the SecretReference CRD.
func (c *secretManagementClient) CreateSecret(ctx context.Context, location SecretLocation, secretData map[string]string) (string, error) {
	// Convert map to JSON bytes
	data, err := json.Marshal(secretData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal secret data: %w", err)
	}

	// Push the secret - provider derives path/labels from location
	metadata := &SecretMetadata{
		ManagedBy: c.managedBy,
	}
	kvRef, err := c.lowLevelClient.PushSecret(ctx, location, data, metadata)
	if err != nil {
		return "", fmt.Errorf("failed to upsert secret: %w", err)
	}

	// If ocClient is configured, handle SecretReference creation/update
	// (Secret Manager API handles this internally, so ocClient will be nil)
	if c.ocClient != nil {
		// Extract secret keys from the data
		secretKeys := make([]string, 0, len(secretData))
		for key := range secretData {
			secretKeys = append(secretKeys, key)
		}
		secretRefName, err := c.upsertSecretReference(ctx, location, kvRef, secretKeys)
		if err != nil {
			return "", err
		}
		return secretRefName, nil
	}

	return kvRef, nil
}

// PatchSecret merges data with an existing secret (server-side merge).
// Keys in data are added/updated, keys in keysToDelete are removed.
// The SecretReference name is derived from location using SecretRefName().
// Returns the KV path where the secret was stored.
// When ocClient is configured (OpenBao provider), also updates the SecretReference CRD.
func (c *secretManagementClient) PatchSecret(ctx context.Context, location SecretLocation, secretData map[string]string, keysToDelete []string) (string, error) {
	// Build patch data: include updates and set deleted keys to null
	patchData := make(map[string]any)
	for k, v := range secretData {
		patchData[k] = v
	}
	for _, k := range keysToDelete {
		patchData[k] = nil // null signals deletion in JSON Merge Patch
	}

	// Convert to JSON bytes
	data, err := json.Marshal(patchData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal patch data: %w", err)
	}

	metadata := &SecretMetadata{
		ManagedBy: c.managedBy,
	}
	kvPath, err := c.lowLevelClient.PatchSecret(ctx, location, data, metadata)
	if err != nil {
		return "", fmt.Errorf("failed to patch secret: %w", err)
	}

	// If ocClient is configured, update the SecretReference with current keys
	if c.ocClient != nil {
		// Get the updated secret info to retrieve all current keys
		secretInfo, infoErr := c.lowLevelClient.GetSecret(ctx, location)
		if infoErr != nil {
			return "", fmt.Errorf("failed to get secret keys after patch: %w", infoErr)
		}
		_, err = c.upsertSecretReference(ctx, location, kvPath, secretInfo.Keys)
		if err != nil {
			return "", err
		}
	}

	return kvPath, nil
}

// DeleteSecret deletes a secret and its associated SecretReference CRD.
// secretRefName is the name of the SecretReference CR to delete.
// When ocClient is configured (OpenBao provider), also deletes the SecretReference.
func (c *secretManagementClient) DeleteSecret(ctx context.Context, location SecretLocation, secretRefName string) error {
	// Delete the KV secret - provider derives path from location
	metadata := &SecretMetadata{
		ManagedBy: c.managedBy,
	}
	if err := c.lowLevelClient.DeleteSecret(ctx, location, metadata); err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	// If ocClient is configured, also delete the SecretReference
	if c.ocClient != nil {
		if err := c.ocClient.DeleteSecretReference(ctx, location.OrgName, secretRefName); err != nil {
			// Ignore not found errors - the SecretReference may not exist
			if !errors.Is(err, utils.ErrNotFound) {
				return fmt.Errorf("failed to delete SecretReference: %w", err)
			}
		}
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

// GetSecretWithValue retrieves a secret by its KV path including actual values.
// Returns the secret data as a key-value map.
// Returns ErrNotSupported if the provider doesn't support value retrieval.
func (c *secretManagementClient) GetSecretWithValue(ctx context.Context, kvPath string) (map[string]string, error) {
	location, err := ParseKVPath(kvPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse KV path %q: %w", kvPath, err)
	}
	raw, err := c.lowLevelClient.GetSecretWithValue(ctx, location)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret at path %q: %w", kvPath, err)
	}

	var data map[string]string
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal secret data: %w", err)
	}

	return data, nil
}

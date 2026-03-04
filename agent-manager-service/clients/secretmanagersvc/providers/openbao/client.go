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

package openbao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	vault "github.com/hashicorp/vault/api"

	secretmanagersvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/secretmanagersvc"
)

// Client implements the secretmanagersvc.SecretsClient interface for OpenBao/Vault.
type Client struct {
	client  *vault.Client
	path    string
	version string
}

// Ensure Client implements the interface.
var _ secretmanagersvc.SecretsClient = &Client{}

func validateMetadata(metadata *secretmanagersvc.SecretMetadata) error {
	if metadata == nil {
		return fmt.Errorf("secret metadata is required")
	}
	if strings.TrimSpace(metadata.ManagedBy) == "" {
		return fmt.Errorf("secret metadata managedBy is required")
	}
	return nil
}

// PushSecret writes a secret to OpenBao.
func (c *Client) PushSecret(ctx context.Context, key string, value []byte, metadata *secretmanagersvc.SecretMetadata) error {
	if err := validateMetadata(metadata); err != nil {
		return err
	}
	secretPath := c.buildPath(key)
	// Check if secret already exists and verify ownership
	_, err := c.readSecret(ctx, key)
	if err != nil && !errors.Is(err, secretmanagersvc.ErrSecretNotFound) {
		return err
	}

	secretExists := err == nil

	// If secret exists, verify it's managed by the same owner
	if secretExists {
		existingMetadata, err := c.readMetadata(ctx, key)
		if err != nil {
			if errors.Is(err, secretmanagersvc.ErrMetadataNotFound) {
				return secretmanagersvc.ErrNotManaged
			}
			return err
		}
		manager, ok := existingMetadata["managed-by"]
		if !ok || manager != metadata.ManagedBy {
			return secretmanagersvc.ErrNotManaged
		}
	}

	// Prepare secret data - unmarshal JSON to store as flat key-value pairs
	var secretData map[string]interface{}
	if err := json.Unmarshal(value, &secretData); err != nil {
		// If not valid JSON, store as single "value" key
		secretData = map[string]interface{}{
			"value": string(value),
		}
	}

	// Handle KV v1 vs v2
	var secretToPush map[string]interface{}
	if c.version == "v2" {
		secretToPush = map[string]interface{}{
			"data": secretData,
		}

		// Write metadata separately for v2
		metaPath := c.buildMetadataPath(key)
		_, err = c.client.Logical().WriteWithContext(ctx, metaPath, map[string]interface{}{
			"custom_metadata": map[string]string{
				"managed-by": metadata.ManagedBy,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to write metadata: %w", err)
		}
	} else {
		// For v1, include metadata in the secret itself
		secretData["custom_metadata"] = map[string]string{
			"managed-by": metadata.ManagedBy,
		}
		secretToPush = secretData
	}

	_, err = c.client.Logical().WriteWithContext(ctx, secretPath, secretToPush)
	if err != nil {
		return fmt.Errorf("failed to write secret: %w", err)
	}

	return nil
}

// DeleteSecret removes a secret from OpenBao.
func (c *Client) DeleteSecret(ctx context.Context, key string, metadata *secretmanagersvc.SecretMetadata) error {
	if err := validateMetadata(metadata); err != nil {
		return err
	}
	secretPath := c.buildPath(key)
	// Check if secret exists
	_, err := c.readSecret(ctx, key)
	if errors.Is(err, secretmanagersvc.ErrSecretNotFound) {
		return nil // Idempotent - already deleted
	}
	if err != nil {
		return err
	}

	// Verify ownership
	existingMetadata, err := c.readMetadata(ctx, key)
	if err != nil {
		if errors.Is(err, secretmanagersvc.ErrMetadataNotFound) {
			return nil // No metadata = not managed by us, skip deletion
		}
		return err
	}
	manager, ok := existingMetadata["managed-by"]
	if !ok || manager != metadata.ManagedBy {
		return nil // Not managed by the specified owner, skip deletion
	}

	// Delete the secret
	_, err = c.client.Logical().DeleteWithContext(ctx, secretPath)
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	// For v2, also delete metadata
	if c.version == "v2" {
		metaPath := c.buildMetadataPath(key)
		_, err = c.client.Logical().DeleteWithContext(ctx, metaPath)
		if err != nil {
			return fmt.Errorf("failed to delete metadata: %w", err)
		}
	}

	return nil
}

// GetSecret retrieves a secret from OpenBao.
func (c *Client) GetSecret(ctx context.Context, key string) ([]byte, error) {
	return c.readSecret(ctx, key)
}

// Close cleans up resources.
func (c *Client) Close(ctx context.Context) error {
	// Vault client doesn't require explicit cleanup
	return nil
}

// readSecret reads a secret from OpenBao and returns the value.
func (c *Client) readSecret(ctx context.Context, key string) ([]byte, error) {
	secretPath := c.buildPath(key)

	secret, err := c.client.Logical().ReadWithContext(ctx, secretPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read secret: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return nil, secretmanagersvc.ErrSecretNotFound
	}

	// Handle v2 response (data is nested under "data" key)
	data := secret.Data
	if c.version == "v2" {
		dataMap, ok := data["data"].(map[string]interface{})
		if !ok {
			return nil, secretmanagersvc.ErrSecretNotFound
		}
		data = dataMap
	}

	value, ok := data["value"]
	if !ok {
		// If there's no "value" key, return the entire data as JSON
		jsonBytes, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal secret data: %w", err)
		}
		return jsonBytes, nil
	}

	switch v := value.(type) {
	case string:
		return []byte(v), nil
	case []byte:
		return v, nil
	default:
		jsonBytes, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal secret value: %w", err)
		}
		return jsonBytes, nil
	}
}

// readMetadata reads the custom metadata for a secret.
func (c *Client) readMetadata(ctx context.Context, key string) (map[string]string, error) {
	if c.version == "v1" {
		// For v1, metadata is stored in the secret itself
		secretPath := c.buildPath(key)
		secret, err := c.client.Logical().ReadWithContext(ctx, secretPath)
		if err != nil {
			return nil, err
		}
		if secret == nil || secret.Data == nil {
			return nil, secretmanagersvc.ErrMetadataNotFound
		}

		if customMeta, ok := secret.Data["custom_metadata"].(map[string]interface{}); ok {
			result := make(map[string]string)
			for k, v := range customMeta {
				if str, ok := v.(string); ok {
					result[k] = str
				}
			}
			return result, nil
		}
		return nil, secretmanagersvc.ErrMetadataNotFound
	}

	// For v2, read from metadata endpoint
	metaPath := c.buildMetadataPath(key)
	secret, err := c.client.Logical().ReadWithContext(ctx, metaPath)
	if err != nil {
		return nil, err
	}

	if secret == nil || secret.Data == nil {
		return nil, secretmanagersvc.ErrMetadataNotFound
	}

	if customMeta, ok := secret.Data["custom_metadata"].(map[string]interface{}); ok {
		result := make(map[string]string)
		for k, v := range customMeta {
			if str, ok := v.(string); ok {
				result[k] = str
			}
		}
		return result, nil
	}

	return nil, secretmanagersvc.ErrMetadataNotFound
}

// buildPath constructs the path for reading/writing secrets.
func (c *Client) buildPath(key string) string {
	if c.version == "v2" {
		return path.Join(c.path, "data", key)
	}
	return path.Join(c.path, key)
}

// buildMetadataPath constructs the path for reading/writing metadata (v2 only).
func (c *Client) buildMetadataPath(key string) string {
	return path.Join(c.path, "metadata", key)
}

// buildListPath constructs the path for listing secrets.
func (c *Client) buildListPath(prefix string) string {
	if c.version == "v2" {
		return path.Join(c.path, "metadata", prefix)
	}
	return path.Join(c.path, prefix)
}

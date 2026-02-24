package openbao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"

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

// PushSecret writes a secret to OpenBao.
func (c *Client) PushSecret(ctx context.Context, key string, value []byte, metadata *secretmanagersvc.SecretMetadata) error {
	secretPath := c.buildPath(key)

	// Check if secret already exists and verify ownership
	_, err := c.readSecret(ctx, key)
	if err != nil && !errors.Is(err, secretmanagersvc.ErrSecretNotFound) {
		return err
	}

	secretExists := err == nil

	// If secret exists, verify it's managed by us
	if secretExists {
		existingMetadata, err := c.readMetadata(ctx, key)
		if err != nil {
			return err
		}
		manager, ok := existingMetadata["managed-by"]
		if ok && manager != ManagedByValue {
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
				"managed-by": ManagedByValue,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to write metadata: %w", err)
		}
	} else {
		// For v1, include metadata in the secret itself
		secretData["custom_metadata"] = map[string]string{
			"managed-by": ManagedByValue,
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
func (c *Client) DeleteSecret(ctx context.Context, key string) error {
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
	metadata, err := c.readMetadata(ctx, key)
	if err != nil {
		return nil // If we can't read metadata, assume it's not managed
	}

	manager, ok := metadata["managed-by"]
	if !ok || manager != ManagedByValue {
		return nil // Don't delete secrets not managed by us
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

// SecretExists checks if a secret exists.
func (c *Client) SecretExists(ctx context.Context, key string) (bool, error) {
	_, err := c.readSecret(ctx, key)
	if errors.Is(err, secretmanagersvc.ErrSecretNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetAllSecrets retrieves all secrets matching the prefix.
func (c *Client) GetAllSecrets(ctx context.Context, prefix string) (map[string][]byte, error) {
	listPath := c.buildListPath(prefix)

	secret, err := c.client.Logical().ListWithContext(ctx, listPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return make(map[string][]byte), nil
	}

	keys, ok := secret.Data["keys"].([]interface{})
	if !ok {
		return make(map[string][]byte), nil
	}

	result := make(map[string][]byte)
	for _, k := range keys {
		keyStr, ok := k.(string)
		if !ok {
			continue
		}
		fullKey := path.Join(prefix, keyStr)
		value, err := c.readSecret(ctx, fullKey)
		if err != nil {
			continue // Skip secrets we can't read
		}
		result[fullKey] = value
	}

	return result, nil
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
		if err != nil || secret == nil {
			return nil, err
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
		return nil, nil
	}

	// For v2, read from metadata endpoint
	metaPath := c.buildMetadataPath(key)
	secret, err := c.client.Logical().ReadWithContext(ctx, metaPath)
	if err != nil {
		return nil, err
	}

	if secret == nil || secret.Data == nil {
		return nil, nil
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

	return nil, nil
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

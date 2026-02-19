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
	"fmt"
	"path"
	"time"

	vault "github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
	"github.com/wso2/ai-agent-management-platform/secret-management-service/providers"
)

func init() {
	providers.RegisterProvider(providers.ProviderOpenBao, func(cfg providers.Config) (providers.Provider, error) {
		return NewProvider(cfg)
	})
	providers.RegisterProvider(providers.ProviderHashiCorpVault, func(cfg providers.Config) (providers.Provider, error) {
		return NewProvider(cfg)
	})
}

// Provider implements the providers.Provider interface for OpenBao/Vault
type Provider struct {
	client    *vault.Client
	mountPath string
	namespace string
}

// NewProvider creates a new OpenBao provider
func NewProvider(cfg providers.Config) (*Provider, error) {
	clientCfg := vault.DefaultConfiguration()
	clientCfg.Address = cfg.Address

	if cfg.TLSConfig != nil && cfg.TLSConfig.Insecure {
		clientCfg.TLS.InsecureSkipVerify = true
	}

	client, err := vault.New(vault.WithConfiguration(clientCfg))
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenBao client: %w", err)
	}

	if err := client.SetToken(cfg.Token); err != nil {
		return nil, fmt.Errorf("failed to set token: %w", err)
	}

	if cfg.Namespace != "" {
		if err := client.SetNamespace(cfg.Namespace); err != nil {
			return nil, fmt.Errorf("failed to set namespace: %w", err)
		}
	}

	mountPath := cfg.MountPath
	if mountPath == "" {
		mountPath = "secret"
	}

	return &Provider{
		client:    client,
		mountPath: mountPath,
		namespace: cfg.Namespace,
	}, nil
}

// CreateSecret creates a new secret at the specified path
func (p *Provider) CreateSecret(ctx context.Context, req providers.CreateSecretRequest) (*providers.Secret, error) {
	// Convert map[string]string to map[string]interface{}
	data := make(map[string]interface{})
	for k, v := range req.Data {
		data[k] = v
	}

	_, err := p.client.Secrets.KvV2Write(ctx, req.Path, schema.KvV2WriteRequest{
		Data: data,
	}, vault.WithMountPath(p.mountPath))
	if err != nil {
		return nil, fmt.Errorf("failed to create secret: %w", err)
	}

	// Return the created secret
	return p.GetSecret(ctx, req.Path, nil)
}

// GetSecret retrieves a secret from the specified path
func (p *Provider) GetSecret(ctx context.Context, secretPath string, version *int) (*providers.Secret, error) {
	var opts []vault.RequestOption
	opts = append(opts, vault.WithMountPath(p.mountPath))

	resp, err := p.client.Secrets.KvV2Read(ctx, secretPath, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to read secret: %w", err)
	}

	// Convert data to map[string]string
	data := make(map[string]string)
	if resp.Data.Data != nil {
		for k, v := range resp.Data.Data {
			if strVal, ok := v.(string); ok {
				data[k] = strVal
			} else {
				data[k] = fmt.Sprintf("%v", v)
			}
		}
	}

	var createdTime, updatedTime time.Time
	if resp.Data.Metadata != nil {
		if ct, ok := resp.Data.Metadata["created_time"].(string); ok {
			createdTime, _ = time.Parse(time.RFC3339Nano, ct)
		}
		updatedTime = createdTime // KV v2 doesn't have separate update time
	}

	ver := 0
	if resp.Data.Metadata != nil {
		if v, ok := resp.Data.Metadata["version"].(float64); ok {
			ver = int(v)
		}
	}

	return &providers.Secret{
		Path: secretPath,
		Data: data,
		Metadata: providers.SecretMetadata{
			Path:        secretPath,
			Version:     ver,
			CreatedTime: createdTime,
			UpdatedTime: updatedTime,
		},
	}, nil
}

// UpdateSecret updates an existing secret at the specified path
func (p *Provider) UpdateSecret(ctx context.Context, secretPath string, req providers.UpdateSecretRequest) (*providers.Secret, error) {
	// Convert map[string]string to map[string]interface{}
	data := make(map[string]interface{})
	for k, v := range req.Data {
		data[k] = v
	}

	_, err := p.client.Secrets.KvV2Write(ctx, secretPath, schema.KvV2WriteRequest{
		Data: data,
	}, vault.WithMountPath(p.mountPath))
	if err != nil {
		return nil, fmt.Errorf("failed to update secret: %w", err)
	}

	return p.GetSecret(ctx, secretPath, nil)
}

// DeleteSecret deletes a secret at the specified path
func (p *Provider) DeleteSecret(ctx context.Context, secretPath string) error {
	_, err := p.client.Secrets.KvV2DeleteMetadataAndAllVersions(ctx, secretPath, vault.WithMountPath(p.mountPath))
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}
	return nil
}

// ListSecrets lists secrets under the specified path prefix
func (p *Provider) ListSecrets(ctx context.Context, pathPrefix string) ([]providers.SecretMetadata, error) {
	resp, err := p.client.Secrets.KvV2List(ctx, pathPrefix, vault.WithMountPath(p.mountPath))
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	var secrets []providers.SecretMetadata
	if resp.Data.Keys != nil {
		for _, key := range resp.Data.Keys {
			secrets = append(secrets, providers.SecretMetadata{
				Path: path.Join(pathPrefix, key),
			})
		}
	}

	return secrets, nil
}

// GetSecretVersions retrieves version history for a secret
func (p *Provider) GetSecretVersions(ctx context.Context, secretPath string) ([]providers.SecretVersion, error) {
	resp, err := p.client.Secrets.KvV2ReadMetadata(ctx, secretPath, vault.WithMountPath(p.mountPath))
	if err != nil {
		return nil, fmt.Errorf("failed to read secret metadata: %w", err)
	}

	var versions []providers.SecretVersion
	if resp.Data.Versions != nil {
		for verStr, verData := range resp.Data.Versions {
			ver := 0
			fmt.Sscanf(verStr, "%d", &ver)

			var createdTime time.Time
			deleted := false

			if verMap, ok := verData.(map[string]interface{}); ok {
				if ct, ok := verMap["created_time"].(string); ok {
					createdTime, _ = time.Parse(time.RFC3339Nano, ct)
				}
				if dt, ok := verMap["deletion_time"].(string); ok && dt != "" {
					deleted = true
				}
				if d, ok := verMap["destroyed"].(bool); ok && d {
					deleted = true
				}
			}

			versions = append(versions, providers.SecretVersion{
				Version:     ver,
				CreatedTime: createdTime,
				Deleted:     deleted,
			})
		}
	}

	return versions, nil
}

// GetProviderType returns the provider type
func (p *Provider) GetProviderType() providers.ProviderType {
	return providers.ProviderOpenBao
}

// HealthCheck checks if the provider is healthy
func (p *Provider) HealthCheck(ctx context.Context) error {
	_, err := p.client.System.ReadHealthStatus(ctx)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	return nil
}

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

package providers

import (
	"context"
	"time"
)

// ProviderType represents the type of KV store provider
type ProviderType string

const (
	ProviderOpenBao         ProviderType = "openbao"
	ProviderHashiCorpVault  ProviderType = "hashicorp-vault"
	ProviderAWSSecretsManager ProviderType = "aws-secrets-manager"
)

// Provider defines the interface for KV store providers
type Provider interface {
	// CreateSecret creates a new secret at the specified path
	CreateSecret(ctx context.Context, req CreateSecretRequest) (*Secret, error)

	// GetSecret retrieves a secret from the specified path
	// If version is nil, returns the latest version
	GetSecret(ctx context.Context, path string, version *int) (*Secret, error)

	// UpdateSecret updates an existing secret at the specified path
	UpdateSecret(ctx context.Context, path string, req UpdateSecretRequest) (*Secret, error)

	// DeleteSecret deletes a secret at the specified path
	DeleteSecret(ctx context.Context, path string) error

	// ListSecrets lists secrets under the specified path prefix
	ListSecrets(ctx context.Context, pathPrefix string) ([]SecretMetadata, error)

	// GetSecretVersions retrieves version history for a secret
	GetSecretVersions(ctx context.Context, path string) ([]SecretVersion, error)

	// GetProviderType returns the provider type
	GetProviderType() ProviderType

	// HealthCheck checks if the provider is healthy
	HealthCheck(ctx context.Context) error
}

// Secret represents a secret with its data and metadata
type Secret struct {
	Path     string            `json:"path"`
	Data     map[string]string `json:"data"`
	Metadata SecretMetadata    `json:"metadata"`
}

// SecretMetadata contains metadata about a secret
type SecretMetadata struct {
	Path           string    `json:"path"`
	Version        int       `json:"version"`
	CreatedTime    time.Time `json:"createdTime"`
	UpdatedTime    time.Time `json:"updatedTime"`
	OrganizationID string    `json:"organizationId,omitempty"`
}

// SecretVersion represents a version of a secret
type SecretVersion struct {
	Version     int       `json:"version"`
	CreatedTime time.Time `json:"createdTime"`
	Deleted     bool      `json:"deleted"`
}

// CreateSecretRequest for creating a new secret
type CreateSecretRequest struct {
	Path           string            `json:"path"`
	Data           map[string]string `json:"data"`
	OrganizationID string            `json:"organizationId,omitempty"`
}

// UpdateSecretRequest for updating a secret
type UpdateSecretRequest struct {
	Data map[string]string `json:"data"`
}

// Config holds provider configuration
type Config struct {
	Address   string    // KV store address
	Token     string    // Authentication token
	Namespace string    // Namespace/tenant (for multi-tenancy)
	MountPath string    // KV engine mount path (e.g., "secret/")
	TLSConfig *TLSConfig // TLS configuration
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	CACert     string
	ClientCert string
	ClientKey  string
	Insecure   bool
}

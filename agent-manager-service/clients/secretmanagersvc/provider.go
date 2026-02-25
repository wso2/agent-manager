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

package secretmanagersvc

import (
	"context"
)

// StoreCapabilities defines what operations a provider supports.
type StoreCapabilities string

const (
	// StoreCapabilityReadOnly indicates the provider can only read secrets.
	StoreCapabilityReadOnly StoreCapabilities = "ReadOnly"
	// StoreCapabilityWriteOnly indicates the provider can only write secrets.
	StoreCapabilityWriteOnly StoreCapabilities = "WriteOnly"
	// StoreCapabilityReadWrite indicates the provider can read and write secrets.
	StoreCapabilityReadWrite StoreCapabilities = "ReadWrite"
)

// Provider creates SecretsClient instances for a specific backend.
// This interface follows the external-secrets provider pattern.
type Provider interface {
	// NewClient creates a new SecretsClient for the given configuration.
	NewClient(ctx context.Context, config *StoreConfig) (SecretsClient, error)

	// ValidateConfig validates the provider configuration.
	ValidateConfig(config *StoreConfig) error

	// Capabilities returns the provider's capabilities (ReadOnly, WriteOnly, ReadWrite).
	Capabilities() StoreCapabilities
}

// SecretsClient performs secret operations on a backend.
// This interface follows the external-secrets SecretsClient pattern.
type SecretsClient interface {
	// PushSecret writes a secret to the backend.
	// If the secret already exists, it will be updated.
	// Metadata is used for ownership tracking (managed-by).
	PushSecret(ctx context.Context, key string, value []byte, metadata *SecretMetadata) error

	// DeleteSecret removes a secret from the backend.
	// Returns nil if the secret doesn't exist (idempotent).
	// Only deletes secrets where the managed-by metadata matches the provided metadata.
	DeleteSecret(ctx context.Context, key string, metadata *SecretMetadata) error

	// GetSecret retrieves a secret from the backend.
	// Returns ErrSecretNotFound if the secret doesn't exist.
	GetSecret(ctx context.Context, key string) ([]byte, error)

	// Close cleans up any resources held by the client.
	Close(ctx context.Context) error
}

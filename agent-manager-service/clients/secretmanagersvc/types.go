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

import "errors"

// ErrSecretNotFound is returned when a secret does not exist.
var ErrSecretNotFound = errors.New("secret not found")

// ErrNotManaged is returned when attempting to delete a secret not managed by this client.
var ErrNotManaged = errors.New("secret not managed by this client")

// ErrMetadataNotFound is returned when secret metadata does not exist.
var ErrMetadataNotFound = errors.New("secret metadata not found")

// ErrNotSupported is returned when an operation is not supported by the provider.
var ErrNotSupported = errors.New("operation not supported by this provider")

// SecretMetadata contains metadata for a secret.
type SecretMetadata struct {
	// ManagedBy identifies who manages this secret.
	// Used to prevent accidental deletion of secrets created outside this client.
	ManagedBy string `json:"managedBy,omitempty"`

	// Labels are optional key-value pairs for additional metadata.
	Labels map[string]string `json:"labels,omitempty"`
}

// SecretInfo contains information about a secret without the actual values.
type SecretInfo struct {
	// ID is the unique identifier for the secret (e.g., secretReferenceName).
	ID string `json:"id"`

	// Name is the logical name of the secret.
	Name string `json:"name,omitempty"`

	// Keys is the list of keys available in the secret (without values).
	Keys []string `json:"keys,omitempty"`

	// Labels are optional key-value pairs for additional metadata.
	Labels map[string]string `json:"labels,omitempty"`

	// CreatedAt is the timestamp when the secret was created.
	CreatedAt string `json:"createdAt,omitempty"`
}

// StoreConfig holds configuration for secret store backends.
type StoreConfig struct {
	// Provider is the name of the provider to use (e.g., "openbao", "vault", "aws").
	Provider string `json:"provider"`

	// OpenBao contains OpenBao/Vault-specific configuration.
	OpenBao *OpenBaoConfig `json:"openbao,omitempty"`
}

// OpenBaoConfig contains configuration for OpenBao/Vault.
// Only KV v2 secrets engine is supported.
type OpenBaoConfig struct {
	// Server is the OpenBao server address (e.g., "https://openbao.example.com").
	Server string `json:"server"`

	// Path is the mount path for the KV secrets engine (e.g., "secret").
	Path string `json:"path"`

	// Auth contains authentication configuration.
	Auth *OpenBaoAuth `json:"auth"`
}

// OpenBaoAuth contains authentication configuration for OpenBao.
type OpenBaoAuth struct {
	// Token is a static token for authentication.
	Token string `json:"token,omitempty"`
}

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
	"errors"

	"gorm.io/gorm"

	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
)

// ErrSecretNotFound is returned when a secret does not exist.
var ErrSecretNotFound = errors.New("secret not found")

// ErrNotManaged is returned when attempting to delete a secret not managed by this client.
var ErrNotManaged = errors.New("secret not managed by this client")

// ErrMetadataNotFound is returned when secret metadata does not exist.
var ErrMetadataNotFound = errors.New("secret metadata not found")

// ErrNotSupported is returned when an operation is not supported by the provider.
var ErrNotSupported = errors.New("operation not supported by this provider")

// ErrReferenceNameTaken is returned when a secret cannot be created because its
// SecretReference name is already taken by a reference the secret API cannot see
// or update — one created outside this client, directly as a SecretReference.
// The caller can delete that reference and create the secret again.
var ErrReferenceNameTaken = errors.New("secret reference name is taken by a reference this client cannot manage")

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
	// Provider is the secret store provider name.
	Provider string `json:"provider"`

	// InHouse configures the in-house secret store.
	InHouse *InHouseConfig `json:"-"`
}

// InHouseConfig configures the in-house secret store: values are encrypted with
// EncryptionKey and stored in DB, and each secret is published as a
// SecretReference through Client.
type InHouseConfig struct {
	// DB is the agent manager database holding the secrets table.
	DB *gorm.DB
	// EncryptionKey is the 32-byte AES-256 key secrets are sealed with.
	EncryptionKey []byte
	// Client manages the SecretReference CRs that expose secrets to workloads.
	Client occlient.OpenChoreoClient
	// RefreshInterval is set on each SecretReference (e.g. "1h").
	RefreshInterval string
}

// SecretResolver returns the plaintext value of one key of a stored secret.
// Only the internal secret store endpoint uses it.
type SecretResolver interface {
	Resolve(ctx context.Context, ouID, name, key string) (string, error)
}

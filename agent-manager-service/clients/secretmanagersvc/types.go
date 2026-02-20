package secretmanagersvc

import "errors"

// ErrSecretNotFound is returned when a secret does not exist.
var ErrSecretNotFound = errors.New("secret not found")

// ErrNotManaged is returned when attempting to delete a secret not managed by this client.
var ErrNotManaged = errors.New("secret not managed by this client")

// SecretMetadata contains metadata for a secret.
type SecretMetadata struct {
	// ManagedBy identifies who manages this secret.
	// Used to prevent accidental deletion of secrets created outside this client.
	ManagedBy string `json:"managedBy,omitempty"`

	// Labels are optional key-value pairs for additional metadata.
	Labels map[string]string `json:"labels,omitempty"`
}

// StoreConfig holds configuration for secret store backends.
type StoreConfig struct {
	// Provider is the name of the provider to use (e.g., "openbao", "vault", "aws").
	Provider string `json:"provider"`

	// OpenBao contains OpenBao/Vault-specific configuration.
	OpenBao *OpenBaoConfig `json:"openbao,omitempty"`
}

// OpenBaoConfig contains configuration for OpenBao/Vault.
type OpenBaoConfig struct {
	// Server is the OpenBao server address (e.g., "https://openbao.example.com").
	Server string `json:"server"`

	// Path is the mount path for the KV secrets engine (e.g., "secret").
	Path string `json:"path"`

	// Version is the KV secrets engine version ("v1" or "v2").
	// Defaults to "v2" if not specified.
	Version string `json:"version,omitempty"`

	// Auth contains authentication configuration.
	Auth *OpenBaoAuth `json:"auth"`
}

// OpenBaoAuth contains authentication configuration for OpenBao.
type OpenBaoAuth struct {
	// Token is a static token for authentication.
	Token string `json:"token,omitempty"`
}

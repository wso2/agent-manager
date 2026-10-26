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

package models

import (
	"time"

	"github.com/google/uuid"
)

// API key purpose distinguishes the lifecycle/ownership of a stored key:
//   - UserManaged: explicitly created and managed by users (name + expiry),
//     e.g. the agent Credentials tab, LLM provider keys, and MCP proxy keys.
//     These are surfaced in user-facing key listings.
//   - Test: short-lived key minted by the console for the Try-It flow.
//   - ConsoleManaged: created and used internally by the console and never
//     shown to users, e.g. the keys an LLM proxy mints for itself and for its
//     upstream LLM provider during provisioning.
const (
	APIKeyPurposeUserManaged    = 1
	APIKeyPurposeTest           = 2
	APIKeyPurposeConsoleManaged = 3
	// APIKeyTestKeyPrefix is the name prefix for per-user console test keys.
	// Each user gets their own row (prefix + truncated SHA-256 of their JWT sub),
	// so concurrent sessions across users don't invalidate each other's keys.
	APIKeyTestKeyPrefix = "console-test-"
	// MinAPIKeyNameLength is the shortest key name the gateway accepts. It
	// rejects shorter names asynchronously, after the API has answered.
	MinAPIKeyNameLength = 3
)

// StoredAPIKey represents an API key persisted in the database for gateway bulk-sync
type StoredAPIKey struct {
	UUID         uuid.UUID  `gorm:"column:uuid;primaryKey" json:"uuid"`
	Name         string     `gorm:"column:name" json:"name"`
	DisplayName  string     `gorm:"column:display_name" json:"displayName"`
	ArtifactUUID uuid.UUID  `gorm:"column:artifact_uuid" json:"artifactUuid"`
	OUID         string     `gorm:"column:ou_id" json:"organizationName"`
	APIKeyHash   string     `gorm:"column:api_key_hash" json:"-"`
	MaskedAPIKey string     `gorm:"column:masked_api_key" json:"maskedApiKey"`
	Status       string     `gorm:"column:status" json:"status"`
	Purpose      int        `gorm:"column:purpose;not null;default:1" json:"purpose"`
	CreatedAt    time.Time  `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt    time.Time  `gorm:"column:updated_at" json:"updatedAt"`
	ExpiresAt    *time.Time `gorm:"column:expires_at" json:"expiresAt,omitempty"`
}

// TableName returns the table name for the StoredAPIKey model
func (StoredAPIKey) TableName() string {
	return "api_keys"
}

// RotateAPIKeyRequest represents the optional parameters when rotating an API key
type RotateAPIKeyRequest struct {
	// DisplayName is the optional updated display name for the API key
	DisplayName *string `json:"displayName,omitempty"`

	// ExpiresAt is the optional new expiration time in ISO 8601 format
	ExpiresAt *string `json:"expiresAt,omitempty"`
}

// CreateAPIKeyRequest represents the request to create an API key for LLM provider or proxy
type CreateAPIKeyRequest struct {
	// Name is the unique identifier for this API key (optional; if omitted, generated from displayName)
	Name string `json:"name,omitempty"`

	// DisplayName is the display name of the API key
	DisplayName string `json:"displayName,omitempty"`

	// Purpose marks the key as user-managed, a console Try-It test key, or
	// console-managed. Zero defaults to user-managed so user-facing call sites
	// (which never set it) are unaffected; console-internal sites set it explicitly.
	Purpose int `json:"purpose,omitempty"`

	// ExpiresAt is the optional expiration time in ISO 8601 format
	ExpiresAt *string `json:"expiresAt,omitempty"`
}

// IssueTestAPIKeyResponse is returned by the test-key issuance endpoint.
// Includes ExpiresAt so the console can schedule rotation before expiry.
type IssueTestAPIKeyResponse struct {
	Status    string `json:"status"`
	Message   string `json:"message"`
	KeyID     string `json:"keyId,omitempty"`
	APIKey    string `json:"apiKey,omitempty"`
	ExpiresAt string `json:"expiresAt"`

	// GatewayConnected reports whether every gateway serving the agent's
	// environment had a live websocket connection when the key was issued.
	// When false, the key is stored but only becomes usable after the gateway
	// reconnects and syncs.
	GatewayConnected *bool `json:"gatewayConnected,omitempty"`
}

// APIKeyInfo is a masked, read-only view of a stored API key for listing.
// The plain key value is never returned; only the masked representation.
type APIKeyInfo struct {
	// Name is the unique name of the API key
	Name string `json:"name"`

	// DisplayName is the display name of the API key
	DisplayName string `json:"displayName"`

	// MaskedAPIKey is the masked representation of the API key for display
	MaskedAPIKey string `json:"maskedApiKey"`

	// Status indicates whether the key is active
	Status string `json:"status"`

	// CreatedAt is the creation timestamp in RFC3339 format
	CreatedAt string `json:"createdAt"`

	// ExpiresAt is the optional expiration time in RFC3339 format
	ExpiresAt *string `json:"expiresAt,omitempty"`
}

// ListAPIKeysResponse represents the response when listing an artifact's API keys.
type ListAPIKeysResponse struct {
	// Keys is the list of masked API keys
	Keys []APIKeyInfo `json:"keys"`
}

// ToUserManagedAPIKeyInfos maps stored API keys to their masked, user-facing
// representation. Only user-managed keys are surfaced; console-managed and test
// keys are hidden. Timestamps are formatted as RFC3339.
func ToUserManagedAPIKeyInfos(stored []StoredAPIKey) []APIKeyInfo {
	keys := make([]APIKeyInfo, 0, len(stored))
	for _, k := range stored {
		if k.Purpose != APIKeyPurposeUserManaged {
			continue
		}
		info := APIKeyInfo{
			Name:         k.Name,
			DisplayName:  k.DisplayName,
			MaskedAPIKey: k.MaskedAPIKey,
			Status:       k.Status,
			CreatedAt:    k.CreatedAt.Format(time.RFC3339),
		}
		if k.ExpiresAt != nil {
			expiresAt := k.ExpiresAt.Format(time.RFC3339)
			info.ExpiresAt = &expiresAt
		}
		keys = append(keys, info)
	}
	return keys
}

// CreateAPIKeyResponse represents the response after creating an API key
type CreateAPIKeyResponse struct {
	// Status indicates the result of the operation ("success" or "error")
	Status string `json:"status"`

	// Message provides additional details about the operation result
	Message string `json:"message"`

	// KeyID is the unique identifier of the generated key
	KeyID string `json:"keyId,omitempty"`

	// APIKey is the generated API key value (returned only once)
	APIKey string `json:"apiKey,omitempty"`

	// GatewayConnected reports whether every target gateway had a live
	// websocket connection to the control plane when the key was written.
	// When false, the key is stored but only becomes usable after the gateway
	// reconnects and syncs. Nil when the issuing path does not check.
	GatewayConnected *bool `json:"gatewayConnected,omitempty"`
}

// APIKeyCreatedEvent represents the event payload for "apikey.created" event type
type APIKeyCreatedEvent struct {
	// UUID is the unique identifier for the API key (UUIDv7)
	UUID string `json:"uuid"`

	// APIID identifies the LLM provider or proxy this key belongs to
	APIID string `json:"apiId"`

	// Name is the unique name of the API key
	Name string `json:"name"`

	// DisplayName is the display name of the API key
	DisplayName string `json:"displayName"`

	// ApiKeyHashes is a JSON string of hashed API key values keyed by algorithm
	// e.g. {"sha256": "<hex_hash>"}
	ApiKeyHashes string `json:"apiKeyHashes"`

	// MaskedApiKey is the masked representation of the API key for display
	MaskedApiKey string `json:"maskedApiKey"`

	// Operations specifies which operations this key can access
	Operations string `json:"operations"`

	// ExpiresAt is the optional expiration time in ISO 8601 format
	ExpiresAt *string `json:"expiresAt,omitempty"`

	// CreatedAt is the creation timestamp in RFC3339 format
	CreatedAt string `json:"createdAt"`

	// UpdatedAt is the last update timestamp in RFC3339 format
	UpdatedAt string `json:"updatedAt"`
}

// APIKeyRevokedEvent represents the event payload for "apikey.revoked" event type
type APIKeyRevokedEvent struct {
	// APIID identifies the LLM provider or proxy this key belongs to
	APIID string `json:"apiId"`

	// KeyName is the unique name of the API key that was revoked
	KeyName string `json:"keyName"`
}

// APIKeyUpdatedEvent represents the event payload for "apikey.updated" event type
type APIKeyUpdatedEvent struct {
	// UUID is the stable identifier for the key being updated, matching the UUID
	// used in the original "apikey.created" event. The gateway controller's
	// replica-sync handler keys its in-memory store on (artifactUUID, keyName, uuid);
	// sending a random UUID here causes "already exists with a different ID" on every
	// rotation, which prevents the rotated key from ever being validated.
	UUID string `json:"uuid,omitempty"`

	// APIID identifies the LLM provider or proxy this key belongs to
	APIID string `json:"apiId"`

	// KeyName is the unique name of the API key being updated
	KeyName string `json:"keyName"`

	// ApiKeyHashes is a JSON string of hashed API key values keyed by algorithm
	// e.g. {"sha256": "<hex_hash>"}
	ApiKeyHashes string `json:"apiKeyHashes"`

	// MaskedApiKey is the masked representation of the API key for display
	MaskedApiKey string `json:"maskedApiKey"`

	// DisplayName is the optional updated display name of the API key
	DisplayName string `json:"displayName,omitempty"`

	// Operations specifies which operations this key can access
	Operations string `json:"operations,omitempty"`

	// ExpiresAt is the optional new expiration time in ISO 8601 format
	ExpiresAt *string `json:"expiresAt,omitempty"`

	// UpdatedAt is the last update timestamp in RFC3339 format
	UpdatedAt string `json:"updatedAt"`
}

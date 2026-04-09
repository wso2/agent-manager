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

// StoredAPIKey represents an API key persisted in the database for gateway bulk-sync
type StoredAPIKey struct {
	UUID             uuid.UUID  `gorm:"column:uuid;primaryKey" json:"uuid"`
	Name             string     `gorm:"column:name" json:"name"`
	ArtifactUUID     uuid.UUID  `gorm:"column:artifact_uuid" json:"artifactUuid"`
	OrganizationName string     `gorm:"column:organization_name" json:"organizationName"`
	APIKeyHash       string     `gorm:"column:api_key_hash" json:"-"`
	MaskedAPIKey     string     `gorm:"column:masked_api_key" json:"maskedApiKey"`
	Status           string     `gorm:"column:status" json:"status"`
	CreatedAt        time.Time  `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt        time.Time  `gorm:"column:updated_at" json:"updatedAt"`
	ExpiresAt        *time.Time `gorm:"column:expires_at" json:"expiresAt,omitempty"`
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

	// ExpiresAt is the optional expiration time in ISO 8601 format
	ExpiresAt *string `json:"expiresAt,omitempty"`
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

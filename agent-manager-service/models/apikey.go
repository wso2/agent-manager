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
	// APIID identifies the LLM provider or proxy this key belongs to
	APIID string `json:"apiId"`

	// Name is the unique name of the API key
	Name string `json:"name"`

	// DisplayName is the display name of the API key
	DisplayName string `json:"displayName"`

	// APIKey is the plain API key value (hashing happens in the gateway)
	APIKey string `json:"apiKey"`

	// Operations specifies which operations this key can access
	Operations []string `json:"operations"`

	// ExpiresAt is the optional expiration time in ISO 8601 format
	ExpiresAt *string `json:"expiresAt,omitempty"`
}

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

package utils

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"github.com/wso2/agent-manager/agent-manager-service/pkg/aesgcm"
	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrInvalidCredentials is returned when credentials are nil
	ErrInvalidCredentials = errors.New("credentials cannot be nil")
	// ErrInvalidAPIKey is returned when API key verification fails
	ErrInvalidAPIKey = errors.New("invalid API key")
)

const (
	// APIKeyHashCost is the bcrypt cost factor for API key hashing
	APIKeyHashCost = bcrypt.DefaultCost
)

// GenerateEncryptionKey generates a cryptographically secure random key for AES-256-GCM.
// This function should be used to generate a new encryption key during initial setup.
// The generated key should be stored securely (e.g., in a key management service).
func GenerateEncryptionKey() ([]byte, error) {
	key := make([]byte, aesgcm.KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}
	return key, nil
}

// HashAPIKey hashes an API key using bcrypt for secure storage.
// The resulting hash should be stored in the database, not the plain API key.
func HashAPIKey(apiKey string) ([]byte, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(apiKey), APIKeyHashCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash API key: %w", err)
	}
	return hash, nil
}

// VerifyAPIKey verifies a plain API key against a stored hash.
// Returns nil if the key matches, ErrInvalidAPIKey otherwise.
func VerifyAPIKey(apiKey string, hash []byte) error {
	err := bcrypt.CompareHashAndPassword(hash, []byte(apiKey))
	if err != nil {
		return ErrInvalidAPIKey
	}
	return nil
}

// GenerateAPIKeyToken generates a base64-encoded token containing the API key.
// This is used for returning the API key in the registration response.
func GenerateAPIKeyToken(apiKey string) string {
	return base64.StdEncoding.EncodeToString([]byte(apiKey))
}

// ParseAPIKeyToken parses a base64-encoded API key token.
func ParseAPIKeyToken(token string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return "", fmt.Errorf("invalid API key token format: %w", err)
	}
	return string(decoded), nil
}

// SecretRefKeys is the key ring for values sealed with the core's encryption key: the current key first, then the
// retained keys a rotation keeps readable.
type SecretRefKeys [][]byte

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
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/bcrypt"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

var (
	// ErrInvalidCiphertext is returned when decryption fails due to invalid ciphertext
	ErrInvalidCiphertext = errors.New("invalid ciphertext")
	// ErrInvalidKeySize is returned when the encryption key has an invalid size
	ErrInvalidKeySize = errors.New("invalid key size: must be 32 bytes for AES-256")
	// ErrInvalidCredentials is returned when credentials are nil
	ErrInvalidCredentials = errors.New("credentials cannot be nil")
	// ErrInvalidAPIKey is returned when API key verification fails
	ErrInvalidAPIKey = errors.New("invalid API key")
)

const (
	// KeySize is the required key size in bytes for AES-256
	KeySize = 32
	// NonceSize is the size of the nonce for GCM
	NonceSize = 12
	// APIKeyLength is the length of the generated API key in bytes
	APIKeyLength = 32
	// APIKeyHashCost is the bcrypt cost factor for API key hashing
	APIKeyHashCost = bcrypt.DefaultCost
)

// EncryptCredentials encrypts gateway credentials using AES-256-GCM.
// The encrypted data includes the nonce prepended to the ciphertext.
func EncryptCredentials(creds *models.GatewayCredentials, key []byte) ([]byte, error) {
	if creds == nil {
		return nil, ErrInvalidCredentials
	}

	if len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}

	// Marshal credentials to JSON
	plaintext, err := json.Marshal(creds)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Create cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher block: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt and authenticate
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	return ciphertext, nil
}

// DecryptCredentials decrypts gateway credentials that were encrypted with EncryptCredentials.
// The input should contain the nonce prepended to the ciphertext.
func DecryptCredentials(encrypted []byte, key []byte) (*models.GatewayCredentials, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}

	if len(encrypted) < NonceSize {
		return nil, ErrInvalidCiphertext
	}

	// Create cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher block: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Extract nonce from ciphertext
	nonce := encrypted[:NonceSize]
	ciphertext := encrypted[NonceSize:]

	// Decrypt and authenticate
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrInvalidCiphertext
	}

	// Unmarshal credentials
	var creds models.GatewayCredentials
	if err := json.Unmarshal(plaintext, &creds); err != nil {
		return nil, fmt.Errorf("failed to unmarshal credentials: %w", err)
	}

	return &creds, nil
}

// GenerateEncryptionKey generates a cryptographically secure random key for AES-256-GCM.
// This function should be used to generate a new encryption key during initial setup.
// The generated key should be stored securely (e.g., in a key management service).
func GenerateEncryptionKey() ([]byte, error) {
	key := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate encryption key: %w", err)
	}
	return key, nil
}

// GenerateAPIKey generates a cryptographically secure random API key for gateway authentication.
// The API key is returned as a plain string (to be shown to the user once).
func GenerateAPIKey() (string, error) {
	keyBytes := make([]byte, APIKeyLength)
	if _, err := io.ReadFull(rand.Reader, keyBytes); err != nil {
		return "", fmt.Errorf("failed to generate API key: %w", err)
	}
	// Return as hex string for easier usage
	return hex.EncodeToString(keyBytes), nil
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

// GenerateHandle generates a URL-safe handle from a display name.
// This is a simplified version that sanitizes the input string.
func GenerateHandle(displayName string) (string, error) {
	if displayName == "" {
		return "", errors.New("display name cannot be empty")
	}

	// Convert to lowercase and replace spaces with hyphens
	handle := ""
	for _, ch := range displayName {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			handle += string(ch)
		} else if ch >= 'A' && ch <= 'Z' {
			handle += string(ch + 32) // Convert to lowercase
		} else if ch == ' ' || ch == '-' || ch == '_' {
			handle += "-"
		}
	}

	// Remove leading/trailing hyphens and ensure not empty
	for len(handle) > 0 && handle[0] == '-' {
		handle = handle[1:]
	}
	for len(handle) > 0 && handle[len(handle)-1] == '-' {
		handle = handle[:len(handle)-1]
	}

	if handle == "" {
		// Fallback to random string if sanitization removed everything
		randomBytes := make([]byte, 8)
		if _, err := rand.Read(randomBytes); err != nil {
			return "", fmt.Errorf("failed to generate random handle: %w", err)
		}
		handle = "key-" + hex.EncodeToString(randomBytes)
	}

	return handle, nil
}

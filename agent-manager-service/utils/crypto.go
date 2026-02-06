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
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

var (
	// ErrInvalidCiphertext is returned when decryption fails due to invalid ciphertext
	ErrInvalidCiphertext = errors.New("invalid ciphertext")
	// ErrInvalidKeySize is returned when the encryption key has an invalid size
	ErrInvalidKeySize = errors.New("invalid key size: must be 32 bytes for AES-256")
	// ErrInvalidCredentials is returned when credentials are nil
	ErrInvalidCredentials = errors.New("credentials cannot be nil")
)

const (
	// KeySize is the required key size in bytes for AES-256
	KeySize = 32
	// NonceSize is the size of the nonce for GCM
	NonceSize = 12
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

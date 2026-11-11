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

// Package vault encrypts secret values before they are stored. It follows the
// API Platform's AI Workspace secret vault (platform-api/internal/vault): a
// pluggable SecretVault interface with one in-house AES-256-GCM implementation.
package vault

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
)

// ProviderInHouse is the provider name recorded on every secret this vault encrypts.
const ProviderInHouse = "in-house"

// KeySize is the AES-256 key length.
const KeySize = 32

// SecretVault is the pluggable encryption backend for secrets.
type SecretVault interface {
	// Encrypt seals plaintext. binding names where the value is stored; Decrypt
	// succeeds only when given the same binding, so a ciphertext moved to
	// another place does not open there.
	Encrypt(ctx context.Context, plaintext, binding []byte) ([]byte, error)
	Decrypt(ctx context.Context, ciphertext, binding []byte) ([]byte, error)
	ProviderName() string
	// Hash returns a keyed digest of plaintext, used to tell whether a value
	// changed without decrypting the stored one.
	Hash(plaintext []byte) string
}

// InHouseVault encrypts secrets using AES-256-GCM with a locally injected key.
type InHouseVault struct {
	aead cipher.AEAD
	key  []byte
}

var _ SecretVault = (*InHouseVault)(nil)

// NewInHouseVault creates an InHouseVault from a 32-byte AES-256 key.
func NewInHouseVault(key []byte) (*InHouseVault, error) {
	if len(key) != KeySize {
		return nil, errors.New("vault key must be 32 bytes for AES-256")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &InHouseVault{aead: aead, key: append([]byte(nil), key...)}, nil
}

// Encrypt seals plaintext with binding as additional authenticated data and
// returns nonce || ciphertext.
func (v *InHouseVault) Encrypt(_ context.Context, plaintext, binding []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, errors.New("plaintext cannot be empty")
	}
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return v.aead.Seal(nonce, nonce, plaintext, binding), nil
}

// Decrypt opens a value produced by Encrypt with the same binding.
func (v *InHouseVault) Decrypt(_ context.Context, ciphertext, binding []byte) ([]byte, error) {
	nonceSize := v.aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}
	return v.aead.Open(nil, ciphertext[:nonceSize], ciphertext[nonceSize:], binding)
}

// ProviderName returns the provider recorded with each stored secret.
func (v *InHouseVault) ProviderName() string {
	return ProviderInHouse
}

// Hash returns "hmac-sha256:" followed by the hex HMAC-SHA256 of plaintext.
// HMAC rather than a bare SHA-256 keeps the stored hash from being checked
// against guesses offline.
func (v *InHouseVault) Hash(plaintext []byte) string {
	mac := hmac.New(sha256.New, v.key)
	mac.Write(plaintext)
	return "hmac-sha256:" + hex.EncodeToString(mac.Sum(nil))
}

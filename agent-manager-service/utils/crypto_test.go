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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

func TestEncryptCredentials(t *testing.T) {
	t.Run("Successfully encrypt username/password credentials", func(t *testing.T) {
		key := make([]byte, 32) // Valid 32-byte key
		creds := &models.GatewayCredentials{
			Username: "testuser",
			Password: "testpass123",
		}

		encrypted, err := EncryptCredentials(creds, key)
		require.NoError(t, err)
		assert.NotEmpty(t, encrypted)
		assert.Greater(t, len(encrypted), 12) // At least 12 bytes for nonce
	})

	t.Run("Successfully encrypt API key credentials", func(t *testing.T) {
		key := make([]byte, 32)
		creds := &models.GatewayCredentials{
			APIKey: "sk-test-api-key-12345",
		}

		encrypted, err := EncryptCredentials(creds, key)
		require.NoError(t, err)
		assert.NotEmpty(t, encrypted)
	})

	t.Run("Successfully encrypt token credentials", func(t *testing.T) {
		key := make([]byte, 32)
		creds := &models.GatewayCredentials{
			Token: "bearer-token-xyz789",
		}

		encrypted, err := EncryptCredentials(creds, key)
		require.NoError(t, err)
		assert.NotEmpty(t, encrypted)
	})

	t.Run("Fail with invalid key size", func(t *testing.T) {
		invalidKey := make([]byte, 16) // Wrong size
		creds := &models.GatewayCredentials{
			Username: "testuser",
		}

		_, err := EncryptCredentials(creds, invalidKey)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidKeySize)
	})

	t.Run("Produce different ciphertext for same credentials", func(t *testing.T) {
		key := make([]byte, 32)
		creds := &models.GatewayCredentials{
			Username: "testuser",
			Password: "testpass",
		}

		encrypted1, err1 := EncryptCredentials(creds, key)
		encrypted2, err2 := EncryptCredentials(creds, key)

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, encrypted1, encrypted2, "Each encryption should use unique nonce")
	})
}

func TestDecryptCredentials(t *testing.T) {
	t.Run("Successfully decrypt username/password credentials", func(t *testing.T) {
		key := make([]byte, 32)
		original := &models.GatewayCredentials{
			Username: "testuser",
			Password: "testpass123",
		}

		encrypted, err := EncryptCredentials(original, key)
		require.NoError(t, err)

		decrypted, err := DecryptCredentials(encrypted, key)
		require.NoError(t, err)
		assert.Equal(t, original.Username, decrypted.Username)
		assert.Equal(t, original.Password, decrypted.Password)
	})

	t.Run("Successfully decrypt API key credentials", func(t *testing.T) {
		key := make([]byte, 32)
		original := &models.GatewayCredentials{
			APIKey: "sk-test-api-key-12345",
		}

		encrypted, err := EncryptCredentials(original, key)
		require.NoError(t, err)

		decrypted, err := DecryptCredentials(encrypted, key)
		require.NoError(t, err)
		assert.Equal(t, original.APIKey, decrypted.APIKey)
	})

	t.Run("Successfully decrypt token credentials", func(t *testing.T) {
		key := make([]byte, 32)
		original := &models.GatewayCredentials{
			Token: "bearer-token-xyz789",
		}

		encrypted, err := EncryptCredentials(original, key)
		require.NoError(t, err)

		decrypted, err := DecryptCredentials(encrypted, key)
		require.NoError(t, err)
		assert.Equal(t, original.Token, decrypted.Token)
	})

	t.Run("Fail with invalid key size", func(t *testing.T) {
		key := make([]byte, 32)
		invalidKey := make([]byte, 16)

		creds := &models.GatewayCredentials{Username: "test"}
		encrypted, err := EncryptCredentials(creds, key)
		require.NoError(t, err)

		_, err = DecryptCredentials(encrypted, invalidKey)
		assert.Error(t, err)
	})

	t.Run("Fail with invalid ciphertext", func(t *testing.T) {
		key := make([]byte, 32)
		invalidCiphertext := []byte("invalid-encrypted-data")

		_, err := DecryptCredentials(invalidCiphertext, key)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidCiphertext)
	})

	t.Run("Fail with corrupted ciphertext", func(t *testing.T) {
		key := make([]byte, 32)
		creds := &models.GatewayCredentials{Username: "test"}

		encrypted, err := EncryptCredentials(creds, key)
		require.NoError(t, err)

		// Corrupt the ciphertext
		encrypted[0] ^= 0xFF

		_, err = DecryptCredentials(encrypted, key)
		assert.Error(t, err)
	})

	t.Run("Fail with truncated ciphertext", func(t *testing.T) {
		key := make([]byte, 32)
		truncated := []byte("too-short")

		_, err := DecryptCredentials(truncated, key)
		assert.Error(t, err)
	})
}

func TestGenerateEncryptionKey(t *testing.T) {
	t.Run("Generate valid 32-byte key", func(t *testing.T) {
		key, err := GenerateEncryptionKey()
		require.NoError(t, err)
		assert.Len(t, key, 32)
	})

	t.Run("Generate different keys each time", func(t *testing.T) {
		key1, err1 := GenerateEncryptionKey()
		key2, err2 := GenerateEncryptionKey()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotEqual(t, key1, key2, "Each generated key should be unique")
	})

	t.Run("Generated key can be used for encryption", func(t *testing.T) {
		key, err := GenerateEncryptionKey()
		require.NoError(t, err)

		creds := &models.GatewayCredentials{Username: "test"}
		encrypted, err := EncryptCredentials(creds, key)
		require.NoError(t, err)

		decrypted, err := DecryptCredentials(encrypted, key)
		require.NoError(t, err)
		assert.Equal(t, creds.Username, decrypted.Username)
	})
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	t.Run("All credential types survive round trip", func(t *testing.T) {
		key := make([]byte, 32)

		testCases := []struct {
			name  string
			creds *models.GatewayCredentials
		}{
			{
				name: "username/password",
				creds: &models.GatewayCredentials{
					Username: "user1",
					Password: "pass1",
				},
			},
			{
				name: "API key",
				creds: &models.GatewayCredentials{
					APIKey: "key-abc-123",
				},
			},
			{
				name: "token",
				creds: &models.GatewayCredentials{
					Token: "token-xyz-789",
				},
			},
			{
				name: "all fields",
				creds: &models.GatewayCredentials{
					Username: "user",
					Password: "pass",
					APIKey:   "key",
					Token:    "token",
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				encrypted, err := EncryptCredentials(tc.creds, key)
				require.NoError(t, err)

				decrypted, err := DecryptCredentials(encrypted, key)
				require.NoError(t, err)
				assert.Equal(t, tc.creds, decrypted)
			})
		}
	})
}

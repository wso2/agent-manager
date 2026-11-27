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

package aihub

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha3"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// webhookFieldKeyInfo is the HKDF info of the key a sender encrypts a delivery's secret fields (a subscription
// key) with. It is part of the delivery protocol: it must stay byte-identical to the sender's.
const webhookFieldKeyInfo = "api-portal-webhook-field-encryption-v1"

// encryptedField is a secret field of a delivery (one named in encrypted_fields), as the sender seals it:
// AES-256-GCM with a key derived from the webhook secret, all parts base64.
type encryptedField struct {
	IV         string `json:"iv"`
	Tag        string `json:"tag"`
	Ciphertext string `json:"ciphertext"`
}

// webhookFieldKey derives the field key from the webhook secret: HKDF-SHA3-256, no salt.
func webhookFieldKey(secret string) ([]byte, error) {
	return hkdf.Key(sha3.New256, []byte(secret), nil, webhookFieldKeyInfo, 32)
}

// decryptField opens a secret field of a delivery. The field must be sealed: a plaintext value is refused,
// so a secret never arrives unprotected.
func decryptField(secret string, raw json.RawMessage) (string, error) {
	var f encryptedField
	if err := json.Unmarshal(raw, &f); err != nil || f.IV == "" || f.Ciphertext == "" || f.Tag == "" {
		return "", fmt.Errorf("%w: a secret field is not sealed", errInvalidEnvelope)
	}
	iv, err1 := base64.StdEncoding.DecodeString(f.IV)
	tag, err2 := base64.StdEncoding.DecodeString(f.Tag)
	ct, err3 := base64.StdEncoding.DecodeString(f.Ciphertext)
	if err1 != nil || err2 != nil || err3 != nil {
		return "", fmt.Errorf("%w: a secret field is not base64", errInvalidEnvelope)
	}
	key, err := webhookFieldKey(secret)
	if err != nil {
		return "", fmt.Errorf("derive the field key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("field cipher: %w", err)
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, len(iv))
	if err != nil {
		return "", fmt.Errorf("%w: a secret field has a bad nonce", errInvalidEnvelope)
	}
	plain, err := gcm.Open(nil, iv, append(ct, tag...), nil)
	if err != nil {
		return "", fmt.Errorf("%w: a secret field does not open with the webhook secret", errInvalidEnvelope)
	}
	return string(plain), nil
}

// SealField seals a secret field as a sender does (a random IV), for tests and tools that send deliveries.
func SealField(secret, value string, iv []byte) (json.RawMessage, error) {
	key, err := webhookFieldKey(secret)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, len(iv))
	if err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, iv, []byte(value), nil)
	ct, tag := sealed[:len(sealed)-gcm.Overhead()], sealed[len(sealed)-gcm.Overhead():]
	return json.Marshal(encryptedField{
		IV: base64.StdEncoding.EncodeToString(iv), Tag: base64.StdEncoding.EncodeToString(tag),
		Ciphertext: base64.StdEncoding.EncodeToString(ct),
	})
}

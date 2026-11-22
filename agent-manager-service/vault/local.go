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
	"fmt"
	"io"
)

// ProviderLocal is the in-house vault: values are encrypted with AES-256-GCM
// and kept in the secrets row itself.
const ProviderLocal = "local"

// KeySize is the AES-256 key length.
const KeySize = 32

func init() {
	Register(ProviderLocal, func(cfg Config) (Vault, error) {
		local, err := NewLocal(cfg.EncryptionKey, cfg.PreviousKeys...)
		if err != nil {
			return nil, err
		}
		local.legacyWrites = cfg.LegacyLocalWrites
		return local, nil
	})
}

// Local is the in-house vault.
type Local struct {
	legacyWrites bool
	current      string
	keys         map[string]cipher.AEAD
	// order lists key ids newest first, for values stored before key ids were recorded.
	order []string
}

var _ Vault = (*Local)(nil)

// NewLocal builds the in-house vault. New values are sealed with key; previous
// keys only open what they sealed, which lets a deployment rotate its key.
func NewLocal(key []byte, previous ...[]byte) (*Local, error) {
	v := &Local{keys: map[string]cipher.AEAD{}}
	for i, k := range append([][]byte{key}, previous...) {
		if len(k) != KeySize {
			return nil, fmt.Errorf("vault key %d must be %d bytes for AES-256, got %d", i, KeySize, len(k))
		}
		block, err := aes.NewCipher(k)
		if err != nil {
			return nil, err
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
		id := keyID(k)
		if _, dup := v.keys[id]; dup {
			continue
		}
		v.keys[id] = aead
		v.order = append(v.order, id)
	}
	v.current = v.order[0]
	return v, nil
}

// keyID names a key without revealing it: a truncated HMAC of a fixed label
// under the key itself.
func keyID(key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("amp-secret-vault-key-id"))
	return hex.EncodeToString(mac.Sum(nil))[:16]
}

// Provider returns "local".
func (v *Local) Provider() string { return ProviderLocal }

// binding is the additional authenticated data that ties a value to its row.
func binding(ref Ref) []byte {
	return []byte(ref.OrgID + "/" + ref.SecretID.String())
}

// Seal returns nonce || ciphertext, authenticated against ref unless legacy writes are enabled.
func (v *Local) Seal(_ context.Context, ref Ref, plaintext []byte, _ *Sealed) (Sealed, error) {
	if len(plaintext) == 0 {
		return Sealed{}, ErrEmptyValue
	}
	aead := v.keys[v.current]
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return Sealed{}, err
	}
	if v.legacyWrites {
		return Sealed{Ciphertext: aead.Seal(nonce, nonce, plaintext, nil), KeyID: "", ExternalRef: ""}, nil
	}
	return Sealed{Ciphertext: aead.Seal(nonce, nonce, plaintext, binding(ref)), KeyID: v.current, ExternalRef: ""}, nil
}

// Open reverses Seal.
func (v *Local) Open(_ context.Context, ref Ref, sealed Sealed) ([]byte, error) {
	if sealed.KeyID == "" {
		return v.openUnbound(sealed.Ciphertext)
	}
	aead, ok := v.keys[sealed.KeyID]
	if !ok {
		return nil, fmt.Errorf("%w (key id %s)", ErrUnknownKey, sealed.KeyID)
	}
	return openWith(aead, sealed.Ciphertext, binding(ref))
}

// Destroy is a no-op: the value lives only in the row.
func (v *Local) Destroy(context.Context, Ref, Sealed) error { return nil }

func openWith(aead cipher.AEAD, ciphertext, aad []byte) ([]byte, error) {
	n := aead.NonceSize()
	if len(ciphertext) < n {
		return nil, errors.New("secret ciphertext is too short")
	}
	plaintext, err := aead.Open(nil, ciphertext[:n], ciphertext[n:], aad)
	if err != nil {
		return nil, errors.New("secret could not be decrypted: wrong key, or the value does not belong to this row")
	}
	return plaintext, nil
}

// openUnbound opens a value stored before the vault recorded key ids. Those
// rows hold nonce || ciphertext with no row binding, as the owning service
// encrypted it. The next write reseals them.
func (v *Local) openUnbound(stored []byte) ([]byte, error) {
	for _, id := range v.order {
		if plaintext, err := openWith(v.keys[id], stored, nil); err == nil {
			return plaintext, nil
		}
	}
	return nil, errors.New("secret could not be decrypted with any configured key")
}

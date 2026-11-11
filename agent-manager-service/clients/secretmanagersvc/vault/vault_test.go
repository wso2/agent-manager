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
	"bytes"
	"context"
	"strings"
	"testing"
)

func testKey() []byte { return bytes.Repeat([]byte{7}, KeySize) }

func TestInHouseVaultRoundTrip(t *testing.T) {
	v, err := NewInHouseVault(testKey())
	if err != nil {
		t.Fatal(err)
	}
	ct, err := v.Encrypt(context.Background(), []byte("s3cret"), []byte("ou-1/name"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ct, []byte("s3cret")) {
		t.Fatal("ciphertext contains the plaintext")
	}
	pt, err := v.Decrypt(context.Background(), ct, []byte("ou-1/name"))
	if err != nil {
		t.Fatal(err)
	}
	if string(pt) != "s3cret" {
		t.Fatalf("Decrypt = %q, want s3cret", pt)
	}
	if v.ProviderName() != ProviderInHouse {
		t.Fatalf("ProviderName = %q", v.ProviderName())
	}
}

func TestInHouseVaultRejectsTamperingAndOtherKeys(t *testing.T) {
	v, _ := NewInHouseVault(testKey())
	ct, _ := v.Encrypt(context.Background(), []byte("value"), []byte("ou-1/name"))

	tampered := append([]byte(nil), ct...)
	tampered[len(tampered)-1] ^= 1
	if _, err := v.Decrypt(context.Background(), tampered, []byte("ou-1/name")); err == nil {
		t.Fatal("Decrypt accepted a tampered ciphertext")
	}
	other, _ := NewInHouseVault(bytes.Repeat([]byte{9}, KeySize))
	if _, err := other.Decrypt(context.Background(), ct, []byte("ou-1/name")); err == nil {
		t.Fatal("Decrypt accepted a ciphertext sealed under another key")
	}
	if _, err := v.Decrypt(context.Background(), []byte{1}, []byte("ou-1/name")); err == nil {
		t.Fatal("Decrypt accepted a truncated ciphertext")
	}
}

func TestInHouseVaultValidation(t *testing.T) {
	if _, err := NewInHouseVault([]byte("short")); err == nil {
		t.Fatal("NewInHouseVault accepted a short key")
	}
	v, _ := NewInHouseVault(testKey())
	if _, err := v.Encrypt(context.Background(), nil, []byte("ou-1/name")); err == nil {
		t.Fatal("Encrypt accepted empty plaintext")
	}
}

func TestInHouseVaultHash(t *testing.T) {
	v, _ := NewInHouseVault(testKey())
	h := v.Hash([]byte("a"))
	if !strings.HasPrefix(h, "hmac-sha256:") || h != v.Hash([]byte("a")) || h == v.Hash([]byte("b")) {
		t.Fatalf("unexpected hash behaviour: %q", h)
	}
	other, _ := NewInHouseVault(bytes.Repeat([]byte{9}, KeySize))
	if other.Hash([]byte("a")) == h {
		t.Fatal("hash does not depend on the key")
	}
}

func TestInHouseVaultRejectsAnotherBinding(t *testing.T) {
	v, _ := NewInHouseVault(testKey())
	ct, err := v.Encrypt(context.Background(), []byte("value"), []byte("ou-1/name"))
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range [][]byte{[]byte("ou-2/name"), []byte("ou-1/other"), nil} {
		if _, err := v.Decrypt(context.Background(), ct, binding); err == nil {
			t.Fatalf("Decrypt accepted a ciphertext under binding %q", binding)
		}
	}
}

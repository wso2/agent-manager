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
package aesgcm

import (
	"bytes"
	"errors"
	"testing"
)

func testKey(b byte) []byte { return bytes.Repeat([]byte{b}, KeySize) }

func TestEncryptDecryptRoundTrip(t *testing.T) {
	for _, plaintext := range [][]byte{[]byte("s3cr3t value"), {}} {
		ct, err := Encrypt(plaintext, testKey(1))
		if err != nil {
			t.Fatalf("Encrypt: %v", err)
		}
		if len(ct) != NonceSize+len(plaintext)+16 {
			t.Fatalf("ciphertext length = %d, want nonce + plaintext + 16-byte tag", len(ct))
		}
		if len(plaintext) > 0 && bytes.Contains(ct, plaintext) {
			t.Fatal("ciphertext contains the plaintext")
		}
		got, err := Decrypt(ct, testKey(1))
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if !bytes.Equal(got, plaintext) {
			t.Fatalf("Decrypt = %q, want %q", got, plaintext)
		}
	}
}

func TestEncryptUsesFreshNonce(t *testing.T) {
	a, err := Encrypt([]byte("same"), testKey(1))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Encrypt([]byte("same"), testKey(1))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a[:NonceSize], b[:NonceSize]) || bytes.Equal(a, b) {
		t.Fatal("two encryptions of the same plaintext produced the same nonce or ciphertext")
	}
}

func TestRejectsWrongKeySize(t *testing.T) {
	for _, key := range [][]byte{nil, make([]byte, 16), make([]byte, 33)} {
		if _, err := Encrypt([]byte("x"), key); !errors.Is(err, ErrInvalidKeySize) {
			t.Errorf("Encrypt with %d-byte key: err = %v, want ErrInvalidKeySize", len(key), err)
		}
		if _, err := Decrypt(make([]byte, 40), key); !errors.Is(err, ErrInvalidKeySize) {
			t.Errorf("Decrypt with %d-byte key: err = %v, want ErrInvalidKeySize", len(key), err)
		}
	}
}

func TestDecryptRejectsBadCiphertext(t *testing.T) {
	ct, err := Encrypt([]byte("payload"), testKey(1))
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Clone(ct)
	tampered[len(tampered)-1] ^= 0xff

	cases := map[string]struct {
		ct  []byte
		key []byte
	}{
		"shorter than a nonce": {ct: ct[:NonceSize-1], key: testKey(1)},
		"tampered tag":         {ct: tampered, key: testKey(1)},
		"wrong key":            {ct: ct, key: testKey(2)},
		"nonce only":           {ct: ct[:NonceSize], key: testKey(1)},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := Decrypt(tc.ct, tc.key)
			if !errors.Is(err, ErrInvalidCiphertext) {
				t.Fatalf("err = %v, want ErrInvalidCiphertext", err)
			}
			if got != nil {
				t.Fatalf("plaintext = %q, want nil", got)
			}
		})
	}
}

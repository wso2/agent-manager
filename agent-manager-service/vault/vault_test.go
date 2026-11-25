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
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func testKey(b byte) []byte { return bytes.Repeat([]byte{b}, KeySize) }

func testRef() Ref {
	return Ref{OrgID: "org-1", SecretID: uuid.New(), OwnerKind: "a2a_proxy"}
}

func TestLocalSealsAndOpens(t *testing.T) {
	v, err := NewLocal(testKey(1))
	require.NoError(t, err)
	ctx, ref := context.Background(), testRef()

	sealed, err := v.Seal(ctx, ref, []byte("s3cret"), nil)
	require.NoError(t, err)
	require.NotEmpty(t, sealed.KeyID)
	require.Empty(t, sealed.ExternalRef)
	require.NotContains(t, string(sealed.Ciphertext), "s3cret")

	plaintext, err := v.Open(ctx, ref, sealed)
	require.NoError(t, err)
	require.Equal(t, "s3cret", string(plaintext))

	again, err := v.Seal(ctx, ref, []byte("s3cret"), &sealed)
	require.NoError(t, err)
	require.NotEqual(t, sealed.Ciphertext, again.Ciphertext, "each seal uses a fresh nonce")
}

func TestLocalRejectsAnEmptyValueAndABadKey(t *testing.T) {
	_, err := NewLocal([]byte("short"))
	require.Error(t, err)

	v, err := NewLocal(testKey(1))
	require.NoError(t, err)
	_, err = v.Seal(context.Background(), testRef(), nil, nil)
	require.ErrorIs(t, err, ErrEmptyValue)
}

func TestLocalBindsAValueToItsRow(t *testing.T) {
	v, err := NewLocal(testKey(1))
	require.NoError(t, err)
	ctx, ref := context.Background(), testRef()
	sealed, err := v.Seal(ctx, ref, []byte("s3cret"), nil)
	require.NoError(t, err)

	otherRow := ref
	otherRow.SecretID = uuid.New()
	_, err = v.Open(ctx, otherRow, sealed)
	require.Error(t, err)

	otherOrg := ref
	otherOrg.OrgID = "org-2"
	_, err = v.Open(ctx, otherOrg, sealed)
	require.Error(t, err)
}

func TestLocalOpensValuesSealedBeforeARotation(t *testing.T) {
	ctx, ref := context.Background(), testRef()
	before, err := NewLocal(testKey(1))
	require.NoError(t, err)
	sealed, err := before.Seal(ctx, ref, []byte("s3cret"), nil)
	require.NoError(t, err)

	rotated, err := NewLocal(testKey(2), testKey(1))
	require.NoError(t, err)
	plaintext, err := rotated.Open(ctx, ref, sealed)
	require.NoError(t, err)
	require.Equal(t, "s3cret", string(plaintext))

	resealed, err := rotated.Seal(ctx, ref, plaintext, &sealed)
	require.NoError(t, err)
	require.NotEqual(t, sealed.KeyID, resealed.KeyID, "new values use the new key")

	withoutOld, err := NewLocal(testKey(2))
	require.NoError(t, err)
	_, err = withoutOld.Open(ctx, ref, sealed)
	require.ErrorIs(t, err, ErrUnknownKey)
}

// Rows written before the vault hold what the owning service encrypted itself.
func TestLocalOpensValuesStoredBeforeKeyIDs(t *testing.T) {
	key := testKey(1)
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	aead, err := cipher.NewGCM(block)
	require.NoError(t, err)
	nonce := make([]byte, aead.NonceSize())
	raw := aead.Seal(nonce, nonce, []byte("s3cret"), nil)

	v, err := NewLocal(testKey(2), key)
	require.NoError(t, err)
	ctx, ref := context.Background(), testRef()

	plaintext, err := v.Open(ctx, ref, Sealed{Ciphertext: raw})
	require.NoError(t, err)
	require.Equal(t, "s3cret", string(plaintext))

	_, err = v.Open(ctx, ref, Sealed{Ciphertext: []byte("not a ciphertext")})
	require.Error(t, err)
}

// memoryVault is an external vault: the row keeps only a reference.
type memoryVault struct {
	name   string
	values map[string][]byte
}

func (m *memoryVault) Provider() string { return m.name }

func (m *memoryVault) PromotionReference(ref Ref) (string, error) {
	return "amp/" + ref.OrgID + "/" + ref.SecretID.String() + "/" + ref.OperationID.String(), nil
}

func (m *memoryVault) Seal(_ context.Context, ref Ref, plaintext []byte, _ *Sealed) (Sealed, error) {
	path, _ := m.PromotionReference(ref)
	m.values[path] = plaintext
	return Sealed{ExternalRef: path}, nil
}

func (m *memoryVault) Open(_ context.Context, _ Ref, sealed Sealed) ([]byte, error) {
	return m.values[sealed.ExternalRef], nil
}

func (m *memoryVault) Destroy(_ context.Context, _ Ref, sealed Sealed) error {
	delete(m.values, sealed.ExternalRef)
	return nil
}

const memoryProvider = "test-external"

func init() {
	Register(memoryProvider, func(Config) (Vault, error) {
		return &memoryVault{name: memoryProvider, values: map[string][]byte{}}, nil
	})
}

func TestOpenLoadsWriterAndRetainedReaders(t *testing.T) {
	require.Contains(t, Providers(), memoryProvider)
	require.Contains(t, Providers(), ProviderLocal)

	cfg := Config{EncryptionKey: testKey(1)}

	def, err := Open("", cfg)
	require.NoError(t, err)
	require.Equal(t, ProviderLocal, def.Writer().Provider())

	set, err := Open(memoryProvider, cfg)
	require.NoError(t, err)
	require.Equal(t, memoryProvider, set.Writer().Provider())
	local, err := set.For(ProviderLocal)
	require.NoError(t, err)
	require.Equal(t, ProviderLocal, local.Provider())

	retained, err := Open(ProviderLocal, cfg, memoryProvider)
	require.NoError(t, err)
	require.Equal(t, ProviderLocal, retained.Writer().Provider())
	reader, err := retained.For(memoryProvider)
	require.NoError(t, err)
	require.Equal(t, memoryProvider, reader.Provider())

	_, err = set.For("nope")
	require.ErrorIs(t, err, ErrUnknownProvider)
	_, err = Open("nope", cfg)
	require.ErrorIs(t, err, ErrUnknownProvider)
	_, err = Open(ProviderLocal, Config{})
	require.Error(t, err, "the local vault needs a key")
}

func TestRegisterRejectsBadNames(t *testing.T) {
	factory := func(Config) (Vault, error) { return nil, nil } //nolint:nilnil // never called
	require.Panics(t, func() { Register("", factory) })
	require.Panics(t, func() { Register("a-name-longer-than-the-column", factory) })
	require.Panics(t, func() { Register(ProviderLocal, factory) })
	require.Panics(t, func() { Register("no-factory", nil) })
}

type checkingVault struct {
	*memoryVault
	err error
}

func (c checkingVault) Check(context.Context) error { return c.err }

func TestOpenRejectsBrokenProviders(t *testing.T) {
	cfg := Config{EncryptionKey: testKey(1)}
	Register("test-failing", func(Config) (Vault, error) { return nil, errors.New("boom") })
	Register("test-misnamed", func(Config) (Vault, error) { return &memoryVault{name: "other"}, nil })
	Register("test-local-only", func(Config) (Vault, error) { return localOnly{name: "test-local-only"}, nil })

	_, err := Open("test-failing", cfg)
	require.ErrorContains(t, err, "boom")
	_, err = Open("test-misnamed", cfg)
	require.ErrorContains(t, err, "reports itself as")
	_, err = Open("test-local-only", cfg)
	require.ErrorContains(t, err, "does not support external writes")
}

// localOnly seals in place, so it cannot be a writer next to the local vault.
type localOnly struct{ name string }

func (l localOnly) Provider() string { return l.name }
func (localOnly) Seal(context.Context, Ref, []byte, *Sealed) (Sealed, error) {
	return Sealed{}, nil
}
func (localOnly) Open(context.Context, Ref, Sealed) ([]byte, error) { return nil, nil }
func (localOnly) Destroy(context.Context, Ref, Sealed) error         { return nil }

func TestCheckReportsTheFirstFailingVault(t *testing.T) {
	ok := checkingVault{memoryVault: &memoryVault{name: "a"}}
	bad := checkingVault{memoryVault: &memoryVault{name: "b"}, err: errors.New("sealed")}
	require.NoError(t, Of(ok).Check(context.Background()))
	err := Of(ok, bad).Check(context.Background())
	require.ErrorContains(t, err, `secret vault "b": sealed`)
}

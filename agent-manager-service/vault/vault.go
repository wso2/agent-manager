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

// Package vault protects the values kept in the secrets table. A Vault decides
// where a value lives: the in-house "local" vault encrypts it into the row, and
// an external vault (a cloud secret manager, a KMS) keeps it elsewhere and
// leaves a reference in the row. Providers register themselves by name, so a
// deployment adds one by importing its package.
package vault

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// MaxProviderNameLen is the width of the secrets.provider column.
const MaxProviderNameLen = 16

var (
	// ErrUnknownProvider is returned for a provider no package registered.
	ErrUnknownProvider = errors.New("unknown secret vault provider")
	// ErrUnknownKey is returned when a value was sealed with a key this vault was not given.
	ErrUnknownKey = errors.New("secret was sealed with a key that is not configured")
	// ErrEmptyValue is returned when asked to seal an empty value.
	ErrEmptyValue = errors.New("secret value cannot be empty")
)

// Ref names the secrets row a value belongs to. A vault binds the value to it,
// so a sealed value copied into another row does not open there.
type Ref struct {
	// OperationID identifies an external promotion across retries. Row binding ignores it.
	OperationID uuid.UUID
	OrgID       string
	SecretID    uuid.UUID
	// OwnerKind is what uses the secret (a2a_proxy, event_destination, ...).
	// External vaults use it to name and tag what they store.
	OwnerKind string
}

// Sealed is what the secrets row keeps for one value. A vault that encrypts
// into the row sets Ciphertext and KeyID; one that stores the value elsewhere
// sets ExternalRef.
type Sealed struct {
	Ciphertext []byte
	// KeyID names the master key that produced Ciphertext, for rotation.
	KeyID string
	// ExternalRef locates the value in an external vault (a path, an ARN).
	ExternalRef string
}

// Vault is one secret backend.
//
// An external vault is only ever written after the database change that needs
// the value has committed, by the secret store's job worker. Until a write's
// row commits, the row keeps pointing at the previous value, so an external
// vault must keep that value readable: Seal stages a new value and never
// changes what current refers to.
type Vault interface {
	// Provider is the name recorded in secrets.provider for values this vault sealed.
	Provider() string
	// Seal protects plaintext for ref. current is the value this vault already
	// keeps for the secret, nil when it keeps none, so an external vault can
	// write a new version of the same entry rather than a new entry. The
	// returned ExternalRef must differ from current's and must locate exactly
	// the value written: a version id, or a path that names the version. What
	// current refers to must stay readable until Destroy is called for it.
	Seal(ctx context.Context, ref Ref, plaintext []byte, current *Sealed) (Sealed, error)
	// Open returns the value Seal protected for the same ref.
	Open(ctx context.Context, ref Ref, sealed Sealed) ([]byte, error)
	// Destroy releases what sealed refers to outside the row, and nothing else.
	// It runs after no committed row refers to it any more and is retried on
	// failure, so it must succeed when the value is already gone.
	Destroy(ctx context.Context, ref Ref, sealed Sealed) error
}

// ExternalVault supports recoverable writes. PromotionReference is pure and
// returns the immutable reference for ref.OperationID before Seal makes any
// external change. Seal must return that reference and be idempotent for the
// operation. Destroy must prevent retries of a canceled operation from
// recreating the released value.
type ExternalVault interface {
	Vault
	PromotionReference(ref Ref) (string, error)
}

// Config is what a provider is built from. A provider reads any settings of its
// own (a region, an endpoint, credentials) from its environment.
type Config struct {
	// EncryptionKey is the 32-byte master key new values are sealed with.
	EncryptionKey []byte
	// PreviousKeys still open values sealed before a rotation.
	PreviousKeys [][]byte
}

// Factory builds a provider's Vault.
type Factory func(cfg Config) (Vault, error)

var (
	factoriesMu sync.RWMutex
	factories   = map[string]Factory{}
)

// Register makes a provider available under name. It panics on an empty,
// overlong or duplicate name, since those are programming errors.
func Register(name string, factory Factory) {
	if name == "" || len(name) > MaxProviderNameLen {
		panic(fmt.Sprintf("vault: provider name %q must be 1 to %d characters", name, MaxProviderNameLen))
	}
	if factory == nil {
		panic(fmt.Sprintf("vault: provider %q has no factory", name))
	}
	factoriesMu.Lock()
	defer factoriesMu.Unlock()
	if _, exists := factories[name]; exists {
		panic(fmt.Sprintf("vault: provider %q is already registered", name))
	}
	factories[name] = factory
}

// Providers lists the registered provider names.
func Providers() []string {
	factoriesMu.RLock()
	defer factoriesMu.RUnlock()
	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func build(name string, cfg Config) (Vault, error) {
	factoriesMu.RLock()
	factory, ok := factories[name]
	factoriesMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w %q (registered: %v)", ErrUnknownProvider, name, Providers())
	}
	v, err := factory(cfg)
	if err != nil {
		return nil, fmt.Errorf("secret vault provider %q: %w", name, err)
	}
	if v.Provider() != name {
		return nil, fmt.Errorf("secret vault provider %q reports itself as %q", name, v.Provider())
	}
	return v, nil
}

// Vaults is the set of vaults a process uses: one that seals new values, and
// every one whose values may still be in the database.
type Vaults struct {
	writer Vault
	byName map[string]Vault
}

// Open builds the vault named provider for new values. The local vault is
// always loaded as well, so values it sealed stay readable after a deployment
// moves to another provider. Explicit readers keep retired external providers available.
func Open(provider string, cfg Config, readers ...string) (*Vaults, error) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		provider = ProviderLocal
	}
	set := &Vaults{byName: map[string]Vault{}}
	for _, name := range append([]string{ProviderLocal, provider}, readers...) {
		name = strings.TrimSpace(name)
		if _, loaded := set.byName[name]; loaded || name == "" {
			continue
		}
		v, err := build(name, cfg)
		if err != nil {
			return nil, err
		}
		if name != ProviderLocal && name == provider {
			if _, ok := v.(ExternalVault); !ok {
				return nil, fmt.Errorf("secret vault writer %q does not support recoverable writes", name)
			}
		}
		set.byName[name] = v
	}
	set.writer = set.byName[provider]
	return set, nil
}

// Of returns a set that seals with writer and also reads what readers sealed.
// It is for tests and for embedders that build their vaults themselves.
func Of(writer Vault, readers ...Vault) *Vaults {
	set := &Vaults{writer: writer, byName: map[string]Vault{writer.Provider(): writer}}
	for _, r := range readers {
		set.byName[r.Provider()] = r
	}
	return set
}

// Writer is the vault new values are sealed with.
func (s *Vaults) Writer() Vault { return s.writer }

// For returns the vault that sealed a row whose provider column is provider.
func (s *Vaults) For(provider string) (Vault, error) {
	v, ok := s.byName[provider]
	if !ok {
		return nil, fmt.Errorf("%w %q: this deployment is not configured to read it", ErrUnknownProvider, provider)
	}
	return v, nil
}

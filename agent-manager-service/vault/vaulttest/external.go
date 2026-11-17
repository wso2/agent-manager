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

package vaulttest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/vault"
)

// ExternalProvider is the provider name External records.
const ExternalProvider = "test-external"

// ErrInjected is what External returns for a failure a test asked for.
var ErrInjected = errors.New("injected vault failure")

// External is an in-memory external vault that versions each entry the way a
// cloud secret manager does: Seal writes a new version and returns a reference
// to it, and Destroy removes only the version it is given. Tests can make the
// next calls fail.
type External struct {
	mu       sync.Mutex
	versions map[string][]byte
	next     map[string]int

	// FailSeal and FailDestroy make that many following calls fail.
	FailSeal    int
	FailDestroy int
	// Destroyed lists every reference Destroy removed, in order.
	Destroyed []string
}

var _ vault.Vault = (*External)(nil)

// NewExternal returns an empty External.
func NewExternal() *External {
	return &External{versions: map[string][]byte{}, next: map[string]int{}}
}

func (e *External) Provider() string { return ExternalProvider }

func (e *External) Seal(_ context.Context, ref vault.Ref, plaintext []byte, _ *vault.Sealed) (vault.Sealed, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.FailSeal > 0 {
		e.FailSeal--
		return vault.Sealed{}, ErrInjected
	}
	entry := "amp/" + ref.OrgID + "/" + ref.OwnerKind + "/" + ref.SecretID.String()
	e.next[entry]++
	path := fmt.Sprintf("%s#%d", entry, e.next[entry])
	e.versions[path] = slices.Clone(plaintext)
	return vault.Sealed{ExternalRef: path}, nil
}

func (e *External) Open(_ context.Context, _ vault.Ref, sealed vault.Sealed) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	value, ok := e.versions[sealed.ExternalRef]
	if !ok {
		return nil, fmt.Errorf("vault entry %s does not exist", sealed.ExternalRef)
	}
	return slices.Clone(value), nil
}

func (e *External) Destroy(_ context.Context, _ vault.Ref, sealed vault.Sealed) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.FailDestroy > 0 {
		e.FailDestroy--
		return ErrInjected
	}
	delete(e.versions, sealed.ExternalRef)
	e.Destroyed = append(e.Destroyed, sealed.ExternalRef)
	return nil
}

// Has reports whether the version ref names is stored.
func (e *External) Has(ref string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.versions[ref]
	return ok
}

// Len is how many versions are stored.
func (e *External) Len() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.versions)
}

// Value returns the stored value of the version ref names.
func (e *External) Value(ref string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return string(e.versions[ref])
}

// LocalAndExternal returns the local vault under Key and an External, set to
// seal new values with the External.
func LocalAndExternal(t testing.TB) (*vault.Vaults, *External) {
	t.Helper()
	local, err := vault.NewLocal(Key)
	if err != nil {
		t.Fatalf("building the test vault: %v", err)
	}
	external := NewExternal()
	return vault.Of(external, local), external
}

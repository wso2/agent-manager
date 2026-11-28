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

// Package gwsecret keeps the gateway's credentials in the module's secret
// store (modkit.Runtime.Secrets). Each credential lives at a path named after
// the row that owns it; a row's configuration carries that path as its
// secretRef, and the controller-facing routes resolve it when they hand the
// configuration to a gateway.
package gwsecret

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// Paths of the upstream credential of each kind of row.
func LLMProvider(id uuid.UUID) string { return "llm-providers/" + id.String() + "/upstream-auth" }
func LLMProxy(id uuid.UUID) string    { return "llm-proxies/" + id.String() + "/upstream-auth" }
func A2AProxy(id uuid.UUID) string    { return "a2a-proxies/" + id.String() + "/upstream-auth" }

// MCPEndpoint is the path of an MCP proxy endpoint's upstream credential. An
// update re-creates the proxy's endpoints, so the path names the endpoint by
// its handle (hashed: handles are free text) rather than its row id.
func MCPEndpoint(proxyID uuid.UUID, handle string) string {
	sum := sha256.Sum256([]byte(handle))
	return "mcp-proxies/" + proxyID.String() + "/endpoints/" + hex.EncodeToString(sum[:16]) + "/upstream-auth"
}

var errNoStore = errors.New("ai-gateway: the secret store is not available before the module is activated")

// Store is the module's secret store, set when the module is activated.
type Store struct {
	s atomic.Pointer[modkit.Secrets]
}

// NewStore returns a store with nothing behind it yet.
func NewStore() *Store { return &Store{} }

// Use puts s behind the store.
func (st *Store) Use(s modkit.Secrets) { st.s.Store(&s) }

func (st *Store) get() (modkit.Secrets, error) {
	if st == nil {
		return nil, errNoStore
	}
	s := st.s.Load()
	if s == nil {
		return nil, errNoStore
	}
	return *s, nil
}

// In returns the store writing through tx.
func (st *Store) In(tx *gorm.DB) modkit.Secrets { return txStore{st: st, tx: tx} }

func (st *Store) Put(ctx context.Context, orgID, path, value string) error {
	return st.In(nil).Put(ctx, orgID, path, value)
}

func (st *Store) Get(ctx context.Context, orgID, path string) (string, error) {
	return st.In(nil).Get(ctx, orgID, path)
}

func (st *Store) Delete(ctx context.Context, orgID, path string) error {
	return st.In(nil).Delete(ctx, orgID, path)
}

type txStore struct {
	st *Store
	tx *gorm.DB
}

func (t txStore) secrets() (modkit.Secrets, error) {
	s, err := t.st.get()
	if err != nil || t.tx == nil {
		return s, err
	}
	return s.In(t.tx), nil
}

func (t txStore) Put(ctx context.Context, orgID, path, value string) error {
	s, err := t.secrets()
	if err != nil {
		return err
	}
	return s.Put(ctx, orgID, path, value)
}

func (t txStore) Get(ctx context.Context, orgID, path string) (string, error) {
	s, err := t.secrets()
	if err != nil {
		return "", err
	}
	return s.Get(ctx, orgID, path)
}

func (t txStore) Delete(ctx context.Context, orgID, path string) error {
	s, err := t.secrets()
	if err != nil {
		return err
	}
	return s.Delete(ctx, orgID, path)
}

func (t txStore) In(tx *gorm.DB) modkit.Secrets { return txStore{st: t.st, tx: tx} }

// Prepare stages outside the transaction, whatever tx this store writes through.
func (t txStore) Prepare(ctx context.Context, orgID, path, value string) (context.Context, error) {
	return t.st.Prepare(ctx, orgID, path, value)
}

// clearsCredential reports whether auth explicitly carries an empty value,
// which means no credential: Prepare stages nothing and Seal deletes it.
func clearsCredential(auth *gwmodels.UpstreamAuth) bool {
	return auth != nil && auth.Value != nil && *auth.Value == ""
}

// Prepare stages the plaintext, if any, before the owning transaction. An
// empty value is no credential (see Seal), so it stages nothing.
func Prepare(ctx context.Context, store modkit.Secrets, orgID, path string, auth *gwmodels.UpstreamAuth) (context.Context, error) {
	if auth == nil || auth.Value == nil || clearsCredential(auth) {
		return ctx, nil
	}
	return PrepareValue(ctx, store, orgID, path, *auth.Value)
}

func PrepareValue(ctx context.Context, store modkit.Secrets, orgID, path, value string) (context.Context, error) {
	return store.Prepare(ctx, orgID, path, value)
}

func (st *Store) Prepare(ctx context.Context, orgID, path, value string) (context.Context, error) {
	store, err := st.get()
	if err != nil {
		return ctx, err
	}
	return store.Prepare(ctx, orgID, path, value)
}

// Seal stores auth's plaintext value at path and leaves path in its place as
// the secretRef, so the row never holds the value. A row whose auth carries
// no value and no reference holds no credential, so what path held is
// deleted; so does an explicitly empty value, which is cleared from auth along
// with any reference. A reference to another path is refused: each row owns
// its own.
func Seal(ctx context.Context, s modkit.Secrets, orgID, path string, auth *gwmodels.UpstreamAuth) error {
	switch {
	case clearsCredential(auth):
		auth.SecretRef, auth.Value = nil, nil
		return s.Delete(ctx, orgID, path)
	case auth != nil && auth.Value != nil:
		if err := s.Put(ctx, orgID, path, *auth.Value); err != nil {
			return fmt.Errorf("store the upstream credential: %w", err)
		}
		ref := path
		auth.SecretRef, auth.Value = &ref, nil
		return nil
	case auth != nil && auth.SecretRef != nil:
		if *auth.SecretRef != path {
			return fmt.Errorf("upstream credential reference %q does not belong to this resource", *auth.SecretRef)
		}
		return nil
	default:
		return s.Delete(ctx, orgID, path)
	}
}

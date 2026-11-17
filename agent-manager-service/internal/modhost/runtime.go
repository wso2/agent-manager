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

package modhost

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
)

// runtime is the modkit.Runtime a module gets for one activation.
type runtime struct {
	h    *Host
	name string
	mf   modkit.Manifest
	v    *view
	log  *slog.Logger

	// building is true while Activate runs, when v is not yet the current snapshot.
	building atomic.Bool
	// pinned runtimes resolve capabilities from v, the snapshot they were
	// made for, rather than the current one.
	pinned bool

	mu   sync.Mutex
	errs []error
}

var _ modkit.Runtime = (*runtime)(nil)

func (h *Host) runtime(name string, v *view) *runtime {
	return &runtime{h: h, name: name, mf: h.manifests[name], v: v, log: h.log.With("module", name)}
}

func (r *runtime) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errs = append(r.errs, err)
}

func (r *runtime) err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return errors.Join(r.errs...)
}

// unsupported records the use of a Runtime feature this core does not
// implement yet. A module that uses one fails to activate rather than run
// with part of its registrations silently missing.
func (r *runtime) unsupported(feature string) {
	r.fail(fmt.Errorf("uses Runtime.%s, which this core does not implement yet", feature))
}

func (r *runtime) Name() string         { return r.name }
func (r *runtime) DB() *gorm.DB         { return r.h.db }
func (r *runtime) Logger() *slog.Logger { return r.log }

// TODO: the core read API for modules (agents, projects, environments).
func (r *runtime) Core() modkit.CoreAPI { return unimplementedCore{} }

// TODO: mount module routes under the API router with the authz middleware.
func (r *runtime) Handle(string, string, string, http.HandlerFunc) { r.unsupported("Handle") }

// TODO: mount workload routes that verify core-issued workload tokens.
func (r *runtime) HandleInternal(string, string, string, http.HandlerFunc) {
	r.unsupported("HandleInternal")
}

// TODO: run agent and environment hooks inside the core transaction.
func (r *runtime) Hook(string, modkit.Phase, modkit.HookFunc) { r.unsupported("Hook") }

// TODO: deliver events from the event outbox.
func (r *runtime) Subscribe(string, modkit.EventHandler) { r.unsupported("Subscribe") }

// TODO: write events to the event outbox inside tx.
func (r *runtime) PublishTx(context.Context, *gorm.DB, string, any) error {
	return modkit.ErrNotImplemented
}

// TODO: check the caller's permissions against the permission registry.
func (r *runtime) Can(context.Context, string) bool { return false }

// TODO: sign workload tokens with the core's JWT keys.
func (r *runtime) IssueToken(context.Context, modkit.TokenRequest) (string, error) {
	return "", modkit.ErrNotImplemented
}

func (r *runtime) Provide(capability string, impl any) {
	if !slices.Contains(r.mf.Provides, capability) {
		r.fail(fmt.Errorf("provides %s, which its manifest does not list", capability))
		return
	}
	if !r.building.Load() {
		r.fail(fmt.Errorf("provides %s outside Activate", capability))
		return
	}
	if by, ok := r.v.capBy[capability]; ok && by != r.name {
		r.fail(fmt.Errorf("provides %s, already provided by %s", capability, by))
		return
	}
	r.v.caps[capability] = impl
	r.v.capBy[capability] = r.name
}

func (r *runtime) Capability(name string) (any, bool) {
	if !slices.Contains(r.mf.Requires, name) && !slices.Contains(r.mf.Optional, name) {
		r.log.Warn("modhost:capability is not listed in Requires or Optional", "capability", name)
		return nil, false
	}
	v := r.h.current()
	if r.pinned || r.building.Load() {
		v = r.v
	}
	impl, ok := v.caps[name]
	return impl, ok
}

type unimplementedCore struct{}

func (unimplementedCore) ListAgents(context.Context, string) ([]modkit.Agent, error) {
	return nil, modkit.ErrNotImplemented
}

func (unimplementedCore) GetAgent(context.Context, string, string) (modkit.Agent, error) {
	return modkit.Agent{}, modkit.ErrNotImplemented
}

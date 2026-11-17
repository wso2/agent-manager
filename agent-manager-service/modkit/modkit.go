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

// Package modkit is the contract between the Agent Manager core and a module.
//
// A module is a Go package compiled into the amp binary. It registers itself
// from init(), like a database/sql driver, and stays dormant until it is
// installed. Installing, enabling, disabling and uninstalling a module are
// changes to rows in the database: replicas notice them without a restart.
// Module tables are created by `amp migrate`, for every compiled-in module,
// whether it is installed or not.
//
// Modules talk to the core only through this package. They never import core
// packages, never query core tables (they use Runtime.Core), and never query
// another module's tables (they use capabilities).
package modkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// CoreAPIVersion is the version of this contract. A module declares the
// version it was written against; the core refuses modules built for another.
const CoreAPIVersion = 1

// ErrNotImplemented is returned by Runtime methods this core does not
// implement yet.
var ErrNotImplemented = errors.New("modkit: not implemented by this core")

// Manifest describes a module: what it needs, what it offers and what it adds
// to the API and the console.
type Manifest struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	CoreAPI     int    `json:"coreApi"`

	// Builtin modules are part of the core: always active, and they cannot
	// be disabled or uninstalled.
	Builtin bool `json:"builtin,omitempty"`

	// Capabilities are versioned contracts such as "llm.proxy/v1".
	// Dependencies name capabilities, not modules, so a provider can be
	// replaced without touching its consumers.
	Requires []string `json:"requires,omitempty"`
	Optional []string `json:"optional,omitempty"`
	Provides []string `json:"provides,omitempty"`

	// Permissions are namespaced by the core as "<module>:<name>".
	Permissions []Permission `json:"permissions,omitempty"`
	// RoleGrants gives the module's permissions to core roles when the
	// module is active.
	RoleGrants map[string][]string `json:"roleGrants,omitempty"`

	// Events lists the event types this module publishes.
	Events []string `json:"events,omitempty"`

	UI *UI `json:"ui,omitempty"`
}

// Permission is a module-defined permission.
type Permission struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// UI lists what the module adds to the console. Components are named exports
// of the module's bundle: pages under Nav, slot components under Slots.
type UI struct {
	Nav   []NavItem          `json:"nav,omitempty"`
	Slots []SlotContribution `json:"slots,omitempty"`
	// OwnSlots lists extension points this module renders and others may fill.
	OwnSlots []string `json:"ownSlots,omitempty"`
}

// NavItem adds a console navigation entry that opens one of the module's pages.
type NavItem struct {
	Label      string `json:"label"`
	Page       string `json:"page"`
	Permission string `json:"permission,omitempty"`
}

// SlotContribution fills an extension point owned by the core or another module.
type SlotContribution struct {
	Slot       string `json:"slot"`
	Component  string `json:"component"`
	Permission string `json:"permission,omitempty"`
}

// Module is implemented by every module.
type Module interface {
	Manifest() Manifest
	// Migrations returns the module's rendered migrations, one directory per
	// database ("sqlite/001_x.sql", "postgres/001_x.sql"), or nil. They are
	// rendered from the module's schema YAML by `make schema`.
	Migrations() fs.FS
	// UI returns the module's prebuilt console bundle, or nil.
	UI() fs.FS
	// Activate registers routes, hooks, event handlers and capabilities. The
	// core calls it on every replica each time the set of active modules
	// changes, with a fresh Runtime, so it must be cheap and free of side
	// effects.
	Activate(ctx context.Context, rt Runtime) error
}

// Installer is implemented by modules that must catch up with existing core
// state when they are installed, which may be long after the core started.
type Installer interface {
	OnInstall(ctx context.Context, rt Runtime) error
}

// HealthReporter is implemented by modules that report their own health.
type HealthReporter interface {
	Health(ctx context.Context, rt Runtime) map[string]any
}

// Phase of a synchronous resource hook.
type Phase string

const (
	// Validate runs before a resource is created or updated. An error rejects the request.
	Validate Phase = "validate"
	// Apply runs inside the core transaction that writes the resource.
	Apply Phase = "apply"
	// Finalize runs before a resource is deleted. An error blocks the delete.
	Finalize Phase = "finalize"
)

// HookRequest is passed to a hook.
type HookRequest struct {
	Resource string `json:"resource"` // e.g. "agent"
	Action   string `json:"action"`   // create, update, delete
	OrgID    string `json:"orgId,omitempty"`
	ID       string `json:"id,omitempty"`
	// Object is the resource as submitted or stored.
	Object map[string]any `json:"object,omitempty"`
	// Extension is the part of the request body addressed to this module
	// (body.extensions["<module>"]), or nil.
	Extension json.RawMessage `json:"extension,omitempty"`
	// Tx is the core transaction, set for the apply phase.
	Tx *gorm.DB `json:"-"`
}

// HookFunc handles a resource hook.
type HookFunc func(ctx context.Context, req HookRequest) error

// Event is an asynchronous notification.
type Event struct {
	Type   string          `json:"type"`
	Source string          `json:"source"`
	Time   time.Time       `json:"time"`
	Data   json.RawMessage `json:"data"`
}

// EventHandler handles an event. Handlers run asynchronously.
type EventHandler func(ctx context.Context, e Event) error

// Agent is the core resource modules most often attach data to.
type Agent struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"orgId"`
	ProjectID   string    `json:"projectId"`
	Name        string    `json:"name"`
	DisplayName string    `json:"displayName"`
	CreatedAt   time.Time `json:"createdAt"`
}

// CoreAPI is the read API the core offers to modules.
type CoreAPI interface {
	ListAgents(ctx context.Context, orgID string) ([]Agent, error)
	GetAgent(ctx context.Context, orgID, agentID string) (Agent, error)
}

// Runtime is what the core gives a module when it activates it.
type Runtime interface {
	Name() string
	// DB is the database handle. A module reads and writes only its own
	// tables, named mod_<name>_<table>.
	DB() *gorm.DB
	Core() CoreAPI
	Logger() *slog.Logger

	// Handle mounts an API route under /m/<module>. perm is a module
	// permission name (without the module prefix) or a core permission;
	// "" means any signed-in user.
	Handle(method, path, perm string, h http.HandlerFunc)
	// HandleInternal mounts a route for the module's workloads, which present
	// a core-issued token carrying scope.
	HandleInternal(method, path, scope string, h http.HandlerFunc)
	// Hook registers a synchronous hook on a core resource.
	Hook(resource string, phase Phase, h HookFunc)
	// Subscribe registers an asynchronous event handler.
	Subscribe(eventType string, h EventHandler)
	// PublishTx records an event declared in the manifest inside tx, so it
	// is delivered only if tx commits.
	PublishTx(ctx context.Context, tx *gorm.DB, eventType string, data any) error

	// Can reports whether the caller of the current request holds perm.
	Can(ctx context.Context, perm string) bool
	// IssueToken mints a short-lived token for one of the module's workloads.
	IssueToken(ctx context.Context, req TokenRequest) (string, error)

	// Provide registers the implementation of a capability listed in Provides.
	Provide(capability string, impl any)
	// Capability resolves a capability listed in Requires or Optional at call
	// time. ok is false when no active module provides it.
	Capability(name string) (impl any, ok bool)
}

// TokenRequest describes a workload token to mint.
type TokenRequest struct {
	Subject string            // e.g. "run:<id>"
	Scopes  []string          // module-local scope names
	TTL     time.Duration     // capped by the core
	Claims  map[string]string // extra string claims
}

// Lookup resolves a capability and asserts its contract type.
func Lookup[T any](rt Runtime, capability string) (T, bool) {
	var zero T
	impl, ok := rt.Capability(capability)
	if !ok {
		return zero, false
	}
	t, ok := impl.(T)
	return t, ok
}

var (
	regMu    sync.Mutex
	registry = map[string]Module{}
)

// maxNameLength keeps a module's table prefix short enough to leave room for
// table names within the 63-character limit every database shares.
const maxNameLength = 24

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// tablePrefix is the prefix of a module's tables: mod_<name>_, with "-" written as "_".
func tablePrefix(name string) string {
	return "mod_" + strings.ReplaceAll(name, "-", "_") + "_"
}

// CheckName reports whether name is a valid module name next to the modules
// already named in others. Names are lowercase letters, digits and dashes.
// No two modules may share a table prefix (a-b and a_b would), and no prefix
// may start another (t and t-x would: mod_t_ starts mod_t_x_), so a module's
// tables can never be mistaken for another's.
func CheckName(name string, others []string) error {
	if !namePattern.MatchString(name) || len(name) > maxNameLength {
		return fmt.Errorf("modkit: module name %q must be lowercase letters, digits and dashes, starting with a letter, at most %d characters", name, maxNameLength)
	}
	p := tablePrefix(name)
	for _, o := range others {
		q := tablePrefix(o)
		if strings.HasPrefix(p, q) || strings.HasPrefix(q, p) {
			return fmt.Errorf("modkit: module names %q and %q give overlapping table prefixes %s and %s", name, o, p, q)
		}
	}
	return nil
}

// Register makes a module available in this binary. Call it from init().
// Being registered does not install the module.
func Register(m Module) {
	regMu.Lock()
	defer regMu.Unlock()
	name := m.Manifest().Name
	others := make([]string, 0, len(registry))
	for n := range registry {
		others = append(others, n)
	}
	if err := CheckName(name, others); err != nil {
		panic(err.Error())
	}
	registry[name] = m
}

// Registered returns the modules compiled into this binary, sorted by name.
func Registered() []Module {
	regMu.Lock()
	defer regMu.Unlock()
	out := make([]Module, 0, len(registry))
	for _, m := range registry {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest().Name < out[j].Manifest().Name })
	return out
}

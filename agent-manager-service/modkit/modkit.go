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
	Name string `json:"name"`
	// DisplayName names the module to people, such as its permissions'
	// resource in the identity provider; empty means Name.
	DisplayName string `json:"displayName,omitempty"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	CoreAPI     int    `json:"coreApi"`

	// Capabilities are versioned contracts such as "llm.proxy/v1".
	// Dependencies name capabilities, not modules, so a provider can be
	// replaced without touching its consumers.
	Requires []string `json:"requires,omitempty"`
	Optional []string `json:"optional,omitempty"`
	Provides []string `json:"provides,omitempty"`

	// Permissions are namespaced by the core as "<module>:<name>" and carried
	// in tokens as the scope "amp:<module>:<name>". A name is an action
	// ("widget-read") or a resource and an action ("widgets.read"), each of
	// lowercase letters, digits and dashes.
	Permissions []Permission `json:"permissions,omitempty"`
	// RoleGrants gives the module's permissions (by the names in Permissions)
	// to the predefined roles while the module is active.
	RoleGrants map[string][]string `json:"roleGrants,omitempty"`

	// Events lists the event types this module publishes.
	Events []EventType `json:"events,omitempty"`
	// AuditActions lists the audit actions the module's routes and tools
	// name with Audit, and classifies any of its permissions that name one.
	AuditActions []AuditAction `json:"auditActions,omitempty"`

	UI *UI `json:"ui,omitempty"`

	// Workloads are the parts of the module that run outside the core
	// binary, such as a gateway runtime or a worker pool. They talk to the
	// module only over its internal routes (HandleInternal), with tokens
	// the core issues.
	Workloads []WorkloadSpec `json:"workloads,omitempty"`
	// Settings document the settings the module reads with Runtime.Config
	// (and InitRequest.Config), for the configuration reference.
	Settings []Setting `json:"settings,omitempty"`
}

// Setting documents one module setting: Key as the module reads it
// ("DEFAULT_GATEWAY_NAME"), set as MODULE_<NAME>_<KEY> or in the config:
// block of the module's modules: entry in amp.yaml.
type Setting struct {
	Key         string `json:"key"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description"`
	Secret      bool   `json:"secret,omitempty"`
}

// WorkloadKind says how a workload runs.
type WorkloadKind string

const (
	// WorkloadService is long-running. Its instances heartbeat to the core,
	// which records them for module health and hands each a fresh token.
	WorkloadService WorkloadKind = "service"
	// WorkloadJob runs once per unit of work, started by the module or
	// another workload. Jobs get tokens from IssueToken, not credentials.
	WorkloadJob WorkloadKind = "job"
)

// WorkloadSpec declares one of the module's workloads. (Workload is the
// caller of an internal route.)
type WorkloadSpec struct {
	// Name is lowercase letters, digits and dashes ("gateway-controller").
	Name string       `json:"name"`
	Kind WorkloadKind `json:"kind"`
	// Trusted workloads run first-party code only; untrusted ones run user
	// code and must be isolated by the platform.
	Trusted bool `json:"trusted"`
	// Image is the image this module version expects, if it ships one.
	Image string `json:"image,omitempty"`
	// Scopes are the module-local scopes the workload's tokens carry.
	Scopes []string `json:"scopes"`
	// Protocols this module version accepts in the workload's heartbeats,
	// newest first. A service names at least one.
	Protocols []string `json:"protocols,omitempty"`
	// Required services make the module "degraded" while no instance is
	// connected.
	Required bool `json:"required,omitempty"`
	// Endpoints are internal routes of the module the workload uses, besides
	// "rest", the module's internal root, which is reported whenever the
	// module has internal routes. The heartbeat response gives each as an
	// absolute URL, so a workload is configured with only the heartbeat URL
	// and its credential.
	Endpoints   []Endpoint `json:"endpoints,omitempty"`
	Description string     `json:"description,omitempty"`
}

// Endpoint is a named internal route of the module.
type Endpoint struct {
	// Name is lowercase letters, digits and dashes; "rest" is reserved.
	Name string `json:"name"`
	// Path is a route path the module registers with HandleInternal
	// ("/ws/gateways/connect").
	Path string `json:"path"`
	// WebSocket endpoints are reported with a ws or wss scheme.
	WebSocket bool `json:"websocket,omitempty"`
}

// PermissionName is the full name of one of the module's permissions:
// "<module>:<name>".
func (m Manifest) PermissionName(name string) string {
	return m.Name + ":" + name
}

// Event returns the manifest's event type named name.
func (m Manifest) Event(name string) (EventType, bool) {
	for _, e := range m.Events {
		if e.Name == name {
			return e, true
		}
	}
	return EventType{}, false
}

// Permission is a module-defined permission.
type Permission struct {
	Name string `json:"name"`
	// DisplayName names the permission to people, such as its action in the
	// identity provider; empty means Name.
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description"`
}

// UI lists what the module adds to the console. GET /api/v1/ui-manifest
// lists the UI of the core and of every active module, filtered by the
// caller's permissions, and the console turns on only what it lists.
type UI struct {
	// Pages turns on pages of the UI modules built into the console, by
	// "<console module>/<page>" ("ai-gateway/gateways"). A console module
	// with no page listed by the core or an active module is hidden.
	Pages []Page `json:"pages,omitempty"`
	// Nav and Slots name components of the module's own bundle (UI()):
	// pages under Nav, slot components under Slots.
	Nav   []NavItem          `json:"nav,omitempty"`
	Slots []SlotContribution `json:"slots,omitempty"`
	// OwnSlots lists extension points this module renders and others may fill.
	OwnSlots []string `json:"ownSlots,omitempty"`
}

// Page is a page of a UI module built into the console.
type Page struct {
	// ID is "<console module>/<page>": the console module's id and the
	// page plugin's id.
	ID string `json:"id"`
	// Permission, if set, is one the caller must hold to see the page: one
	// of the module's own (as named in Permissions) or a core permission.
	Permission string `json:"permission,omitempty"`
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

// InitComponent is a Module that writes files of its own when an installation
// is set up: "amp init <module> DIR" calls Init, before any server runs, so
// it has no Runtime and no database. Init reads the module's settings
// through req.Config and writes only into req.Dir; it runs on every
// "amp init" and must keep what an earlier run generated.
type InitComponent interface {
	Init(ctx context.Context, req InitRequest) error
}

// InitRequest is what "amp init" gives an InitComponent.
type InitRequest struct {
	// Dir is the directory the module's files go into.
	Dir string
	// Config reads the module's settings, as Runtime.Config does.
	Config func(key string) string
	// CoreOwner is the "uid:gid" the core's processes run as, for files the
	// module reads back when it serves; "" leaves files as created.
	CoreOwner string
}

// ExternalMigrator is implemented by modules that keep data in a store outside
// the core database, such as ClickHouse. `amp migrate` applies the store's
// schema files after the module's SQL migrations. Installing and enabling the
// module never apply them: they only check that they were applied.
type ExternalMigrator interface {
	// ExternalSchema returns the external store's schema. config reads the
	// module's settings, as Runtime.Config does. apply is true for
	// `amp migrate`, which applies files, and false for install and enable,
	// which only read what was applied. An error wrapping ErrNotConfigured
	// means there is no store to migrate or check: the core logs its message
	// and carries on.
	ExternalSchema(config func(key string) string, apply bool) (ExternalSchema, error)
}

// ErrNotConfigured is returned by ExternalMigrator.ExternalSchema when the
// module's external store, or the login it needs, is not configured.
var ErrNotConfigured = errors.New("modkit: not configured")

// ExternalSchema is the versioned schema of a module's external store. The core
// orders the files, checksums them and refuses what it refuses for SQL
// migrations (an edited applied file, an unknown applied version, a new file
// that sorts before an applied one). The store keeps the record of what it
// applied, next to the data it describes.
type ExternalSchema interface {
	// Store names the store in logs and errors ("clickhouse").
	Store() string
	// Files are the schema files, named NNN_description.<ext>.
	Files() fs.FS
	// Applied returns the version and checksum of every file the store has
	// recorded as applied.
	Applied(ctx context.Context) (map[string]string, error)
	// Apply applies one file and records its version and checksum. A file
	// that was applied but not recorded is applied again, so it must be safe
	// to repeat.
	Apply(ctx context.Context, version, checksum string, body []byte) error
	// Settle runs after the files on every `amp migrate`, for settings that
	// come from config, such as a retention TTL. It must be idempotent.
	Settle(ctx context.Context) error
}

// HealthReporter is implemented by modules that report their own health.
type HealthReporter interface {
	Health(ctx context.Context, rt Runtime) map[string]any
}

// Phase of a synchronous resource hook.
type Phase string

// The hooks of one create, update or delete see the same set of active
// modules and share one deadline, OperationTimeout; each hook also has at most
// HookTimeout. Inside a transaction each hook runs in its own savepoint: if it
// fails or runs out of time its writes are undone and the core's transaction
// carries on. A hook that runs out of time is abandoned, not stopped: its
// goroutine runs until it returns, but the transaction handle it was given
// stops working.
const (
	// Validate runs in activation order and may refuse the change: an error
	// rolls it back and fails the request (a modkit.Error carries its status
	// and message to the caller; any other error is a 500). On create and
	// update it runs inside the core transaction, before the write. On
	// delete it runs before the core tears anything down, outside any
	// transaction (Tx is nil), so it must only read: Publish and
	// IssueCredential called with its ctx are refused.
	Validate Phase = "validate"
	// Apply runs inside the core transaction after a create or update is
	// written, in activation order. An error rolls the change back.
	Apply Phase = "apply"
	// Finalize runs inside the core transaction that deletes a resource, in
	// reverse activation order, to clean up the module's own rows. On agents
	// and environments it cannot refuse the delete: an error is logged, and
	// a delete is refused in Validate. On projects, which take no other hook,
	// it is how a module refuses: an error rolls the delete back, as a
	// foreign key from the module's rows would.
	Finalize Phase = "finalize"
)

// Resources that take hooks, and the actions they report. Projects take only
// Finalize, on delete.
const (
	ResourceAgent       = "agent"
	ResourceEnvironment = "environment"
	ResourceProject     = "project"

	ActionCreate = "create"
	ActionUpdate = "update"
	ActionDelete = "delete"
)

// HookTimeout bounds every hook call.
const HookTimeout = 5 * time.Second

// OperationTimeout bounds all the hooks of one operation together.
const OperationTimeout = 10 * time.Second

// HookRequest is passed to a hook.
type HookRequest struct {
	Resource string `json:"resource"` // ResourceAgent, ResourceEnvironment or ResourceProject
	Action   string `json:"action"`   // ActionCreate, ActionUpdate or ActionDelete
	OrgID    string `json:"orgId,omitempty"`
	// ID is the resource id. It is empty in the validate phase of a create.
	ID string `json:"id,omitempty"`
	// Object is the resource as submitted (validate) or stored (apply, finalize).
	Object map[string]any `json:"object,omitempty"`
	// Extension is the part of the request body addressed to this module
	// (body.extensions["<module>"]), or nil.
	Extension json.RawMessage `json:"extension,omitempty"`
	// Tx is the core transaction (nil in the validate phase of a delete). A
	// hook writes its own tables through it, so its rows commit or roll back
	// with the resource; Publish and IssueCredential called with the hook's
	// ctx write through it too. Never use DB() inside a hook: on SQLite it
	// waits for the core's transaction, and on PostgreSQL its writes escape
	// a rollback. Tx stops working when the hook returns.
	Tx *gorm.DB `json:"-"`
}

// Error is an error a hook or handler returns to reject a request with a
// status and a message the caller may see.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

// Reject returns an Error with status 400.
func Reject(format string, args ...any) error {
	return &Error{Status: http.StatusBadRequest, Message: fmt.Sprintf(format, args...)}
}

// HookFunc handles a resource hook.
type HookFunc func(ctx context.Context, req HookRequest) error

// Core event types. A module subscribes to them by these names; the events a
// module publishes are named "<module>.<event>".
const (
	EventAgentCreated       = "agent.created"
	EventAgentDeleted       = "agent.deleted"
	EventEnvironmentCreated = "environment.created"
)

// Event is an asynchronous notification.
type Event struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Source  string          `json:"source"`
	OrgID   string          `json:"orgId"`
	Subject string          `json:"subject,omitempty"`
	Time    time.Time       `json:"time"`
	Data    json.RawMessage `json:"data"`
}

// EventHandler handles an event. Handlers run in the background, after the
// change that raised the event has committed, on one replica. A module's
// handlers for one event share one deadline. Each subscribing module gets the
// event at least once: a handler that fails is retried with backoff, and one
// that overruns its deadline is abandoned, not stopped, so it may still be
// running when the retry starts on this or another replica. Handlers must be
// idempotent and tolerate running concurrently. Undelivered events expire
// after a retention period.
type EventHandler func(ctx context.Context, e Event) error

// ErrNotFound is returned by the read API for a resource that does not exist
// in the organization.
var ErrNotFound = errors.New("modkit: not found")

// Org is an organization: its id and handle, as the request's token names it. The core keeps no other record of it.
type Org struct {
	ID string `json:"id"`
	// Handle is the organization's handle as the request's token names it. Outside a request (a worker, or a
	// hook or event with no request behind it) the core knows it only for the configured file sign-in
	// organization; for any other it is the organization's id. A caller that looks something up by handle,
	// such as traces, should do so from a request.
	Handle string `json:"handle"`
}

// Project is a project in an organization.
type Project struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"orgId"`
	Name        string    `json:"name"`
	DisplayName string    `json:"displayName"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Agent is the core resource modules most often attach data to.
type Agent struct {
	ID          string `json:"id"`
	OrgID       string `json:"orgId"`
	ProjectID   string `json:"projectId"`
	ProjectName string `json:"projectName"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	// Provisioning is "external" or "internal".
	Provisioning string    `json:"provisioning"`
	CreatedAt    time.Time `json:"createdAt"`
}

// Environment is an environment of an organization.
type Environment struct {
	ID           string    `json:"id"`
	OrgID        string    `json:"orgId"`
	Name         string    `json:"name"`
	DisplayName  string    `json:"displayName"`
	IsProduction bool      `json:"isProduction"`
	CreatedAt    time.Time `json:"createdAt"`
}

// CoreAPI is the read API the core offers to modules. Every call is scoped to
// one organization, by its id; a resource of another organization is
// reported as ErrNotFound. Called with the ctx of a request, hook or event,
// it answers only for that ctx's organization (OrgID).
type CoreAPI interface {
	GetOrg(ctx context.Context, orgID string) (Org, error)
	ListProjects(ctx context.Context, orgID string) ([]Project, error)
	GetProject(ctx context.Context, orgID, projectID string) (Project, error)
	// ListAgents lists the agents of an organization; projectID, when not
	// empty, narrows it to one project.
	ListAgents(ctx context.Context, orgID, projectID string) ([]Agent, error)
	GetAgent(ctx context.Context, orgID, agentID string) (Agent, error)
	ListEnvironments(ctx context.Context, orgID string) ([]Environment, error)

	// GetProjectByName, GetAgentByName and GetEnvironmentByName look a
	// resource up by the name the API addresses it with.
	GetProjectByName(ctx context.Context, orgID, name string) (Project, error)
	GetAgentByName(ctx context.Context, orgID, projectName, name string) (Agent, error)
	GetEnvironmentByName(ctx context.Context, orgID, name string) (Environment, error)
	// LockAgent locks an agent's row inside tx, a hook's Tx or the module's
	// own transaction, until tx ends, so that changes to the agent made
	// meanwhile wait; it returns the agent, or ErrNotFound.
	LockAgent(ctx context.Context, tx *gorm.DB, orgID, agentID string) (Agent, error)
	// EnsureOrg returns the organization, creating it, with its default
	// environment, if it does not exist yet. An organization exists in the
	// core from its first request; a module's route that writes rows of an
	// organization before any core route ran calls it first.
	EnsureOrg(ctx context.Context, orgID string) (Org, error)
}

// OrgID is the organization of the request ctx belongs to, set on every module
// route; empty outside a request.
func OrgID(ctx context.Context) string {
	s, _ := ctx.Value(orgKey{}).(string)
	return s
}

type orgKey struct{}

// WithOrgID returns ctx carrying orgID, for the core to set on module requests.
func WithOrgID(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, orgKey{}, orgID)
}

// Principal is the caller of an API request or MCP tool call.
type Principal struct {
	// User is the subject of the caller's token: the signed-in user, or the
	// client of a client-credentials token.
	User string `json:"user"`
	// ClientID is the OAuth client the token was issued to.
	ClientID string `json:"clientId,omitempty"`
}

type principalKey struct{}

// WithPrincipal returns ctx carrying p, for the core to set on module
// requests and tool calls.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the caller of the current request or tool call; the
// zero Principal outside one.
func PrincipalFrom(ctx context.Context) Principal {
	p, _ := ctx.Value(principalKey{}).(Principal)
	return p
}

// Runtime is what the core gives a module when it activates it.
type Runtime interface {
	Name() string
	// DB is the database handle. A module reads and writes only its own
	// tables, named mod_<name>_<table>.
	DB() *gorm.DB
	Core() CoreAPI
	Logger() *slog.Logger
	// Config returns key of the module's own configuration: the module's
	// config block in amp.yaml (modules: [{name, config: {...}}]), or the
	// environment variable MODULE_<NAME>_<KEY>, which wins. Nested keys are
	// joined with "." ("limits.max"); "" when unset.
	Config(key string) string

	// Handle mounts an API route at /m/<module><path> on the API port, beside
	// the core's /api/v1, behind the same sign-in, permission check and audit
	// trail as the core's routes. path is a net/http pattern path
	// ("/widgets/{id}"); paths under /internal are the workload routes'. perm
	// is one of the module's permissions (as named in its manifest) or a core
	// permission; "" means any signed-in user of the organization, which only
	// reads may use. Every route that changes state is audited, under the
	// permission's name unless an Audit option names the action.
	Handle(method, path, perm string, h http.HandlerFunc, opts ...RouteOption)
	// HandleAgent mounts an API route at /m/<module><path> that agents call
	// with their agent key in the X-Amp-Api-Key header instead of a user's
	// sign-in. The core verifies the key and answers 401 when it is missing
	// or invalid; the handler gets its claims from AgentKeyFrom and the key's
	// organization from OrgID, never the key itself. A request that carries a
	// key and matches no active agent route is not found. A route that
	// changes state names its audit action with Audit.
	HandleAgent(method, path string, h http.HandlerFunc, opts ...RouteOption)
	// HandleInternal mounts a route for the module's workloads at
	// /m/<module>/internal<path> on the internal server. Callers present a
	// token from IssueToken or from a heartbeat that carries scope. While
	// the module is inactive they answer 401.
	HandleInternal(method, path, scope string, h http.HandlerFunc, opts ...RouteOption)
	// HandleInternalAt mounts a route at a fixed path under /api/internal/v1
	// on the internal server ("/api/internal/v1/things"), served only while
	// the module is active. The core does not authenticate its callers: the
	// handler does. A route that changes state names its audit action with
	// Audit. It exists only so the AI Gateway can keep the paths its released
	// controllers call until they can register as workloads
	// (wso2/agent-manager#2493), and goes away then; new modules use
	// HandleInternal.
	HandleInternalAt(method, absPath string, h http.HandlerFunc, opts ...RouteOption)
	// HandleWebhook mounts a route at /m/<module>/webhooks<path> on the
	// internal server for a sender outside the installation that signs each
	// delivery (a developer portal's webhook) rather than presenting a
	// workload token. The core does not authenticate its callers: the handler
	// verifies the signature and names the sender with AuditInternalCaller. A
	// route that changes state names its audit action with Audit. While the
	// module is inactive it answers 401 module_inactive, as HandleInternal
	// does.
	HandleWebhook(method, path string, h http.HandlerFunc, opts ...RouteOption)
	// MCPTool adds a tool to the core's MCP server at /mcp while the module
	// is active. perm is the permission a caller needs, as for Handle; the
	// call is audited under its name unless an Audit option names the action.
	MCPTool(tool MCPTool, perm string, h MCPToolHandler, opts ...RouteOption)
	// Hook registers a synchronous hook on a core resource (ResourceAgent,
	// ResourceEnvironment or ResourceProject).
	Hook(resource string, phase Phase, h HookFunc)
	// Subscribe registers an asynchronous event handler for an event type:
	// a core event, or one a module declares in its manifest.
	Subscribe(eventType string, h EventHandler)
	// Publish records an event declared in the manifest for the organization
	// of ctx, or of data's "orgId" when ctx has none.
	Publish(ctx context.Context, eventType string, data any) error
	// PublishTx records an event declared in the manifest inside tx, so it
	// is delivered only if tx commits.
	PublishTx(ctx context.Context, tx *gorm.DB, eventType string, data any) error

	// Can reports whether the caller of the current request holds perm.
	Can(ctx context.Context, perm string) bool
	// Track reports the current API request as feature usage named event,
	// with props as its dimensions, once the response is written. A second
	// call in the same request adds to props, later values winning. It does
	// nothing outside a request to one of the module's routes, or when the
	// installation does not report feature usage.
	Track(ctx context.Context, event string, props map[string]any)
	// IssueToken mints a short-lived token for one of the module's workloads,
	// signed with the core's key and verifiable with its JWKS.
	IssueToken(ctx context.Context, req TokenRequest) (string, error)
	// IssueCredential creates a credential for one of the module's service
	// workloads, which presents it in its heartbeats to
	// POST /api/internal/v1/workloads/heartbeat. The secret is returned only
	// here.
	IssueCredential(ctx context.Context, req CredentialRequest) (Credential, error)
	// RevokeCredential revokes one of the module's credentials.
	RevokeCredential(ctx context.Context, id string) error

	// Go runs fn in the background while the module is active on this
	// replica: from when the activation that registered it is in effect until
	// the module is disabled or uninstalled, or the server stops. Then fn's
	// ctx is cancelled and fn must return; a worker that does not return in
	// time is abandoned. A module that stays active keeps its running
	// workers when the module set changes. Workers run on every replica where
	// the module is active, so work that must happen once needs its own claim
	// (a row lock or a lease). A worker that returns an error or panics is
	// restarted with backoff; one that returns nil is done. name identifies
	// it in logs and is unique within the module.
	Go(name string, fn func(ctx context.Context) error)
	// Secrets is the core's secret store, limited to the module's own paths.
	Secrets() Secrets

	// Provide registers the implementation of a capability listed in Provides.
	Provide(capability string, impl any)
	// Capability resolves a capability listed in Requires or Optional at call
	// time. ok is false when no active module provides it.
	Capability(name string) (impl any, ok bool)
}

// TokenRequest describes a workload token to mint.
type TokenRequest struct {
	Subject string            // e.g. "run:<id>"
	OrgID   string            // the organization the workload acts for; defaults to the request's
	Scopes  []string          // module-local scope names
	TTL     time.Duration     // capped by the core; zero means the default
	Claims  map[string]string // extra string claims
}

// CredentialRequest describes a workload credential. Tokens issued on its
// heartbeats carry its subject and organization, and the scopes the
// workload declares.
type CredentialRequest struct {
	// Workload is a service workload the manifest declares.
	Workload string
	Subject  string
	OrgID    string
}

// Credential is a workload credential.
type Credential struct {
	ID string `json:"id"`
	// Secret is what the workload presents as "credential" in its
	// heartbeats. Only a hash of it is stored.
	Secret string `json:"secret"`
}

// Workload is the caller of an internal route, from its verified token.
type Workload struct {
	Module string
	// Name and Instance are the declared workload and the instance that
	// heartbeated; both are empty in a token from IssueToken.
	Name     string
	Instance string
	Subject  string
	OrgID    string
	Scopes   []string
	Claims   map[string]string
}

type workloadKey struct{}

// WithWorkload returns ctx carrying w, for the core to set on internal requests.
func WithWorkload(ctx context.Context, w Workload) context.Context {
	return context.WithValue(ctx, workloadKey{}, w)
}

// WorkloadFrom returns the workload calling an internal route.
func WorkloadFrom(ctx context.Context) (Workload, bool) {
	w, ok := ctx.Value(workloadKey{}).(Workload)
	return w, ok
}

// AgentKeyHeader is the header an agent sends its agent key in.
const AgentKeyHeader = "X-Amp-Api-Key"

// AgentKey is the agent calling a HandleAgent route, from its verified key.
type AgentKey struct {
	// KeyID is the key's token ID (jti).
	KeyID string
	// OrgID is the organization's ID; OrgName its handle.
	OrgID         string
	OrgName       string
	ProjectID     string
	ProjectName   string
	AgentID       string
	AgentName     string
	EnvironmentID string
	ExpiresAt     time.Time
}

type agentKeyKey struct{}

// WithAgentKey returns ctx carrying k, for the core to set on agent requests.
func WithAgentKey(ctx context.Context, k AgentKey) context.Context {
	return context.WithValue(ctx, agentKeyKey{}, k)
}

// AgentKeyFrom returns the agent calling a HandleAgent route.
func AgentKeyFrom(ctx context.Context) (AgentKey, bool) {
	k, ok := ctx.Value(agentKeyKey{}).(AgentKey)
	return k, ok
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

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

package modkit

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// RouteOption declares what a route or MCP tool does besides needing its
// permission: how the audit trail records it and which webhook event it
// emits.
type RouteOption func(*RouteOptions)

// RouteOptions is what a route's options declare. The core reads it.
type RouteOptions struct {
	// AuditAction names the route's audit action: one of the module's
	// AuditActions or permissions, by its manifest name. Empty means the
	// route's permission names it.
	AuditAction string
	// AuditRead records a read in the audit trail; reads are not recorded
	// otherwise. Every route that changes state is recorded.
	AuditRead bool
	// AuditCoalesce records the route at most once per caller per window, for
	// routes a machine polls on a timer.
	AuditCoalesce time.Duration
	// AuditFailClosed writes the route's audit record before the handler
	// runs and refuses the request when it cannot be written.
	AuditFailClosed bool
	// Event is the webhook event the route emits after a 2xx response: one of
	// the module's Events that has a Scope.
	Event string
	// EventWithoutData leaves the response body out of the event, for routes
	// whose response carries a secret.
	EventWithoutData bool
	// AlsoRequires are permissions a caller needs besides the route's own.
	AlsoRequires []string
	// AnyPermission lets in a caller holding any of these permissions, on a
	// route registered with no permission of its own.
	AnyPermission []string
	// PlatformAdmin admits only callers from the platform-admin
	// organization, for a route that reads across organizations.
	PlatformAdmin bool
	// AllowOrgOverride lets a caller the deployment trusts act on another
	// organization than its token's, named in the X-Impersonate-Org header.
	AllowOrgOverride bool
}

// Audit names the audit action a route or tool is recorded under, when its
// permission does not describe what it does (one permission gating create,
// rotate and revoke). On a read route it also records the read, for reads
// that disclose credentials or security settings.
func Audit(action string) RouteOption {
	return func(o *RouteOptions) {
		o.AuditAction = action
		o.AuditRead = true
	}
}

// AuditCoalesce records a route at most once per caller per window.
func AuditCoalesce(window time.Duration) RouteOption {
	return func(o *RouteOptions) { o.AuditCoalesce = window }
}

// AuditFailClosed makes a route refuse to run unrecorded: the core writes an
// intent record before the handler runs and answers 503 when it cannot, and
// the route's record after the handler names that intent record. For routes
// whose effect lands outside the core's database, such as minting a
// credential a gateway receives.
func AuditFailClosed() RouteOption {
	return func(o *RouteOptions) { o.AuditFailClosed = true }
}

// Emits names the webhook event a route emits after a 2xx response, with the
// response body as its data.
func Emits(event string) RouteOption {
	return func(o *RouteOptions) { o.Event = event }
}

// EmitsWithoutData is Emits for a route whose response must not reach a
// webhook, such as one that returns a token.
func EmitsWithoutData(event string) RouteOption {
	return func(o *RouteOptions) {
		o.Event = event
		o.EventWithoutData = true
	}
}

// AlsoRequires makes a route need perms besides its own permission, all of
// them, such as an environment tier on top of the capability. Names are
// resolved as the route's permission is.
func AlsoRequires(perms ...string) RouteOption {
	return func(o *RouteOptions) { o.AlsoRequires = append(o.AlsoRequires, perms...) }
}

// AnyPermission lets a caller holding any one of perms use a route registered
// with no permission of its own.
func AnyPermission(perms ...string) RouteOption {
	return func(o *RouteOptions) { o.AnyPermission = append(o.AnyPermission, perms...) }
}

// PlatformAdmin restricts a public route to callers from the platform-admin
// organization, for a route that reads across every organization. Its path
// must not name an organization.
func PlatformAdmin() RouteOption {
	return func(o *RouteOptions) { o.PlatformAdmin = true }
}

// AllowOrgOverride lets a deployment's control plane drive an org-scoped
// public route on a tenant's behalf: a request with the X-Impersonate-Org
// header acts on that organization when the deployment's override resolver
// allows it, and is refused with 403 otherwise. The path must name the
// target organization, and OrgID returns it. Without a resolver installed, a
// request with the header is refused and one without it is unaffected.
func AllowOrgOverride() RouteOption {
	return func(o *RouteOptions) { o.AllowOrgOverride = true }
}

// Options applies opts.
func Options(opts []RouteOption) RouteOptions {
	var o RouteOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return o
}

// AuditAction is an audit action a module declares for its routes and tools.
// It is recorded as "<module>:<name>".
type AuditAction struct {
	// Name is an action ("archive") or a resource and an action
	// ("widgets.archive"), each of lowercase letters, digits and dashes, or
	// the name of one of the module's permissions, to classify the action
	// that permission names.
	Name string `json:"name"`
	// Class is "credential", "identity", "deployment", "config" or "read";
	// empty means "config".
	Class string `json:"class,omitempty"`
	// Severity is "info", "notice", "warning" or "critical"; empty means
	// critical for credential and identity actions and info otherwise.
	Severity    string `json:"severity,omitempty"`
	Description string `json:"description,omitempty"`
	// Details are the detail keys the action's records may carry (set with
	// AuditDetail), each with its kind: "identifier", "name", "email",
	// "name-list", "count", "flag", "enum", "fingerprint" or "url". A key
	// not listed is dropped when the record is written; a "url" is reduced to
	// its scheme, host and path.
	Details map[string]string `json:"details,omitempty"`
}

// EventType is an event type a module publishes, named "<module>.<event>".
type EventType struct {
	Name string `json:"name"`
	// Scope makes the type a webhook event organizations can subscribe to,
	// raised by the routes that declare it with Emits: "org", "project" or
	// "agent", the narrowest resource it is about. Without a scope the event
	// only reaches module subscribers (Publish and Subscribe).
	Scope       string `json:"scope,omitempty"`
	Description string `json:"description,omitempty"`
}

// MCPTool describes a tool a module adds to the core's MCP server at /mcp.
type MCPTool struct {
	// Name is unique across the core and every module; prefix it with
	// something of the module's.
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description"`
	// InputSchema is the JSON Schema of the arguments, an object schema; nil
	// accepts any object.
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	ReadOnly    bool            `json:"readOnly,omitempty"`
	Destructive bool            `json:"destructive,omitempty"`
}

// MCPToolHandler runs a tool call. args are the call's arguments as sent. The
// result is returned to the client as JSON. ctx carries the caller's
// organization (OrgID); a modkit.Error is shown to the caller, any other
// error is reported as a failure without its text.
type MCPToolHandler func(ctx context.Context, args json.RawMessage) (any, error)

// Secrets keeps secret values in the core's secret store, encrypted with the
// core's key. A module addresses its secrets by a path of its own
// ("providers/<id>/api-key"); paths are the module's, so it can never read or
// overwrite another module's secrets or the core's. Every secret belongs to
// one organization. Called with a hook's ctx, they read and delete through
// the hook's transaction.
//
// An external vault (SECRET_VAULT_PROVIDER other than local) is written
// before the SQL transaction that records the value. To Put inside a
// transaction (In or a hook's), first call Prepare with the same orgID, path
// and value outside any transaction, and pass the context it returns to Put.
// Prepare is a no-op for the local vault. A hook has no moment outside its
// transaction, so it can read and delete secrets but cannot Put them under
// an external vault.
type Secrets interface {
	// Prepare stages value for a later Put of the same path inside a
	// transaction. Call it outside any transaction.
	Prepare(ctx context.Context, orgID, path, value string) (context.Context, error)
	// Put stores value at path, replacing what is there.
	Put(ctx context.Context, orgID, path, value string) error
	// Get returns the value at path, or ErrNotFound.
	Get(ctx context.Context, orgID, path string) (string, error)
	// Delete removes the value at path; a missing one is not an error.
	Delete(ctx context.Context, orgID, path string) error
	// In returns Secrets that read and write through tx, so a change commits
	// or rolls back with the module's own transaction.
	In(tx *gorm.DB) Secrets
}

// SecretCreator is implemented by Secrets that can store a path's first value atomically, for a value every replica
// must agree on, such as a key generated on first use.
type SecretCreator interface {
	// Create stores value at path unless path holds one, and returns what path holds afterwards: value, or what a
	// concurrent Create stored first. Concurrent Creates of one path all return the same value. Like Put, it needs
	// Prepare first when called inside a transaction under an external vault.
	Create(ctx context.Context, orgID, path, value string) (string, error)
}

// APIDescriber is implemented by modules that describe the routes they mount
// with Runtime.Handle in an OpenAPI 3 document, usually embedded from
// api/openapi.yaml. Its servers entry is /m/<name>, so paths are relative to
// the module ("/tags" for /m/<name>/tags), and info carries
// x-amp-module: <name>. The document is self-contained: it copies the common
// schemas it needs rather than referring to the core's document.
type APIDescriber interface {
	OpenAPI() []byte
}

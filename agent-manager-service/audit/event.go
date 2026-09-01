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

// Package audit records who did what, to which resource, with what outcome.
//
// The trail has two tiers. The coverage tier is a middleware installed once in
// middleware.RouteRegistrar; it observes every registered route and guarantees
// that no mutating endpoint can ship unaudited. The semantic tier is explicit
// Record/Begin calls at security-critical operations, where the HTTP envelope
// alone ("POST .../permissions/add -> 200") would tell an auditor nothing.
//
// Two properties are structural rather than filtered, and both matter:
//
//   - Request and response bodies are never read. The secrets that flow through
//     this API (git credentials, client secrets, upstream auth values, API keys)
//     are therefore out of reach of an audit record, not merely redacted out of
//     one. This is what makes the coverage tier safe to enable on every route.
//   - RequestPath stores the route pattern, never the raw URL, which removes a
//     whole class of path and query-string leakage.
//
// Everything a caller adds by hand goes through the typed Detail/SecretRef
// options and an allow-list keyed by action (see schema.go), so a field nobody
// thought about fails closed instead of leaking.
package audit

import (
	"time"

	"github.com/google/uuid"
)

// SchemaVersion is the version of the emitted record shape. Bump it when a
// field changes meaning or is removed, so downstream SIEM parsers can branch.
const SchemaVersion = 1

// ActorType classifies the principal behind an event.
type ActorType string

const (
	// ActorUser is a human acting with a user-bearing JWT.
	ActorUser ActorType = "user"
	// ActorService is a machine client using client-credentials (e.g. the
	// evaluation job publishing scores).
	ActorService ActorType = "service"
	// ActorAgent is a deployed agent acting with an AMS-minted token.
	ActorAgent ActorType = "agent"
	// ActorGateway is a data-plane gateway authenticating with an api-key header.
	ActorGateway ActorType = "gateway"
	// ActorSystem is AMS itself: reconcilers, schedulers, startup posture.
	ActorSystem ActorType = "system"
	// ActorAnonymous is an unauthenticated caller, used for authn failures.
	ActorAnonymous ActorType = "anonymous"
)

// Surface identifies which entry point produced the event. The same logical
// operation can arrive over more than one surface (REST and MCP both deploy
// agents), so this is recorded separately from the action.
type Surface string

const (
	SurfaceAPI       Surface = "api"
	SurfaceMCP       Surface = "mcp"
	SurfaceInternal  Surface = "internal"
	SurfacePublisher Surface = "publisher"
	SurfaceSystem    Surface = "system"
)

// Outcome is the result of the audited operation.
type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailure Outcome = "failure"
	// OutcomeDeny is an authorization or authentication refusal, kept distinct
	// from OutcomeFailure because denied privilege escalation is the signal a
	// security reviewer looks for first.
	OutcomeDeny Outcome = "deny"
	// OutcomeUnknown marks an intent record written before an external mutation
	// whose result is not yet known. A record left in this state means the
	// process died mid-operation — that orphan is deliberate forensic signal,
	// not a defect. See Begin/Complete in emit.go.
	OutcomeUnknown Outcome = "unknown"
)

// Severity ranks events for triage and alerting.
type Severity int16

const (
	SeverityInfo     Severity = 1
	SeverityNotice   Severity = 2
	SeverityWarning  Severity = 3
	SeverityCritical Severity = 4
)

// Event is one audit record. Construct it only through Record/Begin so that
// actor resolution and redaction cannot be bypassed.
//
// JSON tags are the wire contract consumed by SIEM tooling: renaming one is a
// breaking change that requires a SchemaVersion bump.
type Event struct {
	EventID       uuid.UUID `json:"eventId"`
	SchemaVersion int16     `json:"schemaVersion"`
	OccurredAt    time.Time `json:"occurredAt"`
	DurationMs    int64     `json:"durationMs,omitempty"`

	Action      Action      `json:"action"`
	ActionClass ActionClass `json:"actionClass"`
	Severity    Severity    `json:"severity"`

	// Actor — who performed the operation.
	ActorType    ActorType `json:"actorType"`
	ActorID      string    `json:"actorId,omitempty"`
	ActorDisplay string    `json:"actorDisplay,omitempty"`
	ActorOUID    string    `json:"actorOuId,omitempty"`
	AuthMethod   string    `json:"authMethod,omitempty"`
	// ActorTokenID is the token's jti. It is the join key to the identity
	// provider's login records: authentication happens in Thunder, not here, so
	// without it an action cannot be traced back to the session that produced
	// it. See docs/audit-logging.md.
	ActorTokenID string `json:"actorTokenId,omitempty"`
	// OnBehalfOf records delegation — a reconciler applying a change a user
	// requested earlier, or an agent acting for a session subject.
	OnBehalfOf string `json:"onBehalfOf,omitempty"`

	// Source — where the request came from.
	Surface       Surface `json:"surface"`
	SourceIP      string  `json:"sourceIp,omitempty"`
	UserAgent     string  `json:"userAgent,omitempty"`
	CorrelationID string  `json:"correlationId,omitempty"`
	RequestMethod string  `json:"requestMethod,omitempty"`
	// RequestPath is the route pattern ("POST /orgs/{orgName}/git-secrets"),
	// never the raw URL. Recording the pattern removes path and query-string
	// leakage structurally rather than by filtering.
	RequestPath string `json:"requestPath,omitempty"`

	// Resource — what was acted upon.
	OUID         string `json:"ouId,omitempty"`
	OrgHandle    string `json:"orgHandle,omitempty"`
	ResourceType string `json:"resourceType,omitempty"`
	ResourceID   string `json:"resourceId,omitempty"`
	ResourceName string `json:"resourceName,omitempty"`
	ProjectName  string `json:"projectName,omitempty"`
	Environment  string `json:"environment,omitempty"`

	// Result.
	Outcome      Outcome `json:"outcome"`
	StatusCode   int     `json:"statusCode,omitempty"`
	ErrorCode    string  `json:"errorCode,omitempty"`
	ErrorMessage string  `json:"errorMessage,omitempty"`
	// RequiredPermission is the scope that gated this call, recorded even when
	// the check was skipped, so a record shows on its face what would have
	// applied.
	RequiredPermission string `json:"requiredPermission,omitempty"`
	// RBACEnforced is false when RBAC_ENABLED=false. The platform defaults to
	// that state, so recording it is the difference between an audit trail that
	// documents its own enforcement gap and one that silently implies a check
	// happened.
	RBACEnforced bool `json:"rbacEnforced"`

	// Details carries operation-specific fields. Only keys present in the
	// action's schema survive; see schema.go.
	Details map[string]any `json:"details,omitempty"`
}

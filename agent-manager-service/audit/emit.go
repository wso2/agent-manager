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

package audit

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// ErrRecorderUnavailable indicates the audit trail could not accept a record
// that the caller required. A fail-closed emit site must refuse its operation
// when it sees this rather than proceeding unrecorded.
var ErrRecorderUnavailable = errors.New("audit: recorder unavailable")

// Option mutates an event under construction.
type Option func(*Event)

// BuildEvent assembles an event from the context and options without recording
// it and without marking the request as semantically described.
//
// The coverage middleware uses this to construct its envelope event: marking
// the scope there would defeat the very suppression rule the scope exists to
// drive. Semantic emit sites should call Record or Begin instead.
func BuildEvent(ctx context.Context, action Action, opts ...Option) Event {
	e := newEvent(ctx, action)
	for _, opt := range opts {
		opt(&e)
	}
	scopeFrom(ctx).applyTo(&e)
	return e
}

// Record emits an event asynchronously. It never blocks and never fails, so it
// is safe on any request path.
func Record(ctx context.Context, action Action, opts ...Option) {
	e := BuildEvent(ctx, action, opts...)
	scopeFrom(ctx).markSemantic()
	FromContext(ctx).Record(ctx, e)
}

// RecordAncillary emits an event that accompanies a request without describing
// its outcome, so the coverage tier still writes its own envelope event.
//
// Use it for facts about how a request was handled rather than what it did —
// an authorization bypass, a rate-limited rejection. Using Record for those
// would suppress the envelope and lose the record of the operation itself.
func RecordAncillary(ctx context.Context, action Action, opts ...Option) {
	FromContext(ctx).Record(ctx, BuildEvent(ctx, action, opts...))
}

// RecordSync emits an event and reports whether it was written.
//
// Use it where an unrecordable action must not happen. The caller is expected
// to abort on error.
func RecordSync(ctx context.Context, action Action, opts ...Option) error {
	e := BuildEvent(ctx, action, opts...)
	scopeFrom(ctx).markSemantic()
	return FromContext(ctx).RecordSync(ctx, e)
}

// Attempt links an intent record to its outcome record.
type Attempt struct {
	eventID uuid.UUID
	action  Action
	base    []Option
}

// Begin writes an intent record synchronously, before an operation whose effect
// lands in an external system (the identity provider, the secret store, a
// gateway broadcast).
//
// Atomicity is impossible across such a boundary, so the trail records intent
// and outcome separately. If Begin fails the caller must not proceed. If the
// process dies between Begin and Complete the intent record is left behind with
// outcome "unknown" — that orphan is the forensic signal that something was
// attempted and its result is unverified, which is strictly better than an
// operation that left no trace at all.
func Begin(ctx context.Context, action Action, opts ...Option) (*Attempt, error) {
	e := newEvent(ctx, action)
	e.Outcome = OutcomeUnknown
	for _, opt := range opts {
		opt(&e)
	}
	scopeFrom(ctx).applyTo(&e)
	scopeFrom(ctx).markSemantic()

	if e.EventID == uuid.Nil {
		e.EventID = uuid.New()
	}
	if err := FromContext(ctx).RecordSync(ctx, e); err != nil {
		return nil, fmt.Errorf("audit: refusing %s because the intent record could not be written: %w",
			action, err)
	}
	return &Attempt{eventID: e.EventID, action: action, base: opts}, nil
}

// Complete records the outcome of an attempt started with Begin.
//
// It is asynchronous: the external mutation has already happened, so failing
// the request now would report a false failure for work that succeeded.
func (a *Attempt) Complete(ctx context.Context, err error, opts ...Option) {
	if a == nil {
		return
	}
	all := make([]Option, 0, len(a.base)+len(opts)+2)
	all = append(all, a.base...)
	all = append(all, opts...)
	all = append(all, Result(err), Detail("attemptEventId", a.eventID.String()))
	Record(ctx, a.action, all...)
}

// newEvent builds the base event from the context alone: actor, org, source and
// correlation id are all ambient, which is why the emit API takes only a
// context rather than requiring every service to hold a recorder.
func newEvent(ctx context.Context, action Action) Event {
	e := Event{
		Action:        action,
		ActionClass:   action.Class(),
		Severity:      action.Severity(),
		Outcome:       OutcomeSuccess,
		ActorType:     ActorAnonymous,
		Surface:       SurfaceSystem,
		CorrelationID: utils.GetCorrelationId(ctx),
		Details:       map[string]any{},
	}

	if claims := jwtassertion.GetTokenClaims(ctx); claims != nil {
		e.ActorType = ActorUser
		e.ActorID = claims.Sub
		e.ActorOUID = claims.OuId
		e.ActorTokenID = claims.ID
		e.AuthMethod = "jwt-bearer"
	}

	if org, ok := orgctx.GetResolvedOrg(ctx); ok {
		e.OUID = org.OUID
		e.OrgHandle = org.OuHandle
	} else if e.ActorOUID != "" {
		// The org middleware has not run yet (or the route is not org-scoped),
		// but the token carries the tenant, and the token is the source of
		// truth for org identity throughout this service.
		e.OUID = e.ActorOUID
	}

	if src, ok := SourceFromContext(ctx); ok {
		e.Surface = src.Surface
		e.SourceIP = src.IP
		e.UserAgent = src.UserAgent
		e.RequestMethod = src.Method
		e.RequestPath = src.Pattern
		e.RBACEnforced = src.RBACEnforced
		e.RequiredPermission = src.RequiredPermission
		if src.ActorType != "" {
			e.ActorType = src.ActorType
		}
		if src.ActorID != "" {
			e.ActorID = src.ActorID
		}
		if src.AuthMethod != "" {
			e.AuthMethod = src.AuthMethod
		}
	}

	// Last, because it is the only source that can have run: on a surface with
	// no JWT the handler authenticates the caller, and Source was built before
	// it. Without this the coverage envelope for an authenticated gateway
	// recorded actorType "anonymous" and no id — which is precisely the
	// attribution those records exist to carry.
	if typ, id, authMethod := scopeFrom(ctx).Actor(); id != "" {
		e.ActorID = id
		if typ != "" {
			e.ActorType = typ
		}
		if authMethod != "" {
			e.AuthMethod = authMethod
		}
	}

	return e
}

// --- Options -----------------------------------------------------------------

// Resource records what the operation acted upon.
func Resource(resourceType, id string) Option {
	return func(e *Event) {
		e.ResourceType = resourceType
		e.ResourceID = id
	}
}

// ResourceNamed records what the operation acted upon, including a
// human-readable name.
//
// Names are recorded deliberately, even for secrets: "which git secret was
// deleted" is the question an investigation starts from, and a name is not
// itself a credential.
func ResourceNamed(resourceType, id, name string) Option {
	return func(e *Event) {
		e.ResourceType = resourceType
		e.ResourceID = id
		e.ResourceName = name
	}
}

// Org records the organisation the resource belongs to. Normally redundant with
// the token, but explicit for events emitted outside a request.
func Org(ouID string) Option {
	return func(e *Event) { e.OUID = ouID }
}

// OrgHandle records the organisation handle.
func OrgHandle(handle string) Option {
	return func(e *Event) { e.OrgHandle = handle }
}

// Project records the project scope.
func Project(name string) Option {
	return func(e *Event) { e.ProjectName = name }
}

// Environment records the environment scope. For deployment events this is the
// field that distinguishes a production change from a sandbox one.
func Environment(name string) Option {
	return func(e *Event) { e.Environment = name }
}

// Actor overrides the acting principal. The token subject is still recorded
// when one is present.
func Actor(actorType ActorType, id, display string) Option {
	return func(e *Event) {
		e.ActorType = actorType
		if id != "" {
			e.ActorID = id
		}
		e.ActorDisplay = display
	}
}

// OnBehalfOf records delegation — a background worker applying a change that a
// user requested earlier.
func OnBehalfOf(subject string) Option {
	return func(e *Event) { e.OnBehalfOf = subject }
}

// AuthMethod records how the caller authenticated.
func AuthMethod(method string) Option {
	return func(e *Event) { e.AuthMethod = method }
}

// SurfaceOpt overrides the entry point.
func SurfaceOpt(s Surface) Option {
	return func(e *Event) { e.Surface = s }
}

// SeverityOpt overrides the action's default severity.
func SeverityOpt(s Severity) Option {
	return func(e *Event) { e.Severity = s }
}

// Status records the HTTP status and derives the outcome from it.
func Status(code int) Option {
	return func(e *Event) {
		e.StatusCode = code
		e.Outcome = outcomeForStatus(code)
	}
}

// Result classifies an operation by its error. A nil error is success.
func Result(err error) Option {
	return func(e *Event) {
		if err == nil {
			e.Outcome = OutcomeSuccess
			return
		}
		e.Outcome = OutcomeFailure
		e.ErrorMessage = err.Error()
		var coded interface{ Code() string }
		if errors.As(err, &coded) {
			e.ErrorCode = coded.Code()
		}
	}
}

// Outcome overrides the recorded outcome.
func OutcomeOpt(o Outcome) Option {
	return func(e *Event) { e.Outcome = o }
}

// Denied records an authorization refusal and the permission that was missing.
func Denied(perm rbac.Permission) Option {
	return func(e *Event) {
		e.Outcome = OutcomeDeny
		e.RequiredPermission = perm.Scope()
	}
}

// RequiredPermissions records the permissions that gate a route. Recorded even
// when the check was skipped, so a record shows what would have applied.
func RequiredPermissions(perms ...rbac.Permission) Option {
	return func(e *Event) {
		if scope := ScopesOf(perms); scope != "" {
			e.RequiredPermission = scope
		}
	}
}

// ScopesOf renders the permissions gating a route as they are recorded: one
// scope, or several space-separated. Exported so the middleware can put the
// same string on the audit source, which is how a semantic emit — which knows
// what it changed but not which permission gated it — comes to carry one.
func ScopesOf(perms []rbac.Permission) string {
	switch len(perms) {
	case 0:
		return ""
	case 1:
		return perms[0].Scope()
	default:
		scopes := make([]string, 0, len(perms))
		for _, p := range perms {
			scopes = append(scopes, p.Scope())
		}
		return joinScopes(scopes)
	}
}

// RBACEnforced records whether authorization was actually checked.
func RBACEnforced(enforced bool) Option {
	return func(e *Event) { e.RBACEnforced = enforced }
}

// Detail attaches one structured field.
//
// Only scalars and string slices are accepted. Refusing structs and maps is
// what keeps request payloads — and the secrets in them — structurally unable
// to reach a record. Anything not declared in the action's schema is dropped at
// write time; see schema.go.
func Detail(key string, value any) Option {
	return func(e *Event) {
		if e.Details == nil {
			e.Details = map[string]any{}
		}
		switch v := value.(type) {
		case string, bool, int, int32, int64, float64, []string:
			e.Details[key] = v
		case fmt.Stringer:
			e.Details[key] = v.String()
		default:
			// Recording the type rather than the value keeps an unvetted payload
			// out of the trail while still showing that the emit site tried.
			e.Details[key] = fmt.Sprintf("[unsupported:%T]", value)
		}
	}
}

// SecretRef records a non-reversible reference to a secret, so a record can
// correlate credential lifecycle events without ever holding usable material.
func SecretRef(key, value string) Option {
	return func(e *Event) {
		if e.Details == nil {
			e.Details = map[string]any{}
		}
		e.Details[key] = Fingerprint(value)
	}
}

// outcomeForStatus maps an HTTP status to an outcome. 401 and 403 are
// deliberately separated from other failures: a refusal is a security event,
// while a 400 is a client mistake.
func outcomeForStatus(code int) Outcome {
	switch {
	case code == 401 || code == 403:
		return OutcomeDeny
	case code >= 400:
		return OutcomeFailure
	default:
		return OutcomeSuccess
	}
}

func joinScopes(scopes []string) string {
	out := ""
	for i, s := range scopes {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out
}

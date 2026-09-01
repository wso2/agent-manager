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
	"sync"
)

// Source describes the entry point a request arrived through. It is installed
// once per request by the surface's middleware, so that a semantic emit deep in
// a service does not need to know how the call reached it.
type Source struct {
	Surface   Surface
	IP        string
	UserAgent string
	Method    string
	// Pattern is the route pattern, never the raw URL.
	Pattern string
	// ActorType overrides the default actor classification for surfaces that
	// know better than the token does — the internal gateway server, the score
	// publisher, background workers.
	ActorType ActorType
	// ActorID names the principal on surfaces that do not carry a JWT.
	ActorID string
	// AuthMethod records how the caller authenticated.
	AuthMethod string
	// RBACEnforced is false when authorization checks were skipped.
	RBACEnforced bool
	// RequiredPermission is the scope gating this route, carried alongside
	// RBACEnforced because the pair is only meaningful together: "no check
	// happened" is half an answer without "and this is the check that would
	// have run". The coverage tier passes it explicitly, but a semantic emit
	// does not know its route, so it reaches those records through here.
	RequiredPermission string
}

type (
	recorderKey struct{}
	sourceKey   struct{}
	scopeKey    struct{}
)

// WithRecorder returns ctx carrying the recorder that emits for this request.
func WithRecorder(ctx context.Context, r Recorder) context.Context {
	return context.WithValue(ctx, recorderKey{}, r)
}

// FromContext returns the recorder installed on ctx. It never returns nil: a
// context without a recorder yields one that records nothing but complains
// loudly, so a missing installation surfaces as noise in the logs rather than
// as a silently empty audit trail.
func FromContext(ctx context.Context) Recorder {
	if r, ok := ctx.Value(recorderKey{}).(Recorder); ok && r != nil {
		return r
	}
	return uninstalled
}

// WithSource returns ctx carrying the request's source description.
func WithSource(ctx context.Context, s Source) context.Context {
	return context.WithValue(ctx, sourceKey{}, s)
}

// SourceFromContext returns the source installed on ctx, if any.
func SourceFromContext(ctx context.Context) (Source, bool) {
	s, ok := ctx.Value(sourceKey{}).(Source)
	return s, ok
}

// RequestScope tracks what has already been recorded for one request.
//
// It is what keeps the two tiers from double-recording: the coverage middleware
// suppresses its own envelope event when a semantic emit already described the
// operation and the request succeeded. On failure the envelope event is still
// written, because a request that failed before reaching the service emits
// nothing semantic and would otherwise vanish from the trail entirely.
type RequestScope struct {
	mu       sync.Mutex
	semantic bool
	suppress bool
	actor    scopeActor
	resource resourceRef
}

// scopeActor is a principal named by the handler after it authenticated the
// caller, on a surface where the middleware could not know it up front.
type scopeActor struct {
	typ        ActorType
	id         string
	authMethod string
}

type resourceRef struct {
	kind string
	id   string
	name string
}

// NewRequestScope returns ctx carrying a fresh scope, and the scope itself.
func NewRequestScope(ctx context.Context) (context.Context, *RequestScope) {
	scope := &RequestScope{}
	return context.WithValue(ctx, scopeKey{}, scope), scope
}

// scopeFrom returns the scope on ctx, or nil when there is none — which is the
// normal case outside an HTTP request (background workers, unit tests).
func scopeFrom(ctx context.Context) *RequestScope {
	scope, _ := ctx.Value(scopeKey{}).(*RequestScope)
	return scope
}

// SemanticEmitted reports whether a semantic event was recorded for this request.
func (s *RequestScope) SemanticEmitted() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.semantic
}

// Suppressed reports whether the envelope event was explicitly suppressed.
func (s *RequestScope) Suppressed() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.suppress
}

// IdentifyActor names the principal behind a request on a surface that carries
// no JWT, once the handler has authenticated it.
//
// Source is built before the handler runs, so on those surfaces it holds only
// an address — and the handler is what authenticates the caller. Without this,
// two things went wrong on the internal gateway server: the per-caller
// coalescing fell back to the source IP, so two gateways sharing an egress
// address suppressed each other's records; and the coverage envelope recorded
// an authenticated gateway as `actorType: anonymous` with no id, so a record
// whose stated purpose is "which gateway pulled key material, and when" could
// not name the gateway.
//
// The scope is a pointer on the context, so a value set here is visible to the
// envelope emitted after the handler returns. Outside a request this is a
// no-op. An explicit audit.Actor option still wins, since options are applied
// after the event is built.
func IdentifyActor(ctx context.Context, actorType ActorType, id, authMethod string) {
	if id == "" {
		return
	}
	scope := scopeFrom(ctx)
	if scope == nil {
		return
	}
	scope.mu.Lock()
	defer scope.mu.Unlock()
	scope.actor = scopeActor{typ: actorType, id: id, authMethod: authMethod}
}

// Actor returns the principal named by IdentifyActor. The id is "" when none
// was named.
func (s *RequestScope) Actor() (actorType ActorType, id, authMethod string) {
	if s == nil {
		return "", "", ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.actor.typ, s.actor.id, s.actor.authMethod
}

func (s *RequestScope) markSemantic() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.semantic = true
}

// Skip suppresses the envelope audit event for this request. Intended for the
// rare chatty endpoint whose records would drown the trail; it does not affect
// semantic emits.
func Skip(ctx context.Context) {
	scope := scopeFrom(ctx)
	if scope == nil {
		return
	}
	scope.mu.Lock()
	defer scope.mu.Unlock()
	scope.suppress = true
}

// Annotate attaches resource identity to the request scope so the envelope
// event can name what was acted upon. A handler that knows the entity it
// touched calls this; it costs one line and no new dependency.
//
// It is a no-op outside an audited request, which keeps services testable
// without any audit wiring.
func Annotate(ctx context.Context, resourceType, id, name string) {
	scope := scopeFrom(ctx)
	if scope == nil {
		return
	}
	scope.mu.Lock()
	defer scope.mu.Unlock()
	scope.resource = resourceRef{kind: resourceType, id: id, name: name}
}

// applyTo copies scope-collected identity onto an event that does not already
// carry its own. An explicit option on the emit site always wins, so a handler
// annotation acts as a default rather than an override.
func (s *RequestScope) applyTo(e *Event) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.ResourceType == "" {
		e.ResourceType = s.resource.kind
	}
	if e.ResourceID == "" {
		e.ResourceID = s.resource.id
	}
	if e.ResourceName == "" {
		e.ResourceName = s.resource.name
	}
}

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

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// captureRecorder retains events so a test can assert on what the middleware
// produced.
type captureRecorder struct {
	events []audit.Event
}

func (c *captureRecorder) Record(_ context.Context, e audit.Event) {
	c.events = append(c.events, e)
}

func (c *captureRecorder) RecordSync(_ context.Context, e audit.Event) error {
	c.events = append(c.events, e)
	return nil
}

func (c *captureRecorder) Close(context.Context) error { return nil }

// serve runs a handler through WithAudit and returns the captured events.
func serveAudited(t *testing.T, meta audit.RouteMeta, handler http.HandlerFunc) (*captureRecorder, *httptest.ResponseRecorder) {
	t.Helper()

	rec := &captureRecorder{}
	wrapped := WithAudit(rec, meta)(handler)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(meta.Method, "/orgs/acme/projects", nil)
	wrapped(w, r)

	return rec, w
}

func mutatingMeta(t *testing.T) audit.RouteMeta {
	t.Helper()
	pattern := "POST /orgs/{orgName}/projects"
	return audit.NewRouteMeta(pattern, audit.ExtractPathParams(pattern), []rbac.Permission{rbac.ProjectCreate})
}

func TestAuditMiddlewareRecordsSuccess(t *testing.T) {
	rec, _ := serveAudited(t, mutatingMeta(t), func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	if len(rec.events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(rec.events))
	}
	e := rec.events[0]
	if e.Action != "project:create" {
		t.Errorf("Action = %q, want project:create", e.Action)
	}
	if e.Outcome != audit.OutcomeSuccess {
		t.Errorf("Outcome = %q, want success", e.Outcome)
	}
	if e.StatusCode != http.StatusCreated {
		t.Errorf("StatusCode = %d, want 201", e.StatusCode)
	}
	if e.RequiredPermission != "amp:project:create" {
		t.Errorf("RequiredPermission = %q", e.RequiredPermission)
	}
}

// TestAuditMiddlewareRecordsRoutePatternNotRawURL pins a security property:
// recording the pattern rather than the URL removes path and query-string
// leakage structurally.
func TestAuditMiddlewareRecordsRoutePatternNotRawURL(t *testing.T) {
	meta := mutatingMeta(t)
	rec := &captureRecorder{}
	wrapped := WithAudit(rec, meta)(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/orgs/acme/projects?token=super-secret", nil)
	wrapped(w, r)

	if len(rec.events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(rec.events))
	}
	if got := rec.events[0].RequestPath; got != "/orgs/{orgName}/projects" {
		t.Errorf("RequestPath = %q, want the route pattern", got)
	}
}

// TestAuditMiddlewareRecordsDenial covers the case that matters most: a request
// rejected before it reached the handler must still appear in the trail.
func TestAuditMiddlewareRecordsDenial(t *testing.T) {
	rec, _ := serveAudited(t, mutatingMeta(t), func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	if len(rec.events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(rec.events))
	}
	if rec.events[0].Outcome != audit.OutcomeDeny {
		t.Errorf("Outcome = %q, want deny", rec.events[0].Outcome)
	}
}

// TestAuditMiddlewareDefaultsToOKWhenHandlerWritesNothing guards against
// recording a 0 status for the handlers that return without writing.
func TestAuditMiddlewareDefaultsToOKWhenHandlerWritesNothing(t *testing.T) {
	rec, _ := serveAudited(t, mutatingMeta(t), func(http.ResponseWriter, *http.Request) {})

	if len(rec.events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(rec.events))
	}
	if rec.events[0].StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", rec.events[0].StatusCode)
	}
}

// TestAuditMiddlewareRecordsPanicAsFailureAndRepanics is the ordering case.
// RecovererOnPanic is installed outside this middleware, so a panic unwinds
// through here first; without special handling the request would be recorded as
// a success even though it becomes a 500. The panic must also still propagate,
// so the recoverer's behaviour is unchanged.
func TestAuditMiddlewareRecordsPanicAsFailureAndRepanics(t *testing.T) {
	rec := &captureRecorder{}
	wrapped := WithAudit(rec, mutatingMeta(t))(func(http.ResponseWriter, *http.Request) {
		panic("handler exploded")
	})

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("panic did not propagate; the outer recoverer would never run")
			}
		}()
		wrapped(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/orgs/acme/projects", nil))
	}()

	if len(rec.events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(rec.events))
	}
	e := rec.events[0]
	if e.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", e.StatusCode)
	}
	if e.Outcome != audit.OutcomeFailure {
		t.Errorf("Outcome = %q, want failure", e.Outcome)
	}
}

// TestAuditMiddlewareSuppressesEnvelopeAfterSemanticEmit is the deduplication
// rule: one operation should yield one record, the richer one.
func TestAuditMiddlewareSuppressesEnvelopeAfterSemanticEmit(t *testing.T) {
	rec, _ := serveAudited(t, mutatingMeta(t), func(w http.ResponseWriter, r *http.Request) {
		audit.Record(r.Context(), "project:create",
			audit.ResourceNamed("project", "proj-1", "my-project"))
		w.WriteHeader(http.StatusCreated)
	})

	if len(rec.events) != 1 {
		t.Fatalf("recorded %d events, want just the semantic one", len(rec.events))
	}
	if rec.events[0].ResourceID != "proj-1" {
		t.Errorf("the surviving event is the envelope, not the semantic one: %+v", rec.events[0])
	}
	if _, isEnvelope := rec.events[0].Details["envelope"]; isEnvelope {
		t.Error("the envelope event survived instead of the semantic one")
	}
}

// TestAuditMiddlewareKeepsEnvelopeOnFailureAfterSemanticEmit is the other half
// of the rule. A failure after a semantic emit must still be recorded as a
// failure, or a partially applied operation would look like it succeeded.
func TestAuditMiddlewareKeepsEnvelopeOnFailureAfterSemanticEmit(t *testing.T) {
	rec, _ := serveAudited(t, mutatingMeta(t), func(w http.ResponseWriter, r *http.Request) {
		audit.Record(r.Context(), "project:create", audit.Resource("project", "proj-1"))
		w.WriteHeader(http.StatusInternalServerError)
	})

	if len(rec.events) != 2 {
		t.Fatalf("recorded %d events, want the semantic event plus a failure envelope", len(rec.events))
	}
	if rec.events[1].Outcome != audit.OutcomeFailure {
		t.Errorf("envelope outcome = %q, want failure", rec.events[1].Outcome)
	}
}

func TestAuditMiddlewareHonoursSkip(t *testing.T) {
	rec, _ := serveAudited(t, mutatingMeta(t), func(w http.ResponseWriter, r *http.Request) {
		audit.Skip(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	if len(rec.events) != 0 {
		t.Errorf("recorded %d events, want none after Skip", len(rec.events))
	}
}

func TestAuditMiddlewareCarriesHandlerAnnotation(t *testing.T) {
	rec, _ := serveAudited(t, mutatingMeta(t), func(w http.ResponseWriter, r *http.Request) {
		audit.Annotate(r.Context(), "project", "proj-42", "annotated-project")
		w.WriteHeader(http.StatusCreated)
	})

	if len(rec.events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(rec.events))
	}
	e := rec.events[0]
	if e.ResourceID != "proj-42" || e.ResourceName != "annotated-project" {
		t.Errorf("annotation did not reach the envelope event: %+v", e)
	}
}

// TestAuditMiddlewareIsPassThroughForUnauditedRoutes keeps ordinary reads off
// the trail without any per-request cost.
func TestAuditMiddlewareIsPassThroughForUnauditedRoutes(t *testing.T) {
	pattern := "GET /orgs/{orgName}/projects"
	meta := audit.NewRouteMeta(pattern, audit.ExtractPathParams(pattern), []rbac.Permission{rbac.ProjectRead})

	rec, w := serveAudited(t, meta, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	if len(rec.events) != 0 {
		t.Errorf("recorded %d events for an ordinary read, want none", len(rec.events))
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want the handler's response to pass through", w.Code)
	}
}

func TestAuditMiddlewareRecordsSensitiveRead(t *testing.T) {
	pattern := "GET /orgs/{orgName}/identities/permissions"
	meta := audit.NewRouteMeta(pattern, audit.ExtractPathParams(pattern), []rbac.Permission{rbac.RoleRead})

	rec := &captureRecorder{}
	wrapped := WithAudit(rec, meta)(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	wrapped(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/orgs/acme/identities/permissions", nil))

	if len(rec.events) != 1 {
		t.Fatalf("recorded %d events, want 1 for a credential-disclosing read", len(rec.events))
	}
	if rec.events[0].Action != "permission:list" {
		t.Errorf("Action = %q, want permission:list", rec.events[0].Action)
	}
}

// TestResponseRecorderUnwrap matters because wrapping the writer would otherwise
// break http.ResponseController for any streaming handler added later.
func TestResponseRecorderUnwrap(t *testing.T) {
	inner := httptest.NewRecorder()
	wrapped := newResponseRecorder(inner)

	if wrapped.Unwrap() != inner {
		t.Error("Unwrap did not return the underlying writer")
	}
	if _, ok := any(wrapped).(interface{ Unwrap() http.ResponseWriter }); !ok {
		t.Error("responseRecorder does not satisfy the Unwrap contract")
	}
}

func TestResponseRecorderFirstWriteWins(t *testing.T) {
	wrapped := newResponseRecorder(httptest.NewRecorder())

	wrapped.WriteHeader(http.StatusCreated)
	wrapped.WriteHeader(http.StatusInternalServerError)

	if got := wrapped.Status(); got != http.StatusCreated {
		t.Errorf("Status = %d, want the first status written (201)", got)
	}
}

func TestResponseRecorderImpliesOKOnWrite(t *testing.T) {
	wrapped := newResponseRecorder(httptest.NewRecorder())

	if _, err := wrapped.Write([]byte("body")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := wrapped.Status(); got != http.StatusOK {
		t.Errorf("Status = %d, want 200 implied by the first write", got)
	}
}

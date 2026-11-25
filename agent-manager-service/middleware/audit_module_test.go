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
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// serveModuleRoute runs a handler through WithAudit as a module route at
// pattern, requested at target.
func serveModuleRoute(t *testing.T, recorder audit.Recorder, pattern, target string, p audit.RoutePolicy, h http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	meta := audit.NewRouteMetaWithPolicy(pattern, audit.ExtractPathParams(pattern), []rbac.Permission{"widgets:widgets.update"}, audit.SurfaceAPI, p)
	mux := http.NewServeMux()
	mux.HandleFunc(pattern, WithAudit(recorder, meta)(h))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(meta.Method, target, nil))
	return w
}

// A module names what it acted on with modkit.AuditResource and AuditDetail,
// and the route's own record carries them, with the project and environment
// from the path.
func TestModuleRouteRecordCarriesModuleDetails(t *testing.T) {
	rec := &captureRecorder{}
	serveModuleRoute(t, rec, "PUT /orgs/{orgName}/projects/{projName}/widgets/{envName}", "/orgs/acme/projects/shop/widgets/dev",
		audit.RoutePolicy{}, func(w http.ResponseWriter, r *http.Request) {
			modkit.AuditResource(r.Context(), "widget", "w-1", "blue")
			modkit.AuditDetail(r.Context(), "colour", "blue")
			w.WriteHeader(http.StatusOK)
		})

	if len(rec.events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(rec.events))
	}
	e := rec.events[0]
	if e.ResourceType != "widget" || e.ResourceID != "w-1" || e.ResourceName != "blue" {
		t.Errorf("resource = %s/%s/%s", e.ResourceType, e.ResourceID, e.ResourceName)
	}
	if e.Details["colour"] != "blue" {
		t.Errorf("details = %v", e.Details)
	}
	if e.ProjectName != "shop" || e.Environment != "dev" {
		t.Errorf("project/environment = %q/%q, want shop/dev", e.ProjectName, e.Environment)
	}
}

// A fail-closed route writes its intent record before the handler, and its
// own record names that intent record.
func TestFailClosedRouteRecordsIntentFirst(t *testing.T) {
	rec := &captureRecorder{}
	ran := false
	serveModuleRoute(t, rec, "POST /orgs/{orgName}/keys", "/orgs/acme/keys",
		audit.RoutePolicy{FailClosed: true}, func(w http.ResponseWriter, _ *http.Request) {
			ran = len(rec.events) == 1
			w.WriteHeader(http.StatusCreated)
		})

	if !ran {
		t.Fatal("the intent record must be written before the handler runs")
	}
	if len(rec.events) != 2 {
		t.Fatalf("recorded %d events, want the intent and the outcome", len(rec.events))
	}
	if rec.events[0].Outcome != audit.OutcomeUnknown || rec.events[1].Outcome != audit.OutcomeSuccess {
		t.Errorf("outcomes = %s, %s", rec.events[0].Outcome, rec.events[1].Outcome)
	}
	if rec.events[1].Details["attemptEventId"] != rec.events[0].EventID.String() {
		t.Errorf("the outcome record must name its intent record")
	}
}

type refusingRecorder struct{ captureRecorder }

func (*refusingRecorder) RecordSync(context.Context, audit.Event) error {
	return errors.New("sink down")
}

// A fail-closed route refuses with 503, without running, when its intent
// record cannot be written.
func TestFailClosedRouteRefusesUnrecorded(t *testing.T) {
	ran := false
	w := serveModuleRoute(t, &refusingRecorder{}, "POST /orgs/{orgName}/keys", "/orgs/acme/keys",
		audit.RoutePolicy{FailClosed: true}, func(http.ResponseWriter, *http.Request) { ran = true })

	if ran || w.Code != http.StatusServiceUnavailable {
		t.Errorf("ran = %v, status = %d; want a 503 without running", ran, w.Code)
	}
}

// A refused internal caller is recorded once per source and reason; an
// authenticated one names the request's actor.
func TestAuditInternalCaller(t *testing.T) {
	rec := &captureRecorder{}
	ctx, scope := audit.NewRequestScope(audit.WithRecorder(context.Background(), rec))
	ctx = audit.WithSource(ctx, audit.Source{IP: "203.0.113.9"})
	refused := modkit.InternalCaller{Kind: "gateway", AuthMethod: "api-key", Refused: "unit-test-refusal"}
	modkit.AuditInternalCaller(ctx, refused)
	modkit.AuditInternalCaller(ctx, refused)
	if len(rec.events) != 1 || rec.events[0].Action != audit.ActionAuthnFailure {
		t.Fatalf("recorded %v, want one authn failure", rec.events)
	}

	modkit.AuditInternalCaller(ctx, modkit.InternalCaller{Kind: "gateway", ID: "gw-1", AuthMethod: "api-key"})
	if typ, id, _ := scope.Actor(); typ != audit.ActorGateway || id != "gw-1" {
		t.Errorf("actor = %s/%s, want gateway/gw-1", typ, id)
	}
}

// A route without {projName} names its project in the project query parameter, and the path wins over it.
func TestModuleRouteRecordTakesProjectFromQuery(t *testing.T) {
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	cases := []struct{ name, pattern, target, want string }{
		{"query", "PUT /widgets/{widget}", "/widgets/w-1?project=shop", "shop"},
		{"path wins", "PUT /projects/{projName}/widgets/{widget}", "/projects/shop/widgets/w-1?project=other", "shop"},
		{"neither", "PUT /widgets/{widget}", "/widgets/w-1", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &captureRecorder{}
			serveModuleRoute(t, rec, tc.pattern, tc.target, audit.RoutePolicy{}, ok)
			if len(rec.events) != 1 {
				t.Fatalf("recorded %d events, want 1", len(rec.events))
			}
			if got := rec.events[0].ProjectName; got != tc.want {
				t.Errorf("project = %q, want %q", got, tc.want)
			}
		})
	}
}

// A module names a project the request carries in its body with modkit.AuditProject; a query parameter still wins.
func TestModuleRouteRecordCarriesAuditProject(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		modkit.AuditProject(r.Context(), "from-body")
		w.WriteHeader(http.StatusCreated)
	}
	rec := &captureRecorder{}
	serveModuleRoute(t, rec, "POST /widgets", "/widgets", audit.RoutePolicy{}, handler)
	if len(rec.events) != 1 || rec.events[0].ProjectName != "from-body" {
		t.Fatalf("events = %+v, want one naming project from-body", rec.events)
	}

	rec = &captureRecorder{}
	serveModuleRoute(t, rec, "POST /widgets", "/widgets?project=shop", audit.RoutePolicy{}, handler)
	if len(rec.events) != 1 || rec.events[0].ProjectName != "shop" {
		t.Fatalf("events = %+v, want one naming project shop", rec.events)
	}
}

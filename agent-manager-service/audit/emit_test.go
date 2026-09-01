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
	"net/http"
	"testing"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// recordingCtx wires a memory sink into a context the way a request would.
func recordingCtx(t *testing.T) (context.Context, *memorySink, Recorder) {
	t.Helper()

	sink := NewMemorySink()
	rec := NewRecorder(sink, quietLogger(), Config{BufferSize: 32, BatchSize: 1})
	t.Cleanup(func() { _ = rec.Close(context.Background()) })

	return WithRecorder(context.Background(), rec), sink, rec
}

// TestEmitIsNoOpWithoutARecorder is the guarantee that keeps services testable:
// a unit test with a bare context must not need any audit wiring, and must not
// panic when the code under test emits.
func TestEmitIsNoOpWithoutARecorder(t *testing.T) {
	ctx := context.Background()

	// None of these may panic.
	Record(ctx, "agent:create")
	RecordAncillary(ctx, "agent:create")
	Annotate(ctx, "agent", "id", "name")
	Skip(ctx)

	if err := RecordSync(ctx, "git-secret:create"); !errors.Is(err, ErrRecorderUnavailable) {
		t.Errorf("RecordSync = %v, want ErrRecorderUnavailable", err)
	}
}

func TestNewEventReadsActorFromTokenClaims(t *testing.T) {
	claims := &jwtassertion.TokenClaims{
		Sub:              "alice@example.com",
		Scope:            "amp:agent:create amp:agent:read",
		OuId:             "ou-123",
		OuHandle:         "acme",
		RegisteredClaims: jwt.RegisteredClaims{ID: "jti-abc"},
	}
	ctx := jwtassertion.ContextWithTokenClaims(context.Background(), claims)

	e := newEvent(ctx, "agent:create")

	if e.ActorID != "alice@example.com" {
		t.Errorf("ActorID = %q", e.ActorID)
	}
	if e.ActorType != ActorUser {
		t.Errorf("ActorType = %q, want user", e.ActorType)
	}
	// jti is the only join key back to the identity provider's login records,
	// since authentication happens outside this service entirely.
	if e.ActorTokenID != "jti-abc" {
		t.Errorf("ActorTokenID = %q, want the token jti", e.ActorTokenID)
	}
	if e.AuthMethod != "jwt-bearer" {
		t.Errorf("AuthMethod = %q", e.AuthMethod)
	}
}

// TestNewEventFallsBackToTokenOrgBeforeOrgMiddleware matters for denials: a
// request refused by the org middleware still has to be attributable to a tenant.
func TestNewEventFallsBackToTokenOrgBeforeOrgMiddleware(t *testing.T) {
	ctx := jwtassertion.ContextWithTokenClaims(context.Background(), &jwtassertion.TokenClaims{
		Sub: "alice", OuId: "ou-123", OuHandle: "acme",
	})

	if got := newEvent(ctx, "agent:create").OUID; got != "ou-123" {
		t.Errorf("OUID = %q, want the token OU when the org middleware has not run", got)
	}
}

func TestNewEventPrefersResolvedOrg(t *testing.T) {
	ctx := jwtassertion.ContextWithTokenClaims(context.Background(), &jwtassertion.TokenClaims{
		Sub: "alice", OuId: "ou-token", OuHandle: "token-handle",
	})
	ctx = orgctx.WithResolvedOrg(ctx, orgctx.ResolvedOrg{OUID: "ou-resolved", OuHandle: "resolved-handle"})

	e := newEvent(ctx, "agent:create")
	if e.OUID != "ou-resolved" || e.OrgHandle != "resolved-handle" {
		t.Errorf("OUID/OrgHandle = %q/%q, want the resolved org", e.OUID, e.OrgHandle)
	}
}

func TestNewEventReadsSource(t *testing.T) {
	ctx := WithSource(context.Background(), Source{
		Surface:      SurfaceMCP,
		IP:           "203.0.113.5",
		UserAgent:    "amctl/1.0",
		Method:       http.MethodPost,
		Pattern:      "/orgs/{orgName}/projects",
		ActorType:    ActorService,
		ActorID:      "gateway-1",
		AuthMethod:   "api-key",
		RBACEnforced: true,
	})

	e := newEvent(ctx, "project:create")

	if e.Surface != SurfaceMCP || e.SourceIP != "203.0.113.5" || e.RequestPath != "/orgs/{orgName}/projects" {
		t.Errorf("source not applied: %+v", e)
	}
	if e.ActorType != ActorService || e.ActorID != "gateway-1" || e.AuthMethod != "api-key" {
		t.Errorf("surface actor override not applied: %+v", e)
	}
	if !e.RBACEnforced {
		t.Error("RBACEnforced was not carried from the source")
	}
}

func TestRecordMarksScopeSemanticButBuildEventDoesNot(t *testing.T) {
	ctx, _, _ := recordingCtx(t)
	ctx, scope := NewRequestScope(ctx)

	BuildEvent(ctx, "agent:create")
	if scope.SemanticEmitted() {
		t.Error("BuildEvent must not mark the scope; the envelope tier uses it and would suppress itself")
	}

	Record(ctx, "agent:create")
	if !scope.SemanticEmitted() {
		t.Error("Record should mark the scope so the envelope tier stands down")
	}
}

func TestRecordAncillaryDoesNotMarkScope(t *testing.T) {
	ctx, _, _ := recordingCtx(t)
	ctx, scope := NewRequestScope(ctx)

	RecordAncillary(ctx, ActionAuthzRootOUBypass)

	if scope.SemanticEmitted() {
		t.Error("an ancillary event must leave the envelope event in place")
	}
}

func TestAnnotateFillsResourceIdentity(t *testing.T) {
	ctx, sink, rec := recordingCtx(t)
	ctx, _ = NewRequestScope(ctx)

	Annotate(ctx, "agent", "agent-uuid", "my-agent")
	Record(ctx, "agent:create")

	if err := rec.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	events := sink.Events()
	if len(events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(events))
	}
	if events[0].ResourceType != "agent" || events[0].ResourceID != "agent-uuid" || events[0].ResourceName != "my-agent" {
		t.Errorf("annotation not applied: %+v", events[0])
	}
}

func TestExplicitResourceBeatsAnnotation(t *testing.T) {
	ctx, sink, rec := recordingCtx(t)
	ctx, _ = NewRequestScope(ctx)

	Annotate(ctx, "agent", "from-annotation", "annotated")
	Record(ctx, "agent:create", ResourceNamed("agent", "explicit", "explicit-name"))

	if err := rec.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	events := sink.Events()
	if len(events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(events))
	}
	if events[0].ResourceID != "explicit" {
		t.Errorf("ResourceID = %q, want the explicit option to win", events[0].ResourceID)
	}
}

func TestStatusDerivesOutcome(t *testing.T) {
	tests := []struct {
		status int
		want   Outcome
	}{
		{200, OutcomeSuccess},
		{201, OutcomeSuccess},
		{204, OutcomeSuccess},
		{400, OutcomeFailure},
		{401, OutcomeDeny},
		{403, OutcomeDeny},
		{404, OutcomeFailure},
		{500, OutcomeFailure},
	}

	for _, tt := range tests {
		e := Event{}
		Status(tt.status)(&e)
		if e.Outcome != tt.want {
			t.Errorf("status %d gave outcome %q, want %q", tt.status, e.Outcome, tt.want)
		}
	}
}

func TestResultClassifiesErrors(t *testing.T) {
	e := Event{}
	Result(nil)(&e)
	if e.Outcome != OutcomeSuccess {
		t.Errorf("nil error gave %q, want success", e.Outcome)
	}

	e = Event{}
	Result(errors.New("upstream refused"))(&e)
	if e.Outcome != OutcomeFailure {
		t.Errorf("error gave %q, want failure", e.Outcome)
	}
	if e.ErrorMessage != "upstream refused" {
		t.Errorf("ErrorMessage = %q", e.ErrorMessage)
	}
}

func TestDeniedRecordsTheMissingScope(t *testing.T) {
	e := Event{}
	Denied(rbac.AgentCreate)(&e)

	if e.Outcome != OutcomeDeny {
		t.Errorf("Outcome = %q, want deny", e.Outcome)
	}
	if e.RequiredPermission != "amp:agent:create" {
		t.Errorf("RequiredPermission = %q, want the wire-format scope", e.RequiredPermission)
	}
}

func TestRequiredPermissionsJoinsMultiple(t *testing.T) {
	e := Event{}
	RequiredPermissions(rbac.AgentCreate, rbac.AgentRead)(&e)

	if e.RequiredPermission != "amp:agent:create amp:agent:read" {
		t.Errorf("RequiredPermission = %q", e.RequiredPermission)
	}
}

func TestSecretRefStoresOnlyAFingerprint(t *testing.T) {
	e := Event{Details: map[string]any{}}
	SecretRef("keyRef", "sk-live-super-secret")(&e)

	got, _ := e.Details["keyRef"].(string)
	if got == "sk-live-super-secret" {
		t.Fatal("SecretRef stored the raw secret")
	}
	if got != Fingerprint("sk-live-super-secret") {
		t.Errorf("keyRef = %q, want a fingerprint", got)
	}
}

// TestBeginWritesIntentSynchronously covers the fail-closed protocol for
// external mutations: the intent record must exist before the operation runs.
func TestBeginWritesIntentSynchronously(t *testing.T) {
	ctx, sink, _ := recordingCtx(t)

	attempt, err := Begin(ctx, "git-secret:create", ResourceNamed("git-secret", "gs-1", "my-secret"))
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if attempt == nil {
		t.Fatal("Begin returned no attempt")
	}

	events := sink.Events()
	if len(events) != 1 {
		t.Fatalf("recorded %d events, want the intent record written before returning", len(events))
	}
	if events[0].Outcome != OutcomeUnknown {
		t.Errorf("intent outcome = %q, want unknown", events[0].Outcome)
	}
}

// TestBeginRefusesWhenTheTrailIsUnavailable is the point of the whole protocol:
// if the record cannot be written, the caller must be told so it can abort
// rather than perform a privileged operation untraceably.
func TestBeginRefusesWhenTheTrailIsUnavailable(t *testing.T) {
	sentinel := errors.New("sink is down")
	rec := NewRecorder(NewFailingSink(sentinel), quietLogger(), Config{})
	t.Cleanup(func() { _ = rec.Close(context.Background()) })

	attempt, err := Begin(WithRecorder(context.Background(), rec), "git-secret:create")

	if err == nil {
		t.Fatal("Begin succeeded although the intent record could not be written")
	}
	if attempt != nil {
		t.Error("Begin returned an attempt despite failing")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want it to wrap the sink failure", err)
	}
}

func TestCompleteLinksBackToTheIntentRecord(t *testing.T) {
	ctx, sink, rec := recordingCtx(t)

	attempt, err := Begin(ctx, "git-secret:create", Resource("git-secret", "gs-1"))
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	attempt.Complete(ctx, nil)

	if err := rec.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	events := sink.Events()
	if len(events) != 2 {
		t.Fatalf("recorded %d events, want an intent and an outcome", len(events))
	}

	intent, outcome := events[0], events[1]
	if outcome.Outcome != OutcomeSuccess {
		t.Errorf("outcome record = %q, want success", outcome.Outcome)
	}
	if got, _ := outcome.Details["attemptEventId"].(string); got != intent.EventID.String() {
		t.Errorf("attemptEventId = %q, want the intent event id %q", got, intent.EventID)
	}
	// The resource identity from Begin must carry into Complete so the outcome
	// record stands on its own.
	if outcome.ResourceID != "gs-1" {
		t.Errorf("outcome ResourceID = %q, want it inherited from Begin", outcome.ResourceID)
	}
}

func TestCompleteRecordsFailure(t *testing.T) {
	ctx, sink, rec := recordingCtx(t)

	attempt, err := Begin(ctx, "git-secret:create")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	attempt.Complete(ctx, errors.New("vault rejected the write"))

	if err := rec.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	events := sink.Events()
	if len(events) != 2 {
		t.Fatalf("recorded %d events, want 2", len(events))
	}
	if events[1].Outcome != OutcomeFailure {
		t.Errorf("outcome = %q, want failure", events[1].Outcome)
	}
}

func TestCompleteOnNilAttemptIsSafe(t *testing.T) {
	// Begin returns nil on failure; a caller that ignores the error must not
	// crash the process on the deferred Complete.
	var attempt *Attempt
	attempt.Complete(context.Background(), nil)
}

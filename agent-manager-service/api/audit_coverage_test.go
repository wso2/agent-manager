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

package api

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// controllerConstructors is every controller constructor reached by
// registerAPIRoutes.
//
// The routes take method values off each controller, so the fields cannot be
// left nil — a method value on a nil interface panics at registration. The
// constructors are called with zero-valued arguments through reflection rather
// than by hand: no handler is ever invoked, and doing it reflectively keeps
// this list from breaking every time an unrelated constructor gains a
// dependency.
var controllerConstructors = []any{
	controllers.NewAgentController,
	controllers.NewAgentKindController,
	controllers.NewInfraResourceController,
	controllers.NewAgentTokenController,
	controllers.NewRepositoryController,
	controllers.NewEnvironmentController,
	controllers.NewGatewayController,
	controllers.NewLLMController,
	controllers.NewLLMDeploymentController,
	controllers.NewLLMProviderAPIKeyController,
	controllers.NewLLMProxyAPIKeyController,
	controllers.NewAgentAPIKeyController,
	controllers.NewLLMProxyDeploymentController,
	controllers.NewMCPProxyController,
	controllers.NewMonitorController,
	controllers.NewMonitorScoresController,
	controllers.NewMonitorScoresPublisherController,
	controllers.NewEvaluatorController,
	controllers.NewCatalogController,
	controllers.NewAgentBuildOptionsController,
	controllers.NewAgentConfigurationController,
	controllers.NewGitSecretController,
	controllers.NewIdentityController,
	controllers.NewMCPProxyScopeController,
	controllers.NewAgentIdentityController,
	controllers.NewAlertingController,
	// Registered on the internal server rather than by registerAPIRoutes, but
	// still needed here: its routes go through a registrar too.
	controllers.NewGatewayInternalController,
}

// stubAppParams builds an AppParams whose controller fields are non-nil.
func stubAppParams(t *testing.T) *wiring.AppParams {
	t.Helper()

	params := &wiring.AppParams{}
	target := reflect.ValueOf(params).Elem()

	for _, ctor := range controllerConstructors {
		fn := reflect.ValueOf(ctor)
		args := make([]reflect.Value, fn.Type().NumIn())
		for i := range args {
			args[i] = reflect.Zero(fn.Type().In(i))
		}

		out := fn.Call(args)[0]
		if !assignFieldOfType(target, out) {
			t.Fatalf("no AppParams field accepts %s returned by %s",
				out.Type(), fn.Type())
		}
	}
	return params
}

// assignFieldOfType assigns value to the first unset struct field of its exact
// type, reporting whether it found one.
func assignFieldOfType(target reflect.Value, value reflect.Value) bool {
	for i := range target.NumField() {
		field := target.Field(i)
		if field.Type() == value.Type() && field.IsZero() && field.CanSet() {
			field.Set(value)
			return true
		}
	}
	return false
}

// registerAllRoutesForAudit drives the real route registration and returns the
// registrar so its route ledger can be inspected.
//
// It calls the same registerAPIRoutes that MakeHTTPHandler does, which is what
// makes the assertions below total rather than a sample.
func registerAllRoutesForAudit(t *testing.T) *middleware.RouteRegistrar {
	t.Helper()

	rr := middleware.NewRouteRegistrar(http.NewServeMux(), nil, audit.NewNoopRecorder())
	registerAPIRoutes(rr, stubAppParams(t))
	return rr
}

// registerInternalRoutesForAudit drives the gateway-facing internal server's
// registration.
//
// That surface has no JWT and was previously registered on a bare mux, which
// put it outside the ledger every assertion below reads — so its one mutating
// route was audited only by hand, and nothing would have caught the next one.
func registerInternalRoutesForAudit(t *testing.T) *middleware.RouteRegistrar {
	t.Helper()

	rr := middleware.NewInternalRouteRegistrar(http.NewServeMux(), audit.NewNoopRecorder())
	RegisterGatewayInternalRoutes(rr, stubAppParams(t).GatewayInternalController)
	return rr
}

// TestInternalMutatingRoutesAreAudited extends the coverage guarantee to the
// internal server. Its routes carry no permission, so each audited one needs an
// explicit actionOverrides entry; without one, registration panics.
func TestInternalMutatingRoutesAreAudited(t *testing.T) {
	routes := registerInternalRoutesForAudit(t)
	if len(routes.Routes()) == 0 {
		t.Fatal("no internal routes registered; the coverage check would be vacuous")
	}

	for _, meta := range routes.Routes() {
		if meta.Surface != audit.SurfaceInternal {
			t.Errorf("route %q records surface %q, want internal", meta.Pattern, meta.Surface)
		}
		if meta.Method == http.MethodGet {
			continue
		}
		if !meta.Audited {
			t.Errorf("mutating internal route %q is not audited", meta.Pattern)
		}
		if meta.Action == "" {
			t.Errorf("audited internal route %q has no action label", meta.Pattern)
		}
	}
}

// TestInternalKeySyncReadsAreAuditedAndCoalesced covers the bulk-sync
// endpoints, which hand real key material to a gateway. They are polled on a
// timer, so they must be recorded but must not be recorded per request.
func TestInternalKeySyncReadsAreAuditedAndCoalesced(t *testing.T) {
	want := map[string]bool{
		"/llm-providers/api-keys": false,
		"/llm-proxies/api-keys":   false,
		"/apis/api-keys":          false,
	}

	for _, meta := range registerInternalRoutesForAudit(t).Routes() {
		if _, ok := want[meta.Path]; !ok {
			continue
		}
		want[meta.Path] = true

		if !meta.Audited {
			t.Errorf("%q discloses key material but is not audited", meta.Pattern)
		}
		if meta.Coalesce == 0 {
			t.Errorf("%q is polled continuously but is not coalesced; "+
				"one record per request would bury the trail", meta.Pattern)
		}
	}

	for path, seen := range want {
		if !seen {
			t.Errorf("bulk-sync route %q was never registered", path)
		}
	}
}

// TestEveryMutatingRouteIsAudited is the anti-drift guard for the audit trail.
//
// A new endpoint that changes state must not be able to ship without producing
// an audit record. Reviewers cannot be relied on to notice; this can. If it
// fails, either the route belongs in the trail (the usual case, and nothing to
// do — it is audited automatically) or it genuinely changes no state and belongs
// in nonMutatingWritePaths with a reason.
func TestEveryMutatingRouteIsAudited(t *testing.T) {
	exempt := make(map[string]bool)
	for _, p := range audit.ExemptWritePaths() {
		exempt[p] = true
	}

	seenExempt := make(map[string]bool)
	for _, meta := range registerAllRoutesForAudit(t).Routes() {
		if meta.Method == http.MethodGet {
			continue
		}
		if exempt[meta.Path] {
			seenExempt[meta.Path] = true
			if meta.Audited {
				t.Errorf("route %q is listed as exempt but is still audited", meta.Pattern)
			}
			continue
		}
		if !meta.Audited {
			t.Errorf("mutating route %q is not audited", meta.Pattern)
			continue
		}
		if meta.Action == "" {
			t.Errorf("audited route %q has no action label", meta.Pattern)
		}
	}

	// An exemption for a route that no longer exists silently widens over time
	// as paths are renamed, so require each one to still match something.
	for path := range exempt {
		if !seenExempt[path] {
			t.Errorf("exempt write path %q matches no registered route", path)
		}
	}
}

// TestAuditedRoutesHaveWellFormedActions checks that every action reads as
// "<resource>:<verb>", the shape queries and alerting rules are written against.
func TestAuditedRoutesHaveWellFormedActions(t *testing.T) {
	for _, meta := range registerAllRoutesForAudit(t).Routes() {
		if !meta.Audited {
			continue
		}
		action := string(meta.Action)
		if strings.Count(action, ":") != 1 {
			t.Errorf("route %q has malformed action %q: want exactly one colon", meta.Pattern, action)
		}
		if action != strings.ToLower(action) {
			t.Errorf("route %q has non-lowercase action %q", meta.Pattern, action)
		}
		if meta.Action.Resource() == "" || meta.Action.Verb() == "" {
			t.Errorf("route %q has action %q with an empty resource or verb", meta.Pattern, action)
		}
	}
}

// TestNoStaleAuditPolicyEntries catches policy entries that no longer match any
// route.
//
// A stale override is worse than a missing one: it looks like the route is
// deliberately labelled while the route it named has been renamed or removed,
// so the real route silently falls back to a derived label nobody reviewed.
func TestNoStaleAuditPolicyEntries(t *testing.T) {
	// Both surfaces contribute policy entries, so staleness has to be judged
	// against both ledgers — otherwise adding an internal override would look
	// like a stale public one.
	routes := append(registerAllRoutesForAudit(t).Routes(),
		registerInternalRoutesForAudit(t).Routes()...)

	patterns := make(map[string]bool, len(routes))
	paths := make(map[string]bool, len(routes))
	for _, meta := range routes {
		patterns[meta.Pattern] = true
		paths[meta.Path] = true
	}

	for _, key := range audit.OverrideKeys() {
		if !patterns[key] {
			t.Errorf("actionOverrides has entry %q which matches no registered route", key)
		}
	}
	for _, path := range audit.SensitiveReadPaths() {
		if !paths[path] {
			t.Errorf("sensitiveReadPaths has entry %q which matches no registered route", path)
		}
	}
}

// TestSensitiveReadsAreAudited asserts the credential-disclosing reads actually
// made it into the trail. Reads are excluded by default, so an entry that is
// present but ineffective would otherwise go unnoticed.
func TestSensitiveReadsAreAudited(t *testing.T) {
	sensitive := make(map[string]bool)
	for _, p := range audit.SensitiveReadPaths() {
		sensitive[p] = true
	}

	seen := make(map[string]bool)
	allRoutes := append(registerAllRoutesForAudit(t).Routes(),
		registerInternalRoutesForAudit(t).Routes()...)
	for _, meta := range allRoutes {
		if meta.Method != http.MethodGet || !sensitive[meta.Path] {
			continue
		}
		seen[meta.Path] = true
		if !meta.Audited {
			t.Errorf("sensitive read %q is not audited", meta.Pattern)
		}
	}

	for path := range sensitive {
		if !seen[path] {
			t.Errorf("sensitive read %q was never registered as a GET route", path)
		}
	}
}

// TestOrdinaryReadsAreNotAudited guards the other direction: auditing every GET
// would multiply volume for little forensic gain, so an accidental broadening of
// the read policy should fail loudly.
func TestOrdinaryReadsAreNotAudited(t *testing.T) {
	sensitive := make(map[string]bool)
	for _, p := range audit.SensitiveReadPaths() {
		sensitive[p] = true
	}

	for _, meta := range registerAllRoutesForAudit(t).Routes() {
		if meta.Method != http.MethodGet || sensitive[meta.Path] {
			continue
		}
		if meta.Audited {
			t.Errorf("ordinary read %q is audited; add it to sensitiveReadPaths or stop auditing it", meta.Pattern)
		}
	}
}

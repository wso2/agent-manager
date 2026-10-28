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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/services"
)

// platformHealthMux builds the failure-summary route exactly as MakeHTTPHandler
// does — through a real RouteRegistrar — so the whole wrapper chain is under
// test and not just the handler.
//
// This is the level the marker has to survive: RequirePlatformAdminOU stamps the
// context, three more wrappers sit between it and the handler, and the handler
// refuses if the stamp is missing. Testing the middleware and the handler apart
// would not catch a chain that drops the request context between them.
func platformHealthMux(t *testing.T, total, failed, abandoned int64) *http.ServeMux {
	t.Helper()

	repo := &repomocks.GatewayRepositoryMock{
		CountFailureSummaryAllOrgsFunc: func(
			_ context.Context, _ repositories.GatewayFailureWindow,
		) (repositories.GatewayFailureCounts, error) {
			return repositories.GatewayFailureCounts{
				Total: total, Failed: failed, Abandoned: abandoned,
			}, nil
		},
		ListFailedGatewaysAllOrgsFunc: func(
			_ context.Context, _ repositories.GatewayFailureWindow, _ int,
		) ([]*models.Gateway, error) {
			return []*models.Gateway{
				{UUID: uuid.New(), Name: "gw-down", OUID: "ou-abc", UpdatedAt: time.Now().Add(-time.Hour)},
			}, nil
		},
	}
	ctrl := controllers.NewGatewayController(
		services.NewPlatformGatewayService(repo, nil),
		&clientmocks.OpenChoreoClientMock{},
		nil,
	)

	mux := http.NewServeMux()
	RegisterPlatformGatewayHealthRoutes(
		middleware.NewRouteRegistrar(mux, nil, audit.NewNoopRecorder()), ctrl,
	)
	return mux
}

// callPlatformHealth drives the mux with a token carrying tokenOUID.
func callPlatformHealth(
	t *testing.T, mux *http.ServeMux, tokenOUID string, includeDetails bool,
) *httptest.ResponseRecorder {
	t.Helper()
	target := "/platform/gateways/failure-summary"
	if includeDetails {
		target += "?includeDetails=true"
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	// The route is registered with rbac.GatewayRead. Scope checks are
	// unconditional now that the RBAC kill-switch is gone, so the token has to
	// carry that scope for these tests to reach the platform-admin gate they
	// are actually about — an unscoped token would 403 on the scope check first
	// and every assertion below would pass for the wrong reason.
	req = req.WithContext(jwtassertion.ContextWithTokenClaimsAndScope(req.Context(),
		&jwtassertion.TokenClaims{
			Sub: "user-a", OuId: tokenOUID, OuHandle: "handle-a",
			Scope: rbac.GatewayRead.Scope(),
		}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// withPlatformAdminOUID sets the process-global platform-admin OU for one test.
// Tests using it must not run in parallel.
func withPlatformAdminOUID(t *testing.T, ouID string) {
	t.Helper()
	cfg := config.GetConfig()
	orig := cfg.PlatformAdminOUID
	cfg.PlatformAdminOUID = ouID
	t.Cleanup(func() { cfg.PlatformAdminOUID = orig })
}

// withFailurePercentageThreshold sets the process-global fleet health threshold
// for one test. Tests using it must not run in parallel.
func withFailurePercentageThreshold(t *testing.T, pct float64) {
	t.Helper()
	cfg := config.GetConfig()
	orig := cfg.GatewayFailurePercentageThreshold
	cfg.GatewayFailurePercentageThreshold = pct
	t.Cleanup(func() { cfg.GatewayFailurePercentageThreshold = orig })
}

// decodeSummary reads the response body, which must be the summary document
// whatever the status was.
func decodeSummary(t *testing.T, rec *httptest.ResponseRecorder) services.GatewayFailureSummaryResponse {
	t.Helper()
	var body services.GatewayFailureSummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response (status %d, body %q): %v", rec.Code, rec.Body.String(), err)
	}
	return body
}

// TestPlatformGatewayHealthRoute_AdmitsPlatformAdmin is the happy path through
// the real chain. A 403 here with the middleware's own tests passing would mean
// the context marker did not reach the handler.
func TestPlatformGatewayHealthRoute_AdmitsPlatformAdmin(t *testing.T) {
	withPlatformAdminOUID(t, "ou-platform-admin")

	withFailurePercentageThreshold(t, 10)

	rec := callPlatformHealth(t, platformHealthMux(t, 412, 7, 0), "ou-platform-admin", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("platform admin: want 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	body := decodeSummary(t, rec)
	if body.Total != 412 || body.Failed != 7 {
		t.Errorf("counts = %d/%d, want 412/7", body.Total, body.Failed)
	}
	if body.FailedGateways != nil {
		t.Error("details must be absent when includeDetails was not requested")
	}
}

// TestPlatformGatewayHealthRoute_RefusesTenantToken is the isolation guarantee
// at the route level: an ordinary tenant's token must not read the whole fleet.
//
// The RBAC kill-switch this test used to toggle is gone (main removed
// config.RBACEnabled), so scope checks are now unconditional. The assertion is
// unchanged and still the point: the platform-admin gate must refuse a tenant
// token on its own, independently of whatever scopes that token carries.
func TestPlatformGatewayHealthRoute_RefusesTenantToken(t *testing.T) {
	withPlatformAdminOUID(t, "ou-platform-admin")

	rec := callPlatformHealth(t, platformHealthMux(t, 412, 7, 0), "ou-some-tenant", false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("tenant token with RBAC off: want 403, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() > 0 && rec.Code == http.StatusOK {
		t.Error("a refused caller must not receive fleet data")
	}
}

// TestPlatformGatewayHealthRoute_RefusesWhenUnconfigured pins the fail-closed
// default: a deployment that never set PLATFORM_ADMIN_OU_ID exposes nothing,
// including to a caller whose token happens to carry no OU at all.
func TestPlatformGatewayHealthRoute_RefusesWhenUnconfigured(t *testing.T) {
	withPlatformAdminOUID(t, "")

	for _, tokenOUID := range []string{"ou-platform-admin", "ou-some-tenant", ""} {
		if rec := callPlatformHealth(t, platformHealthMux(t, 412, 7, 0), tokenOUID, false); rec.Code != http.StatusForbidden {
			t.Errorf("unconfigured (token %q): want 403, got %d", tokenOUID, rec.Code)
		}
	}
}

// TestPlatformAdminRegistrarRejectsOrgScopedPattern pins the guard that keeps
// this registrar honest.
//
// An org-scoped pattern would make the registrar apply RequireOrgMatch and read
// as tenant-scoped while still serving every tenant's rows — a contradiction
// worth catching at startup rather than in review. Registration happens once, at
// boot, so a panic there is the cheapest possible place to find it.
func TestPlatformAdminRegistrarRejectsOrgScopedPattern(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("registering an org-scoped pattern as platform-admin must panic")
		}
	}()

	rr := middleware.NewRouteRegistrar(http.NewServeMux(), nil, audit.NewNoopRecorder())
	rr.HandleFuncWithValidationAndPlatformAdminAuthz(
		"GET /orgs/{orgName}/gateways/failure-summary", "gateway:read",
		func(http.ResponseWriter, *http.Request) {},
	)
}

// TestPlatformGatewayHealthRoute_UnhealthyFleetAnswers503 is the status-carries-
// the-verdict behaviour: over the threshold, the summary form answers 503 so the
// route can be a monitor's target without the monitor parsing the body.
//
// The body must still be the summary document, not an error envelope — the
// numbers are the whole reason to call it, and they matter most on the side of
// the threshold where the status is an error.
func TestPlatformGatewayHealthRoute_UnhealthyFleetAnswers503(t *testing.T) {
	withPlatformAdminOUID(t, "ou-platform-admin")
	withFailurePercentageThreshold(t, 10)

	// 40 of 100 failing: four times the threshold.
	rec := callPlatformHealth(t, platformHealthMux(t, 100, 40, 0), "ou-platform-admin", false)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unhealthy fleet: want 503, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	body := decodeSummary(t, rec)
	if body.Healthy {
		t.Error("a 503 response must not report healthy: true")
	}
	if body.FailurePercentage != 40 {
		t.Errorf("failurePercentage = %v, want 40", body.FailurePercentage)
	}
	if body.Total != 100 || body.Failed != 40 {
		t.Errorf("counts = %d/%d, want 100/40; the payload must survive the error status",
			body.Total, body.Failed)
	}
}

// TestPlatformGatewayHealthRoute_StatusFollowsTheThreshold pins that the status
// is decided by the configured threshold and not by any fixed notion of "too
// many". Same fleet, two thresholds, two answers.
func TestPlatformGatewayHealthRoute_StatusFollowsTheThreshold(t *testing.T) {
	withPlatformAdminOUID(t, "ou-platform-admin")

	for _, tc := range []struct {
		name       string
		threshold  float64
		wantStatus int
	}{
		// 5 of 100 failing = 5%.
		{"threshold above the failure rate", 10, http.StatusOK},
		{"threshold at the failure rate", 5, http.StatusServiceUnavailable},
		{"threshold below the failure rate", 1, http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withFailurePercentageThreshold(t, tc.threshold)
			rec := callPlatformHealth(t, platformHealthMux(t, 100, 5, 0), "ou-platform-admin", false)
			if rec.Code != tc.wantStatus {
				t.Errorf("threshold %v: status = %d, want %d", tc.threshold, rec.Code, tc.wantStatus)
			}
			if body := decodeSummary(t, rec); body.Healthy != (tc.wantStatus == http.StatusOK) {
				t.Errorf("threshold %v: healthy = %v, disagrees with status %d",
					tc.threshold, body.Healthy, rec.Code)
			}
		})
	}
}

// TestPlatformGatewayHealthRoute_DetailedRequestAlwaysAnswers200 is the
// exception: a detailed request answers 200 however bad the fleet is.
//
// The verdict has to survive that, or a caller asking for detail would be told
// everything is fine. Both halves are asserted here, because the 200 alone is
// only correct if "healthy": false is still in the body — which is also why the
// service sets Healthy independently of the status the controller picks.
func TestPlatformGatewayHealthRoute_DetailedRequestAlwaysAnswers200(t *testing.T) {
	withPlatformAdminOUID(t, "ou-platform-admin")
	withFailurePercentageThreshold(t, 10)

	// The whole fleet is down — the worst case the summary form would 503 on.
	rec := callPlatformHealth(t, platformHealthMux(t, 100, 100, 0), "ou-platform-admin", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("detailed request: want 200 regardless of fleet health, got %d", rec.Code)
	}

	body := decodeSummary(t, rec)
	if body.Healthy {
		t.Error("a fully failed fleet must still report healthy: false in the detailed form")
	}
	if body.FailurePercentage != 100 {
		t.Errorf("failurePercentage = %v, want 100", body.FailurePercentage)
	}
	if len(body.FailedGateways) == 0 {
		t.Error("a detailed request must return the failed gateways")
	}
}

// TestPlatformGatewayHealthRoute_DetailedRequestIsStillAuthorized guards against
// the always-200 rule being read as "the detailed form is the lenient one". The
// platform-admin gate is applied at registration and so covers both forms; this
// pins it, because a tenant discovering that includeDetails=true returns 200
// would be the obvious thing to try.
func TestPlatformGatewayHealthRoute_DetailedRequestIsStillAuthorized(t *testing.T) {
	withPlatformAdminOUID(t, "ou-platform-admin")
	withFailurePercentageThreshold(t, 10)

	rec := callPlatformHealth(t, platformHealthMux(t, 100, 100, 0), "ou-some-tenant", true)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("tenant token on the detailed form: want 403, got %d", rec.Code)
	}
}

// TestPlatformGatewayHealthRoute_AbandonedGatewaysDoNotDiluteTheVerdict is the
// end-to-end version of the denominator rule, and it is a status-code test
// rather than a payload one because that is the part an operator feels.
//
// 5 of 5 live gateways are down. Counting the 95 abandoned rows would put the
// fleet at 5% — under the threshold — and answer 200 while every gateway anyone
// still uses is unreachable. Excluding them reports 100% and answers 503.
func TestPlatformGatewayHealthRoute_AbandonedGatewaysDoNotDiluteTheVerdict(t *testing.T) {
	withPlatformAdminOUID(t, "ou-platform-admin")
	withFailurePercentageThreshold(t, 10)

	rec := callPlatformHealth(t, platformHealthMux(t, 100, 5, 95), "ou-platform-admin", false)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("every live gateway down: want 503, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	body := decodeSummary(t, rec)
	if body.FailurePercentage != 100 {
		t.Errorf("failurePercentage = %v, want 100 (5 failed of 5 considered)", body.FailurePercentage)
	}
	if body.Considered != 5 {
		t.Errorf("considered = %d, want 5 (100 total less 95 abandoned)", body.Considered)
	}
	if body.Abandoned != 95 || body.Total != 100 {
		t.Errorf("total/abandoned = %d/%d, want 100/95; both must be reported so the "+
			"denominator is checkable", body.Total, body.Abandoned)
	}
}

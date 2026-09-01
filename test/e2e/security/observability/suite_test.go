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

// Package observability holds the security suite for agent-manager-observer.
//
// The observer is a separate service with its own rbac package, its own
// RBAC_ENABLED flag, and its own JWT middleware — none of which the
// agent-manager-service suites cover. Traces, logs, and metrics are the most
// sensitive read surface on the platform (prompts, completions, tool calls),
// so its scope enforcement gets its own suite rather than riding along.
package observability

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// Cfg is the shared test configuration.
var Cfg *framework.Config

var observerHTTPClient = &http.Client{Timeout: 30 * time.Second}

// The four observability scopes, as the observer's rbac package builds them
// (rbac.Permission.Scope() → "amp:observability:<action>").
const (
	scopeTraceRead    = "amp:observability:trace-read"
	scopeLogRead      = "amp:observability:log-read"
	scopeBuildLogRead = "amp:observability:build-log-read"
	scopeMetricRead   = "amp:observability:metric-read"
)

// observabilityScopes is every scope the observer's data routes distinguish.
// Each route is driven with its own scope (must pass) and with every other one
// (must 403) — that off-diagonal is what proves the scopes are not
// interchangeable, which a single positive test per route would miss.
var observabilityScopes = []string{scopeTraceRead, scopeLogRead, scopeBuildLogRead, scopeMetricRead}

func TestSecurityObservability(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Security: Observability Suite")
}

var _ = BeforeSuite(func(ctx SpecContext) {
	Cfg = framework.LoadConfig()

	By("Checking the observer is reachable")
	verifyObserverReachable(ctx)

	By("Verifying RBAC enforcement is enabled on the observer")
	verifyObserverRBACEnabled(ctx)
})

// verifyObserverReachable fails fast with an actionable message rather than
// letting every spec fail on a connection error. The observer is a separate
// deployment from agent-manager-service, so it can be absent or unexposed on a
// cluster where the rest of the platform is fine.
func verifyObserverReachable(ctx context.Context) {
	resp, err := getObsUnauthenticated(ctx, "/api/v1/traces", nil)
	Expect(err).NotTo(HaveOccurred(),
		"ABORTING: cannot reach the observer at %s. Set AM_OBSERVER_BASE_URL, or check the "+
			"observability extension is installed and port-forwarded.", Cfg.ObserverBaseURL)
	defer resp.Body.Close()

	Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
		"ABORTING: the observer returned %d for an unauthenticated request to /api/v1/traces, "+
			"expected 401. Either this is not the observer, or its JWT middleware is not applied — "+
			"in which case traces are readable by anyone who can reach the service.",
		resp.StatusCode)
}

// verifyObserverRBACEnabled proves the observer enforces scopes. It has its OWN
// RBAC_ENABLED (amObserver.auth.rbacEnabled in the observability extension
// chart), independent of agent-manager-service's — so the authz suite passing
// says nothing about this service. With it off, AuthorizePermission returns nil
// for any non-publisher token and every trace in the cluster is readable by any
// authenticated caller.
func verifyObserverRBACEnabled(ctx context.Context) {
	unscoped, err := framework.FetchTokenWithScopes(ctx, Cfg, nil)
	Expect(err).NotTo(HaveOccurred(), "failed to fetch an unscoped token")

	resp := getObs(ctx, unscoped, "/api/v1/traces", nil)
	defer resp.Body.Close()

	Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
		"ABORTING: an unscoped token reached the observer's /api/v1/traces and got %d instead "+
			"of 403. RBAC enforcement appears disabled on the observer "+
			"(amObserver.auth.rbacEnabled=false). Every spec in this suite would be vacuous.",
		resp.StatusCode)
}

// obsURL builds an absolute observer URL with query parameters.
func obsURL(path string, query map[string]string) string {
	url := strings.TrimSuffix(Cfg.ObserverBaseURL, "/") + path
	if len(query) == 0 {
		return url
	}
	parts := make([]string, 0, len(query))
	for k, v := range query {
		parts = append(parts, k+"="+v)
	}
	return url + "?" + strings.Join(parts, "&")
}

// getObs issues an authenticated GET against the observer.
func getObs(ctx context.Context, token, path string, query map[string]string) *http.Response {
	resp, err := framework.NewAMPClientWithToken(Cfg, token).
		DoRawWithContext(ctx, http.MethodGet, obsURL(path, query))
	Expect(err).NotTo(HaveOccurred(), "observer request failed: %s", path)
	return resp
}

func getObsUnauthenticated(ctx context.Context, path string, query map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, obsURL(path, query), nil)
	if err != nil {
		return nil, err
	}
	return observerHTTPClient.Do(req)
}

// tokenCache avoids re-minting the same single-scope token for every route.
var (
	tokenCache sync.Map
)

func tokenWithScope(ctx context.Context, scope string) string {
	if cached, ok := tokenCache.Load(scope); ok {
		return cached.(string)
	}

	token, err := framework.FetchTokenWithScopes(ctx, Cfg, []string{scope})
	Expect(err).NotTo(HaveOccurred(), "failed to mint a token for %s", scope)

	// Same vacuity guard as the authz suite: confirm the IDP issued only what
	// was asked for, so an off-diagonal 403 assertion cannot pass by accident.
	issued, err := framework.TokenScopes(token)
	Expect(err).NotTo(HaveOccurred(), "failed to decode the issued token")
	Expect(issued).To(HaveKey(scope), "the IDP did not issue the requested scope %s", scope)
	for _, other := range observabilityScopes {
		if other != scope {
			Expect(issued).NotTo(HaveKey(other),
				"the IDP issued %s alongside %s — this spec would be vacuous", other, scope)
		}
	}

	actual, _ := tokenCache.LoadOrStore(scope, token)
	return actual.(string)
}

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

package authz

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// SEC-AUTHZ-001 — every guarded route rejects a token missing its scope.
//
// For each route the suite mints deny and allow tokens from the same IDP client:
//
//	deny  = every amp: scope except one required scope -> must be 403
//	allow = the minimum complete required scope set    -> must NOT be 401/403
//
// The deny case catches an unguarded route. The allow case is the control that
// keeps the deny case honest: without it, a route that 403s for an unrelated
// reason (wrong permission constant, an extra guard) would look correctly
// protected. Both must hold for the route to be considered covered.
//
// Every request in this matrix is side-effect free by construction:
//   - GET/DELETE target deliberately absent resources (absentName) and 404.
//   - POST/PUT send NO request body, so the handler fails at JSON decode with
//     400 before touching any state.
//
// That matters because the deny case is expected to be stopped by middleware,
// but the allow case reaches the handler on purpose — and must not create,
// mutate, or delete anything when it gets there.

// absentName is used for every path parameter naming a resource. The
// "e2e-test-" prefix means the root cleanup suite would sweep it if a
// regression ever caused one of these requests to create something.
const absentName = "e2e-test-sec-absent"

// guardedRoute is one cell of the route × permission matrix.
type guardedRoute struct {
	// Method and Path form the request. Path is relative to the API base URL.
	Method string
	Path   string
	// Scopes are the complete set required by this route. A multi-scope entry
	// models an all-of authorization registrar.
	Scopes []string
}

func (r guardedRoute) String() string {
	separator := " or "
	if len(r.Scopes) > 1 {
		separator = " and "
	}
	return fmt.Sprintf("%s %s [%s]", r.Method, r.Path, strings.Join(r.Scopes, separator))
}

type scopeCase struct {
	label  string
	scopes []string
}

// denyCases returns the least-privileged token sets that must be rejected. For
// an AND route, each case removes exactly one required scope while retaining
// every other AMP scope, proving that every axis is independently enforced.
func (r guardedRoute) denyCases() []scopeCase {
	if len(r.Scopes) > 1 {
		cases := make([]scopeCase, 0, len(r.Scopes))
		for _, missing := range r.Scopes {
			cases = append(cases, scopeCase{label: missing, scopes: framework.ScopesExcept(missing)})
		}
		return cases
	}

	return []scopeCase{{
		label:  strings.Join(r.Scopes, "/"),
		scopes: framework.ScopesExcept(r.Scopes...),
	}}
}

// allowCases returns the minimum token set that must pass authorization. A
// multi-scope route needs all of its required scopes together.
func (r guardedRoute) allowCases() []scopeCase {
	if len(r.Scopes) > 1 {
		return []scopeCase{{label: strings.Join(r.Scopes, " and "), scopes: r.Scopes}}
	}

	cases := make([]scopeCase, 0, len(r.Scopes))
	for _, scope := range r.Scopes {
		cases = append(cases, scopeCase{label: scope, scopes: []string{scope}})
	}
	return cases
}

// guardedRoutes is a representative slice of the API surface: one entry per
// permission family, favouring the destructive and privilege-granting routes.
//
// It is deliberately NOT exhaustive. Exhaustiveness is enforced statically by
// TestEveryRouteIsAuthorized in agent-manager-service/api, which fails when a
// route is registered without an authz-bearing registrar method. This matrix
// checks the complementary property that the wiring actually behaves at
// runtime, on the routes where being wrong is most expensive.
func guardedRoutes(org string) []guardedRoute {
	agentBase := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", org, absentName)
	orgBase := fmt.Sprintf("/api/v1/orgs/%s", org)
	identityBase := orgBase + "/identities"
	// The AI Gateway module's API.
	gwOrgBase := fmt.Sprintf("/m/ai-gateway/orgs/%s", org)
	gwAgentBase := gwOrgBase + "/projects/" + absentName + "/agents"

	return []guardedRoute{
		// --- Agent lifecycle -------------------------------------------------
		{http.MethodGet, agentBase, []string{"amp:agent:read"}},
		{http.MethodPost, agentBase, []string{"amp:agent:create"}},
		{http.MethodGet, agentBase + "/" + absentName, []string{"amp:agent:read"}},
		{http.MethodPut, agentBase + "/" + absentName, []string{"amp:agent:update"}},
		{http.MethodDelete, agentBase + "/" + absentName, []string{"amp:agent:delete"}},
		{
			http.MethodPost, gwAgentBase + "/" + absentName + "/environments/" + absentName + "/api-keys",
			[]string{"amp:ai-gateway:agent-api-keys.manage"},
		},

		// --- Projects --------------------------------------------------------
		{http.MethodPost, orgBase + "/projects", []string{"amp:project:create"}},
		{http.MethodPut, orgBase + "/projects/" + absentName, []string{"amp:project:update"}},
		{http.MethodDelete, orgBase + "/projects/" + absentName, []string{"amp:project:delete"}},

		// --- Identity — privilege granting -----------------------------------
		{http.MethodPost, identityBase + "/users/invite", []string{"amp:org:invite-member"}},
		{http.MethodDelete, identityBase + "/users/" + absentName, []string{"amp:org:remove-member"}},
		{http.MethodPost, identityBase + "/roles", []string{"amp:role:create"}},
		{http.MethodPut, identityBase + "/roles/" + absentName, []string{"amp:role:update"}},
		{http.MethodDelete, identityBase + "/roles/" + absentName, []string{"amp:role:delete"}},
		{http.MethodPost, identityBase + "/roles/" + absentName + "/permissions/add", []string{"amp:role:update"}},
		{http.MethodPost, identityBase + "/roles/" + absentName + "/assignees/add", []string{"amp:role:update"}},
		{http.MethodPost, identityBase + "/groups", []string{"amp:group:create"}},
		{http.MethodDelete, identityBase + "/groups/" + absentName, []string{"amp:group:delete"}},

		// --- Gateways --------------------------------------------------------
		{http.MethodPost, gwOrgBase + "/gateways", []string{"amp:ai-gateway:gateways.create"}},
		{http.MethodDelete, gwOrgBase + "/gateways/" + absentName, []string{"amp:ai-gateway:gateways.delete"}},
		{http.MethodPost, gwOrgBase + "/gateways/" + absentName + "/tokens", []string{"amp:ai-gateway:gateways.token-manage"}},

		{http.MethodDelete, gwOrgBase + "/self-hosted-gateways/" + absentName, []string{"amp:ai-gateway:gateways.delete"}},

		// --- Events: delivery to external endpoints and their secrets ---------
		{http.MethodDelete, orgBase + "/events/destinations/" + absentName, []string{"amp:events:manage"}},
		{http.MethodPost, orgBase + "/events/destinations/" + absentName + "/rotate-secret", []string{"amp:events:manage"}},
		{http.MethodDelete, orgBase + "/events/subscriptions/" + absentName, []string{"amp:events:manage"}},

		// --- LLM and MCP catalog: removal and key revocation ------------------
		{http.MethodDelete, gwOrgBase + "/llm-provider-templates/" + absentName, []string{"amp:ai-gateway:llm-provider-templates.delete"}},
		{http.MethodDelete, gwOrgBase + "/llm-providers/" + absentName + "/api-keys/" + absentName, []string{"amp:ai-gateway:llm-providers.api-key-manage"}},
		{http.MethodDelete, gwOrgBase + "/projects/" + absentName + "/llm-proxies/" + absentName, []string{"amp:ai-gateway:llm-proxies.delete"}},
		{http.MethodDelete, gwOrgBase + "/mcp-proxies/" + absentName, []string{"amp:ai-gateway:mcp-servers.delete"}},

		// --- Agent identity roles ---------------------------------------------
		{http.MethodDelete, gwOrgBase + "/environments/" + absentName + "/agent-identities/roles/" + absentName, []string{"amp:ai-gateway:agent-identities.delete"}},

		// --- Catalog ----------------------------------------------------------
		{http.MethodGet, gwOrgBase + "/catalog", []string{"amp:ai-gateway:catalog.read"}},
	}
}

var _ = Describe("SEC-AUTHZ-001: scope matrix", Label("security"), func() {
	for _, route := range guardedRoutes(framework.LoadConfig().DefaultOrg) {
		route := route
		for _, denyCase := range route.denyCases() {
			denyCase := denyCase
			It(fmt.Sprintf("denies %s to a token without %s",
				route.Method+" "+route.Path, denyCase.label), func(ctx SpecContext) {
				By("calling the route with the required scope deliberately omitted")
				deny := clientWithScopes(ctx, denyCase.scopes)
				resp := send(ctx, deny, route)
				defer resp.Body.Close()
				framework.ExpectForbidden(Default, resp, route.String())
			})
		}

		for _, allowCase := range route.allowCases() {
			allowCase := allowCase
			It(fmt.Sprintf("allows %s to a token with %s",
				route.Method+" "+route.Path, allowCase.label), func(ctx SpecContext) {
				By("calling the same route with the minimum scopes that satisfy it")
				allow := clientWithScopes(ctx, allowCase.scopes)
				resp := send(ctx, allow, route)
				defer resp.Body.Close()
				framework.ExpectNotForbidden(Default, resp, route.String())
			})
		}
	}
})

// send issues the route's request. POST/PUT deliberately carry no body so the
// handler rejects them at JSON decode, before any state is touched.
func send(ctx SpecContext, client *framework.AMPClient, route guardedRoute) *http.Response {
	resp, err := client.DoWithContext(ctx, route.Method, route.Path, nil)
	Expect(err).NotTo(HaveOccurred(), "request failed: %s", route)
	return resp
}

// tokenCache avoids re-minting identical scope sets. Ginkgo runs a suite as N
// processes with separate memory, so this is a per-process cache; it still cuts
// the token round-trips for this matrix roughly in half.
var (
	tokenCache sync.Map
)

func clientWithScopes(ctx SpecContext, scopes []string) *framework.AMPClient {
	key := strings.Join(scopes, " ")

	if cached, ok := tokenCache.Load(key); ok {
		return cached.(*framework.AMPClient)
	}

	token, err := framework.FetchTokenWithScopes(ctx, Cfg, scopes)
	Expect(err).NotTo(HaveOccurred(), "failed to mint a token for %d scope(s)", len(scopes))

	// Guard against a silently-unreduced token. BeforeSuite proves the IDP
	// honours reduction in general; this proves it for this exact scope set,
	// so a spec can never pass because it was handed more privilege than asked.
	issued, err := framework.TokenScopes(token)
	Expect(err).NotTo(HaveOccurred(), "failed to decode the issued token")
	for _, s := range scopes {
		Expect(issued).To(HaveKey(s),
			"the IDP did not issue requested scope %q — the scope may have been removed or renamed", s)
	}
	for _, s := range framework.AllScopes() {
		if !slices.Contains(scopes, s) {
			Expect(issued).NotTo(HaveKey(s),
				"the IDP issued %q which was not requested — this spec would be vacuous", s)
		}
	}

	c := framework.NewAMPClientWithToken(Cfg, token)
	actual, _ := tokenCache.LoadOrStore(key, c)
	return actual.(*framework.AMPClient)
}

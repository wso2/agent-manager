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

// Package authz holds the authorization security suite: specs that assert a
// principal CANNOT do something. Every spec here must fail loudly when a guard
// is missing, so the suite refuses to run at all unless it has first proved
// that (a) RBAC enforcement is switched on and (b) it can actually mint an
// under-privileged token. Without both, every negative spec would pass
// vacuously — the single worst failure mode for a security suite.
package authz

import (
	"fmt"
	"net/http"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// Client is the full-privilege API client, used for fixtures and positive controls.
var Client *framework.AMPClient

// Cfg is the shared test configuration.
var Cfg *framework.Config

// rbacProbeScope guards the endpoint used to prove RBAC is enforced. Any
// single-permission, always-present, side-effect-free GET works; agent-kinds
// is used because it needs no fixtures.
const (
	rbacProbeScope = "amp:agent-kind:read"
	rbacProbePath  = "/api/v1/orgs/%s/agent-kinds"
)

func TestSecurityAuthz(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Security: Authorization Suite")
}

var _ = BeforeSuite(func(ctx SpecContext) {
	Cfg = framework.LoadConfig()
	verifyScopeMatrixUsesKnownScopes()

	By("Waiting for API readiness")
	framework.WaitForAPIReady(Cfg)

	By("Creating full-privilege API client")
	var err error
	Client, err = framework.NewAMPClientWithContext(ctx, Cfg)
	Expect(err).NotTo(HaveOccurred(), "failed to create API client")

	By("Verifying default organization")
	framework.VerifyDefaultOrg(Client, Cfg.DefaultOrg)

	By("Verifying the IDP honours scope reduction")
	verifyScopeReductionWorks(ctx)

	By("Verifying RBAC enforcement is enabled on the target deployment")
	verifyRBACEnabled(ctx)
})

// verifyScopeMatrixUsesKnownScopes catches stale permission names before the
// suite performs any network I/O. Thunder drops unknown requested scopes, which
// would otherwise make both the deny and allow controls fail less clearly.
func verifyScopeMatrixUsesKnownScopes() {
	knownScopes := framework.AllScopes()
	for _, route := range guardedRoutes(Cfg.DefaultOrg) {
		Expect(route.Scopes).NotTo(BeEmpty(), "%s has no required scopes", route)
		for _, scope := range route.Scopes {
			Expect(knownScopes).To(ContainElement(scope),
				"%s references unknown scope %q; update the security matrix after an RBAC catalog change",
				route, scope)
		}
	}
}

// verifyScopeReductionWorks proves the harness can mint a token that genuinely
// lacks a scope. Thunder returns requested ∩ allowed, but that is a property of
// the IDP and its client registration, not something this suite controls — if
// it ever stops holding, every negative spec would silently pass with a
// full-privilege token.
func verifyScopeReductionWorks(ctx SpecContext) {
	reduced, err := framework.FetchTokenWithScopes(ctx, Cfg, framework.ScopesExcept(rbacProbeScope))
	Expect(err).NotTo(HaveOccurred(), "failed to fetch a scope-reduced token")

	scopes, err := framework.TokenScopes(reduced)
	Expect(err).NotTo(HaveOccurred(), "failed to decode the scope-reduced token")

	Expect(scopes).NotTo(HaveKey(rbacProbeScope),
		"ABORTING: the IDP issued %q even though it was excluded from the scope request. "+
			"Scope reduction is the entire basis of this suite — every negative spec would "+
			"pass vacuously against a full-privilege token. Check the amp-api-client scope "+
			"policy on Thunder before trusting any result from this suite.", rbacProbeScope)

	Expect(scopes).To(HaveKey("amp:agent:read"),
		"the scope-reduced token is missing scopes it should have kept — the reduction "+
			"removed more than requested, which would make positive controls unreliable")
}

// verifyRBACEnabled proves the deployment actually enforces scopes. RBAC_ENABLED
// defaults to false in the Go config loader (true in the Helm chart and in
// docker-compose), and when it is off RequirePermission short-circuits and every
// route is reachable by any authenticated caller. Running the negative specs
// against such a deployment would report a clean bill of health for a platform
// with authorization switched off entirely.
func verifyRBACEnabled(ctx SpecContext) {
	unscoped, err := framework.FetchTokenWithScopes(ctx, Cfg, nil)
	Expect(err).NotTo(HaveOccurred(), "failed to fetch an unscoped token")

	resp, err := framework.NewAMPClientWithToken(Cfg, unscoped).
		GetWithContext(ctx, fmt.Sprintf(rbacProbePath, Cfg.DefaultOrg))
	Expect(err).NotTo(HaveOccurred(), "RBAC probe request failed")
	defer resp.Body.Close()

	Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
		"ABORTING: an unscoped token reached a %s-guarded endpoint and got %d instead of 403. "+
			"RBAC enforcement appears to be disabled on this deployment (RBAC_ENABLED=false). "+
			"Set RBAC_ENABLED=true and re-run — results from this suite are meaningless otherwise.",
		rbacProbeScope, resp.StatusCode)
}

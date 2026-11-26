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

// Validates that an MCP proxy offers subscription plans: it refuses a plan that doesn't exist, offers an
// existing one, which counts the offer without locking the plan, and refuses to offer a deprecated plan, while
// a proxy that already offers it keeps it. The proxies are deployed to the default environment, so the stack
// needs its AI gateway.

package aihub

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	aihubop "github.com/wso2/agent-manager/test/e2e/operations/aihub"
	"github.com/wso2/agent-manager/test/e2e/operations/gateway"
	mcpproxyop "github.com/wso2/agent-manager/test/e2e/operations/mcpproxy"
)

var _ = Describe("Subscription Plans offered by an MCP proxy", Label("ai-hub", "subscription-plans", "mcp-proxy"), Ordered, func() {
	var planID, proxyID, otherProxyID, envUUID, gatewayID string

	proxySpec := func(id string, plans ...string) framework.MCPProxySpec {
		ctxPath := "/" + id
		upstream := framework.TestMCPServerURL
		return framework.MCPProxySpec{
			DisplayName: "E2E Plans Proxy " + id[len(id)-8:], Version: "v1.0", Context: &ctxPath,
			Endpoints: []framework.MCPProxyEndpoint{{
				ID:       "primary",
				Upstream: framework.UpstreamConfig{Main: &framework.UpstreamEndpoint{URL: &upstream}},
				// Named, since an environment may have more than one egress gateway.
				Environments: []framework.MCPEndpointEnvironment{{EnvironmentUUID: envUUID, GatewayID: gatewayID}},
			}},
			SubscriptionPlans: plans,
		}
	}
	proxyDocument := func(id string, plans ...string) framework.MCPProxyDocument {
		return framework.MCPProxyDocument{
			APIVersion: framework.MCPProxyAPIVersion, Kind: framework.MCPProxyKind,
			Metadata: framework.DocumentMetadata{Name: id}, Spec: proxySpec(id, plans...),
		}
	}

	BeforeAll(func() {
		skipWithoutAIHub()
		gatewayID, envUUID = gateway.WaitForActiveGatewayForEnvWithEnvUUID(Client, Cfg.DefaultOrg, Cfg.DefaultEnv, 3*time.Minute)
		Expect(envUUID).NotTo(BeEmpty(), "expected an environment UUID for %s", Cfg.DefaultEnv)
		suffix := uuid.New().String()[:8]
		planID = "e2e-test-offered-plan-" + suffix
		proxyID = "e2e-test-plans-proxy-" + suffix
		otherProxyID = "e2e-test-plans-proxy-b-" + suffix
		aihubop.CreatePlan(Default, Client, framework.SubscriptionPlanRequest{ID: planID, DisplayName: "E2E Offered " + suffix})
	})

	AfterAll(func() {
		for _, id := range []string{proxyID, otherProxyID} {
			if id != "" {
				mcpproxyop.DeleteMCPProxyBestEffort(context.Background(), Client, id)
			}
		}
		if planID != "" {
			aihubop.DeprecatePlanBestEffort(Client, planID)
		}
	})

	It("should refuse to offer a plan that doesn't exist", func() {
		resp, err := Client.Post("/m/ai-gateway/mcp-proxies",
			proxyDocument(proxyID, "e2e-test-no-such-plan-"+uuid.New().String()[:8]))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
	})

	It("should offer the plan", func() {
		proxy := mcpproxyop.CreateMCPProxy(Default, Client, proxyID, proxySpec(proxyID, planID))
		Expect(proxy.Spec.SubscriptionPlans).To(ConsistOf(planID))
		Expect(mcpproxyop.GetMCPProxy(Default, Client, proxyID).Spec.SubscriptionPlans).To(ConsistOf(planID))
	})

	It("should count the offer without locking the plan", func() {
		plan := aihubop.GetPlan(Default, Client, planID)
		Expect(plan.Usage).NotTo(BeNil())
		Expect(plan.Usage.MCPProxies).To(Equal(1))
		Expect(plan.Usage.InUse).To(BeFalse(), "only subscriptions lock a plan")
		renamed := aihubop.UpdatePlan(Default, Client, planID, framework.SubscriptionPlanRequest{DisplayName: "E2E Offered renamed"})
		Expect(renamed.DisplayName).To(Equal("E2E Offered renamed"))
	})

	It("should keep a deprecated plan where it is offered, and refuse to offer it again", func() {
		Expect(aihubop.DeprecatePlan(Default, Client, planID).Deprecated).To(BeTrue())
		Expect(mcpproxyop.GetMCPProxy(Default, Client, proxyID).Spec.SubscriptionPlans).To(ConsistOf(planID))

		resp, err := Client.Post("/m/ai-gateway/mcp-proxies", proxyDocument(otherProxyID, planID))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest), "a deprecated plan can't be offered")
	})
})

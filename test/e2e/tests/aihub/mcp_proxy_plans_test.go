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
// existing one, and the plan it offers is in use (it can't be renamed or deleted) until the proxy is gone.
// The proxy is deployed to the default environment, so the stack needs its AI gateway.

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
	var planID, proxyID, envUUID string

	proxyRequest := func(id string, plans ...string) framework.CreateMCPProxyRequest {
		ctxPath := "/" + id
		upstream := framework.TestMCPServerURL
		return framework.CreateMCPProxyRequest{
			ID: id, Name: "E2E Plans Proxy " + id[len(id)-8:], Version: "v1.0", Context: &ctxPath,
			Endpoints: []framework.MCPProxyEndpoint{{
				ID:           "primary",
				Upstream:     framework.UpstreamConfig{Main: &framework.UpstreamEndpoint{URL: &upstream}},
				Environments: []framework.MCPEndpointEnvironment{{EnvironmentUUID: envUUID}},
			}},
			SubscriptionPlans: plans,
		}
	}

	BeforeAll(func() {
		skipWithoutAIHub()
		_, envUUID = gateway.WaitForActiveGatewayForEnvWithEnvUUID(Client, Cfg.DefaultOrg, Cfg.DefaultEnv, 3*time.Minute)
		Expect(envUUID).NotTo(BeEmpty(), "expected an environment UUID for %s", Cfg.DefaultEnv)
		suffix := uuid.New().String()[:8]
		planID = "e2e-test-offered-plan-" + suffix
		proxyID = "e2e-test-plans-proxy-" + suffix
		aihubop.CreatePlan(Default, Client, framework.SubscriptionPlanRequest{ID: planID, DisplayName: "E2E Offered " + suffix})
	})

	AfterAll(func() {
		if proxyID != "" {
			mcpproxyop.DeleteMCPProxyBestEffort(context.Background(), Client, Cfg.DefaultOrg, proxyID)
		}
		if planID != "" {
			aihubop.DeletePlan(Default, Client, planID)
		}
	})

	It("should refuse to offer a plan that doesn't exist", func() {
		resp, err := Client.Post("/m/ai-gateway/orgs/"+Cfg.DefaultOrg+"/mcp-proxies",
			proxyRequest(proxyID, "e2e-test-no-such-plan-"+uuid.New().String()[:8]))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
	})

	It("should offer the plan", func() {
		proxy := mcpproxyop.CreateMCPProxy(Default, Client, Cfg.DefaultOrg, proxyRequest(proxyID, planID))
		Expect(proxy.SubscriptionPlans).To(ConsistOf(planID))
		Expect(mcpproxyop.GetMCPProxy(Default, Client, Cfg.DefaultOrg, proxyID).SubscriptionPlans).To(ConsistOf(planID))
	})

	It("should keep the offered plan in use", func() {
		plan := aihubop.GetPlan(Default, Client, planID)
		Expect(plan.Usage).NotTo(BeNil())
		Expect(plan.Usage.InUse).To(BeTrue())
		Expect(plan.Usage.MCPProxies).To(Equal(1))

		resp, err := Client.Put(aihubop.PlansPath+"/"+planID, framework.SubscriptionPlanRequest{DisplayName: "Renamed"})
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusConflict), "an offered plan keeps its name")

		del, err := Client.Delete(aihubop.PlansPath + "/" + planID)
		Expect(err).NotTo(HaveOccurred())
		defer del.Body.Close()
		Expect(del.StatusCode).To(Equal(http.StatusConflict), "an offered plan can't be deleted")
	})

	It("should free the plan once the proxy is gone", func() {
		mcpproxyop.DeleteMCPProxy(Default, Client, Cfg.DefaultOrg, proxyID)
		Expect(aihubop.GetPlan(Default, Client, planID).Usage.InUse).To(BeFalse())
		aihubop.DeletePlan(Default, Client, planID)
		aihubop.GetPlanExpectStatus(Default, Client, planID, http.StatusNotFound)
	})
})

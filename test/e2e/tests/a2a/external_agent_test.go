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

// Validates that an external agent can take the shared platform agent's card
// as its source and be deployed to the gateway as a proxy. The proxied card
// fetch and invocation are not covered: the gateway runtime resolves the
// platform agent's *.localhost host to loopback and cannot reach it.

package a2a

import (
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	a2aops "github.com/wso2/agent-manager/test/e2e/operations/a2a"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
)

var _ = Describe("Proxied external A2A agent:", Label("a2a", "external-agent"), Ordered, func() {
	var agentName string

	BeforeAll(func() {
		if len(Cfg.A2ACardTestUpstreamHosts) == 0 {
			Skip("set A2A_CARD_TEST_UPSTREAM_HOSTS for the suite and the service to run the external A2A specs")
		}
		parsed, err := url.Parse(Agent.CardURL)
		Expect(err).NotTo(HaveOccurred())
		host := strings.ToLower(parsed.Hostname())
		Expect(slices.Contains(Cfg.A2ACardTestUpstreamHosts, host)).To(BeTrue(),
			"configuration error: the platform agent's card host %q is not in A2A_CARD_TEST_UPSTREAM_HOSTS %v",
			host, Cfg.A2ACardTestUpstreamHosts)

		agentName = "e2e-test-a2a-ext-" + uuid.New().String()[:8]
	})

	AfterAll(func() {
		if agentName != "" {
			agentops.DeleteAgentBestEffort(Client, Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName)
		}
	})

	It("registers and fetches the platform agent's card as its source", func() {
		agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			Request:     framework.NewExternalAgentRequest(agentName, "External agent proxied to the shared A2A agent by the a2a e2e tests"),
		})

		a2aops.SetCardSource(Default, Client, Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv, Agent.CardURL)
		// Waited for before SetProxy: once proxied, the card is fetched through the gateway, which cannot reach this upstream.
		card := a2aops.WaitForCardFetched(Client, Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv, 2*time.Minute)
		Expect(card.SourceURL).To(Equal(Agent.CardURL))
		a2aops.SetProxy(Default, Client, Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv, "X-API-Key", Agent.APIKey)
	})

	It("is deployed to the gateway as a proxy", func() {
		a2aops.WaitForProxyPublished(Client, Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv, 5*time.Minute)
	})
})

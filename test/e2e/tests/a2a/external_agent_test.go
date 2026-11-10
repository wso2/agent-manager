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

// Validates an external agent proxied through the gateway, using the shared
// platform agent as its origin.

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

// proxiedAgentNeedsOwnAPIKey is the one place that says how callers reach a
// proxied external agent. The service gives a proxied environment the platform
// agent's inbound defaults (api-key-auth on), so callers present a key minted
// for the external agent itself. Set false if a live run shows otherwise.
const proxiedAgentNeedsOwnAPIKey = true

var _ = Describe("Proxied external A2A agent:", Label("a2a", "external-agent"), Ordered, func() {
	var (
		agentName string
		card      framework.AgentCardResponse
	)

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

	It("registers with the platform agent's card as its source", func() {
		agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			Request:     framework.NewExternalAgentRequest(agentName, "External agent proxied to the shared A2A agent by the a2a e2e tests"),
		})

		a2aops.SetCardSource(Default, Client, Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv, Agent.CardURL)
		a2aops.SetProxy(Default, Client, Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv, "X-API-Key", Agent.APIKey)
	})

	It("is published to the gateway and has its card fetched through it", func() {
		a2aops.WaitForProxyPublished(Client, Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv, 16*time.Minute)
		card = a2aops.WaitForCardFetched(Client, Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv, 12*time.Minute)

		Expect(card.Source).To(Equal("external"))
		platformCard := a2aops.GetAgentCard(Default, Client, Cfg.DefaultOrg, Agent.ProjectName, Agent.AgentName, Cfg.DefaultEnv)
		Expect(a2aops.JSONRPCURL(card.Card)).NotTo(BeEmpty())
		Expect(a2aops.JSONRPCURL(card.Card)).NotTo(Equal(a2aops.JSONRPCURL(platformCard.Card)))
	})

	It("answers a message sent through the gateway", func() {
		apiKey := ""
		if proxiedAgentNeedsOwnAPIKey {
			apiKey = agentops.CreateAgentAPIKey(Default, Client,
				Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv,
				framework.CreateAgentAPIKeyRequest{
					DisplayName: "e2e-a2a-ext-key",
					ExpiresAt:   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				}).ApiKey
			Expect(apiKey).NotTo(BeEmpty())
		}

		reply := a2aops.SendMessage(a2aops.JSONRPCURL(card.Card), apiKey, "Standup: the rollout is on track.")
		Expect(reply).NotTo(BeEmpty())
	})
})

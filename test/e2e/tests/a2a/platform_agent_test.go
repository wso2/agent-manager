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

// Validates a platform A2A agent's card and message exchange through the gateway.

package a2a

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	a2aops "github.com/wso2/agent-manager/test/e2e/operations/a2a"
)

var _ = Describe("Platform A2A agent:", Label("a2a", "platform-agent"), Ordered, func() {
	var card framework.AgentCardResponse

	BeforeAll(func() {
		card = a2aops.GetAgentCard(Default, Client, Cfg.DefaultOrg, Agent.ProjectName, Agent.AgentName, Cfg.DefaultEnv)
	})

	It("has its card fetched through the gateway", func() {
		Expect(card.Status).To(Equal("fetched"), "lastError: %s", card.LastError)
		Expect(card.Source).To(Equal("platform"))

		var parsed struct {
			Name                string `json:"name"`
			SupportedInterfaces []struct {
				URL string `json:"url"`
			} `json:"supportedInterfaces"`
			Skills *[]json.RawMessage `json:"skills"`
		}
		Expect(json.Unmarshal(card.Card, &parsed)).To(Succeed())
		Expect(parsed.Name).NotTo(BeEmpty())
		Expect(parsed.SupportedInterfaces).NotTo(BeEmpty())
		for _, iface := range parsed.SupportedInterfaces {
			Expect(iface.URL).NotTo(BeEmpty())
		}
		Expect(parsed.Skills).NotTo(BeNil(), "card has no skills array")
	})

	It("serves the same card at its gateway URL", func() {
		resp, err := (&http.Client{Timeout: 30 * time.Second}).Get(Agent.CardURL)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())

		var served, stored struct {
			Name string `json:"name"`
		}
		Expect(json.Unmarshal(body, &served)).To(Succeed())
		Expect(json.Unmarshal(card.Card, &stored)).To(Succeed())
		Expect(served.Name).To(Equal(stored.Name))
	})

	It("answers a message sent through the gateway", func() {
		rpcURL := a2aops.JSONRPCURL(card.Card)
		Expect(rpcURL).NotTo(BeEmpty(), "card has no JSONRPC interface: %s", card.Card)
		a2aops.SendMessage(rpcURL, Agent.APIKey, "Standup: the rollout is on track.")
	})

	// Runs last so a slow refresh cannot delay the other specs.
	It("re-fetches the card on refresh", func() {
		before := card.FetchedAt
		a2aops.RefreshAgentCard(Default, Client, Cfg.DefaultOrg, Agent.ProjectName, Agent.AgentName, Cfg.DefaultEnv)

		Eventually(func(g Gomega) {
			current := a2aops.GetAgentCard(g, Client, Cfg.DefaultOrg, Agent.ProjectName, Agent.AgentName, Cfg.DefaultEnv)
			g.Expect(current.Status).To(Equal("fetched"), "lastError: %s", current.LastError)
			g.Expect(current.FetchedAt).NotTo(Equal(before))
		}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
	})
})

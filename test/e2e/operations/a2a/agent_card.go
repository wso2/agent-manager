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

package a2a

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

func agentCardPath(org, proj, agent, env string) string {
	return fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/environments/%s/agent-card", org, proj, agent, env)
}

// GetAgentCard fetches the stored agent card, expecting 200.
func GetAgentCard(g Gomega, c *framework.AMPClient, org, proj, agent, env string) framework.AgentCardResponse {
	resp, err := c.Get(agentCardPath(org, proj, agent, env))
	g.Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, http.StatusOK)
	return framework.DecodeBody[framework.AgentCardResponse](g, resp)
}

// RefreshAgentCard asks the service to re-fetch the agent card, expecting 202.
func RefreshAgentCard(g Gomega, c *framework.AMPClient, org, proj, agent, env string) {
	resp, err := c.Post(agentCardPath(org, proj, agent, env)+"/refresh", nil)
	g.Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, http.StatusAccepted)
}

// SetCardSource points the agent card at an explicit URL, expecting 202.
func SetCardSource(g Gomega, c *framework.AMPClient, org, proj, agent, env, cardURL string) {
	resp, err := c.Put(agentCardPath(org, proj, agent, env)+"/source", map[string]string{"url": cardURL})
	g.Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, http.StatusAccepted)
}

// WaitForCardFetched polls until the agent card is fetched. A card that does
// not exist yet counts as not ready; a failed fetch stops the wait at once.
func WaitForCardFetched(c *framework.AMPClient, org, proj, agent, env string, timeout time.Duration) framework.AgentCardResponse {
	var card framework.AgentCardResponse
	waitForStatus(func(g Gomega) (string, string, bool) {
		resp, err := c.Get(agentCardPath(org, proj, agent, env))
		g.Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return "", "", false
		}
		card = framework.ExpectStatusAndDecode[framework.AgentCardResponse](g, resp, http.StatusOK)
		return card.Status, card.LastError, true
	}, "fetched", "agent card", timeout)
	return card
}

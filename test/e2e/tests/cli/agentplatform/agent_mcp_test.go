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

// Validates the amctl agent mcp lifecycle end to end against the CLI-owned
// platform agent: set (create→POST), get, list, set (update→GET+PUT), unset
// (whole-config DELETE), and the post-unset not-found envelope. Asserts JSON
// envelopes and exit codes only.
//
// Unlike agent llm — which binds a config-only provider — an mcp config can only
// reference a catalog MCP proxy, and a proxy becomes catalog-eligible only after
// it is deployed to a gateway. The CLI cannot provision proxies, so BeforeAll
// creates a gateway-deployed proxy through the HTTP API first. This makes the
// suite depend on a running AI gateway in DefaultEnv and the reachability of
// framework.TestMCPServerURL.

package cliagentplatformtests

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	cliagent "github.com/wso2/agent-manager/test/e2e/operations/cli/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/gateway"
	mcpproxyop "github.com/wso2/agent-manager/test/e2e/operations/mcpproxy"
)

var _ = Describe("amctl agent mcp (CLI-owned agent)", Label("cli", "agent", "mcp"), Ordered, Serial, func() {
	const (
		// Distinct from the llm spec: config names are unique per agent across types and both specs share this agent.
		configName = "mcp-primary"
		urlEnv     = "E2E_MCP_URL"
		apiKeyEnv  = "E2E_MCP_KEY"
	)
	var proxyID string

	BeforeAll(func() {
		// The agent's mcp config can only bind a catalog proxy, which requires a
		// gateway deployment — provision one over HTTP (the CLI can't).
		gatewayUUID, envUUID := gateway.WaitForActiveGatewayForEnvWithEnvUUID(apiClient, H.Org(), cfg.DefaultEnv, 3*time.Minute)
		Expect(gatewayUUID).NotTo(BeEmpty())
		Expect(envUUID).NotTo(BeEmpty(), "expected an environment UUID for %s", cfg.DefaultEnv)

		proxyID = framework.E2EMCPProxyPrefix + uuid.New().String()[:8]
		upstreamURL := framework.TestMCPServerURL
		ctx := "/" + proxyID
		proxy := mcpproxyop.CreateMCPProxy(Default, apiClient, H.Org(),
			framework.CreateMCPProxyRequest{
				ID:      proxyID,
				Name:    "CLI E2E MCP Proxy " + proxyID[len(proxyID)-8:],
				Version: "v1.0",
				Context: &ctx,
				// A single endpoint deployed to DefaultEnv, which deploys the gateway
				// artifact that makes the proxy catalog-eligible.
				Endpoints: []framework.MCPProxyEndpoint{
					{
						ID:       "primary",
						Upstream: framework.UpstreamConfig{Main: &framework.UpstreamEndpoint{URL: &upstreamURL}},
						Security: &framework.SecurityConfig{
							Enabled: true,
							APIKey: &framework.SecurityAPIKey{
								Enabled: true,
								Key:     "X-API-Key",
								In:      "header",
							},
						},
						Environments: []framework.MCPEndpointEnvironment{
							{EnvironmentUUID: envUUID},
						},
					},
				},
			})
		Expect(proxy.ID).To(Equal(proxyID))

		// LIFO cleanup: unset the agent config first, then delete the proxy.
		DeferCleanup(func() {
			mcpproxyop.DeleteMCPProxy(Default, apiClient, H.Org(), proxyID)
		})
		DeferCleanup(func() {
			_ = H.Run("agent", "mcp", "unset", owned.AgentName, "--name", configName, "--yes",
				"--org", H.Org(), "--project", owned.ProjectName, "--json")
		})
		GinkgoWriter.Printf("CLI agent mcp: agent=%s proxy=%s\n", owned.AgentName, proxyID)
	})

	It("sets a new mcp config (create → POST)", func() {
		c := cliagent.SetAgentMCP(Default, H, H.Org(), owned.ProjectName, owned.AgentName, cliagent.SetAgentMCPParams{
			Name:        configName,
			Env:         cfg.DefaultEnv,
			Proxy:       proxyID,
			URLEnv:      urlEnv,
			APIKeyEnv:   apiKeyEnv,
			Description: "created by agent mcp e2e",
		})
		Expect(c.Name).To(Equal(configName))
		Expect(c.Type).To(Equal("mcp"))
		Expect(c.EnvMappings).To(HaveKey(cfg.DefaultEnv))
		Expect(c.EnvMappings[cfg.DefaultEnv].Configuration).NotTo(BeNil())
		Expect(c.EnvMappings[cfg.DefaultEnv].Configuration.Handle()).To(Equal(proxyID))
	})

	It("gets the mcp config", func() {
		c := cliagent.GetAgentMCP(Default, H, H.Org(), owned.ProjectName, owned.AgentName, configName)
		Expect(c.Name).To(Equal(configName))
		Expect(c.Description).To(Equal("created by agent mcp e2e"))
		Expect(c.EnvMappings).To(HaveKey(cfg.DefaultEnv))
	})

	It("lists mcp configs (type=mcp only)", func() {
		list := cliagent.ListAgentMCP(Default, H, H.Org(), owned.ProjectName, owned.AgentName)
		Expect(list.Names()).To(ContainElement(configName))
		for _, c := range list.Configs {
			Expect(c.Type).To(Equal("mcp"))
		}
	})

	It("updates the mcp config (existing → GET+PUT)", func() {
		c := cliagent.SetAgentMCP(Default, H, H.Org(), owned.ProjectName, owned.AgentName, cliagent.SetAgentMCPParams{
			Name:        configName,
			Env:         cfg.DefaultEnv,
			Proxy:       proxyID,
			Description: "updated by agent mcp e2e",
		})
		Expect(c.Name).To(Equal(configName))
		Expect(c.Description).To(Equal("updated by agent mcp e2e"))
	})

	It("unsets the whole mcp config (DELETE)", func() {
		res := cliagent.UnsetAgentMCP(Default, H, H.Org(), owned.ProjectName, owned.AgentName, configName)
		Expect(res.Name).To(Equal(configName))
		Expect(res.Deleted).To(BeTrue())
	})

	It("reports the mcp config gone after unset", func() {
		Eventually(func(g Gomega) {
			cliagent.GetAgentMCPExpectError(g, H, H.Org(), owned.ProjectName, owned.AgentName, configName)
		}).WithTimeout(30 * time.Second).WithPolling(3 * time.Second).Should(Succeed())
	})
})

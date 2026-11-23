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

// Validates MCP proxy live invocation through the gateway: an external agent
// attaches an MCP proxy (which surfaces the gateway proxy URL + API key), the
// proxy is invoked with a real MCP `initialize` handshake, and the api-key
// security policy is shown to be enforced (a keyless request is rejected).

package mcpproxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/configuration"
	"github.com/wso2/agent-manager/test/e2e/operations/gateway"
	mcpproxyop "github.com/wso2/agent-manager/test/e2e/operations/mcpproxy"
)

var _ = Describe("MCP Proxy with External Agent and Policy Enforcement", Label("mcp-proxy", "external-agent"), Ordered, func() {
	var (
		suffix      string
		agentName   string
		proxyID     string
		gatewayUUID string
		envUUID     string

		proxyURL    string
		proxyAPIKey string
		authHeader  string
		configID    string

		createReq framework.CreateAgentRequest
	)

	// mcpInitializeBody is a minimal MCP `initialize` JSON-RPC request — the first
	// call in the MCP handshake, valid without a prior session.
	mcpInitializeBody := func() []byte {
		body, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
			"params": map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]any{},
				"clientInfo":      map[string]any{"name": "e2e", "version": "1.0"},
			},
		})
		return body
	}

	BeforeAll(func() {
		suffix = uuid.New().String()[:8]
		// Agent names are capped at 25 chars; keep the prefix short.
		agentName = "e2e-mcp-agent-" + suffix
		proxyID = "e2e-mcp-inv-proxy-" + suffix
		createReq = framework.NewExternalAgentRequest(agentName, "External agent for MCP proxy invocation test")
	})

	It("should have a running AI gateway", func() {
		gatewayUUID, envUUID = gateway.WaitForActiveGatewayForEnvWithEnvUUID(Client, Cfg.DefaultOrg, Cfg.DefaultEnv, 3*time.Minute)
		Expect(gatewayUUID).NotTo(BeEmpty())
		Expect(envUUID).NotTo(BeEmpty(), "expected an environment UUID for %s", Cfg.DefaultEnv)
	})

	It("should create an MCP proxy with api-key security deployed to the gateway", func() {
		upstreamURL := framework.TestMCPServerURL
		ctx := "/" + proxyID

		proxy := mcpproxyop.CreateMCPProxy(Default, Client, Cfg.DefaultOrg,
			framework.CreateMCPProxyRequest{
				ID:      proxyID,
				Name:    "E2E MCP Inv Proxy " + suffix,
				Version: "v1.0",
				Context: &ctx,
				// The api-key security policy lives on the endpoint; the proxy deploys the
				// endpoint's artifact to the env's gateway, and per-agent inbound keys are
				// minted against it.
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
	})

	It("should create an external agent", func() {
		ag := agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			Request:     createReq,
		})
		Expect(ag.Name).To(Equal(agentName))
		Expect(ag.Provisioning.Type).To(Equal("external"))
	})

	It("should attach the MCP proxy and extract gateway proxy credentials", func() {
		// MCP proxies attach via the dedicated mcp-configs endpoint; the server
		// forces type "mcp", so we don't set it on the request.
		config := configuration.CreateAgentMCPConfig(Default, Client,
			Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName,
			framework.CreateAgentModelConfigRequest{
				Name: "e2e-mcp-cfg-" + suffix,
				EnvMappings: map[string]framework.EnvModelConfigRequest{
					Cfg.DefaultEnv: {MCPProxyName: proxyID},
				},
			})
		Expect(config.UUID).NotTo(BeEmpty())
		configID = config.UUID

		envMapping, exists := config.EnvMappings[Cfg.DefaultEnv]
		Expect(exists).To(BeTrue(), "expected env mapping for %s", Cfg.DefaultEnv)
		Expect(envMapping.Configuration).NotTo(BeNil(), "expected configuration in env mapping")

		proxyURL = envMapping.Configuration.URL
		Expect(proxyURL).NotTo(BeEmpty(), "MCP proxy URL should be set")

		Expect(envMapping.Configuration.AuthInfo).NotTo(BeNil(), "expected authInfo with API key")
		Expect(envMapping.Configuration.AuthInfo.Value).NotTo(BeNil(), "expected API key value")
		proxyAPIKey = *envMapping.Configuration.AuthInfo.Value
		authHeader = envMapping.Configuration.AuthInfo.Name
		Expect(authHeader).NotTo(BeEmpty(), "expected auth header name")

		GinkgoWriter.Printf("MCP proxy attached: url=%s, authHeader=%s\n", proxyURL, authHeader)
	})

	It("should invoke the MCP proxy successfully with the API key", func() {
		Expect(proxyURL).NotTo(BeEmpty())
		Expect(proxyAPIKey).NotTo(BeEmpty())

		httpClient := &http.Client{Timeout: 30 * time.Second}

		// The gateway needs a few seconds to sync the new route after deployment.
		Eventually(func(g Gomega) {
			req, err := http.NewRequest("POST", proxyURL, bytes.NewBuffer(mcpInitializeBody()))
			g.Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header[authHeader] = []string{proxyAPIKey}

			resp, err := httpClient.Do(req)
			g.Expect(err).NotTo(HaveOccurred(), "MCP proxy request to %s failed: %v", proxyURL, err)
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			g.Expect(resp.StatusCode).To(Equal(http.StatusOK),
				"MCP initialize returned %d: %s", resp.StatusCode, string(body))
			// The upstream server answered the handshake, not just the gateway.
			g.Expect(string(body)).To(ContainSubstring(`"protocolVersion"`))
			g.Expect(string(body)).To(ContainSubstring(`"serverInfo"`))
		}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
	})

	It("should reject invocation without the API key (api-key security policy)", func() {
		Expect(proxyURL).NotTo(BeEmpty())

		httpClient := &http.Client{Timeout: 30 * time.Second}

		// Retry past 404 (gateway still syncing the route); once a non-404
		// response arrives, the keyless request must be rejected by the
		// api-key-auth security policy with 401/403.
		Eventually(func(g Gomega) {
			req, err := http.NewRequest("POST", proxyURL, bytes.NewBuffer(mcpInitializeBody()))
			g.Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")

			resp, err := httpClient.Do(req)
			g.Expect(err).NotTo(HaveOccurred(), "MCP proxy request to %s failed: %v", proxyURL, err)
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode == http.StatusNotFound {
				g.Expect(resp.StatusCode).NotTo(Equal(http.StatusNotFound), "waiting for gateway route to sync")
				return
			}
			g.Expect(resp.StatusCode).To(SatisfyAny(
				Equal(http.StatusUnauthorized), Equal(http.StatusForbidden),
			),
				"expected keyless request to be rejected, got %d: %s", resp.StatusCode, string(body))
		}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
	})
	// invokeStatus calls the agent's MCP binding with key in the binding's header.
	invokeStatus := func(g Gomega, key string) int {
		req, err := http.NewRequest(http.MethodPost, proxyURL, bytes.NewBuffer(mcpInitializeBody()))
		g.Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header[authHeader] = []string{key}
		resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
		g.Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		return resp.StatusCode
	}

	It("reads, lists and updates the agent's MCP binding", func() {
		base := fmt.Sprintf("/m/ai-gateway/orgs/%s/projects/%s/agents/%s/mcp-configs", Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName)
		got := framework.CallObj(Default, Client, http.MethodGet, base+"/"+configID, nil, http.StatusOK)
		Expect(got["name"]).To(Equal("e2e-mcp-cfg-" + suffix))
		list := framework.CallObj(Default, Client, http.MethodGet, base, nil, http.StatusOK)
		Expect(framework.Field(framework.Items(list, "configs"), "uuid")).To(ContainElement(configID))
		framework.Call(Default, Client, http.MethodPut, base+"/"+configID, framework.Obj{
			"name": "e2e-mcp-cfg-" + suffix, "description": "updated",
			"envMappings": framework.Obj{Cfg.DefaultEnv: framework.Obj{"mcpProxyName": proxyID}},
		}, http.StatusOK)
		Expect(framework.CallObj(Default, Client, http.MethodGet, base+"/"+configID, nil, http.StatusOK)["description"]).To(Equal("updated"))
	})

	It("mints a key for the binding that works until it's rotated and then revoked", func() {
		keys := fmt.Sprintf("/m/ai-gateway/orgs/%s/projects/%s/agents/%s/mcp-configs/%s/environments/%s/api-keys",
			Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, configID, Cfg.DefaultEnv)
		minted := framework.CallObj(Default, Client, http.MethodPost, keys, framework.Obj{"name": "e2e-mcp-key", "displayName": "e2e-mcp-key"}, http.StatusCreated)
		first := framework.Str(minted, "apiKey")
		Expect(first).NotTo(BeEmpty())
		Expect(framework.Field(framework.Items(framework.CallObj(Default, Client, http.MethodGet, keys, nil, http.StatusOK), "keys"), "name")).
			To(ContainElement("e2e-mcp-key"))
		Eventually(func(g Gomega) int { return invokeStatus(g, first) }).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Equal(http.StatusOK))

		rotated := framework.Str(framework.CallObj(Default, Client, http.MethodPut, keys+"/e2e-mcp-key", framework.Obj{}, http.StatusOK), "apiKey")
		Expect(rotated).NotTo(Equal(first))
		Eventually(func(g Gomega) int { return invokeStatus(g, rotated) }).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Equal(http.StatusOK))
		Eventually(func(g Gomega) int { return invokeStatus(g, first) }).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Equal(http.StatusUnauthorized))

		framework.Call(Default, Client, http.MethodDelete, keys+"/e2e-mcp-key", nil, http.StatusNoContent)
		Eventually(func(g Gomega) int { return invokeStatus(g, rotated) }).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Equal(http.StatusUnauthorized))
		// The binding's own key is untouched.
		Expect(invokeStatus(Default, proxyAPIKey)).To(Equal(http.StatusOK))
	})
})

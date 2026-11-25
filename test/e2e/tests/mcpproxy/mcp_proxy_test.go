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

// Validates the MCP proxy lifecycle: discovering a live upstream MCP server,
// creating a proxy from it (with a single endpoint deployed to the default
// environment's AI gateway), reading it back by handle, listing, and deleting it.

package mcpproxy

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	"github.com/wso2/agent-manager/test/e2e/operations/gateway"
	mcpproxyop "github.com/wso2/agent-manager/test/e2e/operations/mcpproxy"
)

var _ = Describe("MCP Proxy Lifecycle", Label("mcp-proxy"), Ordered, func() {
	var (
		suffix      string
		proxyID     string
		gatewayUUID string
		envUUID     string
	)

	BeforeAll(func() {
		suffix = uuid.New().String()[:8]
		proxyID = "e2e-test-mcp-proxy-" + suffix
	})

	It("should have a running AI gateway", func() {
		gatewayUUID, envUUID = gateway.WaitForActiveGatewayForEnvWithEnvUUID(Client, Cfg.DefaultOrg, Cfg.DefaultEnv, 3*time.Minute)
		Expect(gatewayUUID).NotTo(BeEmpty())
		Expect(envUUID).NotTo(BeEmpty(), "expected an environment UUID for %s", Cfg.DefaultEnv)
	})

	It("should discover the upstream MCP server", func() {
		info := mcpproxyop.FetchServerInfo(Default, Client,
			framework.MCPServerInfoFetchRequest{
				URL: framework.TestMCPServerURL,
			})
		Expect(len(info.Tools)).To(BeNumerically(">", 0), "expected the everything server to report tools")
		GinkgoWriter.Printf("Discovered MCP server: %d tools, %d prompts, %d resources\n",
			len(info.Tools), len(info.Prompts), len(info.Resources))
	})

	It("should create an MCP proxy with an endpoint deployed to the gateway", func() {
		upstreamURL := framework.TestMCPServerURL
		ctx := "/" + proxyID

		proxy := mcpproxyop.CreateMCPProxy(Default, Client, proxyID,
			framework.MCPProxySpec{
				DisplayName: "E2E MCP Proxy " + suffix,
				Version:     "v1.0",
				Context:     &ctx,
				// The proxy carries a single endpoint whose upstream + security define the
				// deployable definition; it is deployed to DefaultEnv's gateway.
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
		Expect(proxy.Metadata.Name).To(Equal(proxyID))
		GinkgoWriter.Printf("Created MCP proxy %s (env %s deployed to gateway %s)\n", proxy.Metadata.Name, envUUID, gatewayUUID)
	})

	It("should get the MCP proxy by handle", func() {
		proxy := mcpproxyop.GetMCPProxy(Default, Client, proxyID)
		Expect(proxy.Metadata.Name).To(Equal(proxyID))
		Expect(proxy.Spec.DisplayName).To(Equal("E2E MCP Proxy " + suffix))
		Expect(proxy.Spec.Endpoints).To(HaveLen(1), "expected a single endpoint")
		endpoint := proxy.Spec.Endpoints[0]
		Expect(endpoint.ID).To(Equal("primary"))
		Expect(endpoint.Upstream.Main).NotTo(BeNil(), "expected an upstream in the endpoint")
		Expect(endpoint.Upstream.Main.URL).NotTo(BeNil())
		Expect(*endpoint.Upstream.Main.URL).To(Equal(framework.TestMCPServerURL))

		envUUIDs := make([]string, 0, len(endpoint.Environments))
		for _, e := range endpoint.Environments {
			envUUIDs = append(envUUIDs, e.EnvironmentUUID)
		}
		Expect(envUUIDs).To(ContainElement(envUUID), "expected the endpoint deployed to %s", envUUID)
	})

	It("should list the MCP proxy", func() {
		list := mcpproxyop.ListMCPProxies(Default, Client)
		found := false
		for _, p := range list.List {
			if p.Metadata.Name == proxyID {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(), "expected created proxy %s in the list", proxyID)
	})

	It("should update the MCP proxy's description", func() {
		// The spec replaces the stored one, so the update carries the spec just read back.
		proxy := mcpproxyop.GetMCPProxy(Default, Client, proxyID)
		desc := "updated by e2e"
		proxy.Spec.Description = &desc
		mcpproxyop.UpdateMCPProxy(Default, Client, proxyID, proxy.Spec)
		got := mcpproxyop.GetMCPProxy(Default, Client, proxyID)
		Expect(got.Spec.Description).NotTo(BeNil())
		Expect(*got.Spec.Description).To(Equal(desc))
	})

	It("should manage the proxy's tool scopes", func() {
		scopes := fmt.Sprintf("/m/ai-gateway/mcp-proxies/%s/scopes", proxyID)
		// A scope is named by metadata.name: the action. Renaming one means delete + create.
		scopeDoc := func(action, description string, tools []string) framework.MCPProxyScopeDocument {
			return framework.MCPProxyScopeDocument{
				APIVersion: framework.MCPProxyAPIVersion,
				Kind:       framework.MCPProxyScopeKind,
				Metadata:   framework.DocumentMetadata{Name: action},
				Spec:       framework.MCPProxyScopeSpec{Description: description, Tools: tools},
			}
		}
		actions := func(list framework.MCPProxyScopeListResponse) []string {
			out := make([]string, 0, len(list.List))
			for _, sc := range list.List {
				out = append(out, sc.Metadata.Name)
			}
			return out
		}
		listScopes := func() framework.MCPProxyScopeListResponse {
			return framework.CallJSON[framework.MCPProxyScopeListResponse](Default, Client, http.MethodGet, scopes, nil, http.StatusOK)
		}

		framework.Call(Default, Client, http.MethodPost, scopes, scopeDoc("e2e-read", "read", []string{"echo"}), http.StatusCreated)
		Expect(actions(listScopes())).To(ContainElement("e2e-read"))

		framework.Call(Default, Client, http.MethodPut, scopes+"/e2e-read", scopeDoc("e2e-read", "renamed", []string{"echo", "add"}), http.StatusOK)
		for _, sc := range listScopes().List {
			if sc.Metadata.Name == "e2e-read" {
				Expect(sc.Spec.Tools).To(ConsistOf("echo", "add"))
			}
		}

		// The scope is offered to agent identity roles in the proxy's environments.
		agentScopes := framework.CallObj(Default, Client, http.MethodGet, framework.GW(Cfg, "/environments/%s/agent-identities/scopes", Cfg.DefaultEnv), nil, http.StatusOK)
		Expect(agentScopes).To(HaveKey("scopes"))

		framework.Call(Default, Client, http.MethodDelete, scopes+"/e2e-read", nil, http.StatusNoContent)
		Expect(actions(listScopes())).NotTo(ContainElement("e2e-read"))
		framework.CallError(Default, Client, http.MethodDelete, scopes+"/e2e-read", nil, http.StatusNotFound)
	})

	It("should list the policies an MCP proxy can use", func() {
		policies := framework.CallObj(Default, Client, http.MethodGet, "/m/ai-gateway/mcp-proxies/policies", nil, http.StatusOK)
		Expect(policies["count"]).To(BeNumerically(">", 0))
	})

	It("should delete the MCP proxy and then return 404", func() {
		mcpproxyop.DeleteMCPProxy(Default, Client, proxyID)
		mcpproxyop.GetMCPProxyExpectStatus(Default, Client, proxyID, 404)
	})
})

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

// J3: a developer registers an agent hosted elsewhere with amctl, edits it
// through the API, and mints the token it publishes traces with. Its gateway
// settings and keys need the agent routed through the gateway first, which an
// external agent is only once it has an A2A card; until then those calls are
// refused and change nothing. (The gateway key journey for an agent runs on
// its MCP binding, in tests/mcpproxy.)

package agent

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	cliagent "github.com/wso2/agent-manager/test/e2e/operations/cli/agent"
)

var _ = Describe("External agent registration:", Label("agent", "external-agent", "cli"), Ordered, func() {
	var agentName string
	proj := framework.E2ESharedProjectName
	gw := func(f string, a ...any) string { return framework.GWAgent(proj, agentName, f, a...) }

	BeforeAll(func() {
		agentName = framework.E2EAgentPrefix + uuid.NewString()[:8]
		DeferCleanup(func() { agentops.DeleteAgentBestEffort(Client, Cfg.DefaultOrg, proj, agentName) })
	})

	It("registers the agent with amctl, which the API then reads", func() {
		res := cliagent.CreateExternalAgent(Default, H, H.Org(), proj, agentName, "CLI agent", "registered by amctl")
		Expect(res.Agent.Metadata.Name).To(Equal(agentName))
		Expect(res.Agent.Spec.Provisioning.Type).To(Equal("external"))

		got := agentops.GetAgent(Default, Client, Cfg.DefaultOrg, proj, agentName)
		Expect(got.Spec.DisplayName).To(Equal("CLI agent"))
		Expect(agentops.ListAgents(Default, Client, Cfg.DefaultOrg, proj).List).To(ContainElement(
			HaveField("Metadata.Name", agentName)))

		framework.CallError(Default, Client, http.MethodPost, "/api/v1/agents",
			framework.NewExternalAgentRequest(agentName, "duplicate").Document(proj), http.StatusConflict)
	})

	It("edits the agent through the API, which amctl then shows", func() {
		edited := agentops.GetAgent(Default, Client, Cfg.DefaultOrg, proj, agentName)
		edited.Status = nil
		edited.Spec.DisplayName, edited.Spec.Description = "Edited agent", "edited"
		framework.Call(Default, Client, http.MethodPut, agentops.AgentPath(proj, agentName), edited, http.StatusOK)
		Expect(cliagent.GetAgent(Default, H, H.Org(), proj, agentName).Spec.DisplayName).To(Equal("Edited agent"))
		Expect(cliagent.ListAgents(Default, H, H.Org(), proj).Names()).To(ContainElement(agentName))
	})

	It("mints a signed token naming the agent", func() {
		tok := agentops.GenerateAgentToken(Default, Client, Cfg.DefaultOrg, proj, agentName, Cfg.DefaultEnv, "1h")
		Expect(strings.Count(tok.Token, ".")).To(Equal(2))
		claims, err := framework.DecodeTokenClaims(tok.Token)
		Expect(err).NotTo(HaveOccurred())
		Expect(claims).NotTo(BeEmpty())
		Expect(tok.ExpiresAt).To(BeNumerically(">", tok.IssuedAt))

		framework.CallError(Default, Client, http.MethodPost,
			"/api/v1/agents/"+framework.E2EAgentPrefix+"absent00/token?project="+proj, framework.TokenRequest{ExpiresIn: "1h"}, http.StatusNotFound)
	})

	It("shows where the agent runs and what the gateway provisions for it", func() {
		deps := framework.CallObj(Default, Client, http.MethodGet, gw("/deployments"), nil, http.StatusOK)
		Expect(deps).To(HaveKey(Cfg.DefaultEnv))
		conf := framework.CallObj(Default, Client, http.MethodGet, gw("/configurations"), nil, http.StatusOK)
		Expect(conf["agentName"]).To(Equal(agentName))
	})

	It("refuses gateway settings and keys until the agent is routed through the gateway", func() {
		framework.CallError(Default, Client, http.MethodPut, gw("/deploy-settings"),
			framework.Obj{"enableApiKeySecurity": true}, http.StatusBadRequest)
		framework.CallError(Default, Client, http.MethodPost, gw("/api-keys"), framework.Obj{"displayName": "k"}, http.StatusNotFound)
		framework.CallError(Default, Client, http.MethodGet, gw("/api-keys"), nil, http.StatusNotFound)
		framework.CallError(Default, Client, http.MethodPost, gw("/api-keys/test"), framework.Obj{}, http.StatusNotFound)

		// The card source is fetched server-side, so a private address is refused.
		framework.CallError(Default, Client, http.MethodPut, gw("/agent-card/source"),
			framework.Obj{"url": "http://127.0.0.1:9/.well-known/agent-card.json"}, http.StatusBadRequest)
		status, _ := framework.Send(Default, Client, http.MethodPut, gw("/a2a-proxy"), framework.Obj{})
		Expect(status).To(BeNumerically(">=", 400), "an A2A proxy needs a card source")
		for _, call := range []struct{ method, path string }{
			{http.MethodPost, gw("/a2a-proxy/refresh")},
			{http.MethodPost, gw("/agent-card/refresh")},
			{http.MethodPut, gw("/api-keys/e2e-absent-key")},
			{http.MethodDelete, gw("/api-keys/e2e-absent-key")},
		} {
			status, body := framework.Send(Default, Client, call.method, call.path, framework.Obj{})
			Expect(status).To(BeNumerically(">=", 400), "%s %s: %s", call.method, call.path, body)
			Expect(status).To(BeNumerically("<", 500), "%s %s: %s", call.method, call.path, body)
		}
		// Reads say there is no card or proxy yet; removing them is idempotent.
		for _, p := range []string{gw("/agent-card"), gw("/a2a-proxy")} {
			status, body := framework.Send(Default, Client, http.MethodGet, p, nil)
			Expect(status).To(BeNumerically("<", 500), "GET %s: %s", p, body)
		}
		for _, p := range []string{gw("/agent-card/source"), gw("/a2a-proxy")} {
			status, body := framework.Send(Default, Client, http.MethodDelete, p, nil)
			Expect(status).To(BeNumerically("<", 500), "DELETE %s: %s", p, body)
		}

		// Nothing was set up by the refused calls.
		conf := framework.CallObj(Default, Client, http.MethodGet, gw("/configurations"), nil, http.StatusOK)
		Expect(conf["bindings"]).To(BeEmpty())
	})

	It("deletes the agent with amctl, after which the API can't find it", func() {
		Expect(cliagent.DeleteAgent(Default, H, H.Org(), proj, agentName).Deleted).To(BeTrue())
		framework.CallError(Default, Client, http.MethodGet, agentops.AgentPath(proj, agentName), nil, http.StatusNotFound)
		Expect(cliagent.GetAgentExpectError(Default, H, H.Org(), proj, agentName).Status).To(Equal(http.StatusNotFound))
	})
})

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
	var agentName, envUUID string
	proj := framework.E2ESharedProjectName
	core := func(f string) string { return framework.API(Cfg, "/projects/%s/agents/%s"+f, proj, agentName) }
	gw := func(f string, a ...any) string {
		return framework.GW(Cfg, "/projects/%s/agents/%s"+f, append([]any{proj, agentName}, a...)...)
	}

	BeforeAll(func() {
		agentName = framework.E2EAgentPrefix + uuid.NewString()[:8]
		for _, e := range framework.CallJSON[framework.EnvironmentListResponse](Default, Client, http.MethodGet, framework.API(Cfg, "/environments"), nil, http.StatusOK) {
			if e.Name == Cfg.DefaultEnv {
				envUUID = e.ID
			}
		}
		DeferCleanup(func() { agentops.DeleteAgentBestEffort(Client, Cfg.DefaultOrg, proj, agentName) })
	})

	It("registers the agent with amctl, which the API then reads", func() {
		res := cliagent.CreateExternalAgent(Default, H, H.Org(), proj, agentName, "CLI agent", "registered by amctl")
		Expect(res.Agent.Name).To(Equal(agentName))
		Expect(res.Agent.Provisioning.Type).To(Equal("external"))

		got := framework.CallJSON[framework.AgentResponse](Default, Client, http.MethodGet, core(""), nil, http.StatusOK)
		Expect(got.DisplayName).To(Equal("CLI agent"))
		list := framework.CallObj(Default, Client, http.MethodGet, framework.API(Cfg, "/projects/%s/agents", proj), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(list, "agents"), "name")).To(ContainElement(agentName))

		framework.CallError(Default, Client, http.MethodPost, framework.API(Cfg, "/projects/%s/agents", proj),
			framework.NewExternalAgentRequest(agentName, "duplicate"), http.StatusConflict)
	})

	It("edits the agent through the API, which amctl then shows", func() {
		framework.Call(Default, Client, http.MethodPut, core(""),
			framework.Obj{"displayName": "Edited agent", "description": "edited"}, http.StatusOK)
		Expect(cliagent.GetAgent(Default, H, H.Org(), proj, agentName).DisplayName).To(Equal("Edited agent"))
		Expect(cliagent.ListAgents(Default, H, H.Org(), proj).Names()).To(ContainElement(agentName))
	})

	It("mints a signed token naming the agent", func() {
		tok := agentops.GenerateAgentToken(Default, Client, Cfg.DefaultOrg, proj, agentName, Cfg.DefaultEnv, "1h")
		Expect(strings.Count(tok.Token, ".")).To(Equal(2))
		claims, err := framework.DecodeTokenClaims(tok.Token)
		Expect(err).NotTo(HaveOccurred())
		Expect(claims).NotTo(BeEmpty())
		Expect(tok.ExpiresAt).To(BeNumerically(">", tok.IssuedAt))

		framework.CallError(Default, Client, http.MethodPost, core("/token?environment=no-such-env"), framework.TokenRequest{ExpiresIn: "1h"}, http.StatusNotFound)
	})

	It("shows where the agent runs and what the gateway provisions for it", func() {
		deps := framework.CallObj(Default, Client, http.MethodGet, gw("/deployments"), nil, http.StatusOK)
		Expect(deps).To(HaveKey(Cfg.DefaultEnv))
		conf := framework.CallObj(Default, Client, http.MethodGet, gw("/configurations?environment=%s", Cfg.DefaultEnv), nil, http.StatusOK)
		Expect(conf["agentName"]).To(Equal(agentName))
	})

	It("refuses gateway settings and keys until the agent is routed through the gateway", func() {
		framework.CallError(Default, Client, http.MethodPut, gw("/deploy-settings"),
			framework.Obj{"environmentName": Cfg.DefaultEnv, "enableApiKeySecurity": true}, http.StatusBadRequest)
		framework.CallError(Default, Client, http.MethodPost, gw("/environments/%s/api-keys", envUUID), framework.Obj{"displayName": "k"}, http.StatusNotFound)
		framework.CallError(Default, Client, http.MethodGet, gw("/environments/%s/api-keys", envUUID), nil, http.StatusNotFound)
		framework.CallError(Default, Client, http.MethodPost, gw("/environments/%s/api-keys/test", envUUID), framework.Obj{}, http.StatusNotFound)

		// The card source is fetched server-side, so a private address is refused.
		framework.CallError(Default, Client, http.MethodPut, gw("/environments/%s/agent-card/source", Cfg.DefaultEnv),
			framework.Obj{"url": "http://127.0.0.1:9/.well-known/agent-card.json"}, http.StatusBadRequest)
		status, _ := framework.Send(Default, Client, http.MethodPut, gw("/environments/%s/a2a-proxy", Cfg.DefaultEnv), framework.Obj{})
		Expect(status).To(BeNumerically(">=", 400), "an A2A proxy needs a card source")
		for _, call := range []struct{ method, path string }{
			{http.MethodPost, gw("/environments/%s/a2a-proxy/refresh", Cfg.DefaultEnv)},
			{http.MethodPost, gw("/environments/%s/agent-card/refresh", Cfg.DefaultEnv)},
			{http.MethodPut, gw("/environments/%s/api-keys/e2e-absent-key", envUUID)},
			{http.MethodDelete, gw("/environments/%s/api-keys/e2e-absent-key", envUUID)},
		} {
			status, body := framework.Send(Default, Client, call.method, call.path, framework.Obj{})
			Expect(status).To(BeNumerically(">=", 400), "%s %s: %s", call.method, call.path, body)
			Expect(status).To(BeNumerically("<", 500), "%s %s: %s", call.method, call.path, body)
		}
		// Reads say there is no card or proxy yet; removing them is idempotent.
		for _, p := range []string{gw("/environments/%s/agent-card", Cfg.DefaultEnv), gw("/environments/%s/a2a-proxy", Cfg.DefaultEnv)} {
			status, body := framework.Send(Default, Client, http.MethodGet, p, nil)
			Expect(status).To(BeNumerically("<", 500), "GET %s: %s", p, body)
		}
		for _, p := range []string{gw("/environments/%s/agent-card/source", Cfg.DefaultEnv), gw("/environments/%s/a2a-proxy", Cfg.DefaultEnv)} {
			status, body := framework.Send(Default, Client, http.MethodDelete, p, nil)
			Expect(status).To(BeNumerically("<", 500), "DELETE %s: %s", p, body)
		}

		// Nothing was set up by the refused calls.
		conf := framework.CallObj(Default, Client, http.MethodGet, gw("/configurations?environment=%s", Cfg.DefaultEnv), nil, http.StatusOK)
		Expect(conf["bindings"]).To(BeEmpty())
	})

	It("deletes the agent with amctl, after which the API can't find it", func() {
		Expect(cliagent.DeleteAgent(Default, H, H.Org(), proj, agentName).Deleted).To(BeTrue())
		framework.CallError(Default, Client, http.MethodGet, core(""), nil, http.StatusNotFound)
		Expect(cliagent.GetAgentExpectError(Default, H, H.Org(), proj, agentName).Status).To(Equal(http.StatusNotFound))
	})
})

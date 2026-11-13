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

// Validates the amctl agent llm lifecycle end to end against the CLI-owned
// platform agent: set (create→POST), get, list, set (update→GET+PUT), unset
// (whole-config DELETE), and the post-unset not-found envelope. Asserts JSON
// envelopes and exit codes only.
//
// Platform-hosted agents can only bind LLM providers deployed to a managed
// gateway in the environment, so BeforeAll deploys the provider to an active
// gateway in DefaultEnv. Like the mcp spec, this makes the suite depend on a
// running AI gateway there.

package cliagentplatformtests

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	cliagent "github.com/wso2/agent-manager/test/e2e/operations/cli/agent"
	cligateway "github.com/wso2/agent-manager/test/e2e/operations/cli/gateway"
	clillmprovider "github.com/wso2/agent-manager/test/e2e/operations/cli/llmprovider"
)

var _ = Describe("amctl agent llm (CLI-owned agent)", Label("cli", "agent", "llm"), Ordered, Serial, func() {
	const (
		// Distinct from the mcp spec: config names are unique per agent across types and both specs share this agent.
		configName = "llm-primary"
		urlEnv     = "E2E_LLM_URL"
		apiKeyEnv  = "E2E_LLM_KEY"
	)
	var providerID string

	BeforeAll(func() {
		gatewayName := cligateway.WaitForActiveEgressGateway(H, H.Org(), cfg.DefaultEnv, 3*time.Minute)
		providerID = framework.E2ELLMProviderPrefix + uuid.New().String()[:8]
		clillmprovider.CreateLLMProvider(Default, H, H.Org(), providerID, "CLI E2E LLM Provider", "openai", gatewayName)

		// LIFO cleanup: unset the agent config first, then delete the provider.
		DeferCleanup(func() {
			_ = H.Run("llm-provider", "delete", providerID, "--org", H.Org(), "--yes", "--json")
		})
		DeferCleanup(func() {
			_ = H.Run("agent", "llm", "unset", owned.AgentName, "--name", configName,
				"--org", H.Org(), "--project", owned.ProjectName, "--yes", "--json")
		})
		GinkgoWriter.Printf("CLI agent llm: agent=%s provider=%s\n", owned.AgentName, providerID)
	})

	It("sets a new llm config (create → POST)", func() {
		c := cliagent.SetAgentLLM(Default, H, H.Org(), owned.ProjectName, owned.AgentName, cliagent.SetAgentLLMParams{
			Name:        configName,
			Env:         cfg.DefaultEnv,
			Provider:    providerID,
			URLEnv:      urlEnv,
			APIKeyEnv:   apiKeyEnv,
			Description: "created by agent llm e2e",
		})
		Expect(c.Name).To(Equal(configName))
		Expect(c.Type).To(Equal("llm"))
		Expect(c.EnvMappings).To(HaveKey(cfg.DefaultEnv))
		// The env mapping must reference the provider we set.
		Expect(c.EnvMappings[cfg.DefaultEnv].Configuration).NotTo(BeNil())
		Expect(c.EnvMappings[cfg.DefaultEnv].Configuration.ProviderName).To(Equal(providerID))
	})

	It("gets the llm config", func() {
		c := cliagent.GetAgentLLM(Default, H, H.Org(), owned.ProjectName, owned.AgentName, configName)
		Expect(c.Name).To(Equal(configName))
		Expect(c.Description).To(Equal("created by agent llm e2e"))
		Expect(c.EnvMappings).To(HaveKey(cfg.DefaultEnv))
	})

	It("lists llm configs (type=llm only)", func() {
		list := cliagent.ListAgentLLM(Default, H, H.Org(), owned.ProjectName, owned.AgentName)
		Expect(list.Names()).To(ContainElement(configName))
		for _, c := range list.Configs {
			Expect(c.Type).To(Equal("llm"))
		}
	})

	It("updates the llm config (existing → GET+PUT)", func() {
		c := cliagent.SetAgentLLM(Default, H, H.Org(), owned.ProjectName, owned.AgentName, cliagent.SetAgentLLMParams{
			Name:        configName,
			Env:         cfg.DefaultEnv,
			Provider:    providerID,
			Description: "updated by agent llm e2e",
		})
		Expect(c.Name).To(Equal(configName))
		Expect(c.Description).To(Equal("updated by agent llm e2e"))
	})

	It("unsets the whole llm config (DELETE)", func() {
		res := cliagent.UnsetAgentLLM(Default, H, H.Org(), owned.ProjectName, owned.AgentName, configName)
		Expect(res.Name).To(Equal(configName))
		Expect(res.Deleted).To(BeTrue())
	})

	It("reports the llm config gone after unset", func() {
		Eventually(func(g Gomega) {
			cliagent.GetAgentLLMExpectError(g, H, H.Org(), owned.ProjectName, owned.AgentName, configName)
		}).WithTimeout(30 * time.Second).WithPolling(3 * time.Second).Should(Succeed())
	})
})

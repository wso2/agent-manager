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

// Validates external agent creation and API token generation.

package agent

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
)

var _ = Describe("External agent:", Label("agent", "external-agent"), Ordered, func() {
	var (
		agentName string
		createReq framework.CreateAgentRequest
	)

	BeforeAll(func() {
		// The shared project is created once in the suite's SynchronizedBeforeSuite;
		// this suite only registers its own agent there.
		suffix := uuid.New().String()[:8]
		agentName = "e2e-test-agent-" + suffix
		createReq = framework.NewExternalAgentRequest(agentName, "Externa agent registered by the external-agent lifecycle e2e tests")
	})

	// This agent is single-use; delete it so it doesn't accumulate.
	AfterAll(func() {
		agentops.DeleteAgentBestEffort(Client, Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName)
	})

	It("registers an external agent that points at an already-hosted agent", func() {
		By("Creating external agent in shared project")
		ag := agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: framework.E2ESharedProjectName,
			Request:     createReq,
		})
		Expect(ag.Metadata.Name).To(Equal(agentName))
		GinkgoWriter.Printf("Agent: %s (type: %s/%s)\n", agentName, ag.Spec.AgentType.Type, ag.Spec.AgentType.SubType)
	})

	It("issues an api token for trace publishing for an external agent", func() {
		By("Generating agent token")
		tokenResp := agentops.GenerateAgentToken(Default, Client, Cfg.DefaultOrg, framework.E2ESharedProjectName, agentName, Cfg.DefaultEnv, "1h")
		Expect(tokenResp.Token).NotTo(BeEmpty(), "expected non-empty agent token")
		GinkgoWriter.Printf("Token type: %s\n", tokenResp.TokenType)
	})
})

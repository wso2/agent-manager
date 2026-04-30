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

package tests

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/project"
)

var _ = Describe("External Agent Lifecycle", Ordered, func() {
	var (
		projName  string
		agentName string

		createProjReq framework.CreateProjectRequest
		createReq     framework.CreateAgentRequest
	)

	BeforeAll(func() {
		suffix := uuid.New().String()[:8]
		projName = e2eProjectPrefix + suffix
		agentName = "e2e-external-" + suffix

		loadTestData("external-agent/create_project.json", &createProjReq)
		createProjReq.Name = projName

		loadTestData("external-agent/create_agent.json", &createReq)
		createReq.Name = agentName
	})

	It("should create a project", func() {
		By("Creating e2e project")
		proj := project.CreateProject(Default, Client, &project.CreateProjectParams{
			OrgName: Cfg.DefaultOrg,
			Request: createProjReq,
		})
		framework.ExpectJSONMatch(Default, "external-agent/expected_create_project.json", proj)
		GinkgoWriter.Printf("Project: %s\n", projName)
	})

	It("should create an external agent", func() {
		By("Creating external agent")
		ag := agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: projName,
			Request:     createReq,
		})
		Expect(ag.Name).To(Equal(agentName))
		framework.ExpectJSONMatch(Default, "external-agent/expected_create_agent.json", ag)
		GinkgoWriter.Printf("Agent: %s (type: %s/%s)\n", agentName, ag.AgentType.Type, ag.AgentType.SubType)
	})

	It("should generate a token for the external agent", func() {
		By("Generating agent token")
		tokenResp := agentops.GenerateAgentToken(Default, Client, Cfg.DefaultOrg, projName, agentName, "1h")
		Expect(tokenResp.Token).NotTo(BeEmpty(), "expected non-empty agent token")
		GinkgoWriter.Printf("Token type: %s\n", tokenResp.TokenType)
	})
})

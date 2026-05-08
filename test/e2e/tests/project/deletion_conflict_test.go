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

package projecttests

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	"github.com/wso2/agent-manager/test/e2e/operations/project"
)

var _ = Describe("Project Deletion Conflict", Label("project", "deletion-conflict"), Ordered, func() {
	var (
		projName  string
		agentName string

		createProjReq framework.CreateProjectRequest
		createReq     framework.CreateAgentRequest
	)

	BeforeAll(func() {
		suffix := uuid.New().String()[:8]
		projName = framework.E2EProjectPrefix + suffix
		agentName = "e2e-conflict-" + suffix

		framework.LoadTestData(TestDataDir, "project-deletion-conflict/create_project.json", &createProjReq)
		createProjReq.Name = projName

		framework.LoadTestData(TestDataDir, "project-deletion-conflict/create_agent.json", &createReq)
		createReq.Name = agentName
	})

	It("should create a project", func() {
		By("Creating e2e project")
		proj := project.CreateProject(Default, Client, &project.CreateProjectParams{
			OrgName: Cfg.DefaultOrg,
			Request: createProjReq,
		})
		framework.ExpectJSONMatch(Default, "project-deletion-conflict/expected_create_project.json", proj)
		GinkgoWriter.Printf("Project: %s\n", projName)
	})

	It("should create an external agent in the project", func() {
		By("Creating external agent")
		ag := agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName:     Cfg.DefaultOrg,
			ProjectName: projName,
			Request:     createReq,
		})
		Expect(ag.Name).To(Equal(agentName))
		framework.ExpectJSONMatch(Default, "project-deletion-conflict/expected_create_agent.json", ag)
		GinkgoWriter.Printf("Agent: %s\n", agentName)
	})

	It("should fail to delete project with active agent (409 Conflict)", func() {
		By("Attempting to delete project with agent")
		errResp := project.DeleteProjectExpectConflict(Default, Client, Cfg.DefaultOrg, projName)
		GinkgoWriter.Printf("Conflict error: %s\n", errResp.Message)
	})

	It("should delete the agent", func() {
		By("Deleting the agent")
		agentops.DeleteAgent(Default, Client, Cfg.DefaultOrg, projName, agentName)
		GinkgoWriter.Printf("Deleted agent: %s\n", agentName)
	})

	It("should successfully delete the empty project", func() {
		By("Deleting the project after agent removal (with retry for async cleanup)")
		Eventually(func(g Gomega) {
			project.DeleteProject(g, Client, Cfg.DefaultOrg, projName)
		}).WithTimeout(30 * time.Second).WithPolling(3 * time.Second).Should(Succeed())
		GinkgoWriter.Printf("Deleted project: %s\n", projName)
	})
})

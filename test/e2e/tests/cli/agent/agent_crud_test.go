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

// Validates the amctl CLI agent lifecycle end to end: create (external and
// internal), get, list, status, and delete via the real binary, asserting
// JSON envelopes and exit codes.

package cliagenttests

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	cliagent "github.com/wso2/agent-manager/test/e2e/operations/cli/agent"
	cliproject "github.com/wso2/agent-manager/test/e2e/operations/cli/project"
)

var _ = Describe("amctl agent CRUD (build-free)", Label("cli", "agent"), Ordered, func() {
	const (
		displayName = "CLI E2E Agent"
		description = "Created by the amctl agent e2e CRUD spec"
	)
	var (
		projName     string
		agentName    string
		platformName string
	)

	BeforeAll(func() {
		projName = framework.E2EProjectPrefix + uuid.New().String()[:8]
		agentName = framework.E2EAgentPrefix + uuid.New().String()[:8]
		platformName = framework.E2EAgentPrefix + uuid.New().String()[:8]
		// LIFO cleanup: agents first, then the project. Best-effort; the stale sweep backstops.
		DeferCleanup(func() {
			_ = H.Run("agent", "delete", platformName, "--org", H.Org(), "--project", projName, "--yes", "--json")
			_ = H.Run("agent", "delete", agentName, "--org", H.Org(), "--project", projName, "--yes", "--json")
			_ = H.Run("project", "delete", projName, "--org", H.Org(), "--yes", "--json")
		})
		cliproject.CreateProject(Default, H, H.Org(), projName, "CLI E2E Agent Project", "Project for the amctl agent e2e CRUD spec")
		GinkgoWriter.Printf("CLI agent project=%s external=%s platform=%s\n", projName, agentName, platformName)
	})

	It("prints an agent manifest template", func() {
		res := H.Run("agent", "create", "--template")
		Expect(res.ExitCode).To(Equal(0), res.Combined())
		Expect(string(res.Stdout)).To(ContainSubstring("kind: Agent"))
		Expect(string(res.Stdout)).To(ContainSubstring("provisioning:"))
	})

	It("creates an external agent", func() {
		r := cliagent.CreateExternalAgent(Default, H, H.Org(), projName, agentName, displayName, description)
		Expect(r.Agent.Name).To(Equal(agentName))
		Expect(r.Agent.AgentType.Type).To(Equal("external-agent-api"))
		Expect(r.Token).NotTo(BeEmpty())
	})

	It("gets the external agent", func() {
		a := cliagent.GetAgent(Default, H, H.Org(), projName, agentName)
		Expect(a.Name).To(Equal(agentName))
		Expect(a.DisplayName).To(Equal(displayName))
		Expect(a.Description).To(Equal(description))
		Expect(a.ProjectName).To(Equal(projName))
	})

	It("lists the external agent", func() {
		list := cliagent.ListAgents(Default, H, H.Org(), projName)
		Expect(list.Names()).To(ContainElement(agentName))
	})

	It("shows status with no deployments", func() {
		st := cliagent.AgentStatus(Default, H, H.Org(), projName, agentName)
		Expect(st.Agent).To(Equal(agentName))
		// The default pipeline may contain several environments on an existing
		// installation. The undeployed agent must report that state in each one.
		Expect(st.Environments).NotTo(BeEmpty())
		for _, env := range st.Environments {
			Expect(env.Name).NotTo(BeEmpty())
			Expect(env.Status).To(Equal("not-deployed"))
		}
	})

	It("creates a platform (internal) agent, triggering build+deploy", func() {
		a := cliagent.CreateInternalAgent(Default, H, cliagent.InternalAgentParams{
			Org:             H.Org(),
			Project:         projName,
			Name:            platformName,
			DisplayName:     "CLI E2E Platform Agent",
			RepoURL:         "https://github.com/wso2/agent-manager",
			RepoBranch:      "main",
			RepoPath:        "/samples/it-helpdesk-agent",
			Language:        "python",
			LanguageVersion: "3.11",
			RunCommand:      "python main.py",
		})
		Expect(a.Name).To(Equal(platformName))
		Expect(a.AgentType.Type).To(Equal("agent-api"))
		Expect(a.AgentType.SubType).To(Equal("chat-api"))
		Expect(a.Provisioning.Type).To(Equal("internal"))
	})

	It("deletes the external agent", func() {
		r := cliagent.DeleteAgent(Default, H, H.Org(), projName, agentName)
		Expect(r.Name).To(Equal(agentName))
		Expect(r.Deleted).To(BeTrue())
	})

	It("reports the external agent gone after deletion", func() {
		Eventually(func(g Gomega) {
			cliagent.GetAgentExpectError(g, H, H.Org(), projName, agentName)
		}).WithTimeout(30 * time.Second).WithPolling(3 * time.Second).Should(Succeed())
	})
})

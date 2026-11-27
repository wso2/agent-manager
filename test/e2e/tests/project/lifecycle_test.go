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

// J2: a developer creates, renames and lists a project, sees the same state
// through amctl, is refused deleting it while it holds an agent (with nothing
// removed), and deletes it once empty.

package projecttests

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
	cliagent "github.com/wso2/agent-manager/test/e2e/operations/cli/agent"
	cliproject "github.com/wso2/agent-manager/test/e2e/operations/cli/project"
)

var _ = Describe("Project lifecycle", Label("project", "lifecycle"), Ordered, func() {
	var name, agentName string
	proj := func(f string, a ...any) string {
		return framework.Projects("/%s"+f, append([]any{name}, a...)...)
	}

	BeforeAll(func() {
		suffix := uuid.NewString()[:8]
		name = framework.E2EProjectPrefix + "life-" + suffix
		agentName = framework.E2EAgentPrefix + suffix
		DeferCleanup(func() {
			agentops.DeleteAgentBestEffort(Client, Cfg.DefaultOrg, name, agentName)
			framework.Send(Default, Client, http.MethodDelete, proj(""), nil)
		})
	})

	It("creates a project that amctl then lists and reads", func() {
		created := framework.CallJSON[framework.ProjectDocument](Default, Client, http.MethodPost, framework.Projects(""),
			framework.NewProjectDocument(name, "Lifecycle", "Project lifecycle journey"), http.StatusAccepted)
		Expect(created.Metadata.Name).To(Equal(name))

		Eventually(func(g Gomega) {
			g.Expect(cliproject.ProjectNames(cliproject.ListProjects(g, H, H.Org()))).To(ContainElement(name))
		}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		got := cliproject.GetProject(Default, H, H.Org(), name)
		Expect(got.Spec.DisplayName).To(Equal("Lifecycle"))
		Expect(got.Spec.Description).To(HaveValue(Equal("Project lifecycle journey")))
	})

	It("refuses a duplicate or malformed project", func() {
		framework.CallError(Default, Client, http.MethodPost, framework.Projects(""),
			framework.NewProjectDocument(name, "Again", ""), http.StatusConflict)
		framework.CallError(Default, Client, http.MethodPost, framework.Projects(""),
			framework.NewProjectDocument("Not A Valid Name!", "Bad", ""), http.StatusBadRequest)
		framework.CallError(Default, Client, http.MethodGet, framework.Projects("/%s", framework.E2EProjectPrefix+"absent-"+uuid.NewString()[:8]), nil, http.StatusNotFound)
	})

	It("renames the project", func() {
		framework.Call(Default, Client, http.MethodPut, proj(""),
			framework.NewProjectDocument(name, "Lifecycle renamed", "updated"), http.StatusOK)
		Expect(cliproject.GetProject(Default, H, H.Org(), name).Spec.DisplayName).To(Equal("Lifecycle renamed"))
		list := framework.CallJSON[framework.ProjectListResponse](Default, Client, http.MethodGet, framework.Projects("?limit=100"), nil, http.StatusOK)
		names := []string{}
		for _, p := range list.List {
			names = append(names, p.Metadata.Name)
		}
		Expect(names).To(ContainElement(name))
	})

	It("refuses to delete the project while it holds an agent, and removes nothing", func() {
		agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName: Cfg.DefaultOrg, ProjectName: name, Request: framework.NewExternalAgentRequest(agentName, "lifecycle"),
		})
		e := framework.CallError(Default, Client, http.MethodDelete, proj(""), nil, http.StatusConflict)
		Expect(e.Message).To(ContainSubstring("agent"))

		Expect(cliproject.GetProject(Default, H, H.Org(), name).Metadata.Name).To(Equal(name))
		Expect(cliagent.ListAgents(Default, H, H.Org(), name).Names()).To(ContainElement(agentName))
		framework.Call(Default, Client, http.MethodGet, agentops.AgentPath(name, agentName), nil, http.StatusOK)
		org := framework.CallJSON[framework.AgentListResponse](Default, Client, http.MethodGet, "/api/v1/agents?limit=100", nil, http.StatusOK)
		names := []string{}
		for _, a := range org.List {
			names = append(names, a.Metadata.Name)
		}
		Expect(names).To(ContainElement(agentName))
	})

	It("deletes the project once its agent is gone, which amctl then can't find", func() {
		agentops.DeleteAgent(Default, Client, Cfg.DefaultOrg, name, agentName)
		Eventually(func(g Gomega) {
			framework.Call(g, Client, http.MethodDelete, proj(""), nil, http.StatusNoContent)
		}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

		e := cliproject.GetProjectExpectError(Default, H, H.Org(), name)
		Expect(e.Status).To(Equal(http.StatusNotFound))
		framework.CallError(Default, Client, http.MethodGet, proj(""), nil, http.StatusNotFound)
	})
})

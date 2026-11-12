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

// Validates the amctl CLI project lifecycle end to end: create, get, list, and
// delete via the real binary, asserting JSON envelopes and exit codes.

package cliprojecttests

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	cliproject "github.com/wso2/agent-manager/test/e2e/operations/cli/project"
)

var _ = Describe("amctl project CRUD", Label("cli", "project"), Ordered, func() {
	const (
		displayName = "CLI E2E Project"
		description = "Created by the amctl e2e CRUD spec"
	)
	var projName string

	BeforeAll(func() {
		projName = framework.E2EProjectPrefix + uuid.New().String()[:8]
		// Best-effort teardown if a leg fails mid-spec; the stale sweep is the backstop.
		DeferCleanup(func() {
			_ = H.Run("project", "delete", projName, "--org", H.Org(), "--yes", "--json")
		})
		GinkgoWriter.Printf("CLI project: %s\n", projName)
	})

	It("creates a project", func() {
		p := cliproject.CreateProject(Default, H, H.Org(), projName, displayName, description)
		Expect(p.Name).To(Equal(projName))
		Expect(p.DisplayName).To(Equal(displayName))
		// Projects have no deployment pipeline: environments are internal.
		Expect(p.DeploymentPipeline).To(BeEmpty())
	})

	It("gets the created project", func() {
		p := cliproject.GetProject(Default, H, H.Org(), projName)
		Expect(p.Name).To(Equal(projName))
		Expect(p.DisplayName).To(Equal(displayName))
		Expect(p.Description).To(Equal(description))
	})

	It("lists the created project", func() {
		list := cliproject.ListProjects(Default, H, H.Org())
		Expect(list.Names()).To(ContainElement(projName))
	})

	It("deletes the project", func() {
		res := cliproject.DeleteProject(Default, H, H.Org(), projName)
		Expect(res.Name).To(Equal(projName))
		Expect(res.Deleted).To(BeTrue())
	})

	It("reports the project as gone after deletion", func() {
		Eventually(func(g Gomega) {
			cliproject.GetProjectExpectError(g, H, H.Org(), projName)
		}).WithTimeout(30 * time.Second).WithPolling(3 * time.Second).Should(Succeed())
	})
})

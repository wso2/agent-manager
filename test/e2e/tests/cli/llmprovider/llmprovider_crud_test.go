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

// Validates the amctl CLI llm-provider lifecycle end to end: create, list, and
// delete via the real binary, asserting JSON envelopes and exit codes. Creates a
// config-only provider (no gateway/upstream/api-key), so the suite is keyless.

package clillmprovidertests

import (
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	clillmprovider "github.com/wso2/agent-manager/test/e2e/operations/cli/llmprovider"
)

var _ = Describe("amctl llm-provider CRUD", Label("cli", "llm-provider"), Ordered, func() {
	const (
		displayName = "CLI E2E LLM Provider"
		template    = "openai"
	)
	var id string

	BeforeAll(func() {
		id = framework.E2ELLMProviderPrefix + uuid.New().String()[:8]
		// Best-effort teardown if a leg fails mid-spec; the stale sweep is the backstop.
		DeferCleanup(func() {
			_ = H.Run("llm-provider", "delete", id, "--org", H.Org(), "--yes", "--json")
		})
		GinkgoWriter.Printf("CLI llm-provider: %s\n", id)
	})

	It("creates a config-only provider", func() {
		p := clillmprovider.CreateLLMProvider(Default, H, H.Org(), id, displayName, template)
		Expect(p.Metadata.Name).To(Equal(id))
		Expect(p.Spec.DisplayName).To(Equal(displayName))
		Expect(p.Spec.Template).To(Equal(template))
	})

	It("lists the created provider", func() {
		list := clillmprovider.ListLLMProviders(Default, H, H.Org())
		Expect(list.Ids()).To(ContainElement(id))
	})

	It("deletes the provider", func() {
		res := clillmprovider.DeleteLLMProvider(Default, H, H.Org(), id)
		Expect(res.Name).To(Equal(id))
		Expect(res.Deleted).To(BeTrue())
	})

	It("reports the provider as gone after deletion", func() {
		Eventually(func(g Gomega) {
			list := clillmprovider.ListLLMProviders(g, H, H.Org())
			g.Expect(list.Ids()).NotTo(ContainElement(id))
		}).WithTimeout(30 * time.Second).WithPolling(3 * time.Second).Should(Succeed())
	})
})

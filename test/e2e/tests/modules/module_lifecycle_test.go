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

// J9: an operator disables the AI Gateway module on a running server. Its
// routes go away and the console's manifest drops it while the core keeps
// working; enabling it again brings the routes back with its data intact.
//
// Disabling a module affects every client of the server, so this suite runs
// its specs serially and alone; Ginkgo runs suite packages one at a time.

package modules

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	"github.com/wso2/agent-manager/test/e2e/operations/gateway"
	llmproviderop "github.com/wso2/agent-manager/test/e2e/operations/llmprovider"
)

const module = "ai-gateway"

func manifestModules(g Gomega) []string {
	m := framework.CallObj(g, Client, http.MethodGet, "/api/v1/ui-manifest", nil, http.StatusOK)
	return framework.Field(framework.Items(m, "modules"), "name")
}

func gatewaysStatus(g Gomega) int {
	status, _ := framework.Send(g, Client, http.MethodGet, framework.GW(Cfg, "/gateways"), nil)
	return status
}

var _ = Describe("Module lifecycle", Label("modules"), Serial, Ordered, func() {
	var (
		providerName string
		projectName  string
	)

	BeforeAll(func() {
		if framework.APIContainer() == "" {
			Skip("no local amp-api container to run the amp CLI in")
		}
		suffix := uuid.NewString()[:8]
		providerName = framework.E2ELLMProviderPrefix + "mod-" + suffix
		projectName = framework.E2EProjectPrefix + "mod-" + suffix

		// Whatever happens below, leave the module enabled for the other suites.
		DeferCleanup(func() {
			if out, err := framework.RunAMP("module", "enable", module); err != nil {
				Expect(out).To(ContainSubstring("is active"), "re-enabling %s", module)
			}
			Eventually(gatewaysStatus).WithTimeout(30 * time.Second).WithPolling(time.Second).Should(Equal(http.StatusOK))
			framework.Send(Default, Client, http.MethodDelete, "/m/ai-gateway/llm-providers/"+providerName, nil)
			framework.Send(Default, Client, http.MethodDelete, framework.API(Cfg, "/projects/%s", projectName), nil)
		})
	})

	It("starts with the module active and its data in place", func() {
		out, err := framework.RunAMP("module", "list")
		Expect(err).NotTo(HaveOccurred(), out)
		Expect(out).To(MatchRegexp(module + `\s+\S+\s+active`))
		Expect(manifestModules(Default)).To(ContainElement(module))

		gwID := gateway.WaitForActiveGatewayForEnv(Client, Cfg.DefaultOrg, Cfg.DefaultEnv, 3*time.Minute)
		url, header, value := "https://api.openai.com/v1", "Authorization", "Bearer sk-e2e-placeholder"
		llmproviderop.CreateLLMProvider(Default, Client, providerName, framework.LLMProviderSpec{
			DisplayName: providerName, Version: "v1.0", Context: "/" + providerName, Template: "openai",
			Upstream: framework.UpstreamConfig{Main: &framework.UpstreamEndpoint{
				URL: &url, Auth: &framework.UpstreamAuth{Type: "api-key", Header: &header, Value: &value},
			}},
			Gateways: []string{gwID},
		})
	})

	It("removes the module's routes and its manifest entry when disabled", func() {
		out, err := framework.RunAMP("module", "disable", module)
		Expect(err).NotTo(HaveOccurred(), out)

		Eventually(gatewaysStatus).WithTimeout(30 * time.Second).WithPolling(time.Second).Should(Equal(http.StatusNotFound))
		framework.Call(Default, Client, http.MethodGet, "/m/ai-gateway/llm-providers/"+providerName, nil, http.StatusNotFound)
		Expect(manifestModules(Default)).To(ConsistOf("core"))

		out, err = framework.RunAMP("module", "list")
		Expect(err).NotTo(HaveOccurred(), out)
		Expect(out).NotTo(MatchRegexp(module + `\s+\S+\s+active`))
	})

	It("keeps the core working while the module is off", func() {
		framework.CallJSON[framework.ProjectResponse](Default, Client, http.MethodPost, framework.API(Cfg, "/projects"),
			framework.NewCreateProjectRequest(projectName, "Module lifecycle", "", "default"), http.StatusAccepted)
		Eventually(func(g Gomega) {
			framework.Call(g, Client, http.MethodGet, framework.API(Cfg, "/projects/%s", projectName), nil, http.StatusOK)
		}).WithTimeout(30 * time.Second).WithPolling(time.Second).Should(Succeed())
		framework.Call(Default, Client, http.MethodGet, framework.API(Cfg, "/environments"), nil, http.StatusOK)
	})

	It("brings the routes back with the module's data intact when enabled", func() {
		out, err := framework.RunAMP("module", "enable", module)
		Expect(err).NotTo(HaveOccurred(), out)

		Eventually(gatewaysStatus).WithTimeout(30 * time.Second).WithPolling(time.Second).Should(Equal(http.StatusOK))
		Expect(manifestModules(Default)).To(ContainElement(module))
		prov := llmproviderop.GetLLMProvider(Default, Client, providerName)
		Expect(prov.Metadata.Name).To(Equal(providerName))
		// The gateway reconnects and is usable again.
		gateway.WaitForActiveGatewayForEnv(Client, Cfg.DefaultOrg, Cfg.DefaultEnv, 3*time.Minute)
	})
})

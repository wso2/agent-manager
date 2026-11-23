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

// J1: a machine client signs in against ThunderID and finds the organization
// the first boot created, its default environment and the identity provider
// behind it, and a UI manifest that lists the modules the server runs.

package onboarding

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

var _ = Describe("Sign-in and first-org bootstrap", Label("onboarding"), func() {
	It("refuses the API without a token, and with a token it didn't issue", func() {
		resp, err := Client.GetUnauthenticated("/api/v1/orgs")
		Expect(err).NotTo(HaveOccurred())
		framework.ExpectUnauthorized(Default, resp, "GET /orgs without a token")
		resp.Body.Close()

		forged, err := framework.HS256Token(framework.PlausibleClaims(Cfg, nil), "not-the-signing-key")
		Expect(err).NotTo(HaveOccurred())
		resp, err = framework.NewAMPClientWithToken(Cfg, forged).Get("/api/v1/orgs")
		Expect(err).NotTo(HaveOccurred())
		framework.ExpectUnauthorized(Default, resp, "GET /orgs with a forged token")
		resp.Body.Close()
	})

	It("lists the bootstrapped organization to the signed-in client", func() {
		orgs := framework.CallJSON[framework.OrganizationListResponse](Default, Client, http.MethodGet, "/api/v1/orgs", nil, http.StatusOK)
		Expect(orgs.Total).To(BeNumerically(">=", 1))
		names := make([]string, 0, len(orgs.Organizations))
		for _, o := range orgs.Organizations {
			names = append(names, o.Name)
		}
		Expect(names).To(ContainElement(Cfg.DefaultOrg))

		org := framework.CallObj(Default, Client, http.MethodGet, "/api/v1/orgs/"+Cfg.DefaultOrg, nil, http.StatusOK)
		Expect(org["name"]).To(Equal(Cfg.DefaultOrg))
		Expect(org["createdAt"]).NotTo(BeEmpty())
	})

	It("has a default environment, readable by list and by id", func() {
		envs := framework.CallJSON[framework.EnvironmentListResponse](Default, Client, http.MethodGet, framework.API(Cfg, "/environments"), nil, http.StatusOK)
		var def *framework.EnvironmentResponse
		for i := range envs {
			if envs[i].Name == Cfg.DefaultEnv {
				def = &envs[i]
			}
		}
		Expect(def).NotTo(BeNil(), "no %q environment among %d", Cfg.DefaultEnv, len(envs))
		Expect(def.ID).NotTo(BeEmpty())

		byID := framework.CallJSON[framework.EnvironmentResponse](Default, Client, http.MethodGet, framework.API(Cfg, "/environments/%s", def.ID), nil, http.StatusOK)
		Expect(byID.Name).To(Equal(Cfg.DefaultEnv))
		Expect(byID.ID).To(Equal(def.ID))
	})

	It("names the environment's identity provider, which issues tokens", func() {
		instances := framework.CallObj(Default, Client, http.MethodGet, framework.API(Cfg, "/thunder-instances"), nil, http.StatusOK)
		items := framework.Items(instances, "thunderInstances")
		Expect(framework.Field(items, "envName")).To(ContainElement(Cfg.DefaultEnv))
		for _, it := range items {
			if framework.Str(it, "envName") != Cfg.DefaultEnv {
				continue
			}
			Expect(framework.Str(it, "tokenUrl")).To(HavePrefix("http"))
			Expect(framework.Str(it, "jwksUrl")).To(HavePrefix("http"))
		}
	})

	It("lists every installed module, with its pages, in the UI manifest", func() {
		manifest := framework.CallObj(Default, Client, http.MethodGet, "/api/v1/ui-manifest", nil, http.StatusOK)
		modules := framework.Items(manifest, "modules")
		Expect(framework.Field(modules, "name")).To(ContainElements("core", "ai-gateway"))
		for _, m := range modules {
			if framework.Str(m, "name") == "ai-gateway" {
				Expect(m["pages"]).To(ContainElement("ai-gateway/llm-providers"))
				Expect(framework.Str(m, "version")).NotTo(BeEmpty())
			}
		}
		// The top-level page list is the union of the modules' pages.
		Expect(manifest["pages"]).To(ContainElements("ai-gateway/gateways", "acp-core/external-agent"))

		resp, err := Client.GetUnauthenticated("/api/v1/ui-manifest")
		Expect(err).NotTo(HaveOccurred())
		framework.ExpectUnauthorized(Default, resp, "GET /ui-manifest without a token")
		resp.Body.Close()
	})

	It("serves the console's runtime configuration", func() {
		cfg := framework.CallObj(Default, Client, http.MethodGet, "/api/v1/config", nil, http.StatusOK)
		Expect(cfg).To(HaveKeyWithValue("eventsEnabled", BeTrue()))
	})

	It("turns a display name into a free resource name", func() {
		gen := framework.CallObj(Default, Client, http.MethodPost, framework.API(Cfg, "/utils/generate-name"),
			framework.Obj{"displayName": "My E2E Agent", "resourceType": "agent", "projectName": Cfg.DefaultProject}, http.StatusOK)
		Expect(gen["name"]).To(Equal("my-e2e-agent"))

		// The default project exists, so its name is taken and a suffix is added.
		gen = framework.CallObj(Default, Client, http.MethodPost, framework.API(Cfg, "/utils/generate-name"),
			framework.Obj{"displayName": Cfg.DefaultProject, "resourceType": "project"}, http.StatusOK)
		Expect(gen["name"]).To(HavePrefix(Cfg.DefaultProject + "-"))

		framework.CallError(Default, Client, http.MethodPost, framework.API(Cfg, "/utils/generate-name"),
			framework.Obj{"displayName": "x", "resourceType": "widget"}, http.StatusBadRequest)
	})
})

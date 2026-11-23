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

// J6: a platform engineer registers gateways, issues the token a gateway
// connects with, maps the gateway to an environment, and removes it again:
// the token stops working, and a gateway still serving a deployment is kept.

package gatewaytests

import (
	"crypto/tls"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	llmproviderop "github.com/wso2/agent-manager/test/e2e/operations/llmprovider"
)

// internalStatus calls the gateway-facing internal API with a gateway token,
// the channel a gateway itself uses. It returns 0 when that API isn't
// reachable from here (AMP_INTERNAL_BASE_URL, default https://localhost:9243).
func internalStatus(token string) int {
	base := os.Getenv("AMP_INTERNAL_BASE_URL")
	if base == "" {
		base = "https://localhost:9243"
	}
	req, err := http.NewRequest(http.MethodGet, base+"/api/internal/v1/subscription-plans", nil)
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("api-key", token)
	c := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // the local stack's self-signed internal listener
	}}
	resp, err := c.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func envID() string {
	for _, e := range framework.CallJSON[framework.EnvironmentListResponse](Default, Client, http.MethodGet, framework.API(Cfg, "/environments"), nil, http.StatusOK) {
		if e.Name == Cfg.DefaultEnv {
			return e.ID
		}
	}
	Fail("no default environment")
	return ""
}

var _ = Describe("Self-hosted gateway", Label("gateway"), Ordered, func() {
	var gwID, handle, token, tokenID string
	sh := func(f string, a ...any) string { return framework.GW(Cfg, "/self-hosted-gateways"+f, a...) }

	BeforeAll(func() {
		handle = "e2e-test-shgw-" + uuid.NewString()[:8]
		DeferCleanup(func() {
			if gwID != "" {
				framework.Send(Default, Client, http.MethodDelete, sh("/%s", gwID), nil)
			}
		})
	})

	It("registers a gateway in the environment", func() {
		g := framework.CallObj(Default, Client, http.MethodPost, sh(""), framework.Obj{
			"displayName": "E2E self-hosted", "handle": handle, "environment": Cfg.DefaultEnv,
			"endpoints": []string{"http://" + handle + ".example.test:19080"},
		}, http.StatusCreated)
		gwID = framework.Str(g, "uuid")
		Expect(g["hosting"]).To(Equal("SELF_HOSTED"))
		Expect(framework.Field(framework.Items(g, "environments"), "name")).To(ConsistOf(Cfg.DefaultEnv))
		// It hasn't connected yet.
		Expect(g["status"]).To(Equal("INACTIVE"))

		got := framework.CallObj(Default, Client, http.MethodGet, sh("/%s", gwID), nil, http.StatusOK)
		Expect(got["name"]).To(Equal(handle))
		list := framework.CallObj(Default, Client, http.MethodGet, sh(""), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(list, "gateways"), "uuid")).To(ContainElement(gwID))
		envGWs := framework.CallObj(Default, Client, http.MethodGet, framework.GW(Cfg, "/environments/%s/gateways", envID()), nil, http.StatusOK)
		Expect(envGWs).NotTo(BeEmpty())

		framework.CallError(Default, Client, http.MethodPost, sh(""), framework.Obj{
			"displayName": "dup", "handle": handle, "environment": Cfg.DefaultEnv, "endpoints": []string{"http://x.example.test"},
		}, http.StatusConflict)
		framework.CallError(Default, Client, http.MethodPost, sh(""), framework.Obj{
			"displayName": "no env", "handle": handle + "-x", "environment": "no-such-env", "endpoints": []string{"http://x.example.test"},
		}, http.StatusNotFound)
	})

	It("updates its display name", func() {
		framework.CallObj(Default, Client, http.MethodPut, sh("/%s", gwID), framework.Obj{"displayName": "Renamed"}, http.StatusOK)
		Expect(framework.CallObj(Default, Client, http.MethodGet, sh("/%s", gwID), nil, http.StatusOK)["displayName"]).To(Equal("Renamed"))
	})

	It("issues a token the gateway can call the platform with, until it's revoked", func() {
		t := framework.CallObj(Default, Client, http.MethodPost, sh("/%s/tokens", gwID), framework.Obj{}, http.StatusCreated)
		token, tokenID = framework.Str(t, "token"), framework.Str(t, "tokenId")
		Expect(token).NotTo(BeEmpty())
		tokens := framework.CallObj(Default, Client, http.MethodGet, sh("/%s/tokens", gwID), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(tokens, "list"), "id")).To(ContainElement(tokenID))
		Expect(tokens).NotTo(HaveKey("token"), "a token's value is shown only once")

		status := internalStatus(token)
		if status == 0 {
			Skip("the internal API isn't reachable from here; set AMP_INTERNAL_BASE_URL")
		}
		Expect(status).To(Equal(http.StatusOK))
		Expect(internalStatus("not-a-token")).To(Equal(http.StatusUnauthorized))

		framework.Call(Default, Client, http.MethodDelete, sh("/%s/tokens/%s", gwID, tokenID), nil, http.StatusNoContent)
		Eventually(func() int { return internalStatus(token) }).WithPolling(time.Second).Should(Equal(http.StatusUnauthorized))
	})

	It("removes the gateway, which takes its tokens with it", func() {
		t := framework.CallObj(Default, Client, http.MethodPost, sh("/%s/tokens", gwID), framework.Obj{}, http.StatusCreated)
		framework.Call(Default, Client, http.MethodDelete, sh("/%s", gwID), nil, http.StatusNoContent)
		framework.CallError(Default, Client, http.MethodGet, sh("/%s", gwID), nil, http.StatusNotFound)
		if status := internalStatus(framework.Str(t, "token")); status != 0 {
			Expect(status).To(Equal(http.StatusUnauthorized))
		}
		gwID = ""
	})
})

var _ = Describe("Platform gateway", Label("gateway"), Ordered, func() {
	var gwID, name, providerName, envUUID string
	gw := func(f string, a ...any) string { return framework.GW(Cfg, "/gateways"+f, a...) }

	BeforeAll(func() {
		suffix := uuid.NewString()[:8]
		name = "e2e-test-gw-" + suffix
		providerName = framework.E2ELLMProviderPrefix + "gw-" + suffix
		envUUID = envID()
		DeferCleanup(func() {
			framework.Send(Default, Client, http.MethodDelete, "/m/ai-gateway/llm-providers/"+providerName, nil)
			if gwID != "" {
				framework.Send(Default, Client, http.MethodDelete, gw("/%s", gwID), nil)
			}
		})
	})

	It("registers a gateway and maps it to the environment", func() {
		g := framework.CallObj(Default, Client, http.MethodPost, gw(""), framework.Obj{
			"name": name, "displayName": "E2E platform gateway", "gatewayType": "EGRESS",
			"vhost": "http://" + name + ".gateway.localhost:19080",
		}, http.StatusCreated)
		gwID = framework.Str(g, "uuid")

		// The environment already has its ingress gateway, so this one is egress-only.
		framework.Call(Default, Client, http.MethodPost, gw("/%s/environments/%s", gwID, envUUID), framework.Obj{}, http.StatusCreated)
		envs := framework.CallObj(Default, Client, http.MethodGet, gw("/%s/environments", gwID), nil, http.StatusOK)
		Expect(envs).NotTo(BeEmpty())
		got := framework.CallObj(Default, Client, http.MethodGet, gw("/%s", gwID), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(got, "environments"), "id")).To(ContainElement(envUUID))

		framework.CallObj(Default, Client, http.MethodPut, gw("/%s", gwID), framework.Obj{"displayName": "Renamed"}, http.StatusOK)
		Expect(framework.CallObj(Default, Client, http.MethodGet, gw("/%s", gwID), nil, http.StatusOK)["displayName"]).To(Equal("Renamed"))
		framework.Call(Default, Client, http.MethodGet, gw("/%s/health", gwID), nil, http.StatusOK)
		status := framework.CallObj(Default, Client, http.MethodGet, gw("/status"), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(status, "list"), "id")).To(ContainElement(gwID))
		framework.Call(Default, Client, http.MethodGet, "/m/ai-gateway/platform/gateways/failure-summary", nil, http.StatusOK)
	})

	It("manages the gateway's token and trusted identity providers", func() {
		t := framework.CallObj(Default, Client, http.MethodPost, gw("/%s/tokens", gwID), framework.Obj{}, http.StatusOK)
		tokenID := framework.Str(t, "tokenId")
		tokens := framework.CallObj(Default, Client, http.MethodGet, gw("/%s/tokens", gwID), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(tokens, "list"), "id")).To(ContainElement(tokenID))
		framework.Call(Default, Client, http.MethodDelete, gw("/%s/tokens/%s", gwID, tokenID), nil, http.StatusNoContent)

		idp := "e2e-test-idp"
		issuer, jwks := "https://idp.example.test", "https://idp.example.test/jwks"
		framework.Call(Default, Client, http.MethodPut, gw("/%s/identity-providers/%s", gwID, idp),
			framework.Obj{"issuer": issuer, "jwksUri": jwks}, http.StatusOK)
		idps := framework.CallObj(Default, Client, http.MethodGet, gw("/%s/identity-providers", gwID), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(idps, "list"), "name")).To(ContainElement(idp))
		org := framework.CallObj(Default, Client, http.MethodGet, framework.GW(Cfg, "/identity-providers"), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(org, "list"), "name")).To(ContainElement(idp))
		framework.Call(Default, Client, http.MethodGet, framework.GW(Cfg, "/environments/%s/identity-providers", envUUID), nil, http.StatusOK)
		framework.CallError(Default, Client, http.MethodGet, framework.GW(Cfg, "/identity-providers/discover"), nil, http.StatusBadRequest)

		framework.Call(Default, Client, http.MethodDelete, gw("/%s/identity-providers/%s", gwID, idp), nil, http.StatusNoContent)
		idps = framework.CallObj(Default, Client, http.MethodGet, gw("/%s/identity-providers", gwID), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(idps, "list"), "name")).NotTo(ContainElement(idp))
	})

	It("keeps a gateway that serves a deployment, and removes it once that is gone", func() {
		url, header, value := "https://api.openai.com/v1", "Authorization", "Bearer sk-e2e-placeholder"
		prov := llmproviderop.CreateLLMProvider(Default, Client, providerName, framework.LLMProviderSpec{
			DisplayName: providerName, Version: "v1.0", Context: "/" + providerName, Template: "openai",
			Upstream: framework.UpstreamConfig{Main: &framework.UpstreamEndpoint{
				URL: &url, Auth: &framework.UpstreamAuth{Type: "api-key", Header: &header, Value: &value},
			}},
			Gateways: []string{gwID},
		})
		deps := func() []framework.Obj {
			return framework.CallJSON[[]framework.Obj](Default, Client, http.MethodGet,
				framework.GW(Cfg, "/llm-providers/%s/deployments", prov.Metadata.UID), nil, http.StatusOK)
		}
		var depID string
		for _, d := range deps() {
			if framework.Str(d, "gatewayId") == gwID {
				depID = framework.Str(d, "deploymentId")
			}
		}
		Expect(depID).NotTo(BeEmpty(), "the provider wasn't deployed to the gateway")

		By("Refusing to remove a gateway that still serves a deployment, and keeping both")
		framework.CallError(Default, Client, http.MethodDelete, gw("/%s", gwID), nil, http.StatusConflict)
		framework.Call(Default, Client, http.MethodGet, gw("/%s", gwID), nil, http.StatusOK)
		Expect(framework.Field(deps(), "deploymentId")).To(ContainElement(depID))

		By("Unmapping it from the environment")
		framework.Call(Default, Client, http.MethodDelete, gw("/%s/environments/%s", gwID, envUUID), nil, http.StatusNoContent)
		got := framework.CallObj(Default, Client, http.MethodGet, gw("/%s", gwID), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(got, "environments"), "id")).NotTo(ContainElement(envUUID))

		By("Removing it once the provider deployed to it is gone")
		// The gateway never connected, so an undeploy would never be acknowledged;
		// deleting the provider takes its deployments off every gateway.
		framework.Call(Default, Client, http.MethodDelete, "/m/ai-gateway/llm-providers/"+providerName, nil, http.StatusNoContent)
		Eventually(func(g Gomega) {
			framework.Call(g, Client, http.MethodDelete, gw("/%s", gwID), nil, http.StatusNoContent)
		}).WithPolling(2 * time.Second).Should(Succeed())
		framework.CallError(Default, Client, http.MethodGet, gw("/%s", gwID), nil, http.StatusNotFound)
		gwID = ""
	})
})

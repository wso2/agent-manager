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

// J6: a platform engineer registers a self-hosted gateway, issues the token it
// connects with, and removes it again: the token stops working, and a gateway
// still serving a deployment is kept.

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

// gatewayDoc is a self-hosted Gateway document; the server places it in the organization's default environment.
func gatewayDoc(name, displayName, hosting string) framework.Obj {
	return framework.Obj{
		"apiVersion": "ai-control-plane.choreolabs.com/v1", "kind": "Gateway",
		"metadata": framework.Obj{"name": name},
		"spec": framework.Obj{
			"displayName": displayName, "hosting": hosting, "type": "EGRESS",
			"endpoint": "http://" + name + ".example.test:19080",
		},
	}
}

func spec(o framework.Obj) framework.Obj      { return o["spec"].(map[string]any) }
func meta(o framework.Obj) framework.Obj      { return o["metadata"].(map[string]any) }
func docStatus(o framework.Obj) framework.Obj { return o["status"].(map[string]any) }
func state(o framework.Obj) any               { return docStatus(o)["state"] }
func names(list framework.Obj) []string {
	var out []string
	for _, item := range framework.Items(list, "list") {
		out = append(out, framework.Str(item, "metadata", "name"))
	}
	return out
}

// expectTokenPrefix checks that a token listing shows the start of the token,
// enough to match a token in use to its entry, but never the whole token.
func expectTokenPrefix(tokens framework.Obj, tokenID, token string) {
	for _, t := range framework.Items(tokens, "list") {
		if framework.Str(t, "id") != tokenID {
			continue
		}
		prefix := framework.Str(t, "prefix")
		Expect(prefix).To(HaveLen(8))
		Expect(token).To(HavePrefix(prefix))
		return
	}
	Fail("token " + tokenID + " is not listed")
}

var _ = Describe("Self-hosted gateway", Label("gateway"), Ordered, func() {
	var name, providerName, token, tokenID string
	gw := func(f string, a ...any) string { return framework.AIGW("/gateways"+f, a...) }

	BeforeAll(func() {
		suffix := uuid.NewString()[:8]
		name = "e2e-test-shgw-" + suffix
		providerName = framework.E2ELLMProviderPrefix + "gw-" + suffix
		DeferCleanup(func() {
			framework.Send(Default, Client, http.MethodDelete, "/m/ai-gateway/llm-providers/"+providerName, nil)
			if name != "" {
				framework.Send(Default, Client, http.MethodDelete, gw("/%s", name), nil)
			}
		})
	})

	It("registers a gateway document in the default environment", func() {
		g := framework.CallObj(Default, Client, http.MethodPost, gw(""), gatewayDoc(name, "E2E self-hosted", "SELF_HOSTED"), http.StatusCreated)
		Expect(g["kind"]).To(Equal("Gateway"))
		Expect(meta(g)["name"]).To(Equal(name))
		Expect(spec(g)["hosting"]).To(Equal("SELF_HOSTED"))
		// Its owner applies identity providers to its config.toml.
		Expect(docStatus(g)["identityProviderMode"]).To(Equal("MANUAL"))
		// It hasn't connected yet.
		Expect(state(g)).To(Equal("INACTIVE"))

		got := framework.CallObj(Default, Client, http.MethodGet, gw("/%s", name), nil, http.StatusOK)
		Expect(spec(got)["displayName"]).To(Equal("E2E self-hosted"))
		Expect(docStatus(got)["identityProviderMode"]).To(Equal("MANUAL"))
		Expect(names(framework.CallObj(Default, Client, http.MethodGet, gw("?hosting=SELF_HOSTED"), nil, http.StatusOK))).To(ContainElement(name))
		Expect(names(framework.CallObj(Default, Client, http.MethodGet, gw("?hosting=MANAGED"), nil, http.StatusOK))).NotTo(ContainElement(name))

		framework.CallError(Default, Client, http.MethodPost, gw(""), gatewayDoc(name, "dup", "SELF_HOSTED"), http.StatusConflict)
		framework.CallError(Default, Client, http.MethodPost, gw(""), gatewayDoc(name+"-m", "managed", "MANAGED"), http.StatusBadRequest)
	})

	It("replaces its editable fields, but not its endpoint", func() {
		doc := gatewayDoc(name, "Renamed", "SELF_HOSTED")
		framework.CallObj(Default, Client, http.MethodPut, gw("/%s", name), doc, http.StatusOK)
		Expect(spec(framework.CallObj(Default, Client, http.MethodGet, gw("/%s", name), nil, http.StatusOK))["displayName"]).To(Equal("Renamed"))

		spec(doc)["endpoint"] = "http://elsewhere.example.test"
		framework.CallError(Default, Client, http.MethodPut, gw("/%s", name), doc, http.StatusBadRequest)

		framework.Call(Default, Client, http.MethodGet, gw("/%s/health", name), nil, http.StatusOK)
		framework.CallObj(Default, Client, http.MethodGet, gw("/status"), nil, http.StatusOK)
		framework.Call(Default, Client, http.MethodGet, "/m/ai-gateway/platform/gateways/failure-summary", nil, http.StatusOK)
	})

	It("issues a token the gateway can call the platform with, until it's revoked", func() {
		t := framework.CallObj(Default, Client, http.MethodPost, gw("/%s/tokens", name), framework.Obj{}, http.StatusCreated)
		token, tokenID = framework.Str(t, "token"), framework.Str(t, "tokenId")
		Expect(token).NotTo(BeEmpty())
		tokens := framework.CallObj(Default, Client, http.MethodGet, gw("/%s/tokens", name), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(tokens, "list"), "id")).To(ContainElement(tokenID))
		Expect(tokens).NotTo(HaveKey("token"), "a token's value is shown only once")
		expectTokenPrefix(tokens, tokenID, token)

		status := internalStatus(token)
		if status == 0 {
			Skip("the internal API isn't reachable from here; set AMP_INTERNAL_BASE_URL")
		}
		Expect(status).To(Equal(http.StatusOK))
		Expect(internalStatus("not-a-token")).To(Equal(http.StatusUnauthorized))

		framework.Call(Default, Client, http.MethodDelete, gw("/%s/tokens/%s", name, tokenID), nil, http.StatusNoContent)
		Eventually(func() int { return internalStatus(token) }).WithPolling(time.Second).Should(Equal(http.StatusUnauthorized))
	})

	It("records trusted identity providers for its owner to apply", func() {
		idp := "e2e-test-idp-" + uuid.NewString()[:8]
		issuer := "https://" + idp + ".example.test"
		DeferCleanup(func() {
			framework.Send(Default, Client, http.MethodDelete, gw("/%s/identity-providers/%s", name, idp), nil)
		})
		p := framework.CallObj(Default, Client, http.MethodPut, gw("/%s/identity-providers/%s", name, idp),
			framework.Obj{"issuer": issuer, "jwksUri": issuer + "/jwks"}, http.StatusOK)
		Expect(p["name"]).To(Equal(idp))
		Expect(p["issuer"]).To(Equal(issuer))

		idps := framework.CallObj(Default, Client, http.MethodGet, gw("/%s/identity-providers", name), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(idps, "list"), "name")).To(ContainElement(idp))
		org := framework.CallObj(Default, Client, http.MethodGet, framework.AIGW("/identity-providers"), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(org, "list"), "name")).To(ContainElement(idp))
		framework.CallError(Default, Client, http.MethodGet, framework.AIGW("/identity-providers/discover"), nil, http.StatusBadRequest)

		framework.Call(Default, Client, http.MethodDelete, gw("/%s/identity-providers/%s", name, idp), nil, http.StatusNoContent)
		idps = framework.CallObj(Default, Client, http.MethodGet, gw("/%s/identity-providers", name), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(idps, "list"), "name")).NotTo(ContainElement(idp))
	})

	It("keeps a gateway that serves a deployment, and removes it, with its tokens, once that is gone", func() {
		uid := framework.Str(meta(framework.CallObj(Default, Client, http.MethodGet, gw("/%s", name), nil, http.StatusOK)), "uid")
		url, header, value := "https://api.openai.com/v1", "Authorization", "Bearer sk-e2e-placeholder"
		prov := llmproviderop.CreateLLMProvider(Default, Client, providerName, framework.LLMProviderSpec{
			DisplayName: providerName, Version: "v1.0", Context: "/" + providerName, Template: "openai",
			Upstream: framework.UpstreamConfig{Main: &framework.UpstreamEndpoint{
				URL: &url, Auth: &framework.UpstreamAuth{Type: "api-key", Header: &header, Value: &value},
			}},
			Gateways: []string{uid},
		})
		deps := func() []framework.Obj {
			return framework.CallJSON[[]framework.Obj](Default, Client, http.MethodGet,
				"/m/ai-gateway/llm-providers/"+prov.Metadata.UID+"/deployments", nil, http.StatusOK)
		}
		var depID string
		for _, d := range deps() {
			if framework.Str(d, "gatewayId") == uid {
				depID = framework.Str(d, "deploymentId")
			}
		}
		Expect(depID).NotTo(BeEmpty(), "the provider wasn't deployed to the gateway")

		By("Refusing to remove a gateway that still serves a deployment, and keeping both")
		framework.CallError(Default, Client, http.MethodDelete, gw("/%s", name), nil, http.StatusConflict)
		framework.Call(Default, Client, http.MethodGet, gw("/%s", name), nil, http.StatusOK)
		Expect(framework.Field(deps(), "deploymentId")).To(ContainElement(depID))

		By("Removing it once the provider deployed to it is gone")
		t := framework.CallObj(Default, Client, http.MethodPost, gw("/%s/tokens", name), framework.Obj{}, http.StatusCreated)
		// The gateway never connected, so an undeploy would never be acknowledged;
		// deleting the provider takes its deployments off every gateway.
		framework.Call(Default, Client, http.MethodDelete, "/m/ai-gateway/llm-providers/"+providerName, nil, http.StatusNoContent)
		Eventually(func(g Gomega) {
			framework.Call(g, Client, http.MethodDelete, gw("/%s", name), nil, http.StatusNoContent)
		}).WithPolling(2 * time.Second).Should(Succeed())
		framework.CallError(Default, Client, http.MethodGet, gw("/%s", name), nil, http.StatusNotFound)
		if status := internalStatus(framework.Str(t, "token")); status != 0 {
			Expect(status).To(Equal(http.StatusUnauthorized))
		}
		name = ""
	})
})

var _ = Describe("Managed gateway", Label("gateway"), Ordered, func() {
	var name string
	gw := func(f string, a ...any) string { return framework.AIGW("/gateways"+f, a...) }

	BeforeAll(func() {
		managed := framework.Items(framework.CallObj(Default, Client, http.MethodGet, gw("?hosting=MANAGED"), nil, http.StatusOK), "list")
		if len(managed) == 0 {
			Skip("the deployment has no managed gateway")
		}
		name = framework.Str(meta(managed[0]), "name")
	})

	It("is read-only", func() {
		got := framework.CallObj(Default, Client, http.MethodGet, gw("/%s", name), nil, http.StatusOK)
		Expect(spec(got)["hosting"]).To(Equal("MANAGED"))
		framework.CallError(Default, Client, http.MethodPut, gw("/%s", name), framework.Obj{
			"apiVersion": got["apiVersion"], "kind": "Gateway", "metadata": framework.Obj{"name": name}, "spec": spec(got),
		}, http.StatusForbidden)
		framework.CallError(Default, Client, http.MethodDelete, gw("/%s", name), nil, http.StatusForbidden)
	})

	It("refuses a custom identity provider it can't apply", func() {
		got := framework.CallObj(Default, Client, http.MethodGet, gw("/%s", name), nil, http.StatusOK)
		// Nothing applies a custom identity provider to a managed gateway's config.
		Expect(docStatus(got)["identityProviderMode"]).To(Equal("UNSUPPORTED"))

		// Refused and not recorded: agent OAuth would otherwise trust an issuer the
		// gateway rejects. Self-hosted gateways record them (see above).
		idp := "e2e-test-idp-" + uuid.NewString()[:8]
		DeferCleanup(func() {
			framework.Send(Default, Client, http.MethodDelete, gw("/%s/identity-providers/%s", name, idp), nil)
		})
		framework.CallError(Default, Client, http.MethodPut, gw("/%s/identity-providers/%s", name, idp),
			framework.Obj{"issuer": "https://idp.example.test", "jwksUri": "https://idp.example.test/jwks"}, http.StatusConflict)
		idps := framework.CallObj(Default, Client, http.MethodGet, gw("/%s/identity-providers", name), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(idps, "list"), "name")).NotTo(ContainElement(idp))
	})

	It("is gone from the old organization paths", func() {
		status, _ := framework.Send(Default, Client, http.MethodGet, framework.GW(Cfg, "/gateways"), nil)
		Expect(status).To(Equal(http.StatusNotFound))
		status, _ = framework.Send(Default, Client, http.MethodGet, framework.GW(Cfg, "/self-hosted-gateways"), nil)
		Expect(status).To(Equal(http.StatusNotFound))
	})
})

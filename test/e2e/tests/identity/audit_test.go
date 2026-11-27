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

// J11: changing a role and revoking a key each leave an audit record naming
// who did it and to what, and never the secret involved.

package identity

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	"github.com/wso2/agent-manager/test/e2e/operations/gateway"
	llmproviderop "github.com/wso2/agent-manager/test/e2e/operations/llmprovider"
)

// awaitAudit waits for the audit record matching match, written since start.
func awaitAudit(start time.Time, match func(framework.AuditRecord) bool) framework.AuditRecord {
	var found framework.AuditRecord
	Eventually(func(g Gomega) {
		records, err := framework.AuditRecordsSince(start)
		g.Expect(err).NotTo(HaveOccurred())
		for _, r := range records {
			if match(r) {
				found = r
				return
			}
		}
		g.Expect(records).To(ContainElement(Satisfy(match)), "no matching audit record among %d", len(records))
	}).WithTimeout(30 * time.Second).WithPolling(time.Second).Should(Succeed())
	return found
}

var _ = Describe("Audit trail", Label("identity", "audit"), func() {
	BeforeEach(func() {
		if framework.APIContainer() == "" {
			Skip("no local amp-api container to read the audit trail from")
		}
	})

	It("records a role's permission change with its actor and target", func() {
		roleName := "e2e-test-audit-role-" + uuid.NewString()[:8]
		r := framework.CallObj(Default, Client, http.MethodPost, framework.API(Cfg, "/identities/roles"),
			framework.Obj{"name": roleName, "description": "audit"}, http.StatusCreated)
		roleID := framework.Str(r, "id")
		DeferCleanup(func() {
			framework.Send(Default, Client, http.MethodDelete, framework.API(Cfg, "/identities/roles/%s", roleID), nil)
		})

		start := time.Now()
		framework.Call(Default, Client, http.MethodPost, framework.API(Cfg, "/identities/roles/%s/permissions/add", roleID),
			framework.Obj{"resourceServerId": "amp-resource-server", "permissions": []string{"amp:project:read"}}, http.StatusOK)

		rec := awaitAudit(start, func(r framework.AuditRecord) bool {
			return r.Action == "role:grant-permission" && r.ResourceID == roleID && r.Outcome == "success"
		})
		Expect(rec.ActorID).To(Equal(Cfg.IDPClientID))
		Expect(rec.ResourceType).To(Equal("role"))
		Expect(rec.ResourceName).To(Equal(roleName))
		Expect(rec.Details).To(HaveKeyWithValue("permissions", ContainElement("amp:project:read")))
		Expect(rec.Raw).NotTo(ContainSubstring(Client.Token()), "the caller's token must not be recorded")

		// A refused change is recorded too, as a failure.
		start = time.Now()
		framework.CallError(Default, Client, http.MethodPost, framework.API(Cfg, "/identities/roles/amp-role-admin/permissions/add"),
			framework.Obj{"resourceServerId": "amp-resource-server", "permissions": []string{"amp:project:read"}}, http.StatusBadRequest)
		awaitAudit(start, func(r framework.AuditRecord) bool {
			return r.Action == "role:grant-permission" && r.Outcome == "failure" && r.StatusCode == http.StatusBadRequest
		})
	})

	It("records an API key's revocation without the key", func() {
		gwID := gateway.WaitForActiveGatewayForEnv(Client, Cfg.DefaultOrg, Cfg.DefaultEnv, 3*time.Minute)
		providerName := framework.E2ELLMProviderPrefix + "audit-" + uuid.NewString()[:8]
		prov := llmproviderop.CreateLLMProvider(Default, Client, providerName, openAIProviderSpec(providerName, gwID))
		providerID := prov.Metadata.UID
		DeferCleanup(func() {
			framework.Send(Default, Client, http.MethodDelete, "/m/ai-gateway/llm-providers/"+providerName, nil)
		})

		keyName := "e2e-audit-key"
		key := framework.CallObj(Default, Client, http.MethodPost, "/m/ai-gateway/llm-providers/"+providerID+"/api-keys",
			framework.Obj{"name": keyName, "displayName": keyName}, http.StatusCreated)
		secret := framework.Str(key, "apiKey")
		Expect(secret).NotTo(BeEmpty())

		rotated := framework.CallObj(Default, Client, http.MethodPut, fmt.Sprintf("/m/ai-gateway/llm-providers/%s/api-keys/%s", providerID, keyName),
			framework.Obj{}, http.StatusOK)
		Expect(framework.Str(rotated, "apiKey")).NotTo(BeEmpty())
		Expect(framework.Str(rotated, "apiKey")).NotTo(Equal(secret))
		secret = framework.Str(rotated, "apiKey")

		start := time.Now()
		framework.Call(Default, Client, http.MethodDelete, fmt.Sprintf("/m/ai-gateway/llm-providers/%s/api-keys/%s", providerID, keyName), nil, http.StatusNoContent)
		keys := framework.CallObj(Default, Client, http.MethodGet, "/m/ai-gateway/llm-providers/"+providerID+"/api-keys", nil, http.StatusOK)
		Expect(framework.Field(framework.Items(keys, "keys"), "name")).NotTo(ContainElement(keyName))

		rec := awaitAudit(start, func(r framework.AuditRecord) bool {
			return r.Action == "ai-gateway:api-keys.revoke" && r.Outcome == "success" && r.Details != nil
		})
		Expect(rec.ActorID).To(Equal(Cfg.IDPClientID))
		Expect(rec.Raw).To(ContainSubstring(keyName))
		Expect(rec.Raw).NotTo(ContainSubstring(secret), "the revoked key's value must not be recorded")
	})
})

// openAIProviderSpec is an OpenAI provider on the gateway, with a placeholder
// upstream key: these journeys only need it to exist.
func openAIProviderSpec(name, gatewayID string) framework.LLMProviderSpec {
	url, header, value := "https://api.openai.com/v1", "Authorization", "Bearer sk-e2e-placeholder"
	return framework.LLMProviderSpec{
		DisplayName: name,
		Version:     "v1.0",
		Context:     "/" + name,
		Template:    "openai",
		Upstream: framework.UpstreamConfig{Main: &framework.UpstreamEndpoint{
			URL:  &url,
			Auth: &framework.UpstreamAuth{Type: "api-key", Header: &header, Value: &value},
		}},
		Gateways: []string{gatewayID},
	}
}

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

package roles

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

const (
	customRoleProbeScope    = "amp:environment:read"
	customRoleRetainedScope = "amp:project:read"
)

type roleResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	IsReadOnly bool   `json:"isReadOnly"`
}

type permissionsResponse struct {
	ResourceServerID string `json:"resourceServerId"`
}

var _ = Describe("SEC-ROLE-002: custom role mutation and revocation", Label("security", "roles"), func() {
	It("applies grants and revocations to newly issued tokens", func(ctx SpecContext) {
		admin := framework.NewAMPClientWithToken(cfg, personas["admin"].Token)
		identityBase := "/api/v1/orgs/" + cfg.DefaultOrg + "/identities"
		probePath := "/api/v1/orgs/" + cfg.DefaultOrg + "/environments"
		retainedPath := "/api/v1/projects"

		By("Resolving the deployed AMP resource server")
		resp, err := admin.GetWithContext(ctx, identityBase+"/permissions")
		Expect(err).NotTo(HaveOccurred())
		catalog := framework.ExpectStatusAndDecode[permissionsResponse](Default, resp, http.StatusOK)
		resp.Body.Close()
		Expect(catalog.ResourceServerID).NotTo(BeEmpty(), "permissions catalog returned no AMP resource-server ID")

		By("Creating a disposable custom role through Agent Manager")
		roleName := "e2e-test-sec-custom-role-" + uuid.NewString()[:8]
		resp, err = admin.PostWithContext(ctx, identityBase+"/roles", map[string]string{
			"name":        roleName,
			"description": "Disposable security-test role",
		})
		Expect(err).NotTo(HaveOccurred())
		role := framework.ExpectStatusAndDecode[roleResponse](Default, resp, http.StatusCreated)
		resp.Body.Close()
		Expect(role.ID).NotTo(BeEmpty())
		Expect(role.Name).To(Equal(roleName))
		Expect(role.IsReadOnly).To(BeFalse())

		roleID := role.ID
		DeferCleanup(func() {
			if roleID == "" {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cleanupResp, cleanupErr := admin.DeleteWithContext(cleanupCtx, identityBase+"/roles/"+roleID)
			Expect(cleanupErr).NotTo(HaveOccurred())
			defer cleanupResp.Body.Close()
			Expect(cleanupResp.StatusCode).To(Or(Equal(http.StatusNoContent), Equal(http.StatusNotFound)))
		})

		permissionRequest := map[string]any{
			"resourceServerId": catalog.ResourceServerID,
			"permissions":      []string{customRoleProbeScope, customRoleRetainedScope},
		}

		By("Granting two harmless read permissions through Agent Manager")
		resp, err = admin.PostWithContext(ctx, identityBase+"/roles/"+role.ID+"/permissions/add", permissionRequest)
		Expect(err).NotTo(HaveOccurred())
		framework.ExpectStatus(Default, resp, http.StatusOK)
		resp.Body.Close()

		By("Assigning a disposable OAuth persona to the custom role")
		persona, err := provisioner.CreateRolePersona(ctx, roleName)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			if persona == nil {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			Expect(provisioner.DeleteRolePersona(cleanupCtx, persona)).To(Succeed())
		})

		By("Proving the grant appears in a fresh token and reaches the API")
		grantedScopes, err := framework.TokenScopes(persona.Token)
		Expect(err).NotTo(HaveOccurred())
		Expect(grantedScopes).To(HaveKey(customRoleProbeScope))
		Expect(grantedScopes).To(HaveKey(customRoleRetainedScope))
		resp, err = framework.NewAMPClientWithToken(cfg, persona.Token).GetWithContext(ctx, probePath)
		Expect(err).NotTo(HaveOccurred())
		framework.ExpectNotForbidden(Default, resp, "custom role with "+customRoleProbeScope)
		resp.Body.Close()

		By("Revoking only the probe permission while retaining another AMP permission")
		probeRemoval := map[string]any{
			"resourceServerId": catalog.ResourceServerID,
			"permissions":      []string{customRoleProbeScope},
		}
		resp, err = admin.PostWithContext(ctx, identityBase+"/roles/"+role.ID+"/permissions/remove", probeRemoval)
		Expect(err).NotTo(HaveOccurred())
		framework.ExpectStatus(Default, resp, http.StatusOK)
		resp.Body.Close()

		By("Waiting until a newly issued token reflects only the partial revocation")
		var revokedToken string
		Eventually(func(g Gomega) bool {
			fresh, refreshErr := provisioner.RefreshRolePersonaToken(ctx, persona)
			g.Expect(refreshErr).NotTo(HaveOccurred())
			scopes, decodeErr := framework.TokenScopes(fresh)
			g.Expect(decodeErr).NotTo(HaveOccurred())
			if _, stillGranted := scopes[customRoleProbeScope]; stillGranted {
				return false
			}
			g.Expect(scopes).To(HaveKey(customRoleRetainedScope),
				"partial revocation also removed the permission that should keep the AMP audience valid")
			revokedToken = fresh
			return true
		}).WithTimeout(10*time.Second).WithPolling(500*time.Millisecond).Should(BeTrue(),
			fmt.Sprintf("new tokens kept %s after role revocation", customRoleProbeScope))

		By("Requiring exactly 403 for the removed permission")
		resp, err = framework.NewAMPClientWithToken(cfg, revokedToken).GetWithContext(ctx, probePath)
		Expect(err).NotTo(HaveOccurred())
		framework.ExpectForbidden(Default, resp, "custom role after revoking only "+customRoleProbeScope)
		resp.Body.Close()

		By("Proving the retained permission still reaches its endpoint")
		resp, err = framework.NewAMPClientWithToken(cfg, revokedToken).GetWithContext(ctx, retainedPath)
		Expect(err).NotTo(HaveOccurred())
		framework.ExpectNotForbidden(Default, resp, "custom role retaining "+customRoleRetainedScope)
		resp.Body.Close()

		By("Revoking the final AMP permission")
		finalRemoval := map[string]any{
			"resourceServerId": catalog.ResourceServerID,
			"permissions":      []string{customRoleRetainedScope},
		}
		resp, err = admin.PostWithContext(ctx, identityBase+"/roles/"+role.ID+"/permissions/remove", finalRemoval)
		Expect(err).NotTo(HaveOccurred())
		framework.ExpectStatus(Default, resp, http.StatusOK)
		resp.Body.Close()

		By("Waiting until a newly issued token has no AMP permissions")
		var zeroScopeToken string
		Eventually(func(g Gomega) bool {
			fresh, refreshErr := provisioner.RefreshRolePersonaToken(ctx, persona)
			g.Expect(refreshErr).NotTo(HaveOccurred())
			scopes, decodeErr := framework.TokenScopes(fresh)
			g.Expect(decodeErr).NotTo(HaveOccurred())
			if _, stillGranted := scopes[customRoleRetainedScope]; stillGranted {
				return false
			}
			zeroScopeToken = fresh
			return true
		}).WithTimeout(10*time.Second).WithPolling(500*time.Millisecond).Should(BeTrue(),
			fmt.Sprintf("new tokens kept %s after final role revocation", customRoleRetainedScope))

		By("Proving the zero-permission token is rejected fail-closed")
		resp, err = framework.NewAMPClientWithToken(cfg, zeroScopeToken).GetWithContext(ctx, retainedPath)
		Expect(err).NotTo(HaveOccurred())
		framework.ExpectStatusIn(Default, resp, http.StatusUnauthorized, http.StatusForbidden)
		if resp.StatusCode == http.StatusUnauthorized {
			audiences, decodeErr := framework.TokenAudiences(zeroScopeToken)
			Expect(decodeErr).NotTo(HaveOccurred())
			Expect(audiences).NotTo(ContainElement("amp"),
				"Thunder issued an AMP-audience token, so Agent Manager's 401 needs investigation rather than being treated as zero-scope revocation")
		}
		resp.Body.Close()

		By("Deleting the persona before deleting its role")
		Expect(provisioner.DeleteRolePersona(ctx, persona)).To(Succeed())
		persona = nil

		By("Deleting the disposable custom role through Agent Manager")
		resp, err = admin.DeleteWithContext(ctx, identityBase+"/roles/"+role.ID)
		Expect(err).NotTo(HaveOccurred())
		framework.ExpectStatus(Default, resp, http.StatusNoContent)
		resp.Body.Close()
		roleID = ""
	})
})

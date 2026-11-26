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

package agentid

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
)

var _ = Describe("SEC-AGENTID-001: external AgentID credential lifecycle", Label("security", "agentid"), Ordered, func() {
	var (
		agentName     string
		identityPath  string
		clientID      string
		firstSecret   string
		currentSecret string
	)

	BeforeAll(func() {
		agentName = "e2e-sec-agentid-" + uuid.NewString()[:8]
		identityPath = agentops.AgentIdentityPath(cfg.DefaultOrg, cfg.DefaultProject, agentName)
	})

	AfterAll(func() {
		agentops.DeleteAgentBestEffort(adminClient, cfg.DefaultOrg, cfg.DefaultProject, agentName)
	})

	It("registers a disposable externally hosted agent without a build", func() {
		agent := agentops.CreateAgent(Default, adminClient, &agentops.CreateAgentParams{
			OrgName:     cfg.DefaultOrg,
			ProjectName: cfg.DefaultProject,
			Request: framework.NewExternalAgentRequest(agentName,
				"Disposable external agent for AgentID credential security tests"),
		})
		Expect(agent.Metadata.Name).To(Equal(agentName))
		Expect(agent.Spec.Provisioning.Type).To(Equal("external"))
		Expect(agent.Spec.AgentType.Type).To(Equal("external-agent-api"))
	})

	It("provisions the environment identity without exposing a secret in safe reads", func(ctx SpecContext) {
		var completed framework.AgentIdentityEnvironmentView
		Eventually(func(g Gomega) string {
			views, _ := agentops.ListAgentIdentities(ctx, g, adminClient,
				cfg.DefaultOrg, cfg.DefaultProject, agentName, cfg.DefaultEnv)
			g.Expect(views).To(HaveLen(1), "expected one AgentID binding for %s", cfg.DefaultEnv)
			view := views[0]
			g.Expect(view.EnvironmentName).To(Equal(cfg.DefaultEnv))
			if strings.EqualFold(view.Status, "failed") {
				StopTrying("AgentID provisioning failed: " + view.LastError).Now()
			}
			completed = view
			return strings.ToLower(view.Status)
		}).WithContext(ctx).WithTimeout(cfg.ReadinessTimeout).WithPolling(2 * time.Second).Should(Equal("completed"))

		Expect(completed.AgentID).NotTo(BeEmpty())
		Expect(completed.ClientID).NotTo(BeEmpty())

		_, raw := agentops.ListAgentIdentities(ctx, Default, adminClient,
			cfg.DefaultOrg, cfg.DefaultProject, agentName, cfg.DefaultEnv)
		expectNoClientSecretField(raw, "completed AgentID safe-read response")
	})

	It("rejects an unauthenticated identity read with 401", func(ctx SpecContext) {
		resp, err := adminClient.GetUnauthenticatedWithContext(ctx, identityPath+"&"+
			url.Values{"environment": {cfg.DefaultEnv}}.Encode())
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		framework.ExpectUnauthorized(Default, resp, "unauthenticated AgentID read")
	})

	It("denies AgentID read, rotation, and revocation without amp:agent:update", func(ctx SpecContext) {
		query := "&" + url.Values{"environment": {cfg.DefaultEnv}}.Encode()

		resp, err := reducedClient.GetWithContext(ctx, identityPath+query)
		Expect(err).NotTo(HaveOccurred())
		framework.ExpectForbidden(Default, resp, "under-scoped AgentID read")
		resp.Body.Close()

		resp, err = reducedClient.PostWithContext(ctx, identityPath,
			framework.AgentIdentityActionRequest{Environment: cfg.DefaultEnv})
		Expect(err).NotTo(HaveOccurred())
		framework.ExpectForbidden(Default, resp, "under-scoped AgentID secret rotation")
		resp.Body.Close()

		resp, err = reducedClient.DeleteWithContext(ctx, identityPath+query)
		Expect(err).NotTo(HaveOccurred())
		framework.ExpectForbidden(Default, resp, "under-scoped AgentID secret revocation")
		resp.Body.Close()
	})

	It("generates a credential that can mint an environment token", func(ctx SpecContext) {
		generated := agentops.RegenerateAgentIdentitySecret(ctx, Default, adminClient,
			cfg.DefaultOrg, cfg.DefaultProject, agentName, cfg.DefaultEnv)
		Expect(generated.EnvironmentName).To(Equal(cfg.DefaultEnv))
		Expect(generated.ProvisioningType).To(Equal("external"))
		Expect(generated.Status).To(Equal("regenerated"))
		Expect(generated.ClientID).NotTo(BeEmpty())
		Expect(generated.ClientSecret).NotTo(BeEmpty())

		clientID = generated.ClientID
		firstSecret = generated.ClientSecret
		currentSecret = generated.ClientSecret
		expectCredentialAccepted(ctx, clientID, currentSecret)
	})

	It("invalidates the old secret when the credential is rotated", func(ctx SpecContext) {
		rotated := agentops.RegenerateAgentIdentitySecret(ctx, Default, adminClient,
			cfg.DefaultOrg, cfg.DefaultProject, agentName, cfg.DefaultEnv)
		Expect(rotated.ClientID).To(Equal(clientID))
		Expect(rotated.ClientSecret).NotTo(BeEmpty())
		Expect(rotated.ClientSecret).NotTo(Equal(firstSecret))
		currentSecret = rotated.ClientSecret

		expectCredentialRejected(ctx, clientID, firstSecret)
		expectCredentialAccepted(ctx, clientID, currentSecret)
	})

	It("revokes the current secret without echoing it and prevents new tokens", func(ctx SpecContext) {
		revoked, raw := agentops.RevokeAgentIdentitySecret(ctx, Default, adminClient,
			cfg.DefaultOrg, cfg.DefaultProject, agentName, cfg.DefaultEnv)
		Expect(revoked.EnvironmentName).To(Equal(cfg.DefaultEnv))
		Expect(revoked.ClientID).To(Equal(clientID))
		Expect(revoked.Status).To(Equal("revoked"))
		expectNoClientSecretField(raw, "AgentID revoke response")
		Expect(raw).NotTo(ContainSubstring(currentSecret), "revoke response echoed the revoked credential")

		expectCredentialRejected(ctx, clientID, currentSecret)

		_, safeRead := agentops.ListAgentIdentities(ctx, Default, adminClient,
			cfg.DefaultOrg, cfg.DefaultProject, agentName, cfg.DefaultEnv)
		expectNoClientSecretField(safeRead, "post-revoke AgentID safe-read response")
		Expect(safeRead).NotTo(ContainSubstring(currentSecret), "safe-read response exposed the revoked credential")
	})
})

func expectNoClientSecretField(raw, context string) {
	normalized := strings.ToLower(strings.ReplaceAll(raw, "_", ""))
	Expect(normalized).NotTo(ContainSubstring(`"clientsecret"`), "%s contains a client-secret field", context)
}

func expectCredentialAccepted(ctx context.Context, clientID, clientSecret string) {
	Eventually(func(g Gomega) framework.ClientCredentialsTokenResult {
		result, err := framework.RequestClientCredentialsToken(ctx, cfg.AgentIDTokenURL, clientID, clientSecret)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.StatusCode).To(Equal(http.StatusOK))
		g.Expect(result.AccessToken).NotTo(BeEmpty())
		return result
	}).WithContext(ctx).WithTimeout(30 * time.Second).WithPolling(time.Second).Should(HaveField("StatusCode", http.StatusOK))
}

func expectCredentialRejected(ctx context.Context, clientID, clientSecret string) {
	Eventually(func(g Gomega) string {
		result, err := framework.RequestClientCredentialsToken(ctx, cfg.AgentIDTokenURL, clientID, clientSecret)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(result.StatusCode).To(Or(Equal(http.StatusBadRequest), Equal(http.StatusUnauthorized)))
		return result.OAuthError
	}).WithContext(ctx).WithTimeout(30 * time.Second).WithPolling(time.Second).Should(Equal("invalid_client"))
}

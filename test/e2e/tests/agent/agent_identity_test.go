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

// J8: an external agent gets an identity in the environment, mints a token
// with its client credentials, and loses the old secret on rotation. The
// environment's agent roles and groups then decide what scopes it carries.

package agent

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
)

var _ = Describe("Agent identity:", Label("agent", "agent-identity"), Ordered, func() {
	var (
		suffix, agentName, tokenURL string
		clientID, secret            string
		roleID, groupID             string
		identityID                  string
	)
	proj := framework.E2ESharedProjectName
	ai := func(f string, a ...any) string {
		return framework.GW(Cfg, "/environments/%s/agent-identities"+f, append([]any{Cfg.DefaultEnv}, a...)...)
	}
	agentPath := func(f string) string { return framework.API(Cfg, "/projects/%s/agents/%s"+f, proj, agentName) }

	BeforeAll(func() {
		suffix = uuid.NewString()[:8]
		agentName = framework.E2EAgentPrefix + suffix
		agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName: Cfg.DefaultOrg, ProjectName: proj,
			Request: framework.NewExternalAgentRequest(agentName, "agent identity journey"),
		})
		DeferCleanup(func() {
			if roleID != "" {
				framework.Send(Default, Client, http.MethodDelete, ai("/roles/%s", roleID), nil)
			}
			if groupID != "" {
				framework.Send(Default, Client, http.MethodDelete, ai("/groups/%s", groupID), nil)
			}
			agentops.DeleteAgentBestEffort(Client, Cfg.DefaultOrg, proj, agentName)
		})

		instances := framework.CallObj(Default, Client, http.MethodGet, framework.API(Cfg, "/thunder-instances"), nil, http.StatusOK)
		for _, it := range framework.Items(instances, "thunderInstances") {
			if framework.Str(it, "envName") == Cfg.DefaultEnv {
				tokenURL = framework.Str(it, "tokenUrl")
			}
		}
		if Cfg.AgentIDTokenURL != "" {
			tokenURL = Cfg.AgentIDTokenURL
		}
		Expect(tokenURL).NotTo(BeEmpty())
	})

	It("provisions the agent's identity in the environment", func(ctx context.Context) {
		framework.Call(Default, Client, http.MethodPut, agentPath("/identities?environment="+Cfg.DefaultEnv), nil, http.StatusOK)
		Eventually(func(g Gomega) {
			views, _ := agentops.ListAgentIdentities(ctx, g, Client, Cfg.DefaultOrg, proj, agentName, Cfg.DefaultEnv)
			g.Expect(views).To(HaveLen(1))
			g.Expect(views[0].Status).To(Equal("completed"), "last error: %s", views[0].LastError)
			clientID = views[0].ClientID
			// A failed attempt that isn't the last stays pending and is retried
			// after 3 minutes, picked up on the reconciler's 1-minute tick. The
			// test identity provider's SQLite can refuse a write under parallel
			// load, so allow for one automatic retry.
		}).WithTimeout(5 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
		Expect(clientID).NotTo(BeEmpty())

		// Retrying a completed provisioning is a no-op that keeps the client.
		framework.Send(Default, Client, http.MethodPost, agentPath("/identities/retry?environment="+Cfg.DefaultEnv), nil)
		views, _ := agentops.ListAgentIdentities(ctx, Default, Client, Cfg.DefaultOrg, proj, agentName, Cfg.DefaultEnv)
		Expect(views[0].ClientID).To(Equal(clientID))

		agents := framework.CallObj(Default, Client, http.MethodGet, ai("/agents"), nil, http.StatusOK)
		for _, a := range framework.Items(agents, "agents") {
			if framework.Str(a, "agentName") == agentName {
				identityID = framework.Str(a, "thunderAgentId")
			}
		}
		Expect(identityID).NotTo(BeEmpty(), "the environment's agents listing has no identity for %s", agentName)
	})

	It("mints a token with its client credentials and loses the old secret on rotation", func(ctx context.Context) {
		first := agentops.RegenerateAgentIdentitySecret(ctx, Default, Client, Cfg.DefaultOrg, proj, agentName, Cfg.DefaultEnv)
		Expect(first.ClientID).To(Equal(clientID))
		Eventually(func(g Gomega) {
			res, err := framework.RequestClientCredentialsToken(ctx, tokenURL, clientID, first.ClientSecret)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(res.StatusCode).To(Equal(http.StatusOK))
			g.Expect(res.AccessToken).NotTo(BeEmpty())
		}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

		rotated := agentops.RegenerateAgentIdentitySecret(ctx, Default, Client, Cfg.DefaultOrg, proj, agentName, Cfg.DefaultEnv)
		Expect(rotated.ClientSecret).NotTo(Equal(first.ClientSecret))
		secret = rotated.ClientSecret
		Eventually(func(g Gomega) {
			res, err := framework.RequestClientCredentialsToken(ctx, tokenURL, clientID, first.ClientSecret)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(res.StatusCode).To(BeNumerically(">=", 400), "the rotated-out secret still mints tokens")
		}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		res, err := framework.RequestClientCredentialsToken(ctx, tokenURL, clientID, secret)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.StatusCode).To(Equal(http.StatusOK))
	})

	It("groups the agent and gives the group a role", func() {
		scopes := framework.CallObj(Default, Client, http.MethodGet, ai("/scopes"), nil, http.StatusOK)
		Expect(scopes).To(HaveKey("scopes"))

		role := framework.CallObj(Default, Client, http.MethodPost, ai("/roles"),
			framework.Obj{"name": "e2e-test-agent-role-" + suffix, "description": "journey"}, http.StatusCreated)
		roleID = framework.Str(role, "id")
		framework.CallObj(Default, Client, http.MethodPut, ai("/roles/%s", roleID),
			framework.Obj{"name": "e2e-test-agent-role-" + suffix, "description": "renamed"}, http.StatusOK)
		Expect(framework.CallObj(Default, Client, http.MethodGet, ai("/roles/%s", roleID), nil, http.StatusOK)["description"]).To(Equal("renamed"))
		Expect(framework.Field(framework.Items(framework.CallObj(Default, Client, http.MethodGet, ai("/roles"), nil, http.StatusOK), "roles"), "id")).
			To(ContainElement(roleID))

		group := framework.CallObj(Default, Client, http.MethodPost, ai("/groups"),
			framework.Obj{"name": "e2e-test-agent-group-" + suffix}, http.StatusCreated)
		groupID = framework.Str(group, "id")
		framework.CallObj(Default, Client, http.MethodPut, ai("/groups/%s", groupID),
			framework.Obj{"name": "e2e-test-agent-group-" + suffix, "description": "renamed"}, http.StatusOK)
		Expect(framework.CallObj(Default, Client, http.MethodGet, ai("/groups/%s", groupID), nil, http.StatusOK)["description"]).To(Equal("renamed"))
		Expect(framework.Field(framework.Items(framework.CallObj(Default, Client, http.MethodGet, ai("/groups"), nil, http.StatusOK), "groups"), "id")).
			To(ContainElement(groupID))

		framework.Call(Default, Client, http.MethodPost, ai("/groups/%s/members/add", groupID),
			framework.Obj{"agentIds": []string{identityID}}, http.StatusOK)
		framework.CallError(Default, Client, http.MethodPost, ai("/groups/%s/members/add", groupID),
			framework.Obj{"agentIds": []string{uuid.NewString()}}, http.StatusBadRequest)
		members := framework.CallObj(Default, Client, http.MethodGet, ai("/groups/%s/members", groupID), nil, http.StatusOK)
		Expect(framework.Items(members, "members")).To(HaveLen(1))

		framework.Call(Default, Client, http.MethodPost, ai("/roles/%s/assignments/add", roleID), framework.AgentIdentityAssignmentsRequest{
			Assignments: []framework.AgentIdentityAssignment{{ID: groupID, Type: "group"}},
		}, http.StatusOK)
		assigned := framework.CallObj(Default, Client, http.MethodGet, ai("/roles/%s/assignments", roleID), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(assigned, "groups"), "id")).To(ContainElement(groupID))
		groupRoles := framework.CallObj(Default, Client, http.MethodGet, ai("/groups/%s/roles", groupID), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(groupRoles, "roles"), "id")).To(ContainElement(roleID))

		// The agent's own view lists the group and, through it, the role.
		agentGroups := framework.CallObj(Default, Client, http.MethodGet, agentPath("/groups?environment="+Cfg.DefaultEnv), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(agentGroups, "groups"), "id")).To(ContainElement(groupID))
		framework.Call(Default, Client, http.MethodGet, agentPath("/roles?environment="+Cfg.DefaultEnv), nil, http.StatusOK)
	})

	It("takes the role and the group away again", func() {
		framework.Call(Default, Client, http.MethodPost, ai("/roles/%s/assignments/remove", roleID), framework.AgentIdentityAssignmentsRequest{
			Assignments: []framework.AgentIdentityAssignment{{ID: groupID, Type: "group"}},
		}, http.StatusOK)
		assigned := framework.CallObj(Default, Client, http.MethodGet, ai("/roles/%s/assignments", roleID), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(assigned, "groups"), "id")).NotTo(ContainElement(groupID))
		framework.Call(Default, Client, http.MethodPost, ai("/groups/%s/members/remove", groupID),
			framework.Obj{"agentIds": []string{identityID}}, http.StatusOK)

		framework.Call(Default, Client, http.MethodDelete, ai("/roles/%s", roleID), nil, http.StatusNoContent)
		framework.Call(Default, Client, http.MethodDelete, ai("/groups/%s", groupID), nil, http.StatusNoContent)
		framework.CallError(Default, Client, http.MethodGet, ai("/roles/%s", roleID), nil, http.StatusNotFound)
		framework.CallError(Default, Client, http.MethodGet, ai("/groups/%s", groupID), nil, http.StatusNotFound)
		roleID, groupID = "", ""
	})

	It("revokes the identity's secret so no new token can be minted", func(ctx context.Context) {
		framework.Call(Default, Client, http.MethodDelete, agentPath("/identities?environment="+Cfg.DefaultEnv), nil, http.StatusOK)
		Eventually(func(g Gomega) {
			res, err := framework.RequestClientCredentialsToken(ctx, tokenURL, clientID, secret)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(res.StatusCode).To(BeNumerically(">=", 400))
		}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
	})
})

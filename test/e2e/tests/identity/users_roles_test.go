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

// J7: an org admin invites and creates a user, puts them in a group, gives the
// group a custom role, and the role decides what its holder can do: allowed
// calls succeed, denied ones get 403 and change nothing, and revoking a
// permission takes it away from the next token.

package identity

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

const (
	// allowedScope is what the custom role grants; deniedScope is what it doesn't.
	allowedScope = "amp:project:read"
	extraScope   = "amp:environment:read"
	deniedScope  = "amp:project:create"
)

var _ = Describe("Users, groups and custom roles", Label("identity"), Ordered, func() {
	var (
		suffix   string
		username string
		userID   string
		groupID  string
		roleID   string
		roleName string
		resSrv   string
		persona  *framework.RolePersona
	)
	id := func(format string, args ...any) string { return framework.API(Cfg, "/identities"+format, args...) }

	BeforeAll(func() {
		suffix = uuid.NewString()[:8]
		username = "e2e-test-user-" + suffix
		roleName = "e2e-test-role-" + suffix
		DeferCleanup(func(ctx context.Context) {
			if persona != nil {
				_ = Provisioner.DeleteRolePersona(ctx, persona)
			}
			for kind, rid := range map[string]string{"roles": roleID, "groups": groupID, "users": userID} {
				if rid != "" {
					framework.Send(Default, Client, http.MethodDelete, id("/%s/%s", kind, rid), nil)
				}
			}
		})
	})

	It("invites a user by email and returns a sign-up link", func() {
		out := framework.CallObj(Default, Client, http.MethodPost, id("/users/invite"),
			framework.Obj{"email": username + "@example.com"}, http.StatusOK)
		Expect(framework.Str(out, "inviteLink")).To(ContainSubstring("inviteToken="))

		framework.CallError(Default, Client, http.MethodPost, id("/users/invite"), framework.Obj{"email": ""}, http.StatusBadRequest)
	})

	It("creates a user, who is then listed and readable", func() {
		u := framework.CallObj(Default, Client, http.MethodPost, id("/users"), framework.Obj{
			"type": "Person",
			"attributes": framework.Obj{
				"username": username, "email": username + "@example.com",
				"password": "E2e!" + uuid.NewString(), "given_name": "E2E", "family_name": "User",
			},
		}, http.StatusCreated)
		userID = framework.Str(u, "id")
		Expect(userID).NotTo(BeEmpty())
		Expect(framework.Str(u, "attributes", "username")).To(Equal(username))
		Expect(u["attributes"]).NotTo(HaveKey("password"), "the password must never be returned")

		got := framework.CallObj(Default, Client, http.MethodGet, id("/users/%s", userID), nil, http.StatusOK)
		Expect(framework.Str(got, "attributes", "email")).To(Equal(username + "@example.com"))

		Eventually(func(g Gomega) {
			list := framework.CallObj(g, Client, http.MethodGet, id("/users?limit=100"), nil, http.StatusOK)
			g.Expect(framework.Field(framework.Items(list, "users"), "id")).To(ContainElement(userID))
		}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

		framework.CallError(Default, Client, http.MethodGet, id("/users/%s", uuid.NewString()), nil, http.StatusNotFound)
	})

	It("updates the user's attributes", func() {
		framework.CallObj(Default, Client, http.MethodPut, id("/users/%s", userID), framework.Obj{"attributes": framework.Obj{
			"username": username, "email": username + "@example.com", "given_name": "Renamed", "family_name": "User",
		}}, http.StatusOK)
		got := framework.CallObj(Default, Client, http.MethodGet, id("/users/%s", userID), nil, http.StatusOK)
		Expect(framework.Str(got, "attributes", "given_name")).To(Equal("Renamed"))

		// The update replaces every attribute, so leaving out a required one is refused.
		framework.CallError(Default, Client, http.MethodPut, id("/users/%s", userID),
			framework.Obj{"attributes": framework.Obj{"given_name": "Partial"}}, http.StatusBadRequest)
		got = framework.CallObj(Default, Client, http.MethodGet, id("/users/%s", userID), nil, http.StatusOK)
		Expect(framework.Str(got, "attributes", "given_name")).To(Equal("Renamed"))
	})

	It("keeps profiles private to their owner", func() {
		before := framework.CallObj(Default, Client, http.MethodGet, id("/users/%s", userID), nil, http.StatusOK)
		framework.CallError(Default, Client, http.MethodGet, id("/users/%s/profile", userID), nil, http.StatusForbidden)
		framework.CallError(Default, Client, http.MethodPut, id("/users/%s/profile", userID),
			framework.Obj{"attributes": framework.Obj{"given_name": "Hijacked"}}, http.StatusForbidden)
		after := framework.CallObj(Default, Client, http.MethodGet, id("/users/%s", userID), nil, http.StatusOK)
		Expect(after["attributes"]).To(Equal(before["attributes"]))

		// The admin here is a machine client, so it has no user profile of its own.
		framework.CallError(Default, Client, http.MethodGet, id("/users/%s/profile", Cfg.IDPClientID), nil, http.StatusNotFound)
		framework.CallError(Default, Client, http.MethodPut, id("/users/%s/profile", Cfg.IDPClientID),
			framework.Obj{"attributes": framework.Obj{"given_name": "x"}}, http.StatusNotFound)
	})

	It("creates a group, renames it and adds the user to it", func() {
		g := framework.CallObj(Default, Client, http.MethodPost, id("/groups"),
			framework.Obj{"name": "e2e-test-group-" + suffix, "description": "E2E group"}, http.StatusCreated)
		groupID = framework.Str(g, "id")
		Expect(groupID).NotTo(BeEmpty())

		framework.CallObj(Default, Client, http.MethodPut, id("/groups/%s", groupID),
			framework.Obj{"name": "e2e-test-group-" + suffix + "-b", "description": "Renamed"}, http.StatusOK)
		got := framework.CallObj(Default, Client, http.MethodGet, id("/groups/%s", groupID), nil, http.StatusOK)
		Expect(got["name"]).To(Equal("e2e-test-group-" + suffix + "-b"))
		list := framework.CallObj(Default, Client, http.MethodGet, id("/groups?limit=100"), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(list, "groups"), "id")).To(ContainElement(groupID))

		framework.Call(Default, Client, http.MethodPost, id("/groups/%s/members/add", groupID), framework.Obj{"userIds": []string{userID}}, http.StatusOK)
		members := framework.CallObj(Default, Client, http.MethodGet, id("/groups/%s/members", groupID), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(members, "users"), "id")).To(ConsistOf(userID))
		groups := framework.CallObj(Default, Client, http.MethodGet, id("/users/%s/groups", userID), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(groups, "groups"), "id")).To(ContainElement(groupID))

		framework.CallError(Default, Client, http.MethodPost, id("/groups/%s/members/add", uuid.NewString()),
			framework.Obj{"userIds": []string{userID}}, http.StatusNotFound)
	})

	It("creates a custom role granting a read permission and gives it to the group", func() {
		catalog := framework.CallObj(Default, Client, http.MethodGet, id("/permissions"), nil, http.StatusOK)
		perms := framework.Items(catalog, "permissions")
		Expect(framework.Field(perms, "name")).To(ContainElements(allowedScope, deniedScope))
		for _, p := range perms {
			if framework.Str(p, "name") == allowedScope {
				resSrv = framework.Str(p, "resourceServerId")
			}
		}
		Expect(resSrv).NotTo(BeEmpty())

		r := framework.CallObj(Default, Client, http.MethodPost, id("/roles"),
			framework.Obj{"name": roleName, "description": "E2E custom role"}, http.StatusCreated)
		roleID = framework.Str(r, "id")
		Expect(r["isReadOnly"]).To(BeFalse())

		framework.CallObj(Default, Client, http.MethodPut, id("/roles/%s", roleID),
			framework.Obj{"name": roleName, "description": "Reads projects"}, http.StatusOK)
		framework.Call(Default, Client, http.MethodPost, id("/roles/%s/permissions/add", roleID),
			framework.Obj{"resourceServerId": resSrv, "permissions": []string{allowedScope, extraScope}}, http.StatusOK)
		framework.Call(Default, Client, http.MethodPost, id("/roles/%s/assignees/add", roleID),
			framework.Obj{"groupIds": []string{groupID}}, http.StatusOK)

		got := framework.CallObj(Default, Client, http.MethodGet, id("/roles/%s", roleID), nil, http.StatusOK)
		Expect(got["description"]).To(Equal("Reads projects"))
		Expect(got).To(HaveKey("permissions"))
		roles := framework.CallObj(Default, Client, http.MethodGet, id("/roles?limit=100"), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(roles, "roles"), "id")).To(ContainElement(roleID))
		assigned := framework.CallObj(Default, Client, http.MethodGet, id("/roles/%s/assignments", roleID), nil, http.StatusOK)
		Expect(assigned).To(HaveKey("groups"))
		Expect(framework.Field(framework.Items(assigned, "groups"), "id")).To(ContainElement(groupID))
		groupRoles := framework.CallObj(Default, Client, http.MethodGet, id("/groups/%s/roles", groupID), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(groupRoles, "roles"), "id")).To(ContainElement(roleID))
	})

	It("refuses to edit or delete a predefined role", func() {
		framework.CallError(Default, Client, http.MethodPut, id("/roles/amp-role-admin"), framework.Obj{"name": "x"}, http.StatusBadRequest)
		framework.CallError(Default, Client, http.MethodDelete, id("/roles/amp-role-admin"), nil, http.StatusBadRequest)
		got := framework.CallObj(Default, Client, http.MethodGet, id("/roles/amp-role-admin"), nil, http.StatusOK)
		Expect(got["isReadOnly"]).To(BeTrue())
	})

	It("lets the role's holder do what the role allows and nothing else", func(ctx context.Context) {
		var err error
		persona, err = Provisioner.CreateRolePersona(ctx, roleName)
		Expect(err).NotTo(HaveOccurred())
		holder := framework.NewAMPClientWithToken(Cfg, persona.Token)

		framework.Call(Default, holder, http.MethodGet, framework.API(Cfg, "/projects"), nil, http.StatusOK)

		denied := "e2e-test-denied-" + suffix
		resp, err := holder.Post(framework.API(Cfg, "/projects"), framework.NewCreateProjectRequest(denied, "Denied", "", "default"))
		Expect(err).NotTo(HaveOccurred())
		framework.ExpectForbidden(Default, resp, "creating a project with a read-only role")
		resp.Body.Close()
		framework.CallError(Default, Client, http.MethodGet, framework.API(Cfg, "/projects/%s", denied), nil, http.StatusNotFound)

		// Neither may the holder widen its own role.
		resp, err = holder.Post(id("/roles/%s/permissions/add", roleID),
			framework.Obj{"resourceServerId": resSrv, "permissions": []string{deniedScope}})
		Expect(err).NotTo(HaveOccurred())
		framework.ExpectForbidden(Default, resp, "a role holder granting itself a permission")
		resp.Body.Close()
	})

	It("takes a revoked permission away from the holder's next token", func(ctx context.Context) {
		framework.Call(Default, Client, http.MethodPost, id("/roles/%s/permissions/remove", roleID),
			framework.Obj{"resourceServerId": resSrv, "permissions": []string{allowedScope}}, http.StatusOK)

		var token string
		Eventually(func(g Gomega) {
			var err error
			token, err = Provisioner.RefreshRolePersonaToken(ctx, persona)
			g.Expect(err).NotTo(HaveOccurred())
			scopes, err := framework.TokenScopes(token)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(scopes).NotTo(HaveKey(allowedScope))
		}).WithTimeout(15 * time.Second).WithPolling(time.Second).Should(Succeed())

		resp, err := framework.NewAMPClientWithToken(Cfg, token).Get(framework.API(Cfg, "/projects"))
		Expect(err).NotTo(HaveOccurred())
		framework.ExpectForbidden(Default, resp, "listing projects after the permission was revoked")
		resp.Body.Close()
	})

	It("assigns the role to a user directly and takes it back", func() {
		framework.Call(Default, Client, http.MethodPost, id("/roles/%s/assignees/add", roleID), framework.Obj{"userIds": []string{userID}}, http.StatusOK)
		roles := framework.CallObj(Default, Client, http.MethodGet, id("/users/%s/roles", userID), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(roles, "roles"), "id")).To(ContainElement(roleID))

		framework.Call(Default, Client, http.MethodPost, id("/roles/%s/assignees/remove", roleID),
			framework.Obj{"userIds": []string{userID}, "groupIds": []string{groupID}}, http.StatusOK)
		roles = framework.CallObj(Default, Client, http.MethodGet, id("/users/%s/roles", userID), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(roles, "roles"), "id")).NotTo(ContainElement(roleID))
		groupRoles := framework.CallObj(Default, Client, http.MethodGet, id("/groups/%s/roles", groupID), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(groupRoles, "roles"), "id")).NotTo(ContainElement(roleID))

		framework.CallError(Default, Client, http.MethodPost, id("/roles/%s/assignees/add", roleID), framework.Obj{}, http.StatusBadRequest)
	})

	It("removes the user from the group", func() {
		framework.Call(Default, Client, http.MethodPost, id("/groups/%s/members/remove", groupID), framework.Obj{"userIds": []string{userID}}, http.StatusOK)
		members := framework.CallObj(Default, Client, http.MethodGet, id("/groups/%s/members", groupID), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(members, "users"), "id")).NotTo(ContainElement(userID))
	})

	It("deletes the role, the group and the user", func(ctx context.Context) {
		Expect(Provisioner.DeleteRolePersona(ctx, persona)).To(Succeed())
		persona = nil
		for _, p := range []string{"/roles/" + roleID, "/groups/" + groupID, "/users/" + userID} {
			framework.Call(Default, Client, http.MethodDelete, id("%s", p), nil, http.StatusNoContent)
			framework.CallError(Default, Client, http.MethodGet, id("%s", p), nil, http.StatusNotFound)
		}
		roleID, groupID, userID = "", "", ""
	})
})

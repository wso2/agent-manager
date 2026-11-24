/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { fireEvent, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type {
  ThunderGroup,
  ThunderPermission,
  ThunderRole,
  ThunderUser,
} from "@agent-management-platform/types";
import { renderRoute } from "./__test__/renderRoute";

const hoisted = vi.hoisted(() => ({
  loading: false,
  role: undefined as unknown as ThunderRole,
  assignedUsers: [] as ThunderUser[],
  assignedGroups: [] as ThunderGroup[],
  allUsers: [] as ThunderUser[],
  allGroups: [] as ThunderGroup[],
  catalog: [] as ThunderPermission[],
  addAssignees: vi.fn(),
  removeAssignees: vi.fn(),
  addPermissions: vi.fn(),
  removePermissions: vi.fn(),
  canWrite: true,
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useGetRole: () => ({ data: hoisted.role, isLoading: hoisted.loading }),
  useGetRoleAssignments: () => ({
    data: { users: hoisted.assignedUsers, groups: hoisted.assignedGroups },
    isLoading: false,
  }),
  useAllUsers: () => ({ data: { users: hoisted.allUsers }, isLoading: false }),
  useAllGroups: () => ({ data: { groups: hoisted.allGroups }, isLoading: false }),
  useListAMPPermissions: () => ({
    data: { permissions: hoisted.catalog, resourceServerId: "rs-1" },
    isLoading: false,
  }),
  useAddRoleAssignees: () => ({ mutateAsync: hoisted.addAssignees }),
  useRemoveRoleAssignees: () => ({ mutateAsync: hoisted.removeAssignees }),
  useAddRolePermissions: () => ({ mutateAsync: hoisted.addPermissions }),
  useRemoveRolePermissions: () => ({ mutateAsync: hoisted.removePermissions }),
}));
vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useResourceAccess: () => ({ canRead: true, canWrite: hoisted.canWrite }),
}));

import { RoleEditPage } from "./RoleEditPage";

const perm = (name: string, resourceName = "", actionName = ""): ThunderPermission => ({
  name,
  resourceServerId: "rs-1",
  resourceName,
  actionName,
});
const user = (id: string, username: string): ThunderUser => ({ id, attributes: { username } });
const PARAMS = { orgName: "acme", roleId: "r1" };

const renderPage = () =>
  renderRoute(
    "/org/acme/settings/identities/roles/r1",
    "/org/:orgId/settings/identities/roles/:roleId",
    <RoleEditPage />,
  );

const save = () => fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
const openTab = (name: string) => fireEvent.click(screen.getByRole("tab", { name }));

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.loading = false;
  hoisted.role = {
    id: "r1",
    name: "Operator",
    description: "Runs things",
    permissions: [{ resourceServerId: "rs-1", permissions: ["agent:read"] }],
  };
  hoisted.assignedUsers = [user("u1", "alice")];
  hoisted.assignedGroups = [{ id: "g1", name: "Engineering", description: "Builders" }];
  hoisted.allUsers = [user("u1", "alice"), user("u2", "bob")];
  hoisted.allGroups = [
    { id: "g1", name: "Engineering", description: "Builders" },
    { id: "g2", name: "Sales" },
  ];
  // Labels come from resource/action names, or from the "resource:action" name.
  hoisted.catalog = [
    perm("agent:read", "agent", "read"),
    perm("agent:write", "agent", "write"),
    perm("role:read"),
  ];
  hoisted.canWrite = true;
  for (const fn of [
    hoisted.addAssignees,
    hoisted.removeAssignees,
    hoisted.addPermissions,
    hoisted.removePermissions,
  ]) {
    fn.mockResolvedValue(undefined);
  }
});

describe("RoleEditPage", () => {
  it("shows a skeleton while the role loads", () => {
    hoisted.loading = true;
    renderPage();

    expect(screen.getByRole("link", { name: "Roles" })).toBeInTheDocument();
    expect(screen.queryByText("Operator")).not.toBeInTheDocument();
  });

  describe("Permissions tab", () => {
    it("checks the role's current permissions in the catalog tree", () => {
      renderPage();

      expect(screen.getByText("Operator")).toBeInTheDocument();
      expect(screen.getByText("Check the permissions to assign to this role.")).toBeInTheDocument();
      expect(screen.getByRole("checkbox", { name: "Agent / Read" })).toBeChecked();
      expect(screen.getByRole("checkbox", { name: "Agent / Write" })).not.toBeChecked();
      expect(screen.getByRole("checkbox", { name: "Role / Read" })).not.toBeChecked();
      expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
    });

    it("saves added and removed permissions against the catalog's resource server", async () => {
      renderPage();

      fireEvent.click(screen.getByRole("checkbox", { name: "Agent / Write" }));
      fireEvent.click(screen.getByRole("checkbox", { name: "Agent / Read" }));
      save();

      expect(await screen.findByText("Role updated successfully.")).toBeInTheDocument();
      expect(hoisted.addPermissions).toHaveBeenCalledWith({
        params: PARAMS,
        body: { resourceServerId: "rs-1", permissions: ["agent:write"] },
      });
      expect(hoisted.removePermissions).toHaveBeenCalledWith({
        params: PARAMS,
        body: { resourceServerId: "rs-1", permissions: ["agent:read"] },
      });
      expect(hoisted.addAssignees).not.toHaveBeenCalled();
    });

    it("locks permissions on a predefined role", () => {
      hoisted.role = { ...hoisted.role, isReadOnly: true };
      renderPage();

      expect(screen.getByText("Read-only")).toBeInTheDocument();
      expect(
        screen.getByText("Permissions for predefined roles cannot be modified."),
      ).toBeInTheDocument();
      expect(screen.getByRole("checkbox", { name: "Agent / Write" })).toBeDisabled();
    });

    it("shows permissions read-only without role write access", () => {
      hoisted.canWrite = false;
      renderPage();

      expect(screen.getByText("Permissions assigned to this role.")).toBeInTheDocument();
      expect(screen.getByRole("checkbox", { name: "Agent / Write" })).toBeDisabled();
    });

    it("says when the catalog is empty", () => {
      hoisted.catalog = [];
      renderPage();

      expect(screen.getByText("No permissions available.")).toBeInTheDocument();
    });
  });

  describe("Users tab", () => {
    it("adds a user and removes an existing one on save", async () => {
      renderPage();
      openTab("Users");

      expect(screen.getByText("alice")).toBeInTheDocument();
      fireEvent.mouseDown(screen.getByPlaceholderText("Search users..."));
      expect(screen.queryByRole("option", { name: "alice" })).not.toBeInTheDocument();
      fireEvent.click(screen.getByRole("option", { name: "bob" }));
      fireEvent.click(screen.getAllByRole("button", { name: "Remove from role" })[0]);
      expect(screen.queryByText("alice")).not.toBeInTheDocument();
      save();

      await screen.findByText("Role updated successfully.");
      expect(hoisted.addAssignees).toHaveBeenCalledWith({
        params: PARAMS,
        body: { userIds: ["u2"] },
      });
      expect(hoisted.removeAssignees).toHaveBeenCalledWith({
        params: PARAMS,
        body: { userIds: ["u1"] },
      });
      expect(hoisted.addPermissions).not.toHaveBeenCalled();
    });

    it("undoes changes without leaving anything to save", () => {
      renderPage();
      openTab("Users");

      // Remove alice, then add her back; add bob, then remove him.
      fireEvent.click(screen.getByRole("button", { name: "Remove from role" }));
      fireEvent.mouseDown(screen.getByPlaceholderText("Search users..."));
      fireEvent.click(screen.getByRole("option", { name: "alice" }));
      fireEvent.mouseDown(screen.getByPlaceholderText("Search users..."));
      fireEvent.click(screen.getByRole("option", { name: "bob" }));
      fireEvent.click(screen.getAllByRole("button", { name: "Remove from role" })[1]);

      expect(screen.getByText("alice")).toBeInTheDocument();
      expect(screen.queryByText("bob")).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
    });

    it("shows an empty state when nobody is assigned", () => {
      hoisted.assignedUsers = [];
      renderPage();
      openTab("Users");

      expect(screen.getByText("No users assigned yet")).toBeInTheDocument();
      expect(screen.getByText("Search and add users above.")).toBeInTheDocument();
    });

    it("is read-only without role write access", () => {
      hoisted.canWrite = false;
      renderPage();
      openTab("Users");

      expect(screen.getByText("Users assigned to this role.")).toBeInTheDocument();
      expect(screen.queryByPlaceholderText("Search users...")).not.toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Remove from role" })).not.toBeInTheDocument();
    });
  });

  describe("Groups tab", () => {
    it("adds a group and removes an existing one on save", async () => {
      renderPage();
      openTab("Groups");

      expect(screen.getByText("Builders")).toBeInTheDocument();
      fireEvent.mouseDown(screen.getByPlaceholderText("Search groups..."));
      fireEvent.click(screen.getByRole("option", { name: "Sales" }));
      // A group without a description shows a dash.
      expect(screen.getByText("-")).toBeInTheDocument();
      fireEvent.click(screen.getAllByRole("button", { name: "Remove from role" })[0]);
      save();

      await screen.findByText("Role updated successfully.");
      expect(hoisted.addAssignees).toHaveBeenCalledWith({
        params: PARAMS,
        body: { groupIds: ["g2"] },
      });
      expect(hoisted.removeAssignees).toHaveBeenCalledWith({
        params: PARAMS,
        body: { groupIds: ["g1"] },
      });
    });

    it("re-adding a removed group just undoes the removal", () => {
      renderPage();
      openTab("Groups");

      fireEvent.click(screen.getByRole("button", { name: "Remove from role" }));
      expect(screen.getByText("No groups assigned yet")).toBeInTheDocument();
      fireEvent.mouseDown(screen.getByPlaceholderText("Search groups..."));
      fireEvent.click(screen.getByRole("option", { name: "Engineering" }));

      expect(screen.getByText("Builders")).toBeInTheDocument();
      expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
    });

    it("is read-only without role write access", () => {
      hoisted.canWrite = false;
      hoisted.assignedGroups = [];
      renderPage();
      openTab("Groups");

      expect(screen.getByText("Groups assigned to this role.")).toBeInTheDocument();
      expect(screen.getByText("No groups assigned yet")).toBeInTheDocument();
      expect(screen.queryByText("Search and add groups above.")).not.toBeInTheDocument();
    });
  });

  it("reports a failed save", async () => {
    hoisted.addPermissions.mockRejectedValue(new Error("nope"));
    renderPage();

    fireEvent.click(screen.getByRole("checkbox", { name: "Agent / Write" }));
    save();

    expect(await screen.findByText("Failed to update role. Please try again.")).toBeInTheDocument();
  });

  it("cancel returns to the roles list", () => {
    renderPage();

    fireEvent.click(screen.getByRole("checkbox", { name: "Agent / Write" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByText("at /org/acme/settings/identities/roles")).toBeInTheDocument();
  });
});

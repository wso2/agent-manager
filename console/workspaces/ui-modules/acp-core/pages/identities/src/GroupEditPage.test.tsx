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
import type { ThunderRole, ThunderUser } from "@agent-management-platform/types";
import { renderRoute } from "./__test__/renderRoute";

const hoisted = vi.hoisted(() => ({
  loading: false,
  members: [] as ThunderUser[],
  membersTotal: 0,
  memberArgs: [] as unknown[][],
  allUsers: [] as ThunderUser[],
  roles: {
    data: undefined as { roles: ThunderRole[] } | undefined,
    isLoading: false,
    isError: false,
  },
  addMembers: vi.fn(),
  removeMembers: vi.fn(),
  canWrite: true,
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useGetGroup: () => ({
    data: { id: "g1", name: "Engineering", description: "Builders" },
    isLoading: hoisted.loading,
  }),
  useGetGroupMembers: (...args: unknown[]) => {
    hoisted.memberArgs.push(args);
    return {
      data: { users: hoisted.members, total: hoisted.membersTotal },
      isLoading: false,
    };
  },
  useAllGroupMemberIds: () => ({
    data: { memberIds: hoisted.members.map((u) => u.id) },
    isLoading: false,
  }),
  useGetGroupRoles: () => hoisted.roles,
  useListUsers: () => ({ data: { users: hoisted.allUsers }, isLoading: false }),
  useAddGroupMembers: () => ({ mutateAsync: hoisted.addMembers }),
  useRemoveGroupMembers: () => ({ mutateAsync: hoisted.removeMembers }),
}));
vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useUnsavedChangesGuard: () => ({ allowNavigation: (fn: () => void) => fn() }),
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useResourceAccess: () => ({ canRead: true, canWrite: hoisted.canWrite }),
}));

import { GroupEditPage } from "./GroupEditPage";

const user = (id: string, username: string): ThunderUser => ({ id, attributes: { username } });
const alice = user("u1", "alice");
const bob = user("u2", "bob");

const renderPage = () =>
  renderRoute(
    "/org/acme/settings/identities/groups/g1",
    "/org/:orgId/settings/identities/groups/:groupId",
    <GroupEditPage />,
  );

const openUserPicker = () => fireEvent.mouseDown(screen.getByPlaceholderText("Search users..."));

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.loading = false;
  hoisted.members = [alice];
  hoisted.membersTotal = 1;
  hoisted.memberArgs = [];
  hoisted.allUsers = [alice, bob];
  hoisted.roles = { data: { roles: [] }, isLoading: false, isError: false };
  hoisted.canWrite = true;
  hoisted.addMembers.mockResolvedValue(undefined);
  hoisted.removeMembers.mockResolvedValue(undefined);
});

describe("GroupEditPage", () => {
  it("shows a skeleton while the group loads", () => {
    hoisted.loading = true;
    renderPage();

    expect(screen.getByRole("link", { name: "Groups" })).toBeInTheDocument();
    expect(screen.queryByText("Engineering")).not.toBeInTheDocument();
  });

  it("shows the group and its members", () => {
    renderPage();

    expect(screen.getByText("Engineering")).toBeInTheDocument();
    expect(screen.getByText("Builders")).toBeInTheDocument();
    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(hoisted.memberArgs[0]).toEqual([
      { orgName: "acme", groupId: "g1" },
      { offset: 0, limit: 10 },
    ]);
  });

  it("only offers users who are not members yet", () => {
    renderPage();

    openUserPicker();
    expect(screen.getByRole("option", { name: "bob" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "alice" })).not.toBeInTheDocument();
  });

  it("stages an added user and saves the new membership", async () => {
    renderPage();

    openUserPicker();
    fireEvent.click(screen.getByRole("option", { name: "bob" }));
    expect(screen.getByText("Pending additions (unsaved)")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(await screen.findByText("Group updated successfully.")).toBeInTheDocument();
    expect(hoisted.addMembers).toHaveBeenCalledWith({
      params: { orgName: "acme", groupId: "g1" },
      body: { userIds: ["u2"] },
    });
    expect(hoisted.removeMembers).not.toHaveBeenCalled();
    expect(screen.queryByText("Pending additions (unsaved)")).not.toBeInTheDocument();
  });

  it("removes an existing member on save", async () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Remove from group" }));
    expect(screen.queryByText("alice")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await screen.findByText("Group updated successfully.");
    expect(hoisted.removeMembers).toHaveBeenCalledWith({
      params: { orgName: "acme", groupId: "g1" },
      body: { userIds: ["u1"] },
    });
    expect(hoisted.addMembers).not.toHaveBeenCalled();
  });

  it("drops a pending addition when it is removed again", () => {
    renderPage();

    openUserPicker();
    fireEvent.click(screen.getByRole("option", { name: "bob" }));
    fireEvent.click(screen.getAllByRole("button", { name: "Remove from group" })[0]);

    expect(screen.queryByText("Pending additions (unsaved)")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
  });

  it("re-adding a removed member just undoes the removal", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Remove from group" }));
    openUserPicker();
    fireEvent.click(screen.getByRole("option", { name: "alice" }));

    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(screen.queryByText("Pending additions (unsaved)")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
  });

  it("reports a failed save", async () => {
    hoisted.addMembers.mockRejectedValue(new Error("nope"));
    renderPage();

    openUserPicker();
    fireEvent.click(screen.getByRole("option", { name: "bob" }));
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(
      await screen.findByText("Failed to update group members. Please try again."),
    ).toBeInTheDocument();
  });

  it("cancel goes back to the groups list", () => {
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Remove from group" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByText("at /org/acme/settings/identities/groups")).toBeInTheDocument();
  });

  it("shows an empty state when the group has no members", () => {
    hoisted.members = [];
    hoisted.membersTotal = 0;
    renderPage();

    expect(screen.getByText("No members yet")).toBeInTheDocument();
    expect(screen.getByText("Search and add users above.")).toBeInTheDocument();
  });

  it("is read-only without group write access", () => {
    hoisted.canWrite = false;
    renderPage();

    expect(screen.getByText("Users in this group.")).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Search users...")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Remove from group" })).not.toBeInTheDocument();
  });

  it("pages through members", () => {
    hoisted.members = Array.from({ length: 10 }, (_, i) => user(`m${i}`, `member${i}`));
    hoisted.membersTotal = 12;
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: /next page/i }));
    expect(hoisted.memberArgs[hoisted.memberArgs.length - 1]).toEqual([
      { orgName: "acme", groupId: "g1" },
      { offset: 10, limit: 10 },
    ]);
  });

  describe("Roles tab", () => {
    const openRoles = () => fireEvent.click(screen.getByRole("tab", { name: "Roles" }));

    it("lists the group's roles", () => {
      hoisted.roles.data = { roles: [{ id: "r1", name: "Admin", description: "Everything" }] };
      renderPage();
      openRoles();

      expect(screen.getByText("Admin")).toBeInTheDocument();
      expect(screen.getByText("Everything")).toBeInTheDocument();
    });

    it("shows a spinner while roles load", () => {
      hoisted.roles.isLoading = true;
      renderPage();
      openRoles();

      expect(screen.getByRole("progressbar")).toBeInTheDocument();
    });

    it("shows an error when roles fail to load", () => {
      hoisted.roles.isError = true;
      renderPage();
      openRoles();

      expect(screen.getByText("Failed to load roles. Please try again.")).toBeInTheDocument();
    });

    it("says when no roles are assigned", () => {
      renderPage();
      openRoles();

      expect(screen.getByText("No roles assigned to this group")).toBeInTheDocument();
    });
  });
});

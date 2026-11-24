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

import { fireEvent, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ThunderGroup, ThunderRole } from "@agent-management-platform/types";
import { renderRoute } from "./__test__/renderRoute";

const hoisted = vi.hoisted(() => ({
  loading: false,
  userGroups: [] as ThunderGroup[],
  userRoles: [] as ThunderRole[],
  allGroups: [] as ThunderGroup[],
  addMembers: vi.fn(),
  removeMembers: vi.fn(),
  canWrite: true,
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useGetUser: () => ({
    data: { id: "u1", attributes: { username: "jane", email: "jane@example.com" } },
    isLoading: hoisted.loading,
  }),
  useGetUserGroups: () => ({ data: { groups: hoisted.userGroups }, isLoading: false }),
  useGetUserRoles: () => ({ data: { roles: hoisted.userRoles }, isLoading: false }),
  useAllGroups: () => ({ data: { groups: hoisted.allGroups }, isLoading: false }),
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

import { UserEditPage } from "./UserEditPage";

const eng: ThunderGroup = { id: "g1", name: "Engineering" };
const ops: ThunderGroup = { id: "g2", name: "Ops" };

const renderPage = () =>
  renderRoute(
    "/org/acme/settings/identities/users/u1",
    "/org/:orgId/settings/identities/users/:userId",
    <UserEditPage />,
  );

const pickGroup = (name: string) => {
  fireEvent.mouseDown(screen.getByPlaceholderText("Search groups..."));
  fireEvent.click(screen.getByRole("option", { name }));
};

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.loading = false;
  hoisted.userGroups = [eng];
  hoisted.userRoles = [];
  hoisted.allGroups = [eng, ops];
  hoisted.canWrite = true;
  hoisted.addMembers.mockResolvedValue(undefined);
  hoisted.removeMembers.mockResolvedValue(undefined);
});

describe("UserEditPage", () => {
  it("shows a skeleton while the user loads", () => {
    hoisted.loading = true;
    renderPage();

    expect(screen.getByRole("link", { name: "Users" })).toBeInTheDocument();
    expect(screen.queryByText("Group Memberships")).not.toBeInTheDocument();
  });

  it("shows the user's header and current groups", () => {
    renderPage();

    expect(screen.getByText("jane")).toBeInTheDocument();
    expect(screen.getByText("jane@example.com")).toBeInTheDocument();
    expect(screen.getByText("Engineering")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
  });

  it("adds and removes group memberships on save", async () => {
    renderPage();

    pickGroup("Ops");
    const engChip = screen.getByText("Engineering").closest(".MuiChip-root") as HTMLElement;
    fireEvent.click(within(engChip).getByTestId("CancelIcon"));
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(await screen.findByText("User updated successfully.")).toBeInTheDocument();
    expect(hoisted.addMembers).toHaveBeenCalledWith({
      params: { orgName: "acme", groupId: "g2" },
      body: { userIds: ["u1"] },
    });
    expect(hoisted.removeMembers).toHaveBeenCalledWith({
      params: { orgName: "acme", groupId: "g1" },
      body: { userIds: ["u1"] },
    });
  });

  it("hides the action row again when an edit is undone", () => {
    renderPage();

    pickGroup("Ops");
    expect(screen.getByRole("button", { name: "Save changes" })).toBeInTheDocument();
    const opsChip = screen.getByText("Ops").closest(".MuiChip-root") as HTMLElement;
    fireEvent.click(within(opsChip).getByTestId("CancelIcon"));
    expect(screen.queryByRole("button", { name: "Save changes" })).not.toBeInTheDocument();
  });

  it("reports a failed save", async () => {
    hoisted.addMembers.mockRejectedValue(new Error("nope"));
    renderPage();

    pickGroup("Ops");
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    expect(
      await screen.findByText("Failed to update group memberships. Please try again."),
    ).toBeInTheDocument();
  });

  it("cancel returns to the users list", () => {
    renderPage();

    pickGroup("Ops");
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.getByText("at /org/acme/settings/identities/users")).toBeInTheDocument();
  });

  it("is read-only without group write access", () => {
    hoisted.canWrite = false;
    renderPage();

    expect(screen.getByText("Groups this user belongs to.")).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Search groups...")).not.toBeInTheDocument();
    expect(screen.queryByTestId("CancelIcon")).not.toBeInTheDocument();
  });

  it("says when a read-only user has no groups", () => {
    hoisted.canWrite = false;
    hoisted.userGroups = [];
    renderPage();

    expect(screen.getByText("This user is not a member of any group.")).toBeInTheDocument();
  });

  it("lists assigned roles on the Roles tab", () => {
    hoisted.userRoles = [{ id: "r1", name: "Admin" }];
    renderPage();

    fireEvent.click(screen.getByRole("tab", { name: "Roles" }));
    expect(screen.getByText("Assigned Roles")).toBeInTheDocument();
    expect(screen.getByText("Admin")).toBeInTheDocument();
  });

  it("says when no roles are assigned", () => {
    renderPage();

    fireEvent.click(screen.getByRole("tab", { name: "Roles" }));
    expect(screen.getByText("No roles assigned to this user.")).toBeInTheDocument();
  });
});

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
import type { ThunderUser } from "@agent-management-platform/types";
import { renderRoute } from "./__test__/renderRoute";

interface Confirmation {
  title: string;
  description: string;
  onConfirm: () => void;
}

const hoisted = vi.hoisted(() => ({
  list: {
    data: undefined as { users: ThunderUser[]; total: number } | undefined,
    isLoading: false,
    error: null as unknown,
  },
  listArgs: [] as unknown[][],
  deleteUser: vi.fn(),
  addConfirmation: vi.fn(),
  flagOn: true,
  access: { member_invite: true, member_remove: true } as Record<string, boolean>,
}));

vi.mock("@agent-management-platform/api-client", () => ({
  useListUsers: (...args: unknown[]) => {
    hoisted.listArgs.push(args);
    return hoisted.list;
  },
  useDeleteUser: () => ({ mutateAsync: hoisted.deleteUser }),
}));
vi.mock("@agent-management-platform/shared-component", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  useConfirmationDialog: () => ({ addConfirmation: hoisted.addConfirmation }),
}));
vi.mock("@agent-management-platform/feature-management", () => ({
  useFeatureFlag: () => hoisted.flagOn,
  useFeatureAccess: (key: string) => hoisted.access[key] ?? false,
}));

import { UsersPage } from "./UsersPage";

const user = (id: string, username: string): ThunderUser => ({
  id,
  attributes: { username },
});

const renderPage = () =>
  renderRoute(
    "/org/acme/settings/identities/users",
    "/org/:orgId/settings/identities/users",
    <UsersPage />,
  );

beforeEach(() => {
  vi.clearAllMocks();
  hoisted.list = { data: undefined, isLoading: false, error: null };
  hoisted.listArgs = [];
  hoisted.flagOn = true;
  hoisted.access = { member_invite: true, member_remove: true };
});

describe("UsersPage", () => {
  it("lists users with their IDs and asks for the first page", () => {
    hoisted.list.data = { users: [user("u1", "alice"), user("u2", "bob")], total: 2 };
    renderPage();

    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(screen.getByText("u2")).toBeInTheDocument();
    expect(hoisted.listArgs[0]).toEqual([{ orgName: "acme" }, { offset: 0, limit: 10 }]);
  });

  it("shows skeleton rows instead of the empty state while loading", () => {
    hoisted.list.isLoading = true;
    renderPage();

    expect(screen.queryByText("No users yet")).not.toBeInTheDocument();
    expect(screen.getByText("Username")).toBeInTheDocument();
  });

  it("shows an error when the list fails to load", () => {
    hoisted.list.error = new Error("boom");
    hoisted.list.data = { users: [], total: 0 };
    renderPage();

    expect(screen.getByText("Failed to load users")).toBeInTheDocument();
  });

  it("offers to add or invite users from the empty state", () => {
    hoisted.list.data = { users: [], total: 0 };
    renderPage();

    expect(screen.getByText("No users yet")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Invite user" }));
    expect(screen.getByText("at /org/acme/settings/identities/users/invite")).toBeInTheDocument();
  });

  it("navigates to the add page from the toolbar", () => {
    hoisted.list.data = { users: [user("u1", "alice")], total: 1 };
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Add user" }));
    expect(screen.getByText("at /org/acme/settings/identities/users/add")).toBeInTheDocument();
  });

  it("disables create buttons while user management is off", () => {
    hoisted.flagOn = false;
    hoisted.list.data = { users: [user("u1", "alice")], total: 1 };
    renderPage();

    expect(screen.getByRole("button", { name: "Add user" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Invite user" })).toBeDisabled();
  });

  it("hides create and delete actions without member permissions", () => {
    hoisted.access = {};
    hoisted.list.data = { users: [user("u1", "alice")], total: 1 };
    renderPage();

    expect(screen.queryByRole("button", { name: "Add user" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Delete user" })).not.toBeInTheDocument();
  });

  it("filters users by username and says when nothing matches", () => {
    hoisted.list.data = { users: [user("u1", "alice"), user("u2", "bob")], total: 2 };
    renderPage();

    const search = screen.getByPlaceholderText("Search users");
    fireEvent.change(search, { target: { value: "ALI" } });
    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(screen.queryByText("bob")).not.toBeInTheDocument();

    fireEvent.change(search, { target: { value: "zed" } });
    expect(screen.getByText("No users found")).toBeInTheDocument();
    expect(screen.getByText('No users match "zed". Try a different search term.')).toBeInTheDocument();
  });

  it("opens a user when the row is clicked", () => {
    hoisted.list.data = { users: [user("u1", "alice")], total: 1 };
    renderPage();

    fireEvent.click(screen.getByText("alice"));
    expect(screen.getByText("at /org/acme/settings/identities/users/u1")).toBeInTheDocument();
  });

  it("asks for confirmation and deletes the user on confirm", () => {
    hoisted.list.data = { users: [user("u1", "alice")], total: 1 };
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: "Delete user" }));

    // The delete button must not also open the row.
    expect(screen.queryByText(/^at /)).not.toBeInTheDocument();
    const confirmation = hoisted.addConfirmation.mock.calls[0][0] as Confirmation;
    expect(confirmation.title).toBe("Delete User");
    expect(confirmation.description).toContain('"alice"');
    expect(hoisted.deleteUser).not.toHaveBeenCalled();
    confirmation.onConfirm();
    expect(hoisted.deleteUser).toHaveBeenCalledWith({ orgName: "acme", userId: "u1" });
  });

  it("pages through users with the right offset", () => {
    hoisted.list.data = {
      users: Array.from({ length: 10 }, (_, i) => user(`u${i}`, `user${i}`)),
      total: 25,
    };
    renderPage();

    fireEvent.click(screen.getByRole("button", { name: /next page/i }));
    expect(hoisted.listArgs[hoisted.listArgs.length - 1]).toEqual([{ orgName: "acme" }, { offset: 10, limit: 10 }]);
  });

  it("jumps back to the last page when the current page comes back empty", () => {
    hoisted.list.data = { users: [], total: 12 };
    renderPage();

    // total 12 at 10 per page → last page index 1 → offset 10.
    expect(hoisted.listArgs[hoisted.listArgs.length - 1]).toEqual([{ orgName: "acme" }, { offset: 10, limit: 10 }]);
  });
});

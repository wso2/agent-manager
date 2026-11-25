/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  invalidatedKeys, renderHook, seed, testQueryClient, waitFor, type RenderedHook,
} from "../../test-utils/render-hook";

const ui = vi.hoisted(() => ({ pushSnackBar: vi.fn(), getToken: async () => "tok" }));
const api = vi.hoisted(() => {
  const names = [
    "listUsers", "getUser", "getUserProfile", "createUser", "inviteUser", "updateUser",
    "deleteUser", "getUserGroups", "getUserRoles", "listGroups", "getGroup", "createGroup",
    "updateGroup", "deleteGroup", "addGroupMembers", "removeGroupMembers", "getGroupMembers",
    "getGroupRoles", "listRoles", "getRole", "createRole", "updateRole", "deleteRole",
    "getRoleAssignments", "addRolePermissions", "removeRolePermissions", "addRoleAssignees",
    "removeRoleAssignees", "listAMPPermissions", "updateUserProfile",
  ] as const;
  return Object.fromEntries(names.map((n) => [n, vi.fn()])) as Record<
    (typeof names)[number],
    ReturnType<typeof vi.fn>
  >;
});

vi.mock("@agent-management-platform/views", () => ({
  useSnackBar: () => ({ pushSnackBar: ui.pushSnackBar }),
}));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: ui.getToken, isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("./telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/identities", () => api);

import * as hooks from "./identities";

type Api = keyof typeof api;
let rendered: RenderedHook<unknown> | undefined;
async function render<T>(useHook: () => T) {
  const hook = await renderHook(useHook);
  rendered = hook as RenderedHook<unknown>;
  return hook;
}

afterEach(() => {
  rendered?.unmount();
  rendered = undefined;
  vi.resetAllMocks();
});

const org = { orgName: "o" };
const user = { orgName: "o", userId: "u1" };
const group = { orgName: "o", groupId: "g1" };
const role = { orgName: "o", roleId: "r1" };
const page = { offset: 0, limit: 10 };

type QueryRow = [string, () => { data?: unknown }, Api, unknown[], unknown[]];
const queries: QueryRow[] = [
  ["useListUsers", () => hooks.useListUsers(org, page), "listUsers", [org, page],
    ["identity-users", org, page]],
  ["useGetUser", () => hooks.useGetUser(user), "getUser", [user], ["identity-user", user]],
  ["useGetUserProfile", () => hooks.useGetUserProfile(user), "getUserProfile", [user],
    ["identity-user-profile", user]],
  ["useGetUserGroups", () => hooks.useGetUserGroups(user), "getUserGroups", [user],
    ["identity-user-groups", user]],
  ["useGetUserRoles", () => hooks.useGetUserRoles(user), "getUserRoles", [user],
    ["identity-user-roles", user]],
  ["useListGroups", () => hooks.useListGroups(org, page), "listGroups", [org, page],
    ["identity-groups", org, page]],
  ["useGetGroup", () => hooks.useGetGroup(group), "getGroup", [group], ["identity-group", group]],
  ["useGetGroupRoles", () => hooks.useGetGroupRoles(group), "getGroupRoles", [group],
    ["identity-group-roles", group]],
  ["useGetGroupMembers", () => hooks.useGetGroupMembers(group, page), "getGroupMembers",
    [group, page], ["identity-group-members", group, page]],
  ["useListRoles", () => hooks.useListRoles(org, page), "listRoles", [org, page],
    ["identity-roles", org, page]],
  ["useGetRole", () => hooks.useGetRole(role), "getRole", [role], ["identity-role", role]],
  ["useGetRoleAssignments", () => hooks.useGetRoleAssignments(role), "getRoleAssignments", [role],
    ["identity-role-assignments", role]],
  ["useListAMPPermissions", () => hooks.useListAMPPermissions(org), "listAMPPermissions", [org],
    ["amp-permissions", org]],
];

describe.each(queries)("%s", (_name, useHook, fn, args, key) => {
  it("loads through the API with the caller's token and caches under its key", async () => {
    api[fn].mockResolvedValue({ id: "x" });
    const hook = await render(useHook);
    await waitFor(() => hook.result.current.data !== undefined);
    expect(hook.result.current.data).toEqual({ id: "x" });
    expect(api[fn]).toHaveBeenCalledWith(...args, ui.getToken);
    expect(hook.client.getQueryData(key)).toEqual({ id: "x" });
  });
});

describe("query gating", () => {
  it.each([
    ["useListUsers", () => hooks.useListUsers({} as never), "listUsers"],
    ["useAllUsers", () => hooks.useAllUsers({} as never), "listUsers"],
    ["useGetUser", () => hooks.useGetUser({ orgName: "o", userId: "" }), "getUser"],
    ["useGetUserProfile", () => hooks.useGetUserProfile({ orgName: "o", userId: "" }), "getUserProfile"],
    ["useGetUserGroups", () => hooks.useGetUserGroups({ orgName: "", userId: "u" }), "getUserGroups"],
    ["useGetUserRoles", () => hooks.useGetUserRoles({ orgName: "o", userId: "" }), "getUserRoles"],
    ["useListGroups", () => hooks.useListGroups({} as never), "listGroups"],
    ["useAllGroups", () => hooks.useAllGroups({} as never), "listGroups"],
    ["useGetGroup", () => hooks.useGetGroup({ orgName: "o", groupId: "" }), "getGroup"],
    ["useGetGroupRoles", () => hooks.useGetGroupRoles({ orgName: "o", groupId: "" }), "getGroupRoles"],
    ["useGetGroupMembers", () => hooks.useGetGroupMembers({ orgName: "o", groupId: "" }), "getGroupMembers"],
    ["useAllGroupMemberIds", () => hooks.useAllGroupMemberIds({ orgName: "o", groupId: "" }), "getGroupMembers"],
    ["useListRoles", () => hooks.useListRoles({} as never), "listRoles"],
    ["useGetRole", () => hooks.useGetRole({ orgName: "o", roleId: "" }), "getRole"],
    ["useGetRoleAssignments", () => hooks.useGetRoleAssignments({ orgName: "o", roleId: "" }), "getRoleAssignments"],
    ["useListAMPPermissions", () => hooks.useListAMPPermissions({} as never), "listAMPPermissions"],
  ] as [string, () => { fetchStatus: string }, Api][])(
    "%s stays idle without its ids",
    async (_name, useHook, fn) => {
      const hook = await render(useHook);
      expect(hook.result.current.fetchStatus).toBe("idle");
      expect(api[fn]).not.toHaveBeenCalled();
    },
  );
});

const people = (from: number, count: number) =>
  Array.from({ length: count }, (_, i) => ({ id: `u${from + i}` }));

describe("paging through everything", () => {
  it("useAllUsers reads every page of a large org", async () => {
    api.listUsers.mockImplementation(async (_p, q: { offset: number }) => ({
      users: people(q.offset, Math.min(100, 230 - q.offset)), total: 230,
    }));
    const hook = await render(() => hooks.useAllUsers(org));
    await waitFor(() => hook.result.current.data !== undefined);
    expect(hook.result.current.data!.users).toHaveLength(230);
    expect(api.listUsers.mock.calls.map(([, q]) => q)).toEqual([
      { offset: 0, limit: 100 }, { offset: 100, limit: 100 }, { offset: 200, limit: 100 },
    ]);
  });

  it("useAllGroups stops at a short page even if total says more", async () => {
    api.listGroups.mockResolvedValue({ groups: people(0, 3), total: 500 });
    const hook = await render(() => hooks.useAllGroups(org));
    await waitFor(() => hook.result.current.data !== undefined);
    expect(hook.result.current.data!.groups).toHaveLength(3);
    expect(api.listGroups).toHaveBeenCalledTimes(1);
  });

  it("useAllGroupMemberIds collects member ids across pages", async () => {
    api.getGroupMembers
      .mockResolvedValueOnce({ users: people(0, 100), total: 101 })
      .mockResolvedValueOnce({ users: people(100, 1), total: 101 });
    const hook = await render(() => hooks.useAllGroupMemberIds(group));
    await waitFor(() => hook.result.current.data !== undefined);
    expect(hook.result.current.data!.memberIds).toHaveLength(101);
    expect(hook.result.current.data!.memberIds[100]).toBe("u100");
    expect(api.getGroupMembers.mock.calls[1])
      .toEqual([group, { offset: 100, limit: 100 }, ui.getToken]);
  });
});

describe("useUserDisplayName", () => {
  const nameOf = async (attributes: Record<string, unknown> | undefined) => {
    api.getUserProfile.mockResolvedValue({ id: "u1", attributes });
    const hook = await render(() => ({ name: hooks.useUserDisplayName(user) }));
    await waitFor(() => hook.result.current.name !== undefined);
    return hook.result.current.name;
  };

  it("shows the full name, or the username without one", async () => {
    expect(await nameOf({ given_name: " Ada ", family_name: "Lovelace" })).toBe("Ada Lovelace");
    rendered?.unmount();
    expect(await nameOf({ given_name: "Ada", username: "ada" })).toBe("Ada");
    rendered?.unmount();
    expect(await nameOf({ given_name: 7, username: "ada" })).toBe("ada");
  });

  it("is undefined while loading or without any name", async () => {
    api.getUserProfile.mockResolvedValue({ id: "u1", attributes: {} });
    const hook = await render(() => ({ name: hooks.useUserDisplayName(user) }));
    expect(hook.result.current.name).toBeUndefined();
    await waitFor(() => hook.client.getQueryData(["identity-user-profile", user]) !== undefined);
    expect(hook.result.current.name).toBeUndefined();
  });

  it.each([403, 404])("keeps the fallback quietly on a %i", async (status) => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    api.getUserProfile.mockRejectedValue(Object.assign(new Error("x"), { status }));
    const hook = await render(() => ({ name: hooks.useUserDisplayName(user) }));
    await waitFor(() => hook.client.getQueryState(["identity-user-profile", user])?.status === "error");
    expect(hook.result.current.name).toBeUndefined();
    expect(warn).not.toHaveBeenCalled();
    expect(ui.pushSnackBar).not.toHaveBeenCalled();
    warn.mockRestore();
  });

  it("logs, without a snackbar, when the identity service is failing", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    api.getUserProfile.mockRejectedValue(Object.assign(new Error("x"), { status: 503 }));
    const hook = await render(() => ({ name: hooks.useUserDisplayName(user) }));
    await waitFor(() => warn.mock.calls.length > 0);
    expect(warn.mock.calls[0][1]).toEqual({ userId: "u1", status: 503 });
    expect(hook.result.current.name).toBeUndefined();
    expect(ui.pushSnackBar).not.toHaveBeenCalled();
    warn.mockRestore();
  });
});

const SEEDED = [
  ["identity-users", org],
  ["identity-users", { orgName: "other" }],
  ["identity-users-all", org],
  ["identity-user", user],
  ["identity-user", { orgName: "o", userId: "u2" }],
  ["identity-user-groups", user],
  ["identity-groups", org],
  ["identity-groups-all", org],
  ["identity-group", group],
  ["identity-group-members", group],
  ["identity-group-member-ids-all", group],
  ["identity-roles", org],
  ["identity-role", role],
  ["identity-role-assignments", role],
  ["identity-role-assignments", { orgName: "o", roleId: "r2" }],
] as const;

const body = { name: "x" };
const members = { userIds: ["u1"] };
type MutationRow = [
  string, () => { mutateAsync: (v: never) => Promise<unknown> }, unknown, Api, unknown[], string,
  unknown[][],
];
const mutations: MutationRow[] = [
  ["useCreateUser", hooks.useCreateUser, { params: org, body }, "createUser", [org, body],
    "User created successfully", [["identity-users", org], ["identity-users-all", org]]],
  ["useInviteUser", hooks.useInviteUser, { params: org, body }, "inviteUser", [org, body],
    "User Invite created successfully", []],
  ["useUpdateUser", hooks.useUpdateUser, { params: user, body }, "updateUser", [user, body],
    "User updated successfully",
    [["identity-users", org], ["identity-users", { orgName: "other" }], ["identity-users-all", org],
      ["identity-user", user]]],
  ["useDeleteUser", hooks.useDeleteUser, user, "deleteUser", [user], "User deleted successfully",
    [["identity-users", org], ["identity-users", { orgName: "other" }], ["identity-users-all", org],
      ["identity-user", user]]],
  ["useUpdateUserProfile", hooks.useUpdateUserProfile, { params: user, body }, "updateUserProfile",
    [user, body], "Profile updated successfully", []],
  ["useCreateGroup", hooks.useCreateGroup, { params: org, body }, "createGroup", [org, body],
    "Group created successfully", [["identity-groups", org], ["identity-groups-all", org]]],
  ["useUpdateGroup", hooks.useUpdateGroup, { params: group, body }, "updateGroup", [group, body],
    "Group updated successfully",
    [["identity-groups", org], ["identity-groups-all", org], ["identity-group", group]]],
  ["useDeleteGroup", hooks.useDeleteGroup, group, "deleteGroup", [group], "Group deleted successfully",
    [["identity-groups", org], ["identity-groups-all", org], ["identity-group", group]]],
  ["useAddGroupMembers", hooks.useAddGroupMembers, { params: group, body: members },
    "addGroupMembers", [group, members], "Group Members updated successfully",
    [["identity-user-groups", user], ["identity-group-members", group],
      ["identity-group-member-ids-all", group]]],
  ["useRemoveGroupMembers", hooks.useRemoveGroupMembers, { params: group, body: members },
    "removeGroupMembers", [group, members], "Group Members updated successfully",
    [["identity-user-groups", user], ["identity-group-members", group],
      ["identity-group-member-ids-all", group]]],
  ["useCreateRole", hooks.useCreateRole, { params: org, body }, "createRole", [org, body],
    "Role created successfully", [["identity-roles", org]]],
  ["useUpdateRole", hooks.useUpdateRole, { params: role, body }, "updateRole", [role, body],
    "Role updated successfully", [["identity-roles", org], ["identity-role", role]]],
  ["useDeleteRole", hooks.useDeleteRole, role, "deleteRole", [role], "Role deleted successfully",
    [["identity-roles", org], ["identity-role", role]]],
  ["useAddRolePermissions", hooks.useAddRolePermissions, { params: role, body },
    "addRolePermissions", [role, body], "Role Permissions updated successfully",
    [["identity-role-assignments", role]]],
  ["useRemoveRolePermissions", hooks.useRemoveRolePermissions, { params: role, body },
    "removeRolePermissions", [role, body], "Role Permissions updated successfully",
    [["identity-role-assignments", role]]],
  ["useAddRoleAssignees", hooks.useAddRoleAssignees, { params: role, body }, "addRoleAssignees",
    [role, body], "Role Assignments updated successfully",
    [["identity-role-assignments", role], ["identity-role-assignments", { orgName: "o", roleId: "r2" }]]],
  ["useRemoveRoleAssignees", hooks.useRemoveRoleAssignees, { params: role, body },
    "removeRoleAssignees", [role, body], "Role Assignments updated successfully",
    [["identity-role-assignments", role], ["identity-role-assignments", { orgName: "o", roleId: "r2" }]]],
];

describe.each(mutations)("%s", (_name, useHook, variables, fn, args, message, invalidated) => {
  it("calls the API, announces success and refreshes exactly the affected queries", async () => {
    api[fn].mockResolvedValue({ ok: true });
    const client = testQueryClient();
    seed(client, SEEDED);
    const hook = await renderHook(useHook, { client });
    rendered = hook as RenderedHook<unknown>;

    await act(async () => { await hook.result.current.mutateAsync(variables as never); });

    expect(api[fn]).toHaveBeenCalledWith(...args, ui.getToken);
    expect(ui.pushSnackBar).toHaveBeenCalledWith({ message, type: "success" });
    const sortKeys = (keys: unknown[][]) => keys.map((k) => JSON.stringify(k)).sort();
    expect(sortKeys(invalidatedKeys(client, SEEDED))).toEqual(sortKeys(invalidated));
  });

  it("shows the server's reason and refreshes nothing when the API fails", async () => {
    api[fn].mockRejectedValue(Object.assign(new Error("Permission denied"), { status: 403 }));
    const client = testQueryClient();
    seed(client, SEEDED);
    const hook = await renderHook(useHook, { client });
    rendered = hook as RenderedHook<unknown>;

    await act(async () => {
      await hook.result.current.mutateAsync(variables as never).catch(() => undefined);
    });

    expect(ui.pushSnackBar).toHaveBeenCalledWith({ message: "Permission denied", type: "error" });
    expect(invalidatedKeys(client, SEEDED)).toEqual([]);
  });
});

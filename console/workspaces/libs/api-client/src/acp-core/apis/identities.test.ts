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

import { afterEach, describe, expect, it, vi } from "vitest";
import type * as Utils from "../../shared/utils";

const http = vi.hoisted(() => ({
  httpGET: vi.fn(),
  httpPOST: vi.fn(),
  httpPUT: vi.fn(),
  httpDELETE: vi.fn(),
}));
vi.mock("../../shared/utils", async (importOriginal) => ({
  ...(await importOriginal<typeof Utils>()),
  ...http,
}));

import * as api from "./identities";

const getToken = async () => "tok";
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });
const BASE = "/api/v1/orgs/my%20org/identities";
const org = { orgName: "my org" };
const user = { ...org, userId: "u/1" };
const group = { ...org, groupId: "g 1" };
const role = { ...org, roleId: "r#1" };
const body = { name: "x" } as never;

afterEach(() => vi.resetAllMocks());

type Method = keyof typeof http;
type Row = [string, Method, () => Promise<unknown>, string, unknown?];

/** Calls that read or write a resource and return its JSON. */
const returning: Row[] = [
  ["getUser", "httpGET", () => api.getUser(user, getToken), `${BASE}/users/u%2F1`],
  ["getUserProfile", "httpGET", () => api.getUserProfile(user, getToken), `${BASE}/users/u%2F1/profile`],
  ["createUser", "httpPOST", () => api.createUser(org, body, getToken), `${BASE}/users`, body],
  ["updateUser", "httpPUT", () => api.updateUser(user, body, getToken), `${BASE}/users/u%2F1`, body],
  ["inviteUser", "httpPOST", () => api.inviteUser(org, body, getToken), `${BASE}/users/invite`, body],
  ["getUserGroups", "httpGET", () => api.getUserGroups(user, getToken), `${BASE}/users/u%2F1/groups`],
  ["getUserRoles", "httpGET", () => api.getUserRoles(user, getToken), `${BASE}/users/u%2F1/roles`],
  ["updateUserProfile", "httpPUT", () => api.updateUserProfile(user, body, getToken),
    `${BASE}/users/u%2F1/profile`, body],
  ["getGroup", "httpGET", () => api.getGroup(group, getToken), `${BASE}/groups/g%201`],
  ["createGroup", "httpPOST", () => api.createGroup(org, body, getToken), `${BASE}/groups`, body],
  ["updateGroup", "httpPUT", () => api.updateGroup(group, body, getToken), `${BASE}/groups/g%201`, body],
  ["getGroupRoles", "httpGET", () => api.getGroupRoles(group, getToken), `${BASE}/groups/g%201/roles`],
  ["getRole", "httpGET", () => api.getRole(role, getToken), `${BASE}/roles/r%231`],
  ["createRole", "httpPOST", () => api.createRole(org, body, getToken), `${BASE}/roles`, body],
  ["updateRole", "httpPUT", () => api.updateRole(role, body, getToken), `${BASE}/roles/r%231`, body],
  ["getRoleAssignments", "httpGET", () => api.getRoleAssignments(role, getToken),
    `${BASE}/roles/r%231/assignments`],
  ["listAMPPermissions", "httpGET", () => api.listAMPPermissions(org, getToken), `${BASE}/permissions`],
];

describe.each(returning)("%s", (_name, method, call, path, sentBody) => {
  it("sends the request with encoded ids and the token, and returns the body", async () => {
    http[method].mockResolvedValue(json({ id: "1" }));

    await expect(call()).resolves.toEqual({ id: "1" });

    const args = http[method].mock.calls[0];
    expect(args[0]).toBe(path);
    if (sentBody !== undefined) expect(args[1]).toBe(sentBody);
    expect(args[args.length - 1]).toEqual({ token: "tok" });
  });

  it("throws the server's error body on a non-OK response", async () => {
    http[method].mockResolvedValue(json({ code: "FORBIDDEN" }, 403));
    await expect(call()).rejects.toEqual({ code: "FORBIDDEN" });
  });
});

/** Membership and permission changes: POSTs that return nothing. */
const members = { userIds: ["u1"] };
const actions: Row[] = [
  ["addGroupMembers", "httpPOST", () => api.addGroupMembers(group, members, getToken),
    `${BASE}/groups/g%201/members/add`, members],
  ["removeGroupMembers", "httpPOST", () => api.removeGroupMembers(group, members, getToken),
    `${BASE}/groups/g%201/members/remove`, members],
  ["addRolePermissions", "httpPOST", () => api.addRolePermissions(role, body, getToken),
    `${BASE}/roles/r%231/permissions/add`, body],
  ["removeRolePermissions", "httpPOST", () => api.removeRolePermissions(role, body, getToken),
    `${BASE}/roles/r%231/permissions/remove`, body],
  ["addRoleAssignees", "httpPOST", () => api.addRoleAssignees(role, body, getToken),
    `${BASE}/roles/r%231/assignees/add`, body],
  ["removeRoleAssignees", "httpPOST", () => api.removeRoleAssignees(role, body, getToken),
    `${BASE}/roles/r%231/assignees/remove`, body],
];

describe.each(actions)("%s", (_name, method, call, path, sentBody) => {
  it("posts the change and resolves to nothing", async () => {
    http[method].mockResolvedValue(new Response(null, { status: 204 }));
    await expect(call()).resolves.toBeUndefined();
    expect(http[method]).toHaveBeenCalledWith(path, sentBody, { token: "tok" });
  });

  it("throws the server's error body on a non-OK response", async () => {
    http[method].mockResolvedValue(json({ code: "NOT_FOUND" }, 404));
    await expect(call()).rejects.toEqual({ code: "NOT_FOUND" });
  });
});

const deletes: Row[] = [
  ["deleteUser", "httpDELETE", () => api.deleteUser(user, getToken), `${BASE}/users/u%2F1`],
  ["deleteGroup", "httpDELETE", () => api.deleteGroup(group, getToken), `${BASE}/groups/g%201`],
  ["deleteRole", "httpDELETE", () => api.deleteRole(role, getToken), `${BASE}/roles/r%231`],
];

describe.each(deletes)("%s", (_name, method, call, path) => {
  it("deletes the encoded resource and accepts a 204", async () => {
    http[method].mockResolvedValue(new Response(null, { status: 204 }));
    await expect(call()).resolves.toBeUndefined();
    expect(http[method]).toHaveBeenCalledWith(path, { token: "tok" });
  });

  it("throws the server's error body on a non-OK response", async () => {
    http[method].mockResolvedValue(json({ code: "CONFLICT" }, 409));
    await expect(call()).rejects.toEqual({ code: "CONFLICT" });
  });
});

const lists: [string, (q?: { offset?: number; limit?: number }) => Promise<unknown>, string][] = [
  ["listUsers", (q) => api.listUsers(org, q, getToken), `${BASE}/users`],
  ["listGroups", (q) => api.listGroups(org, q, getToken), `${BASE}/groups`],
  ["listRoles", (q) => api.listRoles(org, q, getToken), `${BASE}/roles`],
  ["getGroupMembers", (q) => api.getGroupMembers(group, q, getToken), `${BASE}/groups/g%201/members`],
];

describe.each(lists)("%s", (_name, call, path) => {
  it("sends the page it was asked for", async () => {
    http.httpGET.mockResolvedValue(json({ total: 0 }));
    await expect(call({ offset: 40, limit: 20 })).resolves.toEqual({ total: 0 });
    expect(http.httpGET).toHaveBeenCalledWith(path, {
      searchParams: { offset: "40", limit: "20" },
      token: "tok",
    });
  });

  it("fills in a partial page and sends none when not paging", async () => {
    http.httpGET.mockImplementation(async () => json({ total: 0 }));
    await call({});
    await call();
    expect(http.httpGET.mock.calls.map(([, o]) => o.searchParams)).toEqual([
      { offset: "0", limit: "20" },
      undefined,
    ]);
  });

  it("throws the server's error body on a non-OK response", async () => {
    http.httpGET.mockResolvedValue(json({ code: "FORBIDDEN" }, 403));
    await expect(call()).rejects.toEqual({ code: "FORBIDDEN" });
  });
});

describe("default org", () => {
  it("uses the default org and sends no token when none is given", async () => {
    http.httpGET.mockResolvedValue(json({}));
    await api.listAMPPermissions({} as never);
    expect(http.httpGET).toHaveBeenCalledWith("/api/v1/orgs/default/identities/permissions", {
      token: undefined,
    });
  });
});

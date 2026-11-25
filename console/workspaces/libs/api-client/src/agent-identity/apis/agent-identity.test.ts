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

import { describe, expect, it, vi } from "vitest";
import { httpDELETE, httpGET, httpPOST, httpPUT } from "../../shared/utils";
import { describeHttpCases, getToken, type HttpMocks } from "../../test-utils/http-cases";
import {
  addAgentIdentityGroupMembers,
  addAgentIdentityRoleAssignees,
  createAgentIdentityGroup,
  createAgentIdentityRole,
  deleteAgentIdentityGroup,
  deleteAgentIdentityRole,
  getAgentIdentityGroup,
  getAgentIdentityGroupMembers,
  getAgentIdentityGroupRoles,
  getAgentIdentityRole,
  getAgentIdentityRoleAssignments,
  listAgentIdentityAgents,
  listAgentIdentityGroups,
  listAgentIdentityRoles,
  listAgentIdentityScopes,
  removeAgentIdentityGroupMembers,
  removeAgentIdentityRoleAssignees,
  updateAgentIdentityGroup,
  updateAgentIdentityRole,
} from "./agent-identity";

vi.mock("../../shared/utils", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../shared/utils")>()),
  httpGET: vi.fn(),
  httpPOST: vi.fn(),
  httpPUT: vi.fn(),
  httpDELETE: vi.fn(),
}));

const mocks = {
  GET: vi.mocked(httpGET),
  POST: vi.mocked(httpPOST),
  PUT: vi.mocked(httpPUT),
  DELETE: vi.mocked(httpDELETE),
} as unknown as HttpMocks;

const env = { orgName: "acme corp", envName: "dev/1" };
const group = { ...env, groupId: "g 1" };
const role = { ...env, roleId: "r/1" };
const BASE = "/m/ai-gateway/orgs/acme%20corp/environments/dev%2F1/agent-identities";
const groupBody = { name: "Readers", description: "d" };
const members = { members: [{ id: "a1", type: "agent" }] };
const roleBody = { name: "reader", permissions: [] };
const assignments = { assignments: [{ id: "g1", type: "group" }] };

describeHttpCases("agent identity API", mocks, [
  {
    name: "listAgentIdentityGroups pages with offset/limit",
    call: () => listAgentIdentityGroups(env, { offset: 40, limit: 10 }, getToken),
    method: "GET", url: `${BASE}/groups`, searchParams: { offset: "40", limit: "10" },
  },
  {
    name: "listAgentIdentityGroups defaults the page when the query is partial",
    call: () => listAgentIdentityGroups(env, {}, getToken),
    method: "GET", url: `${BASE}/groups`, searchParams: { offset: "0", limit: "20" },
  },
  {
    name: "listAgentIdentityGroups without a query sends no page params",
    call: () => listAgentIdentityGroups(env, undefined, getToken),
    method: "GET", url: `${BASE}/groups`,
  },
  {
    name: "createAgentIdentityGroup",
    call: () => createAgentIdentityGroup(env, groupBody as never, getToken),
    method: "POST", url: `${BASE}/groups`, body: groupBody,
  },
  {
    name: "getAgentIdentityGroup",
    call: () => getAgentIdentityGroup(group, getToken),
    method: "GET", url: `${BASE}/groups/g%201`,
  },
  {
    name: "updateAgentIdentityGroup",
    call: () => updateAgentIdentityGroup(group, groupBody as never, getToken),
    method: "PUT", url: `${BASE}/groups/g%201`, body: groupBody,
  },
  {
    name: "deleteAgentIdentityGroup",
    call: () => deleteAgentIdentityGroup(group, getToken),
    method: "DELETE", url: `${BASE}/groups/g%201`, returnsBody: false,
  },
  {
    name: "getAgentIdentityGroupMembers",
    call: () => getAgentIdentityGroupMembers(group, { offset: 0, limit: 5 }, getToken),
    method: "GET", url: `${BASE}/groups/g%201/members`, searchParams: { offset: "0", limit: "5" },
  },
  {
    name: "addAgentIdentityGroupMembers",
    call: () => addAgentIdentityGroupMembers(group, members as never, getToken),
    method: "POST", url: `${BASE}/groups/g%201/members/add`, body: members, returnsBody: false,
  },
  {
    name: "removeAgentIdentityGroupMembers",
    call: () => removeAgentIdentityGroupMembers(group, members as never, getToken),
    method: "POST", url: `${BASE}/groups/g%201/members/remove`, body: members, returnsBody: false,
  },
  {
    name: "getAgentIdentityGroupRoles",
    call: () => getAgentIdentityGroupRoles(group, getToken),
    method: "GET", url: `${BASE}/groups/g%201/roles`,
  },
  {
    name: "listAgentIdentityRoles",
    call: () => listAgentIdentityRoles(env, { offset: 20 }, getToken),
    method: "GET", url: `${BASE}/roles`, searchParams: { offset: "20", limit: "20" },
  },
  {
    name: "createAgentIdentityRole",
    call: () => createAgentIdentityRole(env, roleBody as never, getToken),
    method: "POST", url: `${BASE}/roles`, body: roleBody,
  },
  {
    name: "getAgentIdentityRole",
    call: () => getAgentIdentityRole(role, getToken),
    method: "GET", url: `${BASE}/roles/r%2F1`,
  },
  {
    name: "updateAgentIdentityRole",
    call: () => updateAgentIdentityRole(role, roleBody as never, getToken),
    method: "PUT", url: `${BASE}/roles/r%2F1`, body: roleBody,
  },
  {
    name: "deleteAgentIdentityRole",
    call: () => deleteAgentIdentityRole(role, getToken),
    method: "DELETE", url: `${BASE}/roles/r%2F1`, returnsBody: false,
  },
  {
    name: "getAgentIdentityRoleAssignments",
    call: () => getAgentIdentityRoleAssignments(role, getToken),
    method: "GET", url: `${BASE}/roles/r%2F1/assignments`,
  },
  {
    name: "addAgentIdentityRoleAssignees",
    call: () => addAgentIdentityRoleAssignees(role, assignments as never, getToken),
    method: "POST", url: `${BASE}/roles/r%2F1/assignments/add`, body: assignments, returnsBody: false,
  },
  {
    name: "removeAgentIdentityRoleAssignees",
    call: () => removeAgentIdentityRoleAssignees(role, assignments as never, getToken),
    method: "POST", url: `${BASE}/roles/r%2F1/assignments/remove`, body: assignments, returnsBody: false,
  },
  {
    name: "listAgentIdentityAgents",
    call: () => listAgentIdentityAgents(env, getToken),
    method: "GET", url: `${BASE}/agents`,
  },
  {
    name: "listAgentIdentityScopes",
    call: () => listAgentIdentityScopes(env, getToken),
    method: "GET", url: `${BASE}/scopes`,
  },
]);

describe("agent identity API refuses a missing org", () => {
  it("does not send the request to another org", async () => {
    mocks.GET.mockReset();
    await expect(listAgentIdentityGroups({ envName: "dev" } as never, undefined, getToken)).rejects.toThrow("orgName");
    expect(mocks.GET).not.toHaveBeenCalled();
  });
});

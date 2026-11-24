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

import { vi } from "vitest";

const modules = vi.hoisted(() => ({ installed: new Set(["agent-identity"]) }));
vi.mock("@agent-management-platform/views", () => ({
  useSnackBar: () => ({ pushSnackBar: vi.fn() }),
  useIsUIModuleInstalled: (id: string) => modules.installed.has(id),
}));
vi.mock("@agent-management-platform/auth", () => ({
  useAuthHooks: () => ({ getToken: async () => "t", isAuthenticated: true, logout: vi.fn() }),
}));
vi.mock("../../acp-core/hooks/telemetry", () => ({
  ConsoleAction: {}, markSessionExpired: vi.fn(), useTrack: () => ({ track: vi.fn() }),
}));
vi.mock("../apis/agent-identity", () => ({
  listAgentIdentityGroups: vi.fn(),
  createAgentIdentityGroup: vi.fn(),
  getAgentIdentityGroup: vi.fn(),
  updateAgentIdentityGroup: vi.fn(),
  deleteAgentIdentityGroup: vi.fn(),
  getAgentIdentityGroupMembers: vi.fn(),
  addAgentIdentityGroupMembers: vi.fn(),
  removeAgentIdentityGroupMembers: vi.fn(),
  getAgentIdentityGroupRoles: vi.fn(),
  listAgentIdentityRoles: vi.fn(),
  createAgentIdentityRole: vi.fn(),
  getAgentIdentityRole: vi.fn(),
  updateAgentIdentityRole: vi.fn(),
  deleteAgentIdentityRole: vi.fn(),
  getAgentIdentityRoleAssignments: vi.fn(),
  addAgentIdentityRoleAssignees: vi.fn(),
  removeAgentIdentityRoleAssignees: vi.fn(),
  listAgentIdentityAgents: vi.fn(),
  listAgentIdentityScopes: vi.fn(),
}));
vi.mock("../apis/thunder-instances", () => ({ listThunderInstances: vi.fn() }));

import * as api from "../apis/agent-identity";
import { listThunderInstances } from "../apis/thunder-instances";
import { describeMutationHooks, describeQueryHooks } from "../../test-utils/hook-cases";
import { useListThunderInstances } from "./thunder-instances";
import {
  useAddAgentIdentityGroupMembers,
  useAddAgentIdentityRoleAssignees,
  useCreateAgentIdentityGroup,
  useCreateAgentIdentityRole,
  useDeleteAgentIdentityGroup,
  useDeleteAgentIdentityRole,
  useGetAgentIdentityGroup,
  useGetAgentIdentityGroupMembers,
  useGetAgentIdentityGroupRoles,
  useGetAgentIdentityRole,
  useGetAgentIdentityRoleAssignments,
  useListAgentIdentityAgents,
  useListAgentIdentityGroups,
  useListAgentIdentityRoles,
  useListAgentIdentityScopes,
  useRemoveAgentIdentityGroupMembers,
  useRemoveAgentIdentityRoleAssignees,
  useUpdateAgentIdentityGroup,
  useUpdateAgentIdentityRole,
} from "./agent-identity";

const m = vi.mocked(api);
const env = { orgName: "o", envName: "dev" };
const group = { ...env, groupId: "g1" };
const role = { ...env, roleId: "r1" };
const page = { offset: 0, limit: 10 };

const setModulePresent = (present: boolean) => {
  if (present) modules.installed.add("agent-identity");
  else modules.installed.clear();
};

describeQueryHooks("agent identity queries", [
  {
    name: "useListAgentIdentityGroups",
    use: () => useListAgentIdentityGroups(env, page),
    api: m.listAgentIdentityGroups, args: [env, page], key: ["agent-identity-groups", env, page],
    idle: [() => useListAgentIdentityGroups({ ...env, envName: "" }), () => useListAgentIdentityGroups({ ...env, orgName: "" })],
  },
  {
    name: "useGetAgentIdentityGroup",
    use: () => useGetAgentIdentityGroup(group),
    api: m.getAgentIdentityGroup, args: [group], key: ["agent-identity-group", group],
    idle: [() => useGetAgentIdentityGroup({ ...group, groupId: "" })],
  },
  {
    name: "useGetAgentIdentityGroupMembers",
    use: () => useGetAgentIdentityGroupMembers(group, page),
    api: m.getAgentIdentityGroupMembers, args: [group, page], key: ["agent-identity-group-members", group, page],
    idle: [() => useGetAgentIdentityGroupMembers({ ...group, groupId: "" })],
  },
  {
    name: "useGetAgentIdentityGroupRoles",
    use: () => useGetAgentIdentityGroupRoles(group),
    api: m.getAgentIdentityGroupRoles, args: [group], key: ["agent-identity-group-roles", group],
    idle: [() => useGetAgentIdentityGroupRoles({ ...group, envName: "" })],
  },
  {
    name: "useListAgentIdentityRoles",
    use: () => useListAgentIdentityRoles(env),
    api: m.listAgentIdentityRoles, args: [env, undefined], key: ["agent-identity-roles", env, undefined],
    idle: [() => useListAgentIdentityRoles({ ...env, orgName: "" })],
  },
  {
    name: "useGetAgentIdentityRole",
    use: () => useGetAgentIdentityRole(role),
    api: m.getAgentIdentityRole, args: [role], key: ["agent-identity-role", role],
    idle: [() => useGetAgentIdentityRole({ ...role, roleId: "" })],
  },
  {
    name: "useGetAgentIdentityRoleAssignments",
    use: () => useGetAgentIdentityRoleAssignments(role),
    api: m.getAgentIdentityRoleAssignments, args: [role], key: ["agent-identity-role-assignments", role],
    idle: [() => useGetAgentIdentityRoleAssignments({ ...role, roleId: "" })],
  },
  {
    name: "useListAgentIdentityAgents",
    use: () => useListAgentIdentityAgents(env),
    api: m.listAgentIdentityAgents, args: [env], key: ["agent-identity-agents", env],
    idle: [() => useListAgentIdentityAgents({ ...env, envName: "" })],
  },
  {
    name: "useListAgentIdentityScopes",
    use: () => useListAgentIdentityScopes(env),
    api: m.listAgentIdentityScopes, args: [env], key: ["agent-identity-scopes", env],
    idle: [() => useListAgentIdentityScopes({ ...env, envName: "" })],
  },
  {
    name: "useListThunderInstances",
    use: () => useListThunderInstances({ orgName: "o" }),
    api: vi.mocked(listThunderInstances), args: [{ orgName: "o" }], key: ["thunder-instances", { orgName: "o" }],
    idle: [() => useListThunderInstances({ orgName: "" })],
  },
], { setModulePresent });

const SEEDED = {
  groups: ["agent-identity-groups", env],
  otherEnvGroups: ["agent-identity-groups", { orgName: "o", envName: "prod" }],
  group: ["agent-identity-group", group],
  otherGroup: ["agent-identity-group", { ...env, groupId: "g2" }],
  members: ["agent-identity-group-members", group],
  otherMembers: ["agent-identity-group-members", { ...env, groupId: "g2" }],
  agentGroups: ["agent-groups", "a1"],
  g1Roles: ["agent-identity-group-roles", group],
  g2Roles: ["agent-identity-group-roles", { ...env, groupId: "g2" }],
  roles: ["agent-identity-roles", env],
  otherEnvRoles: ["agent-identity-roles", { orgName: "o", envName: "prod" }],
  role: ["agent-identity-role", role],
  otherRole: ["agent-identity-role", { ...env, roleId: "r2" }],
  assignments: ["agent-identity-role-assignments", role],
  otherAssignments: ["agent-identity-role-assignments", { ...env, roleId: "r2" }],
  agentRoles: ["agent-roles", "a1"],
} as const;

const groupBody = { name: "Readers" };
const roleBody = { name: "reader" };
const membersBody = { members: [{ id: "a1", type: "agent" }] };
// One group and one agent are assigned; only the group's role view goes stale.
const assignBody = { assignments: [{ id: "g1", type: "group" }, { id: "a1", type: "agent" }] };

describeMutationHooks("agent identity mutations", SEEDED, [
  {
    name: "useCreateAgentIdentityGroup", use: useCreateAgentIdentityGroup, api: m.createAgentIdentityGroup,
    variables: { params: env, body: groupBody }, args: [env, groupBody], invalidates: ["groups"],
  },
  {
    name: "useUpdateAgentIdentityGroup", use: useUpdateAgentIdentityGroup, api: m.updateAgentIdentityGroup,
    variables: { params: group, body: groupBody }, args: [group, groupBody],
    invalidates: ["groups", "otherEnvGroups", "group"],
  },
  {
    name: "useDeleteAgentIdentityGroup", use: useDeleteAgentIdentityGroup, api: m.deleteAgentIdentityGroup,
    variables: group, args: [group], invalidates: ["groups", "otherEnvGroups", "group"],
  },
  {
    name: "useAddAgentIdentityGroupMembers", use: useAddAgentIdentityGroupMembers, api: m.addAgentIdentityGroupMembers,
    variables: { params: group, body: membersBody }, args: [group, membersBody], invalidates: ["members", "agentGroups"],
  },
  {
    name: "useRemoveAgentIdentityGroupMembers", use: useRemoveAgentIdentityGroupMembers,
    api: m.removeAgentIdentityGroupMembers,
    variables: { params: group, body: membersBody }, args: [group, membersBody], invalidates: ["members", "agentGroups"],
  },
  {
    name: "useCreateAgentIdentityRole", use: useCreateAgentIdentityRole, api: m.createAgentIdentityRole,
    variables: { params: env, body: roleBody }, args: [env, roleBody], invalidates: ["roles"],
  },
  {
    name: "useUpdateAgentIdentityRole", use: useUpdateAgentIdentityRole, api: m.updateAgentIdentityRole,
    variables: { params: role, body: roleBody }, args: [role, roleBody], invalidates: ["roles", "otherEnvRoles", "role"],
  },
  {
    name: "useDeleteAgentIdentityRole", use: useDeleteAgentIdentityRole, api: m.deleteAgentIdentityRole,
    variables: role, args: [role], invalidates: ["roles", "otherEnvRoles", "role"],
  },
  {
    name: "useAddAgentIdentityRoleAssignees", use: useAddAgentIdentityRoleAssignees, api: m.addAgentIdentityRoleAssignees,
    variables: { params: role, body: assignBody }, args: [role, assignBody],
    invalidates: ["assignments", "agentRoles", "g1Roles"],
  },
  {
    name: "useRemoveAgentIdentityRoleAssignees", use: useRemoveAgentIdentityRoleAssignees,
    api: m.removeAgentIdentityRoleAssignees,
    variables: { params: role, body: assignBody }, args: [role, assignBody],
    invalidates: ["assignments", "agentRoles", "g1Roles"],
  },
]);

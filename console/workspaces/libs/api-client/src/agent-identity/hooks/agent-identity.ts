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

import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import {
  listAgentIdentityGroups,
  createAgentIdentityGroup,
  getAgentIdentityGroup,
  updateAgentIdentityGroup,
  deleteAgentIdentityGroup,
  getAgentIdentityGroupMembers,
  addAgentIdentityGroupMembers,
  removeAgentIdentityGroupMembers,
  getAgentIdentityGroupRoles,
  listAgentIdentityRoles,
  createAgentIdentityRole,
  getAgentIdentityRole,
  updateAgentIdentityRole,
  deleteAgentIdentityRole,
  getAgentIdentityRoleAssignments,
  addAgentIdentityRoleAssignees,
  removeAgentIdentityRoleAssignees,
  listAgentIdentityAgents,
  listAgentIdentityScopes,
} from "../apis/agent-identity";
import type {
  ThunderGroup,
  ThunderRole,
  AgentIdentityGroupListResponse,
  AgentIdentityGroupRequest,
  AgentIdentityGroupMembersResponse,
  AgentIdentityMembersRequest,
  AgentIdentityGroupRolesResponse,
  AgentIdentityRoleListResponse,
  AgentIdentityRoleRequest,
  AgentIdentityAssignmentsRequest,
  AgentIdentityRoleAssignmentsResponse,
  AgentIdentityAgentListResponse,
  AgentIdentityScopeListResponse,
  AgentIdentityEnvPathParams,
  AgentIdentityGroupPathParams,
  AgentIdentityRolePathParams,
  AgentIdentityListQuery,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "../../shared/react-query-notifications";
import { useUIModulePresent } from "../../shared/useUIModulePresent";

// --- Groups ---

export function useListAgentIdentityGroups(
  params: AgentIdentityEnvPathParams,
  query?: AgentIdentityListQuery,
) {
  const { getToken } = useAuthHooks();
  const identityPresent = useUIModulePresent("agent-identity");
  return useApiQuery<AgentIdentityGroupListResponse>({
    queryKey: ['agent-identity-groups', params, query],
    queryFn: () => listAgentIdentityGroups(params, query, getToken),
    enabled: identityPresent && (!!params.orgName && !!params.envName),
  });
}

export function useCreateAgentIdentityGroup() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    ThunderGroup, unknown, { params: AgentIdentityEnvPathParams; body: AgentIdentityGroupRequest }
  >({
    action: { verb: 'create', target: 'group' },
    mutationFn: ({ params, body }) => createAgentIdentityGroup(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['agent-identity-groups', params] });
    },
  });
}

export function useGetAgentIdentityGroup(params: AgentIdentityGroupPathParams) {
  const { getToken } = useAuthHooks();
  const identityPresent = useUIModulePresent("agent-identity");
  return useApiQuery<ThunderGroup>({
    queryKey: ['agent-identity-group', params],
    queryFn: () => getAgentIdentityGroup(params, getToken),
    enabled: identityPresent && (!!params.orgName && !!params.envName && !!params.groupId),
  });
}

export function useUpdateAgentIdentityGroup() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    ThunderGroup, unknown, { params: AgentIdentityGroupPathParams; body: AgentIdentityGroupRequest }
  >({
    action: { verb: 'update', target: 'group' },
    mutationFn: ({ params, body }) => updateAgentIdentityGroup(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['agent-identity-groups'] });
      queryClient.invalidateQueries({ queryKey: ['agent-identity-group', params] });
    },
  });
}

export function useDeleteAgentIdentityGroup() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, AgentIdentityGroupPathParams>({
    action: { verb: 'delete', target: 'group' },
    mutationFn: (params) => deleteAgentIdentityGroup(params, getToken),
    onSuccess: (_data, params) => {
      queryClient.invalidateQueries({ queryKey: ['agent-identity-groups'] });
      queryClient.invalidateQueries({ queryKey: ['agent-identity-group', params] });
    },
  });
}

export function useGetAgentIdentityGroupMembers(
  params: AgentIdentityGroupPathParams,
  query?: AgentIdentityListQuery,
) {
  const { getToken } = useAuthHooks();
  const identityPresent = useUIModulePresent("agent-identity");
  return useApiQuery<AgentIdentityGroupMembersResponse>({
    queryKey: ['agent-identity-group-members', params, query],
    queryFn: () => getAgentIdentityGroupMembers(params, query, getToken),
    enabled: identityPresent && (!!params.orgName && !!params.envName && !!params.groupId),
    refetchOnMount: 'always',
  });
}

export function useAddAgentIdentityGroupMembers() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    void, unknown, { params: AgentIdentityGroupPathParams; body: AgentIdentityMembersRequest }
  >({
    action: { verb: 'update', target: 'group members' },
    mutationFn: ({ params, body }) => addAgentIdentityGroupMembers(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['agent-identity-group-members', params] });
      queryClient.invalidateQueries({ queryKey: ['agent-groups'] });
    },
  });
}

export function useRemoveAgentIdentityGroupMembers() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    void, unknown, { params: AgentIdentityGroupPathParams; body: AgentIdentityMembersRequest }
  >({
    action: { verb: 'update', target: 'group members' },
    mutationFn: ({ params, body }) => removeAgentIdentityGroupMembers(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['agent-identity-group-members', params] });
      queryClient.invalidateQueries({ queryKey: ['agent-groups'] });
    },
  });
}

export function useGetAgentIdentityGroupRoles(params: AgentIdentityGroupPathParams) {
  const { getToken } = useAuthHooks();
  const identityPresent = useUIModulePresent("agent-identity");
  return useApiQuery<AgentIdentityGroupRolesResponse>({
    queryKey: ['agent-identity-group-roles', params],
    queryFn: () => getAgentIdentityGroupRoles(params, getToken),
    enabled: identityPresent && (!!params.orgName && !!params.envName && !!params.groupId),
  });
}

// --- Roles ---

export function useListAgentIdentityRoles(
  params: AgentIdentityEnvPathParams,
  query?: AgentIdentityListQuery,
) {
  const { getToken } = useAuthHooks();
  const identityPresent = useUIModulePresent("agent-identity");
  return useApiQuery<AgentIdentityRoleListResponse>({
    queryKey: ['agent-identity-roles', params, query],
    queryFn: () => listAgentIdentityRoles(params, query, getToken),
    enabled: identityPresent && (!!params.orgName && !!params.envName),
  });
}

export function useCreateAgentIdentityRole() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    ThunderRole, unknown, { params: AgentIdentityEnvPathParams; body: AgentIdentityRoleRequest }
  >({
    action: { verb: 'create', target: 'role' },
    mutationFn: ({ params, body }) => createAgentIdentityRole(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['agent-identity-roles', params] });
    },
  });
}

export function useGetAgentIdentityRole(params: AgentIdentityRolePathParams) {
  const { getToken } = useAuthHooks();
  const identityPresent = useUIModulePresent("agent-identity");
  return useApiQuery<ThunderRole>({
    queryKey: ['agent-identity-role', params],
    queryFn: () => getAgentIdentityRole(params, getToken),
    enabled: identityPresent && (!!params.orgName && !!params.envName && !!params.roleId),
  });
}

export function useUpdateAgentIdentityRole() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    ThunderRole, unknown, { params: AgentIdentityRolePathParams; body: AgentIdentityRoleRequest }
  >({
    action: { verb: 'update', target: 'role' },
    mutationFn: ({ params, body }) => updateAgentIdentityRole(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['agent-identity-roles'] });
      queryClient.invalidateQueries({ queryKey: ['agent-identity-role', params] });
    },
  });
}

export function useDeleteAgentIdentityRole() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, AgentIdentityRolePathParams>({
    action: { verb: 'delete', target: 'role' },
    mutationFn: (params) => deleteAgentIdentityRole(params, getToken),
    onSuccess: (_data, params) => {
      queryClient.invalidateQueries({ queryKey: ['agent-identity-roles'] });
      queryClient.invalidateQueries({ queryKey: ['agent-identity-role', params] });
    },
  });
}

export function useGetAgentIdentityRoleAssignments(params: AgentIdentityRolePathParams) {
  const { getToken } = useAuthHooks();
  const identityPresent = useUIModulePresent("agent-identity");
  return useApiQuery<AgentIdentityRoleAssignmentsResponse>({
    queryKey: ['agent-identity-role-assignments', params],
    queryFn: () => getAgentIdentityRoleAssignments(params, getToken),
    enabled: identityPresent && (!!params.orgName && !!params.envName && !!params.roleId),
    refetchOnMount: 'always',
  });
}

export function useAddAgentIdentityRoleAssignees() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    void, unknown, { params: AgentIdentityRolePathParams; body: AgentIdentityAssignmentsRequest }
  >({
    action: { verb: 'update', target: 'role assignments' },
    mutationFn: ({ params, body }) => addAgentIdentityRoleAssignees(params, body, getToken),
    onSuccess: (_data, { params, body }) => {
      queryClient.invalidateQueries({ queryKey: ['agent-identity-role-assignments', params] });
      queryClient.invalidateQueries({ queryKey: ['agent-roles'] });
      invalidateAssignedGroupRoles(queryClient, params, body);
    },
  });
}

export function useRemoveAgentIdentityRoleAssignees() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    void, unknown, { params: AgentIdentityRolePathParams; body: AgentIdentityAssignmentsRequest }
  >({
    action: { verb: 'update', target: 'role assignments' },
    mutationFn: ({ params, body }) => removeAgentIdentityRoleAssignees(params, body, getToken),
    onSuccess: (_data, { params, body }) => {
      queryClient.invalidateQueries({ queryKey: ['agent-identity-role-assignments', params] });
      queryClient.invalidateQueries({ queryKey: ['agent-roles'] });
      invalidateAssignedGroupRoles(queryClient, params, body);
    },
  });
}

// Role assignments are also editable from the group side (GroupEditPage's
// Roles tab calls these same role-keyed mutations), so a cached
// agent-identity-group-roles view for any group in this request's
// assignments may now be stale — invalidate just those groups' entries
// instead of every cached group-roles view in the org.
function invalidateAssignedGroupRoles(
  queryClient: QueryClient,
  { orgName, envName }: AgentIdentityRolePathParams,
  { assignments }: AgentIdentityAssignmentsRequest,
) {
  assignments
    .filter((assignment) => assignment.type === 'group')
    .forEach((assignment) => {
      queryClient.invalidateQueries({
        queryKey: ['agent-identity-group-roles', { orgName, envName, groupId: assignment.id }],
      });
    });
}

// --- Agents picker ---

export function useListAgentIdentityAgents(params: AgentIdentityEnvPathParams) {
  const { getToken } = useAuthHooks();
  const identityPresent = useUIModulePresent("agent-identity");
  return useApiQuery<AgentIdentityAgentListResponse>({
    queryKey: ['agent-identity-agents', params],
    queryFn: () => listAgentIdentityAgents(params, getToken),
    enabled: identityPresent && (!!params.orgName && !!params.envName),
  });
}

// --- Scopes ---

export function useListAgentIdentityScopes(params: AgentIdentityEnvPathParams) {
  const { getToken } = useAuthHooks();
  const identityPresent = useUIModulePresent("agent-identity");
  return useApiQuery<AgentIdentityScopeListResponse>({
    queryKey: ['agent-identity-scopes', params],
    queryFn: () => listAgentIdentityScopes(params, getToken),
    enabled: identityPresent && (!!params.orgName && !!params.envName),
  });
}

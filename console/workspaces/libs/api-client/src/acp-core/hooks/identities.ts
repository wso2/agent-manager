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

import { useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import {
  listUsers,
  getUser,
  getUserProfile,
  createUser,
  inviteUser,
  updateUser,
  deleteUser,
  getUserGroups,
  getUserRoles,
  listGroups,
  getGroup,
  createGroup,
  updateGroup,
  deleteGroup,
  addGroupMembers,
  removeGroupMembers,
  getGroupMembers,
  getGroupRoles,
  listRoles,
  getRole,
  createRole,
  updateRole,
  deleteRole,
  getRoleAssignments,
  addRolePermissions,
  removeRolePermissions,
  addRoleAssignees,
  removeRoleAssignees,
  listAMPPermissions,
  updateUserProfile,
} from "../apis/identities";
import type {
  ThunderUserListResponse, ThunderUser,
  CreateUserRequest, InviteUserRequest, InviteUserResponse, UpdateUserRequest,
  ThunderGroupListResponse, ThunderGroup, CreateGroupRequest, UpdateGroupRequest,
  ThunderRoleListResponse, ThunderRole, ThunderRoleAssignments,
  CreateRoleRequest, UpdateRoleRequest, RolePermissionRequest, RoleUserGroupRequest,
  AMPPermissionsResponse,
  IdentityOrgPathParams, UserPathParams, GroupPathParams, RolePathParams,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "../../shared/react-query-notifications";

// --- Users ---

export function useListUsers(
  params: IdentityOrgPathParams,
  query?: { offset?: number; limit?: number },
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ThunderUserListResponse>({
    queryKey: ['identity-users', params, query],
    queryFn: () => listUsers(params, query, getToken),
    enabled: !!params.orgName,
  });
}

export function useAllUsers(params: IdentityOrgPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<{ users: ThunderUser[] }>({
    queryKey: ['identity-users-all', params],
    queryFn: async () => {
      const PAGE_SIZE = 100;
      let offset = 0;
      let all: ThunderUser[] = [];
      while (true) {
        const page = await listUsers(params, { offset, limit: PAGE_SIZE }, getToken);
        all = all.concat(page.users);
        if (all.length >= page.total || page.users.length < PAGE_SIZE) break;
        offset += PAGE_SIZE;
      }
      return { users: all };
    },
    enabled: !!params.orgName,
  });
}

export function useGetUser(params: UserPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ThunderUser>({
    queryKey: ['identity-user', params],
    queryFn: () => getUser(params, getToken),
    enabled: !!params.orgName && !!params.userId,
  });
}

export function useCreateUser() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    ThunderUser, unknown, { params: IdentityOrgPathParams; body: CreateUserRequest }
  >({
    action: { verb: 'create', target: 'user' },
    mutationFn: ({ params, body }) => createUser(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['identity-users', params] });
      queryClient.invalidateQueries({ queryKey: ['identity-users-all', params] });
    },
  });
}

export function useInviteUser() {
  const { getToken } = useAuthHooks();
  return useApiMutation<
    InviteUserResponse, unknown, { params: IdentityOrgPathParams; body: InviteUserRequest }
  >({
    action: { verb: 'create', target: 'user invite' },
    mutationFn: ({ params, body }) => inviteUser(params, body, getToken),
  });
}

export function useUpdateUser() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<ThunderUser, unknown, { params: UserPathParams; body: UpdateUserRequest }>({
    action: { verb: 'update', target: 'user' },
    mutationFn: ({ params, body }) => updateUser(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['identity-users'] });
      queryClient.invalidateQueries({ queryKey: ['identity-users-all'] });
      queryClient.invalidateQueries({ queryKey: ['identity-user', params] });
    },
  });
}

export function useDeleteUser() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, UserPathParams>({
    action: { verb: 'delete', target: 'user' },
    mutationFn: (params) => deleteUser(params, getToken),
    onSuccess: (_data, params) => {
      queryClient.invalidateQueries({ queryKey: ['identity-users'] });
      queryClient.invalidateQueries({ queryKey: ['identity-users-all'] });
      queryClient.invalidateQueries({ queryKey: ['identity-user', params] });
    },
  });
}

/** Shared by the two hooks below so they hit one cache entry, one way. */
function userProfileQuery(params: UserPathParams, getToken: () => Promise<string>) {
  return {
    queryKey: ['identity-user-profile', params] as const,
    queryFn: () => getUserProfile(params, getToken),
    enabled: !!params.orgName && !!params.userId,
  };
}

export function useGetUserProfile(params: UserPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ThunderUser>(userProfileQuery(params, getToken));
}

/** Reads a name out of a user's profile attributes, if it has one. */
function readAttribute(attributes: ThunderUser['attributes'], key: string): string {
  const value = attributes?.[key];
  return typeof value === 'string' ? value.trim() : '';
}

/**
 * Resolves a user id to "First Last" for display, falling back to the profile's
 * username. Returns undefined until (or unless) the profile resolves, so the
 * caller decides what to show in the meantime.
 *
 * Queried with `silent` so a caller without permission to read user profiles
 * just keeps the fallback, rather than getting an error snackbar on a page whose
 * own content loaded fine — auth handling still applies.
 */
export function useUserDisplayName(params: UserPathParams): string | undefined {
  const { getToken } = useAuthHooks();
  const { data, error } = useApiQuery<ThunderUser>({
    ...userProfileQuery(params, getToken),
    retry: false,
    staleTime: 5 * 60 * 1000,
    silent: true,
  });

  // A deleted user (404) or a caller without user-read permission (403) are
  // expected outcomes here and just leave the fallback in place. Anything else
  // means the identity service itself is unhealthy: still no toast for a
  // decoration, but don't let an outage disappear without a trace.
  useEffect(() => {
    if (!error) return;
    const status = (error as { status?: number })?.status;
    if (status === 403 || status === 404) return;
    // eslint-disable-next-line no-console
    console.warn('Failed to resolve user display name', { userId: params.userId, status }, error);
  }, [error, params.userId]);

  const fullName = [
    readAttribute(data?.attributes, 'given_name'),
    readAttribute(data?.attributes, 'family_name'),
  ]
    .filter(Boolean)
    .join(' ');

  return fullName || readAttribute(data?.attributes, 'username') || undefined;
}

export function useUpdateUserProfile() {
  const { getToken } = useAuthHooks();
  return useApiMutation<ThunderUser, unknown, { params: UserPathParams; body: UpdateUserRequest }>({
    action: { verb: 'update', target: 'profile' },
    mutationFn: ({ params, body }) => updateUserProfile(params, body, getToken),
  });
}

export function useGetUserGroups(params: UserPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<{ groups: ThunderGroup[] }>({
    queryKey: ['identity-user-groups', params],
    queryFn: () => getUserGroups(params, getToken),
    enabled: !!params.orgName && !!params.userId,
  });
}

export function useGetUserRoles(params: UserPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<{ roles: ThunderRole[] }>({
    queryKey: ['identity-user-roles', params],
    queryFn: () => getUserRoles(params, getToken),
    enabled: !!params.orgName && !!params.userId,
  });
}

// --- Groups ---

export function useListGroups(
  params: IdentityOrgPathParams,
  query?: { offset?: number; limit?: number },
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ThunderGroupListResponse>({
    queryKey: ['identity-groups', params, query],
    queryFn: () => listGroups(params, query, getToken),
    enabled: !!params.orgName,
  });
}

export function useAllGroups(params: IdentityOrgPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<{ groups: ThunderGroup[] }>({
    queryKey: ['identity-groups-all', params],
    queryFn: async () => {
      const PAGE_SIZE = 100;
      let offset = 0;
      let all: ThunderGroup[] = [];
      while (true) {
        const page = await listGroups(params, { offset, limit: PAGE_SIZE }, getToken);
        all = all.concat(page.groups);
        if (all.length >= page.total || page.groups.length < PAGE_SIZE) break;
        offset += PAGE_SIZE;
      }
      return { groups: all };
    },
    enabled: !!params.orgName,
  });
}

export function useGetGroup(params: GroupPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ThunderGroup>({
    queryKey: ['identity-group', params],
    queryFn: () => getGroup(params, getToken),
    enabled: !!params.orgName && !!params.groupId,
  });
}

export function useCreateGroup() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    ThunderGroup, unknown, { params: IdentityOrgPathParams; body: CreateGroupRequest }
  >({
    action: { verb: 'create', target: 'group' },
    mutationFn: ({ params, body }) => createGroup(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['identity-groups', params] });
      queryClient.invalidateQueries({ queryKey: ['identity-groups-all', params] });
    },
  });
}

export function useUpdateGroup() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    ThunderGroup, unknown, { params: GroupPathParams; body: UpdateGroupRequest }
  >({
    action: { verb: 'update', target: 'group' },
    mutationFn: ({ params, body }) => updateGroup(params, body, getToken),
    onSuccess: (_data, { params }) => {
      const orgParams = { orgName: params.orgName };
      queryClient.invalidateQueries({ queryKey: ['identity-groups'] });
      queryClient.invalidateQueries({ queryKey: ['identity-groups-all', orgParams] });
      queryClient.invalidateQueries({ queryKey: ['identity-group', params] });
    },
  });
}

export function useDeleteGroup() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, GroupPathParams>({
    action: { verb: 'delete', target: 'group' },
    mutationFn: (params) => deleteGroup(params, getToken),
    onSuccess: (_data, params) => {
      const orgParams = { orgName: params.orgName };
      queryClient.invalidateQueries({ queryKey: ['identity-groups'] });
      queryClient.invalidateQueries({ queryKey: ['identity-groups-all', orgParams] });
      queryClient.invalidateQueries({ queryKey: ['identity-group', params] });
    },
  });
}

export function useAddGroupMembers() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, { params: GroupPathParams; body: { userIds: string[] } }>({
    action: { verb: 'update', target: 'group members' },
    mutationFn: ({ params, body }) => addGroupMembers(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['identity-group-members'] });
      queryClient.invalidateQueries({ queryKey: ['identity-group-member-ids-all'] });
      queryClient.invalidateQueries({ queryKey: ['identity-user-groups'] });
    },
  });
}

export function useRemoveGroupMembers() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, { params: GroupPathParams; body: { userIds: string[] } }>({
    action: { verb: 'update', target: 'group members' },
    mutationFn: ({ params, body }) => removeGroupMembers(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['identity-group-members'] });
      queryClient.invalidateQueries({ queryKey: ['identity-group-member-ids-all'] });
      queryClient.invalidateQueries({ queryKey: ['identity-user-groups'] });
    },
  });
}

export function useGetGroupRoles(params: GroupPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<{ roles: ThunderRole[] }>({
    queryKey: ['identity-group-roles', params],
    queryFn: () => getGroupRoles(params, getToken),
    enabled: !!params.orgName && !!params.groupId,
  });
}

export function useGetGroupMembers(
  params: GroupPathParams,
  query?: { offset?: number; limit?: number },
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ThunderUserListResponse>({
    queryKey: ['identity-group-members', params, query],
    queryFn: () => getGroupMembers(params, query, getToken),
    enabled: !!params.orgName && !!params.groupId,
    refetchOnMount: 'always',
  });
}

export function useAllGroupMemberIds(params: GroupPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<{ memberIds: string[] }>({
    queryKey: ['identity-group-member-ids-all', params],
    queryFn: async () => {
      const PAGE_SIZE = 100;
      let offset = 0;
      const ids: string[] = [];
      while (true) {
        const page = await getGroupMembers(
          params,
          { offset, limit: PAGE_SIZE },
          getToken,
        );
        for (const u of page.users) ids.push(u.id);
        if (ids.length >= page.total || page.users.length < PAGE_SIZE) break;
        offset += PAGE_SIZE;
      }
      return { memberIds: ids };
    },
    enabled: !!params.orgName && !!params.groupId,
  });
}

// --- Roles ---

export function useListRoles(
  params: IdentityOrgPathParams,
  query?: { offset?: number; limit?: number },
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ThunderRoleListResponse>({
    queryKey: ['identity-roles', params, query],
    queryFn: () => listRoles(params, query, getToken),
    enabled: !!params.orgName,
  });
}

export function useGetRole(params: RolePathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ThunderRole>({
    queryKey: ['identity-role', params],
    queryFn: () => getRole(params, getToken),
    enabled: !!params.orgName && !!params.roleId,
  });
}

export function useCreateRole() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    ThunderRole, unknown, { params: IdentityOrgPathParams; body: CreateRoleRequest }
  >({
    action: { verb: 'create', target: 'role' },
    mutationFn: ({ params, body }) => createRole(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['identity-roles', params] });
    },
  });
}

export function useUpdateRole() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<ThunderRole, unknown, { params: RolePathParams; body: UpdateRoleRequest }>({
    action: { verb: 'update', target: 'role' },
    mutationFn: ({ params, body }) => updateRole(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['identity-roles'] });
      queryClient.invalidateQueries({ queryKey: ['identity-role', params] });
    },
  });
}

export function useDeleteRole() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, RolePathParams>({
    action: { verb: 'delete', target: 'role' },
    mutationFn: (params) => deleteRole(params, getToken),
    onSuccess: (_data, params) => {
      queryClient.invalidateQueries({ queryKey: ['identity-roles'] });
      queryClient.invalidateQueries({ queryKey: ['identity-role', params] });
    },
  });
}

export function useGetRoleAssignments(params: RolePathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ThunderRoleAssignments>({
    queryKey: ['identity-role-assignments', params],
    queryFn: () => getRoleAssignments(params, getToken),
    enabled: !!params.orgName && !!params.roleId,
    refetchOnMount: 'always',
  });
}

export function useAddRolePermissions() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, { params: RolePathParams; body: RolePermissionRequest }>({
    action: { verb: 'update', target: 'role permissions' },
    mutationFn: ({ params, body }) => addRolePermissions(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['identity-role-assignments', params] });
    },
  });
}

export function useRemoveRolePermissions() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, { params: RolePathParams; body: RolePermissionRequest }>({
    action: { verb: 'update', target: 'role permissions' },
    mutationFn: ({ params, body }) => removeRolePermissions(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['identity-role-assignments', params] });
    },
  });
}

export function useAddRoleAssignees() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, { params: RolePathParams; body: RoleUserGroupRequest }>({
    action: { verb: 'update', target: 'role assignments' },
    mutationFn: ({ params, body }) => addRoleAssignees(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['identity-role-assignments'] });
    },
  });
}

export function useRemoveRoleAssignees() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, { params: RolePathParams; body: RoleUserGroupRequest }>({
    action: { verb: 'update', target: 'role assignments' },
    mutationFn: ({ params, body }) => removeRoleAssignees(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['identity-role-assignments'] });
    },
  });
}

// --- Permissions catalog ---

export function useListAMPPermissions(params: IdentityOrgPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AMPPermissionsResponse>({
    queryKey: ['amp-permissions', params],
    queryFn: () => listAMPPermissions(params, getToken),
    enabled: !!params.orgName,
  });
}

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

import { httpDELETE, httpGET, httpPOST, httpPUT, SERVICE_BASE } from "../../shared/utils";
import type {
  ThunderUserListResponse,
  ThunderUser,
  CreateUserRequest,
  UpdateUserRequest,
  InviteUserRequest,
  InviteUserResponse,
  ThunderGroupListResponse,
  ThunderGroup,
  CreateGroupRequest,
  UpdateGroupRequest,
  ThunderRoleListResponse,
  ThunderRole,
  ThunderRoleAssignments,
  CreateRoleRequest,
  UpdateRoleRequest,
  RolePermissionRequest,
  RoleUserGroupRequest,
  AMPPermissionsResponse,
  IdentityOrgPathParams,
  UserPathParams,
  GroupPathParams,
  RolePathParams,
} from "@agent-management-platform/types";

const orgBase = (orgName: string) =>
  `${SERVICE_BASE}/orgs/${encodeURIComponent(orgName)}/identities`;

// --- Users ---

export async function listUsers(
  params: IdentityOrgPathParams,
  query?: { offset?: number; limit?: number },
  getToken?: () => Promise<string>,
): Promise<ThunderUserListResponse> {
  const { orgName = "default" } = params;
  const token = getToken ? await getToken() : undefined;
  const search = query
    ? { offset: String(query.offset ?? 0), limit: String(query.limit ?? 20) }
    : undefined;
  const res = await httpGET(`${orgBase(orgName)}/users`, { searchParams: search, token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getUser(
  params: UserPathParams,
  getToken?: () => Promise<string>,
): Promise<ThunderUser> {
  const { orgName = "default", userId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(`${orgBase(orgName)}/users/${encodeURIComponent(userId)}`, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getUserProfile(
  params: UserPathParams,
  getToken?: () => Promise<string>,
): Promise<ThunderUser> {
  const { orgName = "default", userId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(`${orgBase(orgName)}/users/${encodeURIComponent(userId)}/profile`, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createUser(
  params: IdentityOrgPathParams,
  body: CreateUserRequest,
  getToken?: () => Promise<string>,
): Promise<ThunderUser> {
  const { orgName = "default" } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(`${orgBase(orgName)}/users`, body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateUser(
  params: UserPathParams,
  body: UpdateUserRequest,
  getToken?: () => Promise<string>,
): Promise<ThunderUser> {
  const { orgName = "default", userId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(
    `${orgBase(orgName)}/users/${encodeURIComponent(userId)}`, body, { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteUser(
  params: UserPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName = "default", userId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(
    `${orgBase(orgName)}/users/${encodeURIComponent(userId)}`, { token },
  );
  if (!res.ok && res.status !== 204) throw await res.json();
}

export async function inviteUser(
  params: IdentityOrgPathParams,
  body: InviteUserRequest,
  getToken?: () => Promise<string>,
): Promise<InviteUserResponse> {
  const { orgName = "default" } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(`${orgBase(orgName)}/users/invite`, body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getUserGroups(
  params: UserPathParams,
  getToken?: () => Promise<string>,
): Promise<{ groups: ThunderGroup[] }> {
  const { orgName = "default", userId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${orgBase(orgName)}/users/${encodeURIComponent(userId)}/groups`, { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getUserRoles(
  params: UserPathParams,
  getToken?: () => Promise<string>,
): Promise<{ roles: ThunderRole[] }> {
  const { orgName = "default", userId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${orgBase(orgName)}/users/${encodeURIComponent(userId)}/roles`, { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

// --- Groups ---

export async function listGroups(
  params: IdentityOrgPathParams,
  query?: { offset?: number; limit?: number },
  getToken?: () => Promise<string>,
): Promise<ThunderGroupListResponse> {
  const { orgName = "default" } = params;
  const token = getToken ? await getToken() : undefined;
  const search = query
    ? { offset: String(query.offset ?? 0), limit: String(query.limit ?? 20) }
    : undefined;
  const res = await httpGET(`${orgBase(orgName)}/groups`, { searchParams: search, token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getGroup(
  params: GroupPathParams,
  getToken?: () => Promise<string>,
): Promise<ThunderGroup> {
  const { orgName = "default", groupId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(`${orgBase(orgName)}/groups/${encodeURIComponent(groupId)}`, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createGroup(
  params: IdentityOrgPathParams,
  body: CreateGroupRequest,
  getToken?: () => Promise<string>,
): Promise<ThunderGroup> {
  const { orgName = "default" } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(`${orgBase(orgName)}/groups`, body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateGroup(
  params: GroupPathParams,
  body: UpdateGroupRequest,
  getToken?: () => Promise<string>,
): Promise<ThunderGroup> {
  const { orgName = "default", groupId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(
    `${orgBase(orgName)}/groups/${encodeURIComponent(groupId)}`, body, { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteGroup(
  params: GroupPathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName = "default", groupId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(
    `${orgBase(orgName)}/groups/${encodeURIComponent(groupId)}`, { token },
  );
  if (!res.ok && res.status !== 204) throw await res.json();
}

export async function addGroupMembers(
  params: GroupPathParams,
  body: { userIds: string[] },
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName = "default", groupId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${orgBase(orgName)}/groups/${encodeURIComponent(groupId)}/members/add`, body, { token },
  );
  if (!res.ok) throw await res.json();
}

export async function removeGroupMembers(
  params: GroupPathParams,
  body: { userIds: string[] },
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName = "default", groupId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${orgBase(orgName)}/groups/${encodeURIComponent(groupId)}/members/remove`, body, { token },
  );
  if (!res.ok) throw await res.json();
}

export async function getGroupRoles(
  params: GroupPathParams,
  getToken?: () => Promise<string>,
): Promise<{ roles: ThunderRole[] }> {
  const { orgName = "default", groupId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${orgBase(orgName)}/groups/${encodeURIComponent(groupId)}/roles`, { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getGroupMembers(
  params: GroupPathParams,
  query?: { offset?: number; limit?: number },
  getToken?: () => Promise<string>,
): Promise<ThunderUserListResponse> {
  const { orgName = "default", groupId } = params;
  const token = getToken ? await getToken() : undefined;
  const search = query
    ? { offset: String(query.offset ?? 0), limit: String(query.limit ?? 20) }
    : undefined;
  const res = await httpGET(
    `${orgBase(orgName)}/groups/${encodeURIComponent(groupId)}/members`,
    { searchParams: search, token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

// --- Roles ---

export async function listRoles(
  params: IdentityOrgPathParams,
  query?: { offset?: number; limit?: number },
  getToken?: () => Promise<string>,
): Promise<ThunderRoleListResponse> {
  const { orgName = "default" } = params;
  const token = getToken ? await getToken() : undefined;
  const search = query
    ? { offset: String(query.offset ?? 0), limit: String(query.limit ?? 20) }
    : undefined;
  const res = await httpGET(`${orgBase(orgName)}/roles`, { searchParams: search, token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function getRole(
  params: RolePathParams,
  getToken?: () => Promise<string>,
): Promise<ThunderRole> {
  const { orgName = "default", roleId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(`${orgBase(orgName)}/roles/${encodeURIComponent(roleId)}`, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function createRole(
  params: IdentityOrgPathParams,
  body: CreateRoleRequest,
  getToken?: () => Promise<string>,
): Promise<ThunderRole> {
  const { orgName = "default" } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(`${orgBase(orgName)}/roles`, body, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function updateRole(
  params: RolePathParams,
  body: UpdateRoleRequest,
  getToken?: () => Promise<string>,
): Promise<ThunderRole> {
  const { orgName = "default", roleId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(
    `${orgBase(orgName)}/roles/${encodeURIComponent(roleId)}`, body, { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function deleteRole(
  params: RolePathParams,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName = "default", roleId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpDELETE(
    `${orgBase(orgName)}/roles/${encodeURIComponent(roleId)}`, { token },
  );
  if (!res.ok && res.status !== 204) throw await res.json();
}

export async function getRoleAssignments(
  params: RolePathParams,
  getToken?: () => Promise<string>,
): Promise<ThunderRoleAssignments> {
  const { orgName = "default", roleId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(
    `${orgBase(orgName)}/roles/${encodeURIComponent(roleId)}/assignments`, { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

export async function addRolePermissions(
  params: RolePathParams,
  body: RolePermissionRequest,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName = "default", roleId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${orgBase(orgName)}/roles/${encodeURIComponent(roleId)}/permissions/add`, body, { token },
  );
  if (!res.ok) throw await res.json();
}

export async function removeRolePermissions(
  params: RolePathParams,
  body: RolePermissionRequest,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName = "default", roleId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${orgBase(orgName)}/roles/${encodeURIComponent(roleId)}/permissions/remove`, body, { token },
  );
  if (!res.ok) throw await res.json();
}

export async function addRoleAssignees(
  params: RolePathParams,
  body: RoleUserGroupRequest,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName = "default", roleId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${orgBase(orgName)}/roles/${encodeURIComponent(roleId)}/assignees/add`, body, { token },
  );
  if (!res.ok) throw await res.json();
}

export async function removeRoleAssignees(
  params: RolePathParams,
  body: RoleUserGroupRequest,
  getToken?: () => Promise<string>,
): Promise<void> {
  const { orgName = "default", roleId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPOST(
    `${orgBase(orgName)}/roles/${encodeURIComponent(roleId)}/assignees/remove`, body, { token },
  );
  if (!res.ok) throw await res.json();
}

// --- User Profile & Credentials (Self) ---

export async function updateUserProfile(
  params: UserPathParams,
  body: UpdateUserRequest,
  getToken?: () => Promise<string>,
): Promise<ThunderUser> {
  const { orgName = "default", userId } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpPUT(
    `${orgBase(orgName)}/users/${encodeURIComponent(userId)}/profile`, body, { token },
  );
  if (!res.ok) throw await res.json();
  return res.json();
}

// --- Permissions catalog ---

export async function listAMPPermissions(
  params: IdentityOrgPathParams,
  getToken?: () => Promise<string>,
): Promise<AMPPermissionsResponse> {
  const { orgName = "default" } = params;
  const token = getToken ? await getToken() : undefined;
  const res = await httpGET(`${orgBase(orgName)}/permissions`, { token });
  if (!res.ok) throw await res.json();
  return res.json();
}

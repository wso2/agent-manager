/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import React, { useEffect, useMemo, useRef, useState } from "react";
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Chip,
  Form,
  Grid,
  IconButton,
  ListingTable,
  Stack,
  Tab,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";

import { Folder, Trash, Users } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  useAllUsers,
  useAllGroups,
  useGetRole,
  useGetRoleAssignments,
  useAddRoleAssignees,
  useRemoveRoleAssignees,
  useAddRolePermissions,
  useRemoveRolePermissions,
  useListAMPPermissions,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type ThunderUser,
  type ThunderGroup,
  type ThunderPermission,
} from "@agent-management-platform/types";
import {
  PermissionTree,
  type PermissionTreeItem,
  useUnsavedChangesGuard,
} from "@agent-management-platform/shared-component";
import { BackButton } from "./components/BackButton";
import { EditFormSkeleton } from "./components/EditFormSkeleton";
import { EntityHeader } from "./components/EntityHeader";

type ActiveTab = "permissions" | "users" | "groups";

const permLabel = (p: ThunderPermission) =>
  p.actionName || p.name.split(":")[1] || p.name;
const permGroup = (p: ThunderPermission) =>
  p.resourceName || p.name.split(":")[0];

export const RoleEditPage: React.FC = () => {
  const { orgId, roleId } = useParams<{ orgId: string; roleId: string }>();
  const navigate = useNavigate();

  const [activeTab, setActiveTab] = useState<ActiveTab>("permissions");
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | undefined>();
  const [saveSuccess, setSaveSuccess] = useState(false);

  const { data: roleData, isLoading: isLoadingRole } = useGetRole({
    orgName: orgId,
    roleId: roleId ?? "",
  });
  const isPermissionsReadOnly = roleData?.isReadOnly ?? false;
  const { data: assignmentsData, isLoading: isLoadingAssignments } =
    useGetRoleAssignments({
      orgName: orgId,
      roleId: roleId ?? "",
    });
  const { data: allUsersData, isLoading: isLoadingUsers } = useAllUsers({
    orgName: orgId,
  });
  const { data: allGroupsData, isLoading: isLoadingGroups } = useAllGroups({
    orgName: orgId,
  });
  const { data: catalogData, isLoading: isLoadingCatalog } =
    useListAMPPermissions({
      orgName: orgId,
    });

  const { mutateAsync: addAssignees } = useAddRoleAssignees();
  const { mutateAsync: removeAssignees } = useRemoveRoleAssignees();
  const { mutateAsync: addPermissions } = useAddRolePermissions();
  const { mutateAsync: removePermissions } = useRemoveRolePermissions();

  // --- Derived server state ---
  const initialUsers: ThunderUser[] = useMemo(
    () => assignmentsData?.users ?? [],
    [assignmentsData],
  );
  const initialGroups: ThunderGroup[] = useMemo(
    () => assignmentsData?.groups ?? [],
    [assignmentsData],
  );
  const initialPermissions: string[] = useMemo(
    () => roleData?.permissions?.flatMap((rp) => rp.permissions) ?? [],
    [roleData],
  );

  const allUsers: ThunderUser[] = useMemo(
    () => allUsersData?.users ?? [],
    [allUsersData],
  );
  const allGroups: ThunderGroup[] = useMemo(
    () => allGroupsData?.groups ?? [],
    [allGroupsData],
  );

  const catalogPermissions: ThunderPermission[] = useMemo(
    () => catalogData?.permissions ?? [],
    [catalogData],
  );
  const resourceServerId: string = catalogData?.resourceServerId ?? "";

  // --- User tab delta tracking ---
  const [pendingUserAdds, setPendingUserAdds] = useState<ThunderUser[]>([]);
  const [removedUserIds, setRemovedUserIds] = useState<Set<string>>(new Set());

  // --- Group tab delta tracking ---
  const [pendingGroupAdds, setPendingGroupAdds] = useState<ThunderGroup[]>([]);
  const [removedGroupIds, setRemovedGroupIds] = useState<Set<string>>(
    new Set(),
  );

  // --- Permissions tab: full selected-state approach ---
  // Held as bare permission names (not ThunderPermission objects) since every
  // consumer below — the tree, the dirty check, and the add/remove diffing —
  // only ever needs the name.
  const [selectedPermissionIds, setSelectedPermissionIds] = useState<
    string[]
  >([]);
  const hasEditedPermissions = useRef(false);

  // Initialise selectedPermissionIds from server data once (guard against refetch overwrites)
  useEffect(() => {
    if (!hasEditedPermissions.current && !isLoadingCatalog) {
      setSelectedPermissionIds(initialPermissions);
    }
  }, [initialPermissions, isLoadingCatalog]);

  const rolesPath = orgId
    ? generatePath(
        absoluteRouteMap.children.org.children.settings.children.identities
          .children.roles.path,
        { orgId },
      )
    : "#";

  // --- Derived displayed lists (users / groups) ---
  const displayedUsers = useMemo(() => {
    const base = initialUsers.filter((u) => !removedUserIds.has(u.id));
    return [...base, ...pendingUserAdds];
  }, [initialUsers, pendingUserAdds, removedUserIds]);

  const displayedGroups = useMemo(() => {
    const base = initialGroups.filter((g) => !removedGroupIds.has(g.id));
    return [...base, ...pendingGroupAdds];
  }, [initialGroups, pendingGroupAdds, removedGroupIds]);

  const displayedUserIds = useMemo(
    () => new Set(displayedUsers.map((u) => u.id)),
    [displayedUsers],
  );
  const displayedGroupIds = useMemo(
    () => new Set(displayedGroups.map((g) => g.id)),
    [displayedGroups],
  );

  const availableUsers = useMemo(
    () => allUsers.filter((u) => !displayedUserIds.has(u.id)),
    [allUsers, displayedUserIds],
  );
  const availableGroups = useMemo(
    () => allGroups.filter((g) => !displayedGroupIds.has(g.id)),
    [allGroups, displayedGroupIds],
  );

  const permissionTreeItems: PermissionTreeItem[] = useMemo(
    () =>
      catalogPermissions.map((p) => ({
        id: p.name,
        path: [permGroup(p), permLabel(p)],
      })),
    [catalogPermissions],
  );

  const getUsername = (user: ThunderUser) =>
    String(user.attributes?.["username"] ?? user.id ?? "");

  // --- User handlers ---
  const handleAddUser = (
    _e: React.SyntheticEvent,
    value: ThunderUser | null,
  ) => {
    if (!value) return;
    if (removedUserIds.has(value.id)) {
      setRemovedUserIds((prev) => {
        const n = new Set(prev);
        n.delete(value.id);
        return n;
      });
    } else {
      setPendingUserAdds((prev) => [...prev, value]);
    }
  };

  const handleRemoveUser = (userId: string) => {
    if (pendingUserAdds.find((u) => u.id === userId)) {
      setPendingUserAdds((prev) => prev.filter((u) => u.id !== userId));
    } else {
      setRemovedUserIds((prev) => new Set([...prev, userId]));
    }
  };

  // --- Group handlers ---
  const handleAddGroup = (
    _e: React.SyntheticEvent,
    value: ThunderGroup | null,
  ) => {
    if (!value) return;
    if (removedGroupIds.has(value.id)) {
      setRemovedGroupIds((prev) => {
        const n = new Set(prev);
        n.delete(value.id);
        return n;
      });
    } else {
      setPendingGroupAdds((prev) => [...prev, value]);
    }
  };

  const handleRemoveGroup = (groupId: string) => {
    if (pendingGroupAdds.find((g) => g.id === groupId)) {
      setPendingGroupAdds((prev) => prev.filter((g) => g.id !== groupId));
    } else {
      setRemovedGroupIds((prev) => new Set([...prev, groupId]));
    }
  };

  // --- Permissions handler ---
  const handlePermissionSelectionChange = (names: string[]) => {
    hasEditedPermissions.current = true;
    setSelectedPermissionIds(names);
  };

  // --- Save ---
  const handleSave = async () => {
    if (!orgId || !roleId) return;
    setSaveError(undefined);
    setSaveSuccess(false);
    setIsSaving(true);
    try {
      const params = { orgName: orgId, roleId };

      // Users
      const addUserIds = pendingUserAdds.map((u) => u.id);
      const removeUserIds = [...removedUserIds];
      if (addUserIds.length > 0) {
        await addAssignees({ params, body: { userIds: addUserIds } });
      }
      if (removeUserIds.length > 0) {
        await removeAssignees({ params, body: { userIds: removeUserIds } });
      }

      // Groups
      const addGroupIds = pendingGroupAdds.map((g) => g.id);
      const removeGroupIds = [...removedGroupIds];
      if (addGroupIds.length > 0) {
        await addAssignees({ params, body: { groupIds: addGroupIds } });
      }
      if (removeGroupIds.length > 0) {
        await removeAssignees({ params, body: { groupIds: removeGroupIds } });
      }

      // Permissions — diff selected vs initial (skip for predefined roles)
      if (
        hasEditedPermissions.current &&
        resourceServerId &&
        !isPermissionsReadOnly
      ) {
        const currentSet = new Set(initialPermissions);
        const nextSet = new Set(selectedPermissionIds);
        const toAdd = [...nextSet].filter((n) => !currentSet.has(n));
        const toRemove = [...currentSet].filter((n) => !nextSet.has(n));
        if (toAdd.length > 0) {
          await addPermissions({
            params,
            body: { resourceServerId, permissions: toAdd },
          });
        }
        if (toRemove.length > 0) {
          await removePermissions({
            params,
            body: { resourceServerId, permissions: toRemove },
          });
        }
      }

      setSaveSuccess(true);
      setPendingUserAdds([]);
      setRemovedUserIds(new Set());
      setPendingGroupAdds([]);
      setRemovedGroupIds(new Set());
      hasEditedPermissions.current = false;
    } catch {
      setSaveError("Failed to update role. Please try again.");
    } finally {
      setIsSaving(false);
    }
  };

  const isLoading =
    isLoadingRole ||
    isLoadingAssignments ||
    isLoadingUsers ||
    isLoadingGroups ||
    isLoadingCatalog;

  // Surface the action row only when something differs from what's saved —
  // any pending user/group add or removal, or a changed permission selection.
  const permissionsDirty = useMemo(() => {
    if (isPermissionsReadOnly) return false;
    const initial = new Set(initialPermissions);
    return (
      initial.size !== selectedPermissionIds.length ||
      selectedPermissionIds.some((id) => !initial.has(id))
    );
  }, [isPermissionsReadOnly, initialPermissions, selectedPermissionIds]);

  const isDirty =
    permissionsDirty ||
    pendingUserAdds.length > 0 ||
    removedUserIds.size > 0 ||
    pendingGroupAdds.length > 0 ||
    removedGroupIds.size > 0;
  const { allowNavigation } = useUnsavedChangesGuard(
    (permissionsDirty && hasEditedPermissions.current) ||
      pendingUserAdds.length > 0 ||
      removedUserIds.size > 0 ||
      pendingGroupAdds.length > 0 ||
      removedGroupIds.size > 0,
  );

  if (isLoading) {
    return (
      <>
        <BackButton to={rolesPath} label="Roles" />
        <EditFormSkeleton tabs={3} />
      </>
    );
  }

  return (
    <>
      <BackButton to={rolesPath} label="Roles" />
      <Stack spacing={3}>
        <EntityHeader
          fallback="R"
          name={roleData?.name ?? ""}
          subtitle={roleData?.description}
          id={roleId ?? ""}
          badge={isPermissionsReadOnly ? <Chip label="Read-only" size="small" /> : undefined}
        />
        {saveError != null && <Alert severity="error">{saveError}</Alert>}
        {saveSuccess && (
          <Alert severity="success">Role updated successfully.</Alert>
        )}

        <Form.Stack spacing={3}>
          <Tabs
            value={activeTab}
            onChange={(_e, v) => setActiveTab(v as ActiveTab)}
            sx={{ borderBottom: 1, borderColor: "divider" }}
          >
            <Tab label="Permissions" value="permissions" />
            <Tab label="Users" value="users" />
            <Tab label="Groups" value="groups" />
          </Tabs>

          {/* ── Permissions tab ── */}
          {activeTab === "permissions" && (
            <>
              <Typography variant="subtitle1">Permissions</Typography>
              <Typography variant="body2" color="text.secondary">
                {isPermissionsReadOnly
                  ? "Permissions for predefined roles cannot be modified."
                  : "Check the permissions to assign to this role."}
              </Typography>

              <Box sx={{ mt: 1 }}>
                <PermissionTree
                  items={permissionTreeItems}
                  selectedIds={selectedPermissionIds}
                  onChange={handlePermissionSelectionChange}
                  readOnly={isPermissionsReadOnly}
                  emptyMessage="No permissions available."
                />
              </Box>
            </>
          )}

          {/* ── Users tab ── */}
          {activeTab === "users" && (
            <>
              <Typography variant="subtitle1">Assigned Users</Typography>
              <Typography variant="body2" color="text.secondary">
                Search and add users to this role.
              </Typography>

              <Grid container spacing={2}>
                <Grid size={{ xs: 12, md: 12, xl: 8 }}>
                <Form.ElementWrapper label="Add User" name="addUser">
                  <Autocomplete
                    id="addUser"
                    options={availableUsers}
                    getOptionLabel={(option) =>
                      getUsername(option as ThunderUser)
                    }
                    onChange={handleAddUser}
                    value={null}
                    renderInput={(params) => (
                      <TextField {...params} placeholder="Search users..." />
                    )}
                    noOptionsText="No users available"
                  />
                </Form.ElementWrapper>
                </Grid>
              </Grid>

              {displayedUsers.length === 0 ? (
                <ListingTable.Container>
                  <ListingTable.EmptyState
                    illustration={<Users size={64} />}
                    title="No users assigned yet"
                    description="Search and add users above."
                  />
                </ListingTable.Container>
              ) : (
                <ListingTable.Container>
                  <ListingTable>
                    <ListingTable.Head>
                      <ListingTable.Row>
                        <ListingTable.Cell>Username</ListingTable.Cell>
                        <ListingTable.Cell>User ID</ListingTable.Cell>
                        <ListingTable.Cell />
                      </ListingTable.Row>
                    </ListingTable.Head>
                    <ListingTable.Body>
                      {displayedUsers.map((user) => (
                        <ListingTable.Row key={user.id}>
                          <ListingTable.Cell>
                            {getUsername(user)}
                          </ListingTable.Cell>
                          <ListingTable.Cell>{user.id}</ListingTable.Cell>
                          <ListingTable.Cell align="right">
                            <Tooltip title="Remove from role">
                              <IconButton
                                size="small"
                                onClick={() => handleRemoveUser(user.id)}
                              >
                                <Trash size={16} />
                              </IconButton>
                            </Tooltip>
                          </ListingTable.Cell>
                        </ListingTable.Row>
                      ))}
                    </ListingTable.Body>
                  </ListingTable>
                </ListingTable.Container>
              )}
            </>
          )}

          {/* ── Groups tab ── */}
          {activeTab === "groups" && (
            <>
              <Typography variant="subtitle1">Assigned Groups</Typography>
              <Typography variant="body2" color="text.secondary">
                Search and add groups to this role.
              </Typography>

              <Grid container spacing={2}>
                <Grid size={{ xs: 12, md: 12, xl: 8 }}>
                <Form.ElementWrapper label="Add Group" name="addGroup">
                  <Autocomplete
                    id="addGroup"
                    options={availableGroups}
                    getOptionLabel={(option) => (option as ThunderGroup).name}
                    onChange={handleAddGroup}
                    value={null}
                    renderInput={(params) => (
                      <TextField {...params} placeholder="Search groups..." />
                    )}
                    noOptionsText="No groups available"
                  />
                </Form.ElementWrapper>
                </Grid>
              </Grid>

              {displayedGroups.length === 0 ? (
                <ListingTable.Container>
                  <ListingTable.EmptyState
                    illustration={<Folder size={64} />}
                    title="No groups assigned yet"
                    description="Search and add groups above."
                  />
                </ListingTable.Container>
              ) : (
                <ListingTable.Container>
                  <ListingTable>
                    <ListingTable.Head>
                      <ListingTable.Row>
                        <ListingTable.Cell>Name</ListingTable.Cell>
                        <ListingTable.Cell>Description</ListingTable.Cell>
                        <ListingTable.Cell />
                      </ListingTable.Row>
                    </ListingTable.Head>
                    <ListingTable.Body>
                      {displayedGroups.map((group) => (
                        <ListingTable.Row key={group.id}>
                          <ListingTable.Cell>{group.name}</ListingTable.Cell>
                          <ListingTable.Cell>
                            {group.description ?? "-"}
                          </ListingTable.Cell>
                          <ListingTable.Cell align="right">
                            <Tooltip title="Remove from role">
                              <IconButton
                                size="small"
                                onClick={() => handleRemoveGroup(group.id)}
                              >
                                <Trash size={16} />
                              </IconButton>
                            </Tooltip>
                          </ListingTable.Cell>
                        </ListingTable.Row>
                      ))}
                    </ListingTable.Body>
                  </ListingTable>
                </ListingTable.Container>
              )}
            </>
          )}
        </Form.Stack>

        {/* Action row shows below the card only when there are unsaved changes. */}
        {isDirty && (
          <Stack direction="row" spacing={1}>
            <Button
              variant="outlined"
              onClick={() => allowNavigation(() => navigate(rolesPath))}
              disabled={isSaving}
            >
              Cancel
            </Button>
            <Button
              variant="contained"
              onClick={handleSave}
              disabled={isSaving}
            >
              {isSaving ? "Saving..." : "Save Changes"}
            </Button>
          </Stack>
        )}
      </Stack>
    </>
  );
};

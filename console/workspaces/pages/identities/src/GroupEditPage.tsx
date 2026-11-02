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

import React, { useMemo, useState } from "react";
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  CircularProgress,
  Form,
  Grid,
  IconButton,
  ListingTable,
  Stack,
  Tab,
  TablePagination,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Shield, Trash, Users } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  useListUsers,
  useGetGroup,
  useGetGroupMembers,
  useAllGroupMemberIds,
  useGetGroupRoles,
  useAddGroupMembers,
  useRemoveGroupMembers,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type ThunderUser,
  type ThunderRole,
} from "@agent-management-platform/types";
import { useUnsavedChangesGuard } from "@agent-management-platform/shared-component";
import { BackButton } from "./components/BackButton";
import { EditFormSkeleton } from "./components/EditFormSkeleton";
import { EntityHeader } from "./components/EntityHeader";

type ActiveTab = "users" | "roles";

export const GroupEditPage: React.FC = () => {
  const { orgId, groupId } = useParams<{ orgId: string; groupId: string }>();
  const navigate = useNavigate();

  const [activeTab, setActiveTab] = useState<ActiveTab>("users");
  const [membersPage, setMembersPage] = useState(0);
  const [membersRowsPerPage, setMembersRowsPerPage] = useState(10);

  const { data: groupData, isLoading: isLoadingGroup } = useGetGroup({
    orgName: orgId,
    groupId: groupId ?? "",
  });

  const { data: membersData, isLoading: isLoadingMembers } = useGetGroupMembers(
    { orgName: orgId, groupId: groupId ?? "" },
    { offset: membersPage * membersRowsPerPage, limit: membersRowsPerPage },
  );
  const { data: allMemberIdsData, isLoading: isLoadingAllMemberIds } =
    useAllGroupMemberIds({ orgName: orgId, groupId: groupId ?? "" });
  const {
    data: rolesData,
    isLoading: isLoadingRoles,
    isError: isRolesError,
  } = useGetGroupRoles({
    orgName: orgId,
    groupId: groupId ?? "",
  });
  const { data: allUsersData, isLoading: isLoadingUsers } = useListUsers(
    { orgName: orgId },
    { offset: 0, limit: 100 },
  );

  const { mutateAsync: addMembers } = useAddGroupMembers();
  const { mutateAsync: removeMembers } = useRemoveGroupMembers();

  const initialMembers: ThunderUser[] = useMemo(
    () => membersData?.users ?? [],
    [membersData],
  );
  const allMemberIds: Set<string> = useMemo(
    () => new Set(allMemberIdsData?.memberIds ?? []),
    [allMemberIdsData],
  );
  const roles: ThunderRole[] = useMemo(
    () => rolesData?.roles ?? [],
    [rolesData],
  );
  const allUsers: ThunderUser[] = useMemo(
    () => allUsersData?.users ?? [],
    [allUsersData],
  );

  const [pendingAdds, setPendingAdds] = useState<ThunderUser[]>([]);
  const [removedIds, setRemovedIds] = useState<Set<string>>(new Set());

  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | undefined>();
  const [saveSuccess, setSaveSuccess] = useState(false);

  const groupsPath = orgId
    ? generatePath(
        absoluteRouteMap.children.org.children.settings.children.identities
          .children.groups.path,
        { orgId },
      )
    : "#";

  const membersTotal = membersData?.total ?? 0;

  const pageMembers = useMemo(
    () => initialMembers.filter((u) => !removedIds.has(u.id)),
    [initialMembers, removedIds],
  );

  const availableUsers = useMemo(() => {
    const excluded = new Set([
      ...[...allMemberIds].filter((id) => !removedIds.has(id)),
      ...pendingAdds.map((u) => u.id),
    ]);
    return allUsers.filter((u) => !excluded.has(u.id));
  }, [allUsers, allMemberIds, pendingAdds, removedIds]);

  const getUsername = (user: ThunderUser) =>
    String(user.attributes?.["username"] ?? user.id ?? "");

  const handleAddUser = (
    _e: React.SyntheticEvent,
    value: ThunderUser | null,
  ) => {
    if (!value) return;
    if (removedIds.has(value.id)) {
      setRemovedIds((prev) => {
        const next = new Set(prev);
        next.delete(value.id);
        return next;
      });
    } else {
      setPendingAdds((prev) => [...prev, value]);
    }
  };

  const handleRemoveUser = (userId: string) => {
    if (pendingAdds.find((u) => u.id === userId)) {
      setPendingAdds((prev) => prev.filter((u) => u.id !== userId));
    } else {
      setRemovedIds((prev) => new Set([...prev, userId]));
    }
  };

  const handleSave = async () => {
    if (!orgId || !groupId) return;
    setSaveError(undefined);
    setSaveSuccess(false);
    setIsSaving(true);
    try {
      const idsToAdd = pendingAdds
        .filter((u) => !allMemberIds.has(u.id))
        .map((u) => u.id);
      const idsToRemove = [...removedIds];
      if (idsToAdd.length > 0) {
        await addMembers({
          params: { orgName: orgId, groupId },
          body: { userIds: idsToAdd },
        });
      }
      if (idsToRemove.length > 0) {
        await removeMembers({
          params: { orgName: orgId, groupId },
          body: { userIds: idsToRemove },
        });
      }
      setSaveSuccess(true);
      setPendingAdds([]);
      setRemovedIds(new Set());
    } catch {
      setSaveError("Failed to update group members. Please try again.");
    } finally {
      setIsSaving(false);
    }
  };

  const isLoading =
    isLoadingGroup ||
    isLoadingMembers ||
    isLoadingAllMemberIds ||
    isLoadingUsers;

  // Show the action row only when there are unsaved member changes.
  const isDirty = pendingAdds.length > 0 || removedIds.size > 0;
  const { allowNavigation } = useUnsavedChangesGuard(isDirty);

  if (isLoading) {
    return (
      <>
        <BackButton to={groupsPath} label="Groups" />
        <EditFormSkeleton tabs={2} />
      </>
    );
  }

  return (
    <>
      <BackButton to={groupsPath} label="Groups" />
      <Stack spacing={3}>
        <EntityHeader
          fallback="G"
          name={groupData?.name ?? ""}
          subtitle={groupData?.description}
          id={groupId ?? ""}
        />
        {saveError != null && <Alert severity="error">{saveError}</Alert>}
        {saveSuccess && (
          <Alert severity="success">Group updated successfully.</Alert>
        )}

        <Form.Stack spacing={3}>
          <Tabs
            value={activeTab}
            onChange={(_e, v) => setActiveTab(v as ActiveTab)}
            sx={{ borderBottom: 1, borderColor: "divider" }}
          >
            <Tab label="Users" value="users" />
            <Tab label="Roles" value="roles" />
          </Tabs>

          {/* ── Users tab ── */}
          {activeTab === "users" && (
            <>
              <Typography variant="subtitle1">Users</Typography>
              <Typography variant="body2" color="text.secondary">
                Search and add users to this group.
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

              {pendingAdds.length > 0 && (
                <Box mb={2}>
                  <Typography variant="body2" fontWeight={500} mb={1}>
                    Pending additions (unsaved)
                  </Typography>
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
                        {pendingAdds.map((user) => (
                          <ListingTable.Row key={user.id}>
                            <ListingTable.Cell>
                              {getUsername(user)}
                            </ListingTable.Cell>
                            <ListingTable.Cell>{user.id}</ListingTable.Cell>
                            <ListingTable.Cell align="right">
                              <Tooltip title="Remove from group">
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
                </Box>
              )}

              {membersTotal === 0 && pendingAdds.length === 0 ? (
                <ListingTable.Container>
                  <ListingTable.EmptyState
                    illustration={<Users size={64} />}
                    title="No members yet"
                    description="Search and add users above."
                  />
                </ListingTable.Container>
              ) : membersTotal > 0 ? (
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
                      {pageMembers.map((user) => (
                        <ListingTable.Row key={user.id}>
                          <ListingTable.Cell>
                            {getUsername(user)}
                          </ListingTable.Cell>
                          <ListingTable.Cell>{user.id}</ListingTable.Cell>
                          <ListingTable.Cell align="right">
                            <Tooltip title="Remove from group">
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
                  {membersTotal > 5 && (
                    <TablePagination
                      component="div"
                      count={membersTotal}
                      page={membersPage}
                      rowsPerPage={membersRowsPerPage}
                      onPageChange={(_e, newPage) => setMembersPage(newPage)}
                      onRowsPerPageChange={(e) => {
                        setMembersRowsPerPage(parseInt(e.target.value, 10));
                        setMembersPage(0);
                      }}
                      rowsPerPageOptions={[5, 10, 25, 50]}
                    />
                  )}
                </ListingTable.Container>
              ) : null}
            </>
          )}

          {/* ── Roles tab ── */}
          {activeTab === "roles" && (
            <>
              <Typography variant="subtitle1">Assigned Roles</Typography>
              <Typography variant="body2" color="text.secondary">
                Roles currently assigned to this group. Manage role assignments
                from the Roles page.
              </Typography>

              <Box sx={{ mt: 1 }}>
                {isLoadingRoles ? (
                  <CircularProgress size={20} />
                ) : isRolesError ? (
                  <Typography variant="body2" color="error">
                    Failed to load roles. Please try again.
                  </Typography>
                ) : roles.length === 0 ? (
                  <ListingTable.Container>
                    <ListingTable.EmptyState
                      illustration={<Shield size={64} />}
                      title="No roles assigned to this group"
                    />
                  </ListingTable.Container>
                ) : (
                  <ListingTable.Container>
                    <ListingTable>
                      <ListingTable.Head>
                        <ListingTable.Row>
                          <ListingTable.Cell>Name</ListingTable.Cell>
                          <ListingTable.Cell>Description</ListingTable.Cell>
                        </ListingTable.Row>
                      </ListingTable.Head>
                      <ListingTable.Body>
                        {roles.map((role) => (
                          <ListingTable.Row key={role.id}>
                            <ListingTable.Cell>{role.name}</ListingTable.Cell>
                            <ListingTable.Cell>
                              {role.description ?? "-"}
                            </ListingTable.Cell>
                          </ListingTable.Row>
                        ))}
                      </ListingTable.Body>
                    </ListingTable>
                  </ListingTable.Container>
                )}
              </Box>
            </>
          )}
        </Form.Stack>

        {/* Action row shows below the card only when there are unsaved changes. */}
        {isDirty && (
          <Stack direction="row" spacing={1}>
            <Button
              variant="outlined"
              onClick={() => allowNavigation(() => navigate(groupsPath))}
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

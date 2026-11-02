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
  Stack,
  Tab,
  Tabs,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  useGetUser,
  useGetUserGroups,
  useGetUserRoles,
  useAllGroups,
  useAddGroupMembers,
  useRemoveGroupMembers,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  type ThunderGroup,
} from "@agent-management-platform/types";
import { useUnsavedChangesGuard } from "@agent-management-platform/shared-component";
import { BackButton } from "./components/BackButton";
import { EditFormSkeleton } from "./components/EditFormSkeleton";
import { EntityHeader } from "./components/EntityHeader";

type TabId = "groups" | "roles";

export const UserEditPage: React.FC = () => {
  const { orgId, userId } = useParams<{ orgId: string; userId: string }>();
  const navigate = useNavigate();
  const [activeTab, setActiveTab] = useState<TabId>("groups");

  const { data: user, isLoading: isLoadingUser } = useGetUser({
    orgName: orgId,
    userId: userId ?? "",
  });

  const { data: userGroupsData, isLoading: isLoadingUserGroups } =
    useGetUserGroups({
      orgName: orgId,
      userId: userId ?? "",
    });

  const { data: userRolesData, isLoading: isLoadingUserRoles } =
    useGetUserRoles({
      orgName: orgId,
      userId: userId ?? "",
    });

  const { data: allGroupsData, isLoading: isLoadingAllGroups } = useAllGroups({
    orgName: orgId,
  });

  const { mutateAsync: addMembers } = useAddGroupMembers();
  const { mutateAsync: removeMembers } = useRemoveGroupMembers();

  const allGroups: ThunderGroup[] = useMemo(
    () => allGroupsData?.groups ?? [],
    [allGroupsData],
  );
  const initialGroups: ThunderGroup[] = useMemo(
    () => userGroupsData?.groups ?? [],
    [userGroupsData],
  );

  const [selectedGroups, setSelectedGroups] = useState<ThunderGroup[]>([]);
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | undefined>();
  const [saveSuccess, setSaveSuccess] = useState(false);
  const hasEdited = useRef(false);

  useEffect(() => {
    if (!hasEdited.current) {
      setSelectedGroups(initialGroups);
    }
  }, [initialGroups]);

  // Only surface the action row once the group selection actually differs from
  // what's saved (add-then-remove resolves back to not-dirty).
  const isGroupsDirty = useMemo(() => {
    const initial = new Set(initialGroups.map((g) => g.id));
    return (
      initial.size !== selectedGroups.length ||
      selectedGroups.some((g) => !initial.has(g.id))
    );
  }, [initialGroups, selectedGroups]);
  const { allowNavigation } = useUnsavedChangesGuard(hasEdited.current && isGroupsDirty);

  const usersPath = orgId
    ? generatePath(
        absoluteRouteMap.children.org.children.settings.children.identities
          .children.users.path,
        { orgId },
      )
    : "#";

  const username = String(user?.attributes?.["username"] ?? userId ?? "");

  const handleSave = async () => {
    if (!orgId || !userId) return;
    setSaveError(undefined);
    setSaveSuccess(false);
    setIsSaving(true);
    try {
      const currentGroupIds = new Set(initialGroups.map((g) => g.id));
      const nextGroupIds = new Set(selectedGroups.map((g) => g.id));

      const toAdd = selectedGroups.filter((g) => !currentGroupIds.has(g.id));
      const toRemove = initialGroups.filter((g) => !nextGroupIds.has(g.id));

      for (const g of toAdd) {
        await addMembers({
          params: { orgName: orgId, groupId: g.id },
          body: { userIds: [userId] },
        });
      }
      for (const g of toRemove) {
        await removeMembers({
          params: { orgName: orgId, groupId: g.id },
          body: { userIds: [userId] },
        });
      }

      setSaveSuccess(true);
      hasEdited.current = false;
    } catch {
      setSaveError("Failed to update group memberships. Please try again.");
    } finally {
      setIsSaving(false);
    }
  };

  const isLoading =
    isLoadingUser ||
    isLoadingUserGroups ||
    isLoadingAllGroups ||
    isLoadingUserRoles;

  if (isLoading) {
    return (
      <>
        <BackButton to={usersPath} label="Users" />
        <EditFormSkeleton tabs={2} />
      </>
    );
  }

  const userRoles = userRolesData?.roles ?? [];

  return (
    <>
      <BackButton to={usersPath} label="Users" />
      <Stack spacing={3}>
        <EntityHeader
          fallback="U"
          name={username}
          subtitle={(user?.attributes?.["email"] as string) || undefined}
          id={userId ?? ""}
        />
        {saveError != null && <Alert severity="error">{saveError}</Alert>}
        {saveSuccess && (
          <Alert severity="success">User updated successfully.</Alert>
        )}

        <Form.Stack spacing={3}>
          <Tabs
            value={activeTab}
            onChange={(_e, newValue) => setActiveTab(newValue as TabId)}
            sx={{ borderBottom: 1, borderColor: "divider" }}
          >
            <Tab label="Groups" value="groups" />
            <Tab label="Roles" value="roles" />
          </Tabs>

          {activeTab === "groups" && (
            <>
              <Typography variant="subtitle1">Group Memberships</Typography>
              <Typography variant="body2" color="text.secondary">
                Search and select groups to assign this user to.
              </Typography>

              <Grid container spacing={2}>
                <Grid size={{ xs: 12, md: 12, xl: 8 }}>
                <Form.ElementWrapper label="Groups" name="groups">
                  <Autocomplete
                    id="groups"
                    multiple
                    options={allGroups}
                    value={selectedGroups}
                    onChange={(_e, newValue) => {
                      hasEdited.current = true;
                      setSelectedGroups(newValue as ThunderGroup[]);
                    }}
                    getOptionLabel={(option) => (option as ThunderGroup).name}
                    isOptionEqualToValue={(option, value) =>
                      (option as ThunderGroup).id === (value as ThunderGroup).id
                    }
                    renderTags={() => null}
                    renderInput={(params) => (
                      <TextField {...params} placeholder="Search groups..." />
                    )}
                    noOptionsText="No groups found"
                  />
                </Form.ElementWrapper>
                </Grid>

                {selectedGroups.length > 0 && (
                  <Grid size={{ xs: 12, md: 12, xl: 8 }}>
                  <Stack direction="row" flexWrap="wrap" gap={1}>
                    {selectedGroups.map((group) => (
                      <Chip
                        key={group.id}
                        label={group.name}
                        size="small"
                        onDelete={() => {
                          hasEdited.current = true;
                          setSelectedGroups((prev) =>
                            prev.filter((g) => g.id !== group.id),
                          );
                        }}
                      />
                    ))}
                  </Stack>
                  </Grid>
                )}
              </Grid>
            </>
          )}

          {activeTab === "roles" && (
            <>
              <Typography variant="subtitle1">Assigned Roles</Typography>
              <Typography variant="body2" color="text.secondary">
                Roles directly assigned to this user. To modify role
                assignments, use the Roles page.
              </Typography>

              <Box sx={{ mt: 1 }}>
                {userRoles.length === 0 ? (
                  <Typography variant="body2" color="text.secondary">
                    No roles assigned to this user.
                  </Typography>
                ) : (
                  <Stack direction="row" flexWrap="wrap" gap={1}>
                    {userRoles.map((role) => (
                      <Chip key={role.id} label={role.name} size="small" />
                    ))}
                  </Stack>
                )}
              </Box>
            </>
          )}
        </Form.Stack>

        {/* Action row shows only on the editable Groups tab once there are changes. */}
        {activeTab === "groups" && isGroupsDirty && (
          <Stack direction="row" spacing={1}>
            <Button
              variant="outlined"
              onClick={() => allowNavigation(() => navigate(usersPath))}
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

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

import React, { useEffect, useMemo, useState } from "react";
import {
  Alert,
  Avatar,
  Button,
  IconButton,
  ListingTable,
  Stack,
  TablePagination,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, Trash, Users } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  useDeleteUser,
  useListUsers,
} from "@agent-management-platform/api-client";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import {
  absoluteRouteMap,
  globalConfig,
  type ThunderUser,
} from "@agent-management-platform/types";
import { ListingSkeletonRows } from "./components/ListingSkeletonRows";

const AVATAR_SX = { width: 28, height: 28, fontSize: 12 } as const;

export const UsersPage: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const navigate = useNavigate();
  const isUserManagementEnabled = globalConfig.featureFlags?.enableUserManagement === true;

  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(10);
  const [search, setSearch] = useState("");

  const { data, isLoading, error } = useListUsers(
    { orgName: orgId },
    { offset: page * rowsPerPage, limit: rowsPerPage },
  );
  const { mutateAsync: deleteUser } = useDeleteUser();
  const { addConfirmation } = useConfirmationDialog();

  const users = useMemo(() => data?.users ?? [], [data]);
  const total = data?.total ?? 0;

  useEffect(() => {
    if (users.length === 0 && total > 0) {
      const lastPage = Math.max(0, Math.ceil(total / rowsPerPage) - 1);
      if (page !== lastPage) {
        setPage(lastPage);
      }
    }
  }, [users.length, total, page, rowsPerPage]);

  const identitiesRoute =
    absoluteRouteMap.children.org.children.settings.children.identities;

  const invitePath = orgId
    ? generatePath(identitiesRoute.children.users.path + "/invite", { orgId })
    : "#";

  const addUserPath = orgId
    ? generatePath(identitiesRoute.children.users.path + "/add", { orgId })
    : "#";

  const editUserPath = (userId: string) =>
    orgId
      ? generatePath(identitiesRoute.children.users.path + "/:userId", {
          orgId,
          userId,
        })
      : "#";

  const getUsername = (user: ThunderUser) =>
    String(user.attributes?.["username"] ?? "");

  const filteredUsers = useMemo(() => {
    if (!search) return users;
    const q = search.toLowerCase();
    return users.filter((u) => getUsername(u).toLowerCase().includes(q));
  }, [users, search]);

  const handleDelete = (user: ThunderUser) => {
    addConfirmation({
      analytics: { entity: "org-user", action: "delete" },
      title: "Delete User",
      description:
        `Are you sure you want to delete "${getUsername(user)}"?` +
        " This action cannot be undone.",
      confirmButtonText: "Delete",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash size={16} />,
      onConfirm: () => deleteUser({ orgName: orgId, userId: user.id }),
    });
  };

  return (
    <>
      {error != null && (
        <Alert severity="error" sx={{ mb: 2 }}>
          Failed to load users
        </Alert>
      )}

      <ListingTable.Provider searchValue={search} onSearchChange={setSearch}>
        <ListingTable.Container>
          <ListingTable.Toolbar
            showSearch
            searchPlaceholder="Search users..."
            actions={
              <Stack direction="row" spacing={1}>
                <Button
                  variant="outlined"
                  startIcon={<Plus />}
                  onClick={() => navigate(addUserPath)}
                  disabled={!isUserManagementEnabled}
                >
                  Add User
                </Button>
                <Button
                  variant="contained"
                  startIcon={<Plus />}
                  onClick={() => navigate(invitePath)}
                  disabled={!isUserManagementEnabled}
                >
                  Invite User
                </Button>
              </Stack>
            }
          />
          {!isLoading && filteredUsers.length === 0 ? (
            search ? (
              <ListingTable.EmptyState
                illustration={<Users size={64} />}
                title="No users found"
                description={`No users match "${search}". Try a different search term.`}
              />
            ) : (
              <ListingTable.EmptyState
                illustration={<Users size={64} />}
                title="No users yet"
                description='Click "Add User" to create one or "Invite User" to invite someone.'
              />
            )
          ) : (
            <ListingTable variant="table">
              <ListingTable.Head>
                <ListingTable.Row>
                  <ListingTable.Cell>Username</ListingTable.Cell>
                  <ListingTable.Cell>User ID</ListingTable.Cell>
                  <ListingTable.Cell align="center" width="80px" />
                </ListingTable.Row>
              </ListingTable.Head>
              <ListingTable.Body>
                {isLoading && <ListingSkeletonRows rows={Math.ceil(rowsPerPage / 2)} />}
                {!isLoading &&
                  filteredUsers.map((user: ThunderUser) => {
                    const username = getUsername(user);
                    return (
                      <ListingTable.Row
                        key={user.id}
                        variant="table"
                        hover
                        clickable
                        onClick={() => navigate(editUserPath(user.id))}
                      >
                        <ListingTable.Cell>
                          <ListingTable.CellIcon
                            icon={
                              <Avatar sx={AVATAR_SX}>
                                {username.charAt(0).toUpperCase() || "U"}
                              </Avatar>
                            }
                            primary={username}
                          />
                        </ListingTable.Cell>
                        <ListingTable.Cell>
                          <Typography
                            variant="body2"
                            color="text.secondary"
                            noWrap
                          >
                            {user.id}
                          </Typography>
                        </ListingTable.Cell>
                        <ListingTable.Cell align="center">
                          <ListingTable.RowActions visibility="hover">
                            <Tooltip title="Delete user">
                              <IconButton
                                size="small"
                                color="error"
                                onClick={(e) => {
                                  e.stopPropagation();
                                  handleDelete(user);
                                }}
                              >
                                <Trash size={16} />
                              </IconButton>
                            </Tooltip>
                          </ListingTable.RowActions>
                        </ListingTable.Cell>
                      </ListingTable.Row>
                    );
                  })}
              </ListingTable.Body>
            </ListingTable>
          )}
          {!isLoading && total >= 5 && (
            <TablePagination
              component="div"
              count={total}
              page={page}
              rowsPerPage={rowsPerPage}
              onPageChange={(_e, newPage) => setPage(newPage)}
              onRowsPerPageChange={(e) => {
                setRowsPerPage(parseInt(e.target.value, 10));
                setPage(0);
              }}
              rowsPerPageOptions={[5, 10, 25, 50]}
            />
          )}
        </ListingTable.Container>
      </ListingTable.Provider>
    </>
  );
};

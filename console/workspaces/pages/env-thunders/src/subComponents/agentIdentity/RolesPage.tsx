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
  TablePagination,
  Tooltip,
} from "@wso2/oxygen-ui";
import { Plus, Shield, Trash } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams, useSearchParams } from "react-router-dom";
import {
  useDeleteAgentIdentityRole,
  useListAgentIdentityRoles,
} from "@agent-management-platform/api-client";
import { ListingSkeletonRows, useConfirmationDialog } from "@agent-management-platform/shared-component";
import { absoluteRouteMap, type ThunderRole } from "@agent-management-platform/types";
import { withSearchParams } from "../../utils/withSearchParams";
import { AgentIdentityEnvironmentTabs } from "./AgentIdentityEnvironmentTabs";

const AVATAR_SX = { width: 28, height: 28, fontSize: 12 } as const;

export const RolesPage: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const [searchParams] = useSearchParams();
  const envName = searchParams.get("envName") ?? "";
  const navigate = useNavigate();

  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(10);
  const [search, setSearch] = useState("");

  const { data, isLoading, error } = useListAgentIdentityRoles(
    { orgName: orgId, envName },
    { offset: page * rowsPerPage, limit: rowsPerPage },
  );
  const { mutateAsync: deleteRole } = useDeleteAgentIdentityRole();
  const { addConfirmation } = useConfirmationDialog();

  const roles = useMemo(() => data?.roles ?? [], [data]);
  const total = data?.total ?? 0;

  useEffect(() => {
    if (roles.length === 0 && total > 0) {
      const lastPage = Math.max(0, Math.ceil(total / rowsPerPage) - 1);
      if (page !== lastPage) {
        setPage(lastPage);
      }
    }
  }, [roles.length, total, page, rowsPerPage]);

  const rolesNode =
    absoluteRouteMap.children.org.children.thunderInstances.children.roles;

  const createPath = orgId
    ? withSearchParams(generatePath(rolesNode.children.create.path, { orgId }), searchParams)
    : "#";

  const editRolePath = (roleId: string) =>
    orgId
      ? withSearchParams(
          generatePath(rolesNode.children.detail.path, { orgId, roleId }),
          searchParams,
        )
      : "#";

  const filteredRoles = useMemo(() => {
    if (!search) return roles;
    const q = search.toLowerCase();
    return roles.filter(
      (r) =>
        r.name.toLowerCase().includes(q) ||
        (r.description ?? "").toLowerCase().includes(q),
    );
  }, [roles, search]);

  const handleDelete = (role: ThunderRole) => {
    addConfirmation({
      analytics: { entity: "agent-identity-role", action: "delete" },
      title: "Delete Role",
      description: `Are you sure you want to delete "${role.name}"? This action cannot be undone.`,
      confirmButtonText: "Delete",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash size={16} />,
      onConfirm: () =>
        deleteRole({ orgName: orgId, envName: envName ?? "", roleId: role.id }),
    });
  };

  return (
    <>
      {error != null && (
        <Alert severity="error" sx={{ mb: 2 }}>
          Failed to load roles
        </Alert>
      )}

      <ListingTable.Provider searchValue={search} onSearchChange={setSearch}>
        <ListingTable.Container>
          <AgentIdentityEnvironmentTabs />
          <ListingTable.Toolbar
            showSearch
            searchPlaceholder="Search roles..."
            actions={
              <Button
                variant="contained"
                startIcon={<Plus />}
                onClick={() => navigate(createPath)}
              >
                Create Role
              </Button>
            }
          />
          {!isLoading && filteredRoles.length === 0 ? (
            search ? (
              <ListingTable.EmptyState
                illustration={<Shield size={64} />}
                title="No roles found"
                description={`No roles match "${search}". Try a different search term.`}
              />
            ) : (
              <ListingTable.EmptyState
                illustration={<Shield size={64} />}
                title="No roles yet"
                description='Click "Create Role" to add one.'
              />
            )
          ) : (
            <ListingTable variant="table">
              <ListingTable.Head>
                <ListingTable.Row>
                  <ListingTable.Cell>Name</ListingTable.Cell>
                  <ListingTable.Cell align="center" width="80px" />
                </ListingTable.Row>
              </ListingTable.Head>
              <ListingTable.Body>
                {isLoading && <ListingSkeletonRows rows={Math.ceil(rowsPerPage / 2)} columns={1} />}
                {!isLoading &&
                  filteredRoles.map((role: ThunderRole) => (
                    <ListingTable.Row
                      key={role.id}
                      variant="table"
                      hover
                      clickable
                      onClick={() => navigate(editRolePath(role.id))}
                    >
                      <ListingTable.Cell>
                        <ListingTable.CellIcon
                          icon={
                            <Avatar sx={AVATAR_SX}>
                              {role.name.charAt(0).toUpperCase() || "R"}
                            </Avatar>
                          }
                          primary={role.name}
                          secondary={role.description ?? undefined}
                        />
                      </ListingTable.Cell>
                      <ListingTable.Cell align="center">
                        <ListingTable.RowActions visibility="hover">
                          {!role.isReadOnly && (
                            <Tooltip title="Delete role">
                              <IconButton
                                size="small"
                                color="error"
                                onClick={(e) => {
                                  e.stopPropagation();
                                  handleDelete(role);
                                }}
                              >
                                <Trash size={16} />
                              </IconButton>
                            </Tooltip>
                          )}
                        </ListingTable.RowActions>
                      </ListingTable.Cell>
                    </ListingTable.Row>
                  ))}
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

export default RolesPage;

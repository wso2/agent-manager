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
import { Folder, Plus, Trash } from "@wso2/oxygen-ui-icons-react";
import { generatePath, useNavigate, useParams, useSearchParams } from "react-router-dom";
import {
  useDeleteAgentIdentityGroup,
  useListAgentIdentityGroups,
} from "@agent-management-platform/api-client";
import { ListingSkeletonRows, useConfirmationDialog } from "@agent-management-platform/shared-component";
import { absoluteRouteMap, type ThunderGroup } from "@agent-management-platform/types";
import { withSearchParams } from "../../utils/withSearchParams";
import { AgentIdentityEnvironmentTabs } from "./AgentIdentityEnvironmentTabs";

const AVATAR_SX = { width: 28, height: 28, fontSize: 12 } as const;

export const GroupsPage: React.FC = () => {
  const { orgId } = useParams<{ orgId: string }>();
  const [searchParams] = useSearchParams();
  const envName = searchParams.get("envName") ?? "";
  const navigate = useNavigate();

  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(10);
  const [search, setSearch] = useState("");

  const { data, isLoading, error } = useListAgentIdentityGroups(
    { orgName: orgId, envName },
    { offset: page * rowsPerPage, limit: rowsPerPage },
  );
  const { mutateAsync: deleteGroup } = useDeleteAgentIdentityGroup();
  const { addConfirmation } = useConfirmationDialog();

  const groups = useMemo(() => data?.groups ?? [], [data]);
  const total = data?.total ?? 0;

  useEffect(() => {
    if (groups.length === 0 && total > 0) {
      const lastPage = Math.max(0, Math.ceil(total / rowsPerPage) - 1);
      if (page !== lastPage) {
        setPage(lastPage);
      }
    }
  }, [groups.length, total, page, rowsPerPage]);

  const groupsNode =
    absoluteRouteMap.children.org.children.thunderInstances.children.groups;

  const createPath = orgId
    ? withSearchParams(generatePath(groupsNode.children.create.path, { orgId }), searchParams)
    : "#";

  const editGroupPath = (groupId: string) =>
    orgId
      ? withSearchParams(
          generatePath(groupsNode.children.detail.path, { orgId, groupId }),
          searchParams,
        )
      : "#";

  const filteredGroups = useMemo(() => {
    if (!search) return groups;
    const q = search.toLowerCase();
    return groups.filter(
      (g) =>
        g.name.toLowerCase().includes(q) ||
        (g.description ?? "").toLowerCase().includes(q),
    );
  }, [groups, search]);

  const handleDelete = (group: ThunderGroup) => {
    addConfirmation({
      analytics: { entity: "agent-identity-group", action: "delete" },
      title: "Delete Group",
      description: `Are you sure you want to delete "${group.name}"? This action cannot be undone.`,
      confirmButtonText: "Delete",
      confirmButtonColor: "error",
      confirmButtonIcon: <Trash size={16} />,
      onConfirm: () =>
        deleteGroup({ orgName: orgId, envName: envName ?? "", groupId: group.id }),
    });
  };

  return (
    <>
      {error != null && (
        <Alert severity="error" sx={{ mb: 2 }}>
          Failed to load groups
        </Alert>
      )}

      <ListingTable.Provider searchValue={search} onSearchChange={setSearch}>
        <ListingTable.Container>
          <AgentIdentityEnvironmentTabs />
          <ListingTable.Toolbar
            showSearch
            searchPlaceholder="Search groups..."
            actions={
              <Button
                variant="contained"
                startIcon={<Plus />}
                onClick={() => navigate(createPath)}
              >
                Create Group
              </Button>
            }
          />
          {!isLoading && filteredGroups.length === 0 ? (
            search ? (
              <ListingTable.EmptyState
                illustration={<Folder size={64} />}
                title="No groups found"
                description={`No groups match "${search}". Try a different search term.`}
              />
            ) : (
              <ListingTable.EmptyState
                illustration={<Folder size={64} />}
                title="No groups yet"
                description='Click "Create Group" to add one.'
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
                  filteredGroups.map((group: ThunderGroup) => (
                    <ListingTable.Row
                      key={group.id}
                      variant="table"
                      hover
                      clickable
                      onClick={() => navigate(editGroupPath(group.id))}
                    >
                      <ListingTable.Cell>
                        <ListingTable.CellIcon
                          icon={
                            <Avatar sx={AVATAR_SX}>
                              {group.name.charAt(0).toUpperCase() || "G"}
                            </Avatar>
                          }
                          primary={group.name}
                          secondary={group.description ?? undefined}
                        />
                      </ListingTable.Cell>
                      <ListingTable.Cell align="center">
                        <ListingTable.RowActions visibility="hover">
                          <Tooltip title="Delete group">
                            <IconButton
                              size="small"
                              color="error"
                              onClick={(e) => {
                                e.stopPropagation();
                                handleDelete(group);
                              }}
                            >
                              <Trash size={16} />
                            </IconButton>
                          </Tooltip>
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

export default GroupsPage;

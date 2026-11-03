/**
 * Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  ListingTable,
  SearchBar,
  Skeleton,
  Stack,
  TablePagination,
  Typography,
} from "@wso2/oxygen-ui";
import {
  Plus as Add,
  Trash2 as DeleteOutlineOutlined,
  User,
} from "@wso2/oxygen-ui-icons-react";
import {
  EntityAvatar,
  displayAgentSubType,
  displayProvisionTypes,
  returnToState,
} from "@agent-management-platform/views";
import { generatePath, useLocation, useNavigate, useParams } from "react-router-dom";
import { absoluteRouteMap } from "@agent-management-platform/types";
import {
  useDeleteAgent,
  useListAgentKinds,
  useListAgents,
} from "@agent-management-platform/api-client";
import {
  ResourceListShell,
  formatRelativeTime,
  getErrorMessage,
  useConfirmationDialog,
} from "@agent-management-platform/shared-component";

const projectRoutes = absoluteRouteMap.children.org.children.projects;
const ROWS_PER_PAGE_OPTIONS = [5, 10, 25];

/** Timestamp swaps for the Delete button on hover/focus; touch shows both. */
const ROW_HOVER_SX = {
  "& .agent-actions": { display: "none" },
  "&:hover .agent-updated, &:focus-within .agent-updated": { display: "none" },
  "&:hover .agent-actions, &:focus-within .agent-actions": { display: "flex" },
  "@media (hover: none)": {
    "& .agent-updated": { display: "block", mr: 1 },
    "& .agent-actions": { display: "flex" },
  },
} as const;

function AgentListSkeleton() {
  return (
    <Stack direction="column" spacing={2}>
      <Skeleton variant="rounded" width="100%" height={40} />
      <Skeleton variant="rounded" width="100%" height={48} />
      {Array.from({ length: 5 }).map((_, index) => (
        <Skeleton key={index} variant="rounded" width="100%" height={64} />
      ))}
    </Stack>
  );
}

/** The project's agents: search, refresh, and a table with a delete action. */
export interface AgentListProps {
  /** Shows "New Agent" beside the search, for pages without a header action. */
  showCreateAction?: boolean;
}

export const AgentList: React.FC<AgentListProps> = ({ showCreateAction = false }) => {
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(ROWS_PER_PAGE_OPTIONS[0]);

  const { orgId = "", projectId = "" } = useParams<{
    orgId: string;
    projectId: string;
  }>();
  const navigate = useNavigate();
  const location = useLocation();
  const { data, isLoading, error, isRefetching } = useListAgents({
    orgName: orgId,
    projName: projectId,
  });
  const { mutate: deleteAgent } = useDeleteAgent();
  const { data: kindsData } = useListAgentKinds({ orgName: orgId });
  const { addConfirmation } = useConfirmationDialog();

  const kindDisplayNameMap = useMemo(
    () => Object.fromEntries((kindsData?.kinds ?? []).map((k) => [k.name, k.displayName])),
    [kindsData?.kinds],
  );

  const handleAddAgent = useCallback(() => {
    navigate(generatePath(projectRoutes.children.newAgent.path, { orgId, projectId }), {
      state: returnToState(location),
    });
  }, [navigate, orgId, projectId, location]);

  const filteredAgents = useMemo(() => {
    const term = search.toLowerCase();
    return (data?.agents ?? []).filter(
      (agent) =>
        agent.displayName.toLowerCase().includes(term) ||
        agent.name.toLowerCase().includes(term) ||
        // Labels match as `key:value`, so "env:prod" (or a fragment) filters by label.
        Object.entries(agent.labels ?? {}).some(([k, v]) =>
          `${k}:${v}`.toLowerCase().includes(term),
        ),
    );
  }, [data?.agents, search]);

  useEffect(() => {
    setPage(0);
  }, [search]);

  // Deleting rows can leave the current page past the end; step back to the last page.
  useEffect(() => {
    const lastPage = Math.max(0, Math.ceil(filteredAgents.length / rowsPerPage) - 1);
    if (page > lastPage) setPage(lastPage);
  }, [filteredAgents.length, rowsPerPage, page]);

  const paginatedAgents = useMemo(
    () => filteredAgents.slice(page * rowsPerPage, (page + 1) * rowsPerPage),
    [filteredAgents, page, rowsPerPage],
  );

  const confirmDelete = (agentName: string, displayName: string) =>
    addConfirmation({
      analytics: { entity: "agent", action: "delete" },
      title: "Delete Agent?",
      description: `Are you sure you want to delete the agent "${displayName}"? This action cannot be undone.`,
      onConfirm: () =>
        deleteAgent({ orgName: orgId, projName: projectId, agentName }),
      confirmButtonColor: "error",
      confirmButtonIcon: <DeleteOutlineOutlined size={16} />,
      confirmButtonText: "Delete",
    });

  if (isLoading) return <AgentListSkeleton />;

  return (
    <Stack direction="column" spacing={2}>
      {error ? (
        <Alert severity="error" variant="outlined">
          {getErrorMessage(error)}
        </Alert>
      ) : null}

      <ResourceListShell
        resourceName="agents"
        searchValue={search}
        onSearchChange={setSearch}
        toolbar={
          <Stack direction="row" alignItems="center" spacing={1}>
            <Box flexGrow={1}>
              <SearchBar
                placeholder="Search agents"
                size="small"
                fullWidth
                onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
                  setSearch(e.target.value)
                }
                value={search}
                disabled={!data?.agents?.length}
              />
            </Box>
            {showCreateAction && (
              <Button
                variant="contained"
                color="primary"
                size="small"
                startIcon={<Add size={16} />}
                onClick={handleAddAgent}
              >
                New Agent
              </Button>
            )}
          </Stack>
        }
        isLoading={isRefetching && !data?.agents?.length}
        isEmpty={!data?.agents?.length}
        isSearchEmpty={filteredAgents.length === 0}
        loadingPlaceholder={<AgentListSkeleton />}
        emptyState={{
          illustration: <User size={64} />,
          title: "No agents found",
          description: "Create a new agent to get started",
          action: (
            <Button variant="contained" startIcon={<Add />} onClick={handleAddAgent}>
              New Agent
            </Button>
          ),
        }}
        searchEmptyState={{
          illustration: <User size={64} />,
          title: "No agents found",
          description: "No agents match your search criteria. Try adjusting your search.",
        }}
      >
        <Box sx={{ minWidth: 600 }}>
          <ListingTable variant="card">
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell>Name</ListingTable.Cell>
                <ListingTable.Cell>Type</ListingTable.Cell>
                <ListingTable.Cell align="right">Last Updated</ListingTable.Cell>
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {paginatedAgents.map((agent) => (
                <ListingTable.Row
                  key={agent.name}
                  variant="card"
                  hover
                  clickable
                  sx={ROW_HOVER_SX}
                  onClick={() =>
                    navigate(
                      generatePath(projectRoutes.children.agents.path, {
                        orgId,
                        projectId: agent.projectName,
                        agentId: agent.name,
                      }),
                    )
                  }
                >
                  <ListingTable.Cell>
                    <Stack direction="row" alignItems="center" spacing={2} sx={{ minWidth: 0 }}>
                      <EntityAvatar
                        name={agent.displayName}
                        color="primary.main"
                        shape="circular"
                        size={32}
                        sx={{ flexShrink: 0 }}
                      />
                      <Typography variant="body1" noWrap>
                        {agent.displayName}
                      </Typography>
                    </Stack>
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Typography variant="body2" noWrap>
                      {[
                        displayProvisionTypes(agent.provisioning?.type),
                        displayAgentSubType(agent.agentType?.subType),
                      ]
                        .filter(Boolean)
                        .join(" · ")}
                    </Typography>
                    {agent.kindName && (
                      <Typography variant="caption" color="text.secondary" noWrap>
                        Kind: {kindDisplayNameMap[agent.kindName] ?? agent.kindName}
                      </Typography>
                    )}
                  </ListingTable.Cell>
                  <ListingTable.Cell align="right">
                    <Stack
                      direction="row"
                      alignItems="center"
                      justifyContent="flex-end"
                      sx={{ minWidth: 150 }}
                    >
                      <Typography
                        className="agent-updated"
                        variant="body2"
                        color="text.secondary"
                        noWrap
                      >
                        {formatRelativeTime(agent.createdAt, { fallback: "—" })}
                      </Typography>
                      <Box className="agent-actions">
                        <Button
                          variant="outlined"
                          color="error"
                          size="small"
                          startIcon={<DeleteOutlineOutlined size={14} />}
                          onClick={(e) => {
                            e.stopPropagation();
                            confirmDelete(agent.name, agent.displayName);
                          }}
                        >
                          Delete
                        </Button>
                      </Box>
                    </Stack>
                  </ListingTable.Cell>
                </ListingTable.Row>
              ))}
            </ListingTable.Body>
          </ListingTable>
          {filteredAgents.length > ROWS_PER_PAGE_OPTIONS[0] && (
            <TablePagination
              rowsPerPageOptions={ROWS_PER_PAGE_OPTIONS}
              component="div"
              count={filteredAgents.length}
              rowsPerPage={rowsPerPage}
              page={page}
              onPageChange={(_, newPage) => setPage(newPage)}
              onRowsPerPageChange={(e) => {
                setRowsPerPage(parseInt(e.target.value, 10));
                setPage(0);
              }}
            />
          )}
        </Box>
      </ResourceListShell>
    </Stack>
  );
};

export default AgentList;

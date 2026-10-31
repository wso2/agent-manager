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
  Box,
  Stack,
  Typography,
  Button,
  Alert,
  Tooltip,
  Skeleton,
  IconButton,
  ListingTable,
  TablePagination,
  DataGrid,
  SearchBar,
} from "@wso2/oxygen-ui";
import {
  Plus as Add,
  Trash2 as DeleteOutlineOutlined,
  User,
} from "@wso2/oxygen-ui-icons-react";

const { DataGrid: DataGridComponent } = DataGrid;

import {
  PageLayout,
  FadeIn,
  EntityAvatar,
} from "@agent-management-platform/views";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  absoluteRouteMap,
  AgentResponse,
} from "@agent-management-platform/types";
import {
  useListAgents,
  useDeleteAgent,
  useListAgentKinds,
} from "@agent-management-platform/api-client";
import {
  AgentTypeChips,
  ResourceListShell,
  formatRelativeTime,
  getErrorMessage,
  useConfirmationDialog,
} from "@agent-management-platform/shared-component";

const agentPath =
  absoluteRouteMap.children.org.children.projects.children.agents.path;

// Detect touch device for alternative interaction pattern
const isTouchDevice =
  typeof window !== "undefined" &&
  ("ontouchstart" in window || navigator.maxTouchPoints > 0);

function ListPageSkeleton() {
  return (
    <Stack direction="column" spacing={4}>
      <Skeleton variant="rounded" width="100%" height={40} />

      {/* Table skeleton */}
      <Box display="flex" flexDirection="column" gap={1}>
        {/* Table header */}
        <Skeleton variant="rounded" width="100%" height={48} />

        {/* Table rows */}
        {Array.from({ length: 5 }).map((_, index) => (
          <Skeleton key={index} variant="rounded" width="100%" height={72} />
        ))}
      </Box>

      {/* Pagination skeleton */}
      <Box display="flex" justifyContent="flex-end">
        <Skeleton variant="rounded" width={300} height={40} />
      </Box>
    </Stack>
  );
}

interface AgentWithHref extends AgentResponse {
  href: string;
  id: string;
}

export const AgentListing: React.FC = () => {
  const [search, setSearch] = useState("");
  const [hoveredAgentId, setHoveredAgentId] = useState<string | null>(null);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(5);

  const { orgId, projectId } = useParams<{
    orgId: string;
    projectId: string;
  }>();
  const navigate = useNavigate();
  const { data, isLoading, error, isRefetching } = useListAgents({
    orgName: orgId,
    projName: projectId,
  });
  const { mutate: deleteAgent, isPending: isDeletingAgent } = useDeleteAgent();
  const { data: kindsData } = useListAgentKinds({ orgName: orgId });

  const kindDisplayNameMap = useMemo(() => {
    const map: Record<string, string> = {};
    kindsData?.kinds?.forEach((k) => {
      map[k.name] = k.displayName;
    });
    return map;
  }, [kindsData?.kinds]);
  const { addConfirmation } = useConfirmationDialog();
  const handleDeleteAgent = useCallback(
    (agentId: string) => {
      deleteAgent({
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
      });
    },
    [deleteAgent, orgId, projectId],
  );

  const handleRowMouseEnter = useCallback(
    (row: AgentResponse & { id: string }) => {
      setHoveredAgentId(row.id);
    },
    [],
  );

  const handleRowMouseLeave = useCallback(() => {
    setHoveredAgentId(null);
  }, []);

  const handleAddAgent = useCallback(() => {
    navigate(
      generatePath(
        absoluteRouteMap.children.org.children.projects.children.newAgent.path,
        { orgId: orgId ?? "", projectId: projectId ?? "" },
      ),
    );
  }, [navigate, orgId, projectId]);

  const agentsWithHref: AgentWithHref[] = useMemo(
    () =>
      data?.agents
        ?.filter((agent: AgentResponse) => {
          const term = search.toLowerCase();
          return (
            agent.displayName.toLowerCase().includes(term) ||
            agent.name.toLowerCase().includes(term) ||
            // Labels match as `key:value`, so typing "env:prod" (or a
            // fragment of it) filters by label.
            Object.entries(agent.labels ?? {}).some(([k, v]) =>
              `${k}:${v}`.toLowerCase().includes(term),
            )
          );
        })
        .map((agent) => ({
          ...agent,
          href: generatePath(agentPath, {
            orgId: orgId ?? "",
            projectId: agent.projectName,
            agentId: agent.name,
          }),
          id: agent.name,
        })) ?? [],
    [data?.agents, search, orgId],
  );

  // Reset page to 0 when search or filtered list changes
  useEffect(() => {
    setPage(0);
  }, [agentsWithHref]);

  // Paginate agents
  const paginatedAgents = useMemo(
    () =>
      agentsWithHref.slice(
        page * rowsPerPage,
        page * rowsPerPage + rowsPerPage,
      ),
    [agentsWithHref, page, rowsPerPage],
  );

  const isPageLoading =
    (isRefetching && !data?.agents?.length) || isDeletingAgent;

  return (
    <PageLayout
      title="Agents"
      isLoading={isPageLoading}
      actions={
        <Button
          variant="contained"
          color="primary"
          size="small"
          startIcon={<Add size={16} />}
          onClick={handleAddAgent}
        >
          Create Agent
        </Button>
      }
    >
      {isLoading ? (
        <ListPageSkeleton />
      ) : (
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
            }
            isLoading={isRefetching && !data?.agents?.length}
            isEmpty={!data?.agents?.length}
            isSearchEmpty={agentsWithHref.length === 0}
            loadingPlaceholder={
              <DataGridComponent
                rows={[]}
                columns={[
                  { field: "name", headerName: "Agent Name", flex: 1 },
                  {
                    field: "lastUpdated",
                    headerName: "Last Updated",
                    flex: 1,
                  },
                ]}
                loading
                hideFooter
              />
            }
            emptyState={{
              illustration: <User size={64} />,
              title: "No agents found",
              description: "Create a new agent to get started",
              action: (
                <Button
                  variant="contained"
                  color="primary"
                  startIcon={<Add />}
                  onClick={handleAddAgent}
                >
                  Create Agent
                </Button>
              ),
            }}
            searchEmptyState={{
              illustration: <User size={64} />,
              title: "No agents found",
              description:
                "No agents match your search criteria. Try adjusting your search.",
            }}
          >
            <Box sx={{ minWidth: 600 }}>
              <ListingTable variant="card">
                <ListingTable.Head>
                  <ListingTable.Row>
                    <ListingTable.Cell>Agent Name</ListingTable.Cell>
                    <ListingTable.Cell align="right">
                      Last Updated
                    </ListingTable.Cell>
                  </ListingTable.Row>
                </ListingTable.Head>
                <ListingTable.Body>
                  {paginatedAgents.map((agent) => (
                    <ListingTable.Row
                      key={agent.id}
                      variant="card"
                      hover
                      clickable
                      onClick={() => navigate(agent.href)}
                      onMouseEnter={() => handleRowMouseEnter(agent)}
                      onMouseLeave={handleRowMouseLeave}
                      onFocus={() => handleRowMouseEnter(agent)}
                      onBlur={handleRowMouseLeave}
                    >
                      <ListingTable.Cell>
                        <Stack
                          direction="row"
                          alignItems="center"
                          spacing={2}
                          sx={{ width: "100%", minWidth: 0 }}
                        >
                          <EntityAvatar
                            name={agent.displayName}
                            color="primary.main"
                            shape="circular"
                            size={32}
                            sx={{ flexShrink: 0 }}
                          />
                          <Stack
                            direction="row"
                            alignItems="flex-start"
                            spacing={1}
                            sx={{ minWidth: 0, flex: 1 }}
                          >
                            <Typography variant="body1" sx={{ flexShrink: 0 }}>
                              {agent.displayName}
                            </Typography>
                            <AgentTypeChips
                              provisioning={agent.provisioning}
                              agentType={agent.agentType}
                              kindName={agent.kindName}
                              kindDisplayName={
                                agent.kindName
                                  ? kindDisplayNameMap[agent.kindName]
                                  : undefined
                              }
                            />
                          </Stack>
                        </Stack>
                      </ListingTable.Cell>
                      <ListingTable.Cell align="right">
                        <Stack
                          direction="row"
                          alignItems="center"
                          spacing={1}
                          justifyContent="flex-end"
                          sx={{ minWidth: 150 }}
                        >
                          {hoveredAgentId === agent.id || isTouchDevice ? (
                            <FadeIn>
                              <Tooltip title="Delete Agent">
                                <IconButton
                                  color="error"
                                  size="small"
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    addConfirmation({
                                      analytics: {
                                        entity: "agent",
                                        action: "delete",
                                      },
                                      title: "Delete Agent?",
                                      description: `Are you sure you want to delete the agent "${agent.displayName}"? This action cannot be undone.`,
                                      onConfirm: () => {
                                        handleDeleteAgent(agent.name);
                                      },
                                      confirmButtonColor: "error",
                                      confirmButtonIcon: (
                                        <DeleteOutlineOutlined size={16} />
                                      ),
                                      confirmButtonText: "Delete",
                                    });
                                  }}
                                >
                                  <DeleteOutlineOutlined size={14} />
                                </IconButton>
                              </Tooltip>
                            </FadeIn>
                          ) : (
                            <Typography
                              variant="body2"
                              color="text.secondary"
                              noWrap
                            >
                              {formatRelativeTime(agent.createdAt, {
                                fallback: "—",
                              })}
                            </Typography>
                          )}
                        </Stack>
                      </ListingTable.Cell>
                    </ListingTable.Row>
                  ))}
                </ListingTable.Body>
              </ListingTable>
              {agentsWithHref.length > 5 && (
                <TablePagination
                  rowsPerPageOptions={[5, 10, 25]}
                  component="div"
                  count={agentsWithHref.length}
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
      )}
    </PageLayout>
  );
};

export default AgentListing;

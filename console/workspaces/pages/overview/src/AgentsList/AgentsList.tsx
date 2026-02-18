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
  Chip,
  IconButton,
  CircularProgress,
  ListingTable,
  TablePagination,
  DataGrid,
  SearchBar,
  Avatar,
} from "@wso2/oxygen-ui";
import {
  Plus as Add,
  Trash2 as DeleteOutlineOutlined,
  RefreshCcw,
  User,
  Edit,
} from "@wso2/oxygen-ui-icons-react";

const { DataGrid: DataGridComponent } = DataGrid;
import {
  PageLayout,
  FadeIn,
  displayProvisionTypes,
} from "@agent-management-platform/views";
import { generatePath, useNavigate, useParams } from "react-router-dom";
import {
  absoluteRouteMap,
  AgentResponse,
  Provisioning,
} from "@agent-management-platform/types";
import {
  useListAgents,
  useDeleteAgent,
  useGetProject,
} from "@agent-management-platform/api-client";
import { AgentTypeSummery } from "./subComponents/AgentTypeSummery";
import { useConfirmationDialog } from "@agent-management-platform/shared-component";
import { EditProjectDrawer } from "../ProjectList/EditProjectDrawer";
import { formatDistanceToNow } from "date-fns";

export function ListPageSkeleton() {
  return (
    <Stack direction="row" justifyContent="space-between" gap={4}>
      <Stack direction="column" sx={{ flexGrow: 1 }} spacing={4}>
        {/* Search and Add Agent button skeleton */}
        <Stack direction="row" spacing={1}>
          <Box flexGrow={1}>
            <Skeleton variant="rounded" width="100%" height={40} />
          </Box>
          <Skeleton variant="rounded" width={120} height={40} />
        </Stack>

        {/* Table skeleton */}
        <Box display="flex" flexDirection="column" gap={1}>
          {/* Table header */}
          <Skeleton variant="rounded" width="100%" height={48} />

          {/* Table rows */}
          {Array.from({ length: 5 }).map((_, index) => (
            <Skeleton
              key={index}
              variant="rounded"
              width="100%"
              height={72}
            />
          ))}
        </Box>

        {/* Pagination skeleton */}
        <Box display="flex" justifyContent="flex-end">
          <Skeleton variant="rounded" width={300} height={40} />
        </Box>
      </Stack>

      {/* Agent Type Summary skeleton */}
      <Box width={250}>
        <Skeleton variant="rounded" width="100%" height={200} />
      </Box>
    </Stack>
  );
}

export interface AgentWithHref extends AgentResponse {
  href: string;
  id: string;
  agentInfo: { name: string; displayName: string; description: string };
}

export const AgentsList: React.FC = () => {
  const [search, setSearch] = useState("");
  const [hoveredAgentId, setHoveredAgentId] = useState<string | null>(null);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(5);
  const [editProjectDrawerOpen, setEditProjectDrawerOpen] = useState(false);

  // Detect touch device for alternative interaction pattern
  const isTouchDevice =
    typeof window !== "undefined" &&
    ("ontouchstart" in window || navigator.maxTouchPoints > 0);

  const { orgId, projectId } = useParams<{
    orgId: string;
    projectId: string;
  }>();
  const navigate = useNavigate();
  const {
    data,
    isLoading,
    error,
    isRefetching,
    refetch: refetchAgents,
  } = useListAgents({
    orgName: orgId,
    projName: projectId,
  });
  const { mutate: deleteAgent, isPending: isDeletingAgent } = useDeleteAgent();
  const { data: project, isLoading: isProjectLoading } = useGetProject({
    orgName: orgId,
    projName: projectId,
  });
  const { addConfirmation } = useConfirmationDialog();
  const handleDeleteAgent = useCallback(
    (agentId: string) => {
      deleteAgent({
        orgName: orgId,
        projName: projectId,
        agentName: agentId,
      });
    },
    [deleteAgent, orgId, projectId]
  );

  const handleRowMouseEnter = useCallback(
    (row: AgentResponse & { id: string }) => {
      setHoveredAgentId(row.id);
    },
    []
  );

  const handleRowMouseLeave = useCallback(() => {
    setHoveredAgentId(null);
  }, []);

  const getRelativeTime = useCallback((date?: string) => {
    if (!date) {
      return "—";
    }
    return formatDistanceToNow(new Date(date), { addSuffix: true });
  }, []);

  const getAgentPath = (isInternal: boolean) => {
    let path =
      absoluteRouteMap.children.org.children.projects.children.agents.path;
    if (isInternal) {
      path =
        absoluteRouteMap.children.org.children.projects.children.agents.path;
    }
    return path;
  };

  useEffect(() => {
    if (
      orgId &&
      projectId &&
      !data?.agents?.length &&
      !isLoading &&
      !isRefetching
    ) {
      navigate(
        generatePath(
          absoluteRouteMap.children.org.children.projects.children.newAgent
            .path,
          { orgId: orgId ?? "", projectId: projectId ?? "" }
        )
      );
    }
  }, [orgId, projectId, data?.agents, isLoading, isRefetching, navigate]);

  const agentsWithHref: AgentWithHref[] = useMemo(
    () =>
      data?.agents
        ?.filter(
          (agent: AgentResponse) =>
            agent.displayName.toLowerCase().includes(search.toLowerCase()) ||
            agent.name.toLowerCase().includes(search.toLowerCase())
        )
        .map((agent) => ({
          ...agent,
          href: generatePath(
            getAgentPath(agent.provisioning.type === "internal"),
            {
              orgId: orgId ?? "",
              projectId: agent.projectName,
              agentId: agent.name,
            }
          ),
          id: agent.name,
          agentInfo: {
            name: agent.name,
            displayName: agent.displayName,
            description: agent.description,
          },
        })) ?? [],
    [data?.agents, search, orgId]
  );

  // Reset page to 0 when search or filtered list changes
  useEffect(() => {
    setPage(0);
  }, [agentsWithHref]);

  // Paginate agents
  const paginatedAgents = useMemo(
    () => agentsWithHref.slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage),
    [agentsWithHref, page, rowsPerPage]
  );

  const isPageLoading = isProjectLoading ||
    (isRefetching && !data?.agents?.length) ||
    isDeletingAgent;

  return (
    <>
      <PageLayout
        title={project?.displayName ?? "Agents"}
        description={
          project?.description ??
          "Manage and monitor all your AI agents across environments"
        }
        isLoading={isPageLoading}
        titleTail={
          <Box
            display="flex"
            alignItems="center"
            minWidth={32}
            justifyContent="center"
            gap={1}
          >
            <Tooltip title="Edit Project">
              <IconButton
                color="primary"
                size="small"
                onClick={() => setEditProjectDrawerOpen(true)}
                disabled={!project}
              >
                <Edit size={18} />
              </IconButton>
            </Tooltip>
            {isRefetching ? (
              <CircularProgress size={18} color="primary" />
            ) : (
              <IconButton
                size="small"
                color="primary"
                onClick={() => refetchAgents()}
              >
                <RefreshCcw size={18} />
              </IconButton>
            )}
          </Box>
        }
      >
        {isLoading ? (
          <ListPageSkeleton />
        ) : (
          <Stack
            direction="row"
            justifyContent="space-between"
            gap={4}
          >
            <Stack
              direction="column"
              sx={{
                flexGrow: 1,
              }}
              spacing={4}
            >
              <Stack direction="row" spacing={1}>
                <Box flexGrow={1}>
                  <SearchBar
                    placeholder="Search agents"
                    size="small"
                    fullWidth
                    onChange={(e: React.ChangeEvent<HTMLInputElement>) => setSearch(e.target.value)}
                    value={search}
                    disabled={!data?.agents?.length}
                  />
                </Box>
                <Button
                  variant="contained"
                  color="primary"
                  size="small"
                  startIcon={<Add size={16} />}
                  onClick={() =>
                    navigate(
                      generatePath(
                        absoluteRouteMap.children.org.children.projects.children
                          .newAgent.path,
                        { orgId: orgId ?? "", projectId: projectId ?? "" }
                      )
                    )
                  }
                >
                  Add Agent
                </Button>
              </Stack>

              {error && (
                <Alert severity="error" variant="outlined">
                  {error.message}
                </Alert>
              )}

              {isLoading ? (
                <DataGridComponent
                  rows={[]}
                  columns={[
                    { field: 'name', headerName: 'Agent Name', flex: 1 },
                    { field: 'description', headerName: 'Description', flex: 2 },
                    { field: 'lastUpdated', headerName: 'Last Updated', flex: 1 },
                  ]}
                  loading
                  hideFooter
                />
              ) : !!data?.agents?.length && agentsWithHref.length > 0 ? (
                <ListingTable.Container sx={{ minWidth: 600 }} disablePaper>
                  <ListingTable variant="card">
                    <ListingTable.Head>
                      <ListingTable.Row>
                        <ListingTable.Cell>Agent Name</ListingTable.Cell>
                        <ListingTable.Cell>Description</ListingTable.Cell>
                        <ListingTable.Cell align="right">Last Updated</ListingTable.Cell>
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
                            <Stack direction="row" alignItems="center" spacing={2}>
                              <Avatar sx={{bgcolor: "primary.main", height: 32, width: 32, color: "primary.contrastText"}}>
                                {agent.displayName.charAt(0).toUpperCase()}
                              </Avatar>
                              <Stack direction="row" alignItems="flex-start" spacing={1}>
                                <Typography variant="body1">
                                  {agent.displayName}
                                </Typography>
                                {agent.provisioning.type !== "internal" && (
                                  <Chip
                                    label={displayProvisionTypes(
                                      (agent.provisioning as Provisioning).type
                                    )}
                                    size="small"
                                    variant="outlined"
                                  />
                                )}
                              </Stack>
                            </Stack>
                          </ListingTable.Cell>
                          <ListingTable.Cell>
                            <Typography
                              variant="body2"
                              noWrap
                              textOverflow="ellipsis"
                              overflow="hidden"
                            >
                              {agent.description.substring(0, 40) +
                                (agent.description.length > 40 ? "..." : "")}
                            </Typography>
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
                                          title: "Delete Agent?",
                                          description: `Are you sure you want to delete the agent "${agent.displayName}"? This action cannot be undone.`,
                                          onConfirm: () => {
                                            handleDeleteAgent(agent.name);
                                          },
                                          confirmButtonColor: "error",
                                          confirmButtonIcon: <DeleteOutlineOutlined size={16} />,
                                          confirmButtonText: "Delete",
                                        });
                                      }}
                                    >
                                      <DeleteOutlineOutlined size={14} />
                                    </IconButton>
                                  </Tooltip>
                                </FadeIn>
                              ) : (
                                <Typography variant="body2" color="text.secondary" noWrap>
                                  {getRelativeTime(agent.createdAt)}
                                </Typography>
                              )}
                            </Stack>
                          </ListingTable.Cell>
                        </ListingTable.Row>
                      ))}
                    </ListingTable.Body>
                  </ListingTable>
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
                </ListingTable.Container>
              ) : !!data?.agents?.length && agentsWithHref.length === 0 ? (
                <ListingTable.Container>
                  <ListingTable.EmptyState
                    illustration={<User size={64} />}
                    title="No agents found"
                    description="No agents match your search criteria. Try adjusting your search."
                  />
                </ListingTable.Container>
              ) : !data?.agents?.length && !isRefetching ? (
                <ListingTable.Container>
                  <ListingTable.EmptyState
                    illustration={<User size={64} />}
                    title="No agents found"
                    description="Create a new agent to get started"
                    action={
                      <Button
                        variant="contained"
                        color="primary"
                        startIcon={<Add />}
                        onClick={() =>
                          navigate(
                            generatePath(
                              absoluteRouteMap.children.org.children.projects.children
                                .newAgent.path,
                              { orgId: orgId ?? "", projectId: projectId ?? "" }
                            )
                          )
                        }
                      >
                        Add New Agent
                      </Button>
                    }
                  />
                </ListingTable.Container>
              ) : null}
            </Stack>
            <Box>
              <AgentTypeSummery />
            </Box>
          </Stack>
        )}
      </PageLayout>

      {project && (
        <EditProjectDrawer
          open={editProjectDrawerOpen}
          onClose={() => setEditProjectDrawerOpen(false)}
          project={project}
          orgId={orgId || 'default'}
        />
      )}
    </>
  );
};

export default AgentsList;

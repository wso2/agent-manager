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
  TextField,
  Typography,
  Avatar,
  ButtonBase,
  Button,
  Alert,
  useTheme,
  Tooltip,
  Skeleton,
  Chip,
  alpha,
  IconButton,
  CircularProgress,
} from "@wso2/oxygen-ui";
import {
  Clock as AccessTimeRounded,
  Plus as Add,
  Trash2 as DeleteOutlineOutlined,
  RefreshCcw,
  Search as SearchRounded,
  User,
} from "@wso2/oxygen-ui-icons-react";
import {
  PageLayout,
  DataListingTable,
  TableColumn,
  NoDataFound,
  FadeIn,
  InitialState,
  displayProvisionTypes,
} from "@agent-management-platform/views";
import { generatePath, Link, useNavigate, useParams } from "react-router-dom";
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
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import { AgentTypeSummery } from "./subComponents/AgentTypeSummery";

dayjs.extend(relativeTime);

export function ListPageSkeleton() {
  return (
    <Box display="flex" flexDirection="column" gap={2} p={2}>
      <Box
        display="flex"
        flexDirection="row"
        justifyContent="space-between"
        gap={2}
      >
        <Box display="flex" gap={2}>
          <Skeleton variant="rounded" width={100} height={100} />
          <Skeleton variant="rounded" width={400} height={100} />
        </Box>
        <Skeleton variant="rounded" height={40} width={150} />
      </Box>
      <Box display="flex" flexDirection="column" gap={2}>
        <Skeleton variant="rounded" width="100%" height={40} />
        <Skeleton variant="rounded" width="100%" height={450} />
      </Box>
    </Box>
  );
}

export interface AgentWithHref extends AgentResponse {
  href: string;
  id: string;
  agentInfo: { name: string; displayName: string; description: string };
}

export const AgentsList: React.FC = () => {
  const theme = useTheme();
  const [search, setSearch] = useState("");
  const [hoveredAgentId, setHoveredAgentId] = useState<string | null>(null);

  // Detect touch device for alternative interaction pattern
  const isTouchDevice =
    typeof window !== "undefined" &&
    ("ontouchstart" in window || navigator.maxTouchPoints > 0);

  const { orgId, projectId } = useParams<{
    orgId: string;
    projectId: string;
  }>();
  const navigate = useNavigate();
  const { data, isLoading, error, isRefetching, refetch: refetchAgents} = useListAgents({
    orgName: orgId ?? "default",
    projName: projectId ?? "default",
  });
  const { mutate: deleteAgent } = useDeleteAgent();
  const { data: project, isLoading: isProjectLoading } = useGetProject({
    orgName: orgId ?? "default",
    projName: projectId ?? "default",
  });
  const handleDeleteAgent = useCallback(
    (agentId: string) => {
      deleteAgent({
        orgName: orgId ?? "default",
        projName: "default",
        agentName: agentId,
      });
    },
    [deleteAgent, orgId]
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

  const columns = useMemo(
    () =>
      [
        {
          id: "agentInfo",
          label: "Agent Name",
          sortable: true,
          width: "25%",
          render: (value, row) => {
            const agentInfo = value as {
              name: string;
              displayName: string;
              description: string;
            };
            return (
              <ButtonBase component={Link} to={row?.href}>
                <Box display="flex" alignItems="center" gap={1}>
                  <Avatar
                    variant="circular"
                    sx={{
                      backgroundColor: alpha(theme.palette.primary.main, 0.1),
                      color: theme.palette.primary.main,
                      height: 32,
                      width: 32,
                    }}
                  >
                    {agentInfo.displayName.substring(0, 1).toUpperCase()}
                  </Avatar>
                  <Box
                    display="flex"
                    alignItems="flex-start"
                    gap={1}
                  >
                    <Typography variant="body1">
                      {agentInfo.displayName}
                    </Typography>
                    {row.provisioning.type !== "internal" && (
                      <Chip
                        label={
                          displayProvisionTypes((row.provisioning as Provisioning).type)
                        }
                        size="small"
                        variant="outlined"
                      />
                    )}
                  </Box>
                </Box>
              </ButtonBase>
            );
          },
        },
        {
          id: "description",
          label: "Description",
          sortable: true,
          width: "30%",
          render: (value) => (
            <Typography variant="body2" noWrap textOverflow="ellipsis" overflow="hidden">
              {(value as string).substring(0, 40) +
                ((value as string).length > 40 ? "..." : "")}
            </Typography>
          ),
        },
        {
          id: "createdAt",
          label: "Last Updated",
          sortable: true,
          width: "20%",
          align: "right",
          render: (value, row) => (
            <Box
              display="flex"
              alignItems="center"
              gap={1}
              justifyContent="flex-end"
              sx={{ minWidth: 150 }} // Prevent layout shift
            >
              {hoveredAgentId === row?.id || isTouchDevice ? (
                <Box
                  display="flex"
                  alignItems="center"
                  gap={1}
                  justifyContent="flex-end"
                >
                  <FadeIn>
                    <Tooltip title="Delete Agent">
                      <Button
                        startIcon={<DeleteOutlineOutlined size={16} />}
                        color="error"
                        variant="outlined"
                        size="small"
                        onClick={(e) => {
                          e.stopPropagation(); // Prevent row click if any
                          handleDeleteAgent(row.name);
                        }}
                      >
                        Delete
                      </Button>
                    </Tooltip>
                  </FadeIn>
                </Box>
              ) : (
                <>
                  <AccessTimeRounded fontSize="small" color="disabled" />
                  <Typography variant="body2" color="text.secondary" noWrap>
                    {dayjs(value as string).fromNow()}
                  </Typography>
                </>
              )}
            </Box>
          ),
        },
      ] as TableColumn<AgentWithHref>[],
    [theme, handleDeleteAgent, hoveredAgentId, isTouchDevice]
  );

  // Define initial state for sorting - most recently updated agents first
  const tableInitialState: InitialState<AgentWithHref> = useMemo(() => ({
    sorting: {
      sortModel: [{
        field: 'createdAt',
        sort: 'desc'
      }]
    }
  }), []);

  if (isLoading || isProjectLoading || (isRefetching && !data?.agents?.length)) {
    return <ListPageSkeleton />;
  }

  return (
    <PageLayout
      title={project?.displayName ?? "Agents"}
      description={
        project?.description ??
        "Manage and monitor all your AI agents across environments"
      }
      titleTail={
        <Box
          display="flex"
          alignItems="center"
          minWidth={32}
          justifyContent="center"
        >
          {isRefetching ? (
            <CircularProgress size={18} color="primary" />
          ) : (
            <IconButton size="small" color="primary" onClick={()=>refetchAgents()}>
              <RefreshCcw size={18} />
            </IconButton>
          )}
        </Box>
      }
    >
    <Box display="flex" justifyContent="space-between" gap={4} minHeight="calc(100vh - 250px)">
        <Box
          sx={{
            display: "flex",
            flexGrow: 1,
            flexDirection: "column",
            gap: 4,
          }}
        >
          <Box display="flex" justifyContent="flex-end" gap={1}>
            <Box flexGrow={1}>
              <TextField
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                slotProps={{
                  input: { endAdornment: <SearchRounded size={16} /> },
                }}
                fullWidth
                size="small"
                variant="outlined"
                placeholder="Search agents"
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
                    { orgId: orgId ?? "", projectId: "default" }
                  )
                )
              }
            >
              Add Agent
            </Button>
          </Box>

          {error && (
            <Alert severity="error" variant="outlined">
              {error.message}
            </Alert>
          )}

          {!isLoading && !!data?.agents?.length && (
            <Box bgcolor="background.paper" borderRadius={1}>
              <DataListingTable
                data={agentsWithHref}
                columns={columns}
                pagination={true}
                pageSize={5}
                maxRows={agentsWithHref?.length}
                initialState={tableInitialState}
                onRowMouseEnter={handleRowMouseEnter}
                onRowMouseLeave={handleRowMouseLeave}
                onRowFocusIn={handleRowMouseEnter}
                onRowFocusOut={handleRowMouseLeave}
                onRowClick={(row) => navigate(row?.href)}
                emptyStateTitle="No agents found"
                emptyStateDescription="Looks like there are no agents matching your search."
              />
            </Box>
          )}

          {!isLoading && !data?.agents?.length && !isRefetching && (
              <NoDataFound
                message="No agents found"
                iconElement={User}
                subtitle="Create a new agent to get started"
                action={
                  <Button
                    variant="contained"
                    color="primary"
                    startIcon={<Add />}
                    onClick={() =>
                      navigate(
                        generatePath(
                          absoluteRouteMap.children.org.children.projects
                            .children.newAgent.path,
                          { orgId: orgId ?? "", projectId: "default" }
                        )
                      )
                    }
                  >
                    Add New Agent
                  </Button>
                }
              />
          )}
        </Box>
        <Box>
          <AgentTypeSummery />
        </Box>
      </Box>
    </PageLayout>
  );
};

export default AgentsList;

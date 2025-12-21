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

import { useMemo, useCallback } from "react";
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import {
  DataListingTable,
  TableColumn,
  InitialState,
  DrawerWrapper,
} from "@agent-management-platform/views";
import {
  CheckCircle,
  Rocket,
  Circle,
  XCircle,
} from "@wso2/oxygen-ui-icons-react";
import {
  generatePath,
  Link,
  useParams,
  useSearchParams,
} from "react-router-dom";
import { BuildLogs } from "@agent-management-platform/shared-component";
import { useGetAgentBuilds } from "@agent-management-platform/api-client";
import {
  BuildStatus,
  BUILD_STATUS_COLOR_MAP,
  absoluteRouteMap,
} from "@agent-management-platform/types";
import dayjs from "dayjs";

interface BuildRow {
  id: string;
  branch: string;
  status: BuildStatus;
  title: string;
  commit: string;
  actions: string;
  startedAt: string;
  imageId: string;
}

export interface StatusConfig {
  color: "success" | "warning" | "error" | "default";
  label: string;
}

const getStatusIcon = (status: StatusConfig) => {
  switch (status.color) {
    case "success":
      return <CheckCircle size={16} />;
    case "warning":
      return <CircularProgress size={14} color="warning" />;
    case "error":
      return <XCircle size={16} />;
    default:
      return <Circle size={16} />;
  }
};
// Generic helper functions for common use cases
export const renderStatusChip = (status: StatusConfig, theme?: any) => (
  <Box display="flex" alignItems="center" gap={theme?.spacing(1) || 1}>
    <Chip
      variant="outlined"
      icon={getStatusIcon(status)}
      label={status.label}
      color={status.color}
      size="small"
    />
  </Box>
);

export function BuildTable() {
  const theme = useTheme();
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedBuildName = searchParams.get("selectedBuild");
  const selectedPanel = searchParams.get("panel"); // 'logs' | 'deploy'
  const { orgId, projectId, agentId } = useParams();
  const { data: builds } = useGetAgentBuilds({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const orderedBuilds = useMemo(
    () =>
      builds?.builds.sort(
        (a, b) =>
          new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime()
      ),
    [builds]
  );

  const rows = useMemo(
    () =>
      orderedBuilds?.map(
        (build) =>
          ({
            id: build.buildName,
            actions: build.buildName,
            branch: build.branch,
            commit: build.commitId,
            startedAt: build.startedAt,
            status: build.status as BuildStatus,
            title: build.buildName,
            imageId: build.imageId ?? "busybox",
          }) as BuildRow
      ) ?? [],
    [orderedBuilds]
  );

  const handleBuildClick = useCallback(
    (buildName: string, panel: "logs" | "deploy") => {
      const next = new URLSearchParams(searchParams);
      next.set("selectedBuild", buildName);
      next.set("panel", panel);
      setSearchParams(next);
    },
    [searchParams, setSearchParams]
  );

  const clearSelectedBuild = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.delete("selectedBuild");
    next.delete("panel");
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  const columns: TableColumn<BuildRow>[] = useMemo(
    () => [
      {
        id: "branch",
        label: "Branch",
        width: "15%",
        render: (value, row) => (
          <Typography noWrap variant="body2">
            {`${value} : ${row.commit}`}
          </Typography>
        ),
      },
      {
        id: "title",
        label: "Build Name",
        width: "15%",
        render: (value) => (
          <Typography noWrap variant="body2" color="text.primary">
            {value}
          </Typography>
        ),
      },
      {
        id: "startedAt",
        label: "Started At",
        width: "15%",
        render: (value) => (
          <Typography noWrap variant="body2" color="text.secondary">
            {dayjs(value as string).format("DD/MM/YYYY HH:mm:ss")}
          </Typography>
        ),
      },
      {
        id: "status",
        label: "Status",
        width: "12%",
        render: (value) =>
          renderStatusChip(
            {
              color: BUILD_STATUS_COLOR_MAP[value as BuildStatus],
              label: value as string,
            },
            theme
          ),
      },
      {
        id: "actions",
        label: "",
        width: "10%",
        render: (_value, row) => (
          <Box display="flex" justifyContent="flex-end" gap={1}>
            <Button
              variant="text"
              color="primary"
              onClick={() => handleBuildClick(row.title, "logs")}
              size="small"
            >
              Details
            </Button>
            <Button
              variant="outlined"
              color="primary"
              disabled={
                row.status === "BuildTriggered" ||
                row.status === "BuildRunning" ||
                row.status === "BuildFailed"
              }
              component={Link}
              to={`${generatePath(
                absoluteRouteMap.children.org.children.projects.children.agents
                  .children.deployment.path,
                { orgId, projectId, agentId }
              )}?deployPanel=open&selectedBuild=${row.id}`}
              size="small"
              startIcon={
                row.status === "BuildRunning" ||
                row.status === "BuildTriggered" ? (
                  <CircularProgress color="inherit" size={14} />
                ) : (
                  <Rocket size={16} />
                )
              }
            >
              {row.status === "BuildRunning" || row.status === "BuildTriggered"
                ? "Building"
                : "Deploy"}
            </Button>
          </Box>
        ),
      },
    ],
    [theme, handleBuildClick, orgId, projectId, agentId]
  );

  // Define initial state for sorting - most recent builds first
  const tableInitialState: InitialState<BuildRow> = useMemo(
    () => ({
      sorting: {
        sortModel: [
          {
            field: "startedAt",
            sort: "desc",
          },
        ],
      },
    }),
    []
  );

  return (
    <Box
      display="flex"
      borderRadius={1}
      flexDirection="column"
      bgcolor={"background.paper"}
    >
      <DataListingTable
        data={rows}
        columns={columns}
        pagination
        pageSize={5}
        maxRows={rows.length}
        initialState={tableInitialState}
      />
      <DrawerWrapper open={!!selectedBuildName} onClose={clearSelectedBuild}>
        {selectedPanel === "logs" && selectedBuildName && (
          <BuildLogs
            onClose={clearSelectedBuild}
            orgName={orgId || ""}
            projName={projectId || ""}
            agentName={agentId || ""}
            buildName={selectedBuildName}
          />
        )}
      </DrawerWrapper>
    </Box>
  );
}

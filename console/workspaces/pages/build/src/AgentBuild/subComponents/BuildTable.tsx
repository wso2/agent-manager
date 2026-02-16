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

import { useMemo, useCallback, useState } from "react";
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  Typography,
  useTheme,
  type Theme,
  ListingTable,
  TablePagination,
  type ListingTableSortDirection,
  DataGrid,
} from "@wso2/oxygen-ui";
import {
  DrawerWrapper,
} from "@agent-management-platform/views";

const { DataGrid: DataGridComponent } = DataGrid;
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
import { format } from "date-fns";

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
export const renderStatusChip = (status: StatusConfig, theme?: Theme) => (
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
  const [sortField, setSortField] = useState<string>('startedAt');
  const [sortDirection, setSortDirection] = useState<ListingTableSortDirection>('desc');
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(5);
  const { data: builds, isLoading } = useGetAgentBuilds({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const orderedBuilds = useMemo(
    () =>
      builds?.builds.sort(
        (a, b) =>
          new Date(b.startedAt).getTime() - new Date(a.startedAt).getTime(),
      ),
    [builds],
  );

  const rows = useMemo(
    () =>
      orderedBuilds?.map(
        (build) =>
          ({
            id: build.buildName,
            actions: build.buildName,
            branch: build.buildParameters.branch,
            commit: build.buildParameters.commitId,
            startedAt: build.startedAt,
            status: build.status as BuildStatus,
            title: build.buildName,
            imageId: build.imageId ?? "busybox",
          }) as BuildRow,
      ) ?? [],
    [orderedBuilds],
  );

  const handleSortChange = useCallback((field: string, direction: ListingTableSortDirection) => {
    setSortField(field);
    setSortDirection(direction);
  }, []);

  const sortedRows = useMemo(() => {
    return [...rows].sort((a, b) => {
      const aVal = a[sortField as keyof BuildRow];
      const bVal = b[sortField as keyof BuildRow];
      const comparison = String(aVal).localeCompare(String(bVal));
      return sortDirection === 'asc' ? comparison : -comparison;
    });
  }, [rows, sortField, sortDirection]);

  const paginatedRows = useMemo(
    () => sortedRows.slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage),
    [sortedRows, page, rowsPerPage]
  );

  const handleBuildClick = useCallback(
    (buildName: string, panel: "logs" | "deploy") => {
      const next = new URLSearchParams(searchParams);
      next.set("selectedBuild", buildName);
      next.set("panel", panel);
      setSearchParams(next);
    },
    [searchParams, setSearchParams],
  );

  const clearSelectedBuild = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.delete("selectedBuild");
    next.delete("panel");
    setSearchParams(next);
  }, [searchParams, setSearchParams]);

  return (
     <>
      <ListingTable.Provider
        sortField={sortField}
        sortDirection={sortDirection}
        onSortChange={handleSortChange}
        page={page}
        rowsPerPage={rowsPerPage}
        totalCount={rows.length}
        onPageChange={setPage}
        onRowsPerPageChange={(rpp) => {
          setRowsPerPage(rpp);
          setPage(0);
        }}
      >
        <ListingTable.Container>
          {isLoading ? (
            <DataGridComponent
              rows={[]}
              columns={[
                { field: 'branch', headerName: 'Branch', flex: 1 },
                { field: 'title', headerName: 'Build Name', flex: 1 },
                { field: 'startedAt', headerName: 'Started At', flex: 1 },
                { field: 'status', headerName: 'Status', flex: 1 },
                { field: 'actions', headerName: '', flex: 1 },
              ]}
              loading
              hideFooter
            />
          ) : rows.length > 0 ? (
            <>
              <ListingTable>
            <ListingTable.Head>
              <ListingTable.Row>
                <ListingTable.Cell width="15%">
                  <ListingTable.SortLabel field="branch">Branch</ListingTable.SortLabel>
                </ListingTable.Cell>
                <ListingTable.Cell width="15%">
                  <ListingTable.SortLabel field="title">Build Name</ListingTable.SortLabel>
                </ListingTable.Cell>
                <ListingTable.Cell width="15%">
                  <ListingTable.SortLabel field="startedAt">Started At</ListingTable.SortLabel>
                </ListingTable.Cell>
                <ListingTable.Cell width="12%">
                  <ListingTable.SortLabel field="status">Status</ListingTable.SortLabel>
                </ListingTable.Cell>
                <ListingTable.Cell width="10%" align="right"></ListingTable.Cell>
              </ListingTable.Row>
            </ListingTable.Head>
            <ListingTable.Body>
              {paginatedRows.map((row) => (
                <ListingTable.Row key={row.id} hover>
                  <ListingTable.Cell>
                    <Typography noWrap variant="body2">
                      {`${row.branch} : ${row.commit.substring(0, 7)}`}
                    </Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Typography noWrap variant="body2" color="text.primary">
                      {row.title}
                    </Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    <Typography noWrap variant="body2" color="text.secondary">
                      {format(new Date(row.startedAt), "dd/MM/yyyy HH:mm:ss")}
                    </Typography>
                  </ListingTable.Cell>
                  <ListingTable.Cell>
                    {renderStatusChip(
                      {
                        color: BUILD_STATUS_COLOR_MAP[row.status],
                        label: row.status,
                      },
                      theme,
                    )}
                  </ListingTable.Cell>
                  <ListingTable.Cell align="right">
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
                          row.status === "Pending" ||
                          row.status === "Running" ||
                          row.status === "Failed"
                        }
                        component={Link}
                        to={`${generatePath(
                          absoluteRouteMap.children.org.children.projects.children.agents
                            .children.deployment.path,
                          { orgId, projectId, agentId },
                        )}?deployPanel=open&selectedBuild=${row.id}`}
                        size="small"
                        startIcon={
                          row.status === "Running" ||
                          row.status === "Pending" ? (
                            <CircularProgress color="inherit" size={14} />
                          ) : (
                            <Rocket size={16} />
                          )
                        }
                      >
                        {row.status === "Running" || row.status === "Pending"
                          ? "Building"
                          : "Deploy"}
                      </Button>
                    </Box>
                  </ListingTable.Cell>
                </ListingTable.Row>
              ))}
            </ListingTable.Body>
          </ListingTable>
            <TablePagination
              rowsPerPageOptions={[5, 10, 25]}
              component="div"
              count={rows.length}
              rowsPerPage={rowsPerPage}
              page={page}
              onPageChange={(_, newPage) => setPage(newPage)}
              onRowsPerPageChange={(e) => {
                setRowsPerPage(parseInt(e.target.value, 10));
                setPage(0);
              }}
            />
          </>
          ) : (
            <ListingTable.EmptyState
              illustration={<Rocket size={64} />}
              title="No builds yet"
              description="Trigger a build to see it listed here"
            />
          )}
        </ListingTable.Container>
      </ListingTable.Provider>
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
     </>
  );
}

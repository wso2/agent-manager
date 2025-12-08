import { useMemo, useCallback } from "react";
import {
  Typography,
  Chip,
  Button,
  Box,
  Skeleton,
  ButtonBase,
  MenuItem,
  Select,
  InputAdornment,
  CircularProgress,
  IconButton,
} from "@wso2/oxygen-ui";
import {
  DataListingTable,
  FadeIn,
  NoDataFound,
  TableColumn,
  InitialState,
} from "@agent-management-platform/views";
import { generatePath, Link, useNavigate } from "react-router-dom";
import {
  useGetAgent,
  useTraceList,
} from "@agent-management-platform/api-client";
import {
  absoluteRouteMap,
  Trace,
  TraceListResponse,
  TraceListTimeRange,
} from "@agent-management-platform/types";
import dayjs from "dayjs";
import {
  Clock as AccessTimeOutlined,
  RefreshCcw,
  Eye as RemoveRedEyeOutlined,
  Workflow,
} from "@wso2/oxygen-ui-icons-react";
import { TracesTopCards } from "./TracesTopCards";

interface TraceRow {
  id: string;
  traceId: string;
  rootSpanName: string;
  startTime: string;
  endTime: string;
  durationInNanos: number;
}

function TracesTableSkeleton() {
  return (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        gap: 1,
      }}
    >
      <Skeleton variant="rectangular" width="100%" height={7} />
      {[...Array(10)].map((_, index) => (
        <Skeleton key={index} variant="rectangular" width="100%" height={6} />
      ))}
    </Box>
  );
}

const TIME_RANGE_OPTIONS = [
  { value: TraceListTimeRange.TEN_MINUTES, label: "10 Minutes" },
  { value: TraceListTimeRange.THIRTY_MINUTES, label: "30 Minutes" },
  { value: TraceListTimeRange.ONE_HOUR, label: "1 Hour" },
  { value: TraceListTimeRange.THREE_HOURS, label: "3 Hours" },
  { value: TraceListTimeRange.SIX_HOURS, label: "6 Hours" },
  { value: TraceListTimeRange.TWELVE_HOURS, label: "12 Hours" },
  { value: TraceListTimeRange.ONE_DAY, label: "1 Day" },
  { value: TraceListTimeRange.THREE_DAYS, label: "3 Days" },
  { value: TraceListTimeRange.SEVEN_DAYS, label: "7 Days" },
];

interface TracesTableProps {
  orgId: string;
  projectId: string;
  agentId: string;
  envId: string;
  timeRange: TraceListTimeRange;
  setTimeRange?: (timeRange: TraceListTimeRange) => void;
}

export function TracesTable({
  orgId,
  projectId,
  agentId,
  envId,
  timeRange,
  setTimeRange,
}: TracesTableProps) {
  const { data: agentData } = useGetAgent({
    orgName: orgId,
    projName: projectId,
    agentName: agentId,
  });
  const isExternalAgent = agentData?.provisioning?.type === "external";
  const navigate = useNavigate();
  const {
    data: traceData,
    isLoading,
    refetch,
    isRefetching,
  } = useTraceList(orgId, projectId, agentId, envId, timeRange);

  const traceListResponse = traceData as unknown as TraceListResponse;

  const rows = useMemo(
    () =>
      traceListResponse?.traces?.map((trace: Trace) => {
        const start = new Date(trace.startTime).getTime();
        const end = new Date(trace.endTime).getTime();
        const durationInNanos = (end - start) / 1000;

        return {
          id: trace.traceId,
          traceId: trace.traceId,
          rootSpanName: trace.rootSpanName,
          startTime: trace.startTime,
          endTime: trace.endTime,
          durationInNanos: durationInNanos,
        } as TraceRow;
      }) ?? [],
    [traceListResponse?.traces]
  );

  const getDurationColor = useCallback((durationInNanos: number) => {
    if (durationInNanos < 2) return "success";
    if (durationInNanos < 5) return "warning";
    return "error";
  }, []);

  const columns: TableColumn<TraceRow>[] = useMemo(
    () => [
      {
        id: "rootSpanName",
        label: "Name",
        width: "20%",
        render: (value, row) => (
          <ButtonBase
            component={Link}
            to={
              isExternalAgent
                ? generatePath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.children.observe.children.traces.children
                      .traceDetails.path,
                    {
                      orgId: orgId ?? "",
                      projectId: projectId ?? "",
                      agentId: agentId ?? "",
                      traceId: row.traceId as string,
                    }
                  )
                : generatePath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.children.environment.children.observability
                      .children.traces.children.traceDetails.path,
                    {
                      orgId: orgId ?? "",
                      projectId: projectId ?? "",
                      agentId: agentId ?? "",
                      envId: envId ?? "",
                      traceId: row.traceId as string,
                    }
                  )
            }
          >
            <Typography
              noWrap
              variant="body2"
            >
              {value}
            </Typography>
          </ButtonBase>
        ),
      },
      {
        id: "traceId",
        label: "Trace ID",
        width: "25%",
        render: (value) => (
          <ButtonBase
            component={Link}
            to={
              isExternalAgent
                ? generatePath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.children.observe.children.traces.children
                      .traceDetails.path,
                    {
                      orgId: orgId ?? "",
                      projectId: projectId ?? "",
                      agentId: agentId ?? "",
                      traceId: value as string,
                    }
                  )
                : generatePath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.children.environment.children.observability
                      .children.traces.children.traceDetails.path,
                    {
                      orgId: orgId ?? "",
                      projectId: projectId ?? "",
                      agentId: agentId ?? "",
                      envId: envId ?? "",
                      traceId: value as string,
                    }
                  )
            }
          >
            <Typography noWrap variant="body2" color="text.secondary">
              {(value as string).substring(0, 8)}.....
              {(value as string).substring((value as string).length - 8)}
            </Typography>
          </ButtonBase>
        ),
      },
      {
        id: "startTime",
        label: "Start Time",
        width: "20%",
        render: (value) => (
          <Typography
            noWrap
            variant="body2"
          >
            {dayjs(value as string).format("DD/MM/YYYY HH:mm:ss")}
          </Typography>
        ),
      },
      {
        id: "durationInNanos",
        label: "Duration",
        width: "15%",
        render: (value) => (
          <Chip
            label={`${(value as number).toFixed(2)}s`}
            size="small"
            color={getDurationColor(value as number)}
            variant="outlined"
          />
        ),
      },
      {
        id: "actions",
        label: "",
        width: "10%",
        align: "center",
        render: (_value, row) => (
          <Button
            variant="text"
            size="small"
            component={Link}
            startIcon={<RemoveRedEyeOutlined size={16} />}
            to={
              isExternalAgent
                ? generatePath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.children.observe.children.traces.children
                      .traceDetails.path,
                    {
                      orgId: orgId ?? "",
                      projectId: projectId ?? "",
                      agentId: agentId ?? "",
                      traceId: row.traceId as string,
                    }
                  )
                : generatePath(
                    absoluteRouteMap.children.org.children.projects.children
                      .agents.children.environment.children.observability
                      .children.traces.children.traceDetails.path,
                    {
                      orgId: orgId ?? "",
                      projectId: projectId ?? "",
                      agentId: agentId ?? "",
                      envId: envId ?? "",
                      traceId: row.traceId as string,
                    }
                  )
            }
          >
            Expand
          </Button>
        ),
      },
    ],
    [
      orgId,
      projectId,
      agentId,
      envId,
      isExternalAgent,
      getDurationColor,
    ]
  );

  // Define initial state for sorting - most recent traces first
  const tableInitialState: InitialState<TraceRow> = useMemo(
    () => ({
      sorting: {
        sortModel: [
          {
            field: "startTime",
            sort: "desc",
          },
        ],
      },
    }),
    []
  );

  return (
    <FadeIn>
      <Box display="flex" flexDirection="column" gap={2}>
        <Box display="flex" justifyContent="flex-end" gap={1} width="100%">
          {setTimeRange && (
            <Select
              size="small"
              variant="outlined"
              value={timeRange}
              startAdornment={
                <InputAdornment position="start">
                  <AccessTimeOutlined size={16} />
                </InputAdornment>
              }
              onChange={(e) =>
                setTimeRange(e.target.value as TraceListTimeRange)
              }
            >
              {TIME_RANGE_OPTIONS.map((option) => (
                <MenuItem key={option.value} value={option.value}>
                  {option.label}
                </MenuItem>
              ))}
            </Select>
          )}
          <IconButton
            disabled={isRefetching}
            color="primary"
            onClick={() => {
              refetch();
            }}
          >
            {isRefetching ? (
              <CircularProgress size={18} color="inherit" />
            ) : (
              <RefreshCcw size={18} />
            )}
          </IconButton>
        </Box>
        <TracesTopCards timeRange={timeRange} />
        {isLoading && <TracesTableSkeleton />}
        {rows.length > 0 && (
          <Box sx={{ backgroundColor: "background.paper", borderRadius: 1 }}>
            <DataListingTable
              data={rows}
              columns={columns}
              onRowClick={(row) => {
                navigate(
                  isExternalAgent
                    ? generatePath(
                        absoluteRouteMap.children.org.children.projects.children
                          .agents.children.observe.children.traces.children
                          .traceDetails.path,
                        {
                          orgId: orgId ?? "",
                          projectId: projectId ?? "",
                          agentId: agentId ?? "",
                          traceId: row.traceId as string,
                        }
                      )
                    : generatePath(
                        absoluteRouteMap.children.org.children.projects.children
                          .agents.children.environment.children.observability
                          .children.traces.children.traceDetails.path,
                        {
                          orgId: orgId ?? "",
                          projectId: projectId ?? "",
                          agentId: agentId ?? "",
                          envId: envId ?? "",
                          traceId: row.traceId as string,
                        }
                      )
                );
              }}
              pagination
              pageSize={10}
              maxRows={rows.length}
              initialState={tableInitialState}
              emptyStateTitle="No traces found"
              emptyStateDescription="No traces found for the selected time range"
            />
          </Box>
        )}
        {(rows.length === 0 && !isLoading )&& (
          <Box p={4}>
            <NoDataFound
              message="No traces found!"
              icon={<Workflow size={32} />}
              subtitle="Try changing the time range"
            />
          </Box>
        )}
      </Box>
    </FadeIn>
  );
}

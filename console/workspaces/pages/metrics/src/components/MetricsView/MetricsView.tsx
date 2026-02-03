import React, { useMemo } from "react";
import dayjs from "dayjs";
import { NoDataFound } from "@agent-management-platform/views";
import {
  Alert,
  Box,
  Card,
  CardContent,
  CardHeader,
  Grid,
  Skeleton,
  Stack,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { LineChart, ChartTooltip } from "@wso2/oxygen-ui-charts-react";
import type {
  MetricDataPoint,
  MetricsResponse,
} from "@agent-management-platform/types";

const toGib = (value: number) => value / 1024 ** 3;

type SeriesDefinition = {
  key: string;
  points?: MetricDataPoint[];
  transform?: (value: number) => number;
};

const buildSeriesData = (series: SeriesDefinition[]) => {
  const map = new Map<
    string,
    {
      time: string;
      label: string;
      [key: string]: string | number;
    }
  >();

  series.forEach(({ key, points, transform }) => {
    points?.forEach((point) => {
      const existing = map.get(point.time) ?? {
        time: point.time,
        label: dayjs(point.time).format("MM/DD HH:mm"),
      };
      map.set(point.time, {
        ...existing,
        [key]: transform ? transform(point.value) : point.value,
      });
    });
  });

  return Array.from(map.values()).sort(
    (a, b) => new Date(a.time).getTime() - new Date(b.time).getTime(),
  );
};

type MetricsTooltipProps = {
  active?: boolean;
  label?: string;
  payload?: Array<{
    name?: string;
    value?: number;
    color?: string;
    dataKey?: string;
  }>;
  formatter?: (value: number) => string;
  title?: string;
};

const MetricsTooltip: React.FC<MetricsTooltipProps> = ({
  active,
  payload,
  formatter,    
}) => {
  if (!active || !payload || payload.length === 0) {
    return null;
  }

  return (
    <Card
      variant="outlined"
      sx={{ "&.MuiCard-root": { backgroundColor: "background.paper" } }}
    >
      <CardContent>
        <Stack direction="column" gap={0.5}>
          {payload.map((entry) => (
            <Stack
              key={entry.dataKey ?? entry.name}
              direction="row"
              alignItems="center"
              gap={1}
            >
              <Box
                sx={{
                  width: 8,
                  height: 8,
                  borderRadius: "50%",
                  bgcolor: entry.color ?? "text.secondary",
                }}
              />
              <Typography variant="body2" color="textSecondary" flex={1}>
                {entry.name ?? entry.dataKey}
              </Typography>
              <Typography variant="body2" fontWeight={600}>
                {typeof entry.value === "number" && formatter
                  ? formatter(entry.value)
                  : (entry.value ?? "--")}
              </Typography>
            </Stack>
          ))}
        </Stack>
      </CardContent>
    </Card>
  );
};

export interface MetricsViewProps {
  metrics?: MetricsResponse;
  isLoading?: boolean;
  error?: unknown;
}

export const MetricsView: React.FC<MetricsViewProps> = ({
  metrics,
  isLoading,
  error,
}) => {
  const theme = useTheme();
  const hasData = useMemo(
    () =>
      (metrics?.cpuUsage?.length ?? 0) > 0 ||
      (metrics?.cpuRequests?.length ?? 0) > 0 ||
      (metrics?.cpuLimits?.length ?? 0) > 0 ||
      (metrics?.memory?.length ?? 0) > 0 ||
      (metrics?.memoryRequests?.length ?? 0) > 0 ||
      (metrics?.memoryLimits?.length ?? 0) > 0,
    [metrics],
  );

  if (error) {
    return (
      <Alert severity="error">
        {error instanceof Error ? error.message : "Failed to load metrics"}
      </Alert>
    );
  }

  if (!isLoading && !hasData) {
    return (
      <NoDataFound
        message="No metrics found!"
        subtitle="Try changing the time range"
      />
    );
  }

  return (
    <Grid container spacing={3}>
      <Grid size={{ xs: 12, md: 6 }}>
        <Card variant="outlined" sx={{ height: "100%" }}>
          <CardHeader title="CPU Usage" />
          <CardContent
            sx={{
              display: "flex",
              flexDirection: "column",
              height: "100%",
              "& svg:focus, & svg:focus-visible, & [tabindex]:focus, & [tabindex]:focus-visible":
                {
                  outline: "none",
                },
            }}
          >
            {isLoading ? (
              <Skeleton variant="rounded" height={260} width="100%" />
            ) : (
              <LineChart
                data={buildSeriesData([
                  { key: "cpuUsage", points: metrics?.cpuUsage },
                  { key: "cpuRequests", points: metrics?.cpuRequests },
                  { key: "cpuLimits", points: metrics?.cpuLimits },
                ])}
                xAxisDataKey="label"
                tooltip={{ show: false }}
                xAxis={{ show: true, interval: "preserveStartEnd" }}
                yAxis={{ show: true, name: "Cores" }}
                lines={[
                  {
                    dataKey: "cpuUsage",
                    name: "Usage",
                    stroke: theme.palette.primary.main,
                    dot: false,
                    connectNulls: true,
                    unit: " cores",
                  },
                  {
                    dataKey: "cpuRequests",
                    name: "Requests",
                    stroke: theme.palette.secondary.main,
                    dot: false,
                    connectNulls: true,
                    unit: " cores",
                  },
                  {
                    dataKey: "cpuLimits",
                    name: "Limits",
                    stroke: theme.palette.error.main,
                    dot: false,
                    connectNulls: true,
                    strokeDasharray: "0",
                    unit: " cores",
                  },
                ]}
              >
                <ChartTooltip
                  content={
                    <MetricsTooltip
                      title="CPU"
                      formatter={(value) => `${value.toFixed(3)} cores`}
                    />
                  }
                />
              </LineChart>
            )}
          </CardContent>
        </Card>
      </Grid>
      <Grid size={{ xs: 12, md: 6 }}>
        <Card variant="outlined" sx={{ height: "100%" }}>
          <CardHeader title="Memory" />
          <CardContent
            sx={{
              display: "flex",
              flexDirection: "column",
              height: "100%",
              "& svg:focus, & svg:focus-visible, & [tabindex]:focus, & [tabindex]:focus-visible":
                {
                  outline: "none",
                },
            }}
          >
            {isLoading ? (
              <Skeleton variant="rounded" height={260} width="100%" />
            ) : (
              <LineChart
                data={buildSeriesData([
                  {
                    key: "memoryUsage",
                    points: metrics?.memory,
                    transform: toGib,
                  },
                  {
                    key: "memoryRequests",
                    points: metrics?.memoryRequests,
                    transform: toGib,
                  },
                  {
                    key: "memoryLimits",
                    points: metrics?.memoryLimits,
                    transform: toGib,
                  },
                ])}
                xAxisDataKey="label"
                xAxis={{ show: true, interval: "preserveStartEnd" }}
                yAxis={{ show: true, name: "GB" }}
                tooltip={{ show: false }}
                lines={[
                  {
                    dataKey: "memoryUsage",
                    name: "Usage",
                    stroke: theme.palette.primary.main,
                    dot: false,
                    connectNulls: true,
                    unit: " GiB",
                  },
                  {
                    dataKey: "memoryRequests",
                    name: "Requests",
                    stroke: theme.palette.secondary.main,
                    dot: false,
                    connectNulls: true,
                    unit: " GiB",
                  },
                  {
                    dataKey: "memoryLimits",
                    name: "Limits",
                    stroke: theme.palette.error.main,
                    dot: false,
                    connectNulls: true,
                    strokeDasharray: "0",
                    unit: " GiB",
                  },
                ]}
              >
                <ChartTooltip
                  content={
                    <MetricsTooltip
                      title="Memory"
                      formatter={(value) => `${value.toFixed(2)} GiB`}
                    />
                  }
                />
              </LineChart>
            )}
          </CardContent>
        </Card>
      </Grid>
    </Grid>
  );
};

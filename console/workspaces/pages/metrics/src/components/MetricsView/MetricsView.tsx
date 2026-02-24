/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
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

import React, { useMemo } from "react";
import { format } from "date-fns";
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

const toGb = (value: number) => value / 1024 ** 3;

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
        label: format(new Date(point.time), "MM/dd HH:mm"),
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

  const hasCpuData = useMemo(
    () =>
      (metrics?.cpuUsage?.length ?? 0) > 0 ||
      (metrics?.cpuRequests?.length ?? 0) > 0 ||
      (metrics?.cpuLimits?.length ?? 0) > 0,
    [metrics],
  );

  const hasMemoryData = useMemo(
    () =>
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
            ) : !hasCpuData ? (
              <Box
                display="flex"
                justifyContent="center"
                alignItems="center"
                height={260}
              >
                <NoDataFound
                  message="No CPU metrics available"
                  subtitle="Try changing the time range"
                  disableBackground
                />
              </Box>
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
                syncId="metricsSync"
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
          <CardHeader title="Memory Usage" />
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
            ) : !hasMemoryData ? (
              <Box
                display="flex"
                justifyContent="center"
                alignItems="center"
                height={260}
              >
                <NoDataFound
                  message="No memory metrics available"
                  subtitle="Try changing the time range"
                  disableBackground
                />
              </Box>
            ) : (
              <LineChart
                data={buildSeriesData([
                  {
                    key: "memoryUsage",
                    points: metrics?.memory,
                    transform: toGb,
                  },
                  {
                    key: "memoryRequests",
                    points: metrics?.memoryRequests,
                    transform: toGb,
                  },
                  {
                    key: "memoryLimits",
                    points: metrics?.memoryLimits,
                    transform: toGb,
                  },
                ])}
                xAxisDataKey="label"
                xAxis={{ show: true, interval: "preserveStartEnd" }}
                yAxis={{ show: true, name: "GiB" }}
                tooltip={{ show: false }}
                syncId="metricsSync"
                lines={[
                  {
                    dataKey: "memoryUsage",
                    name: "Usage",
                    stroke: theme.palette.primary.main,
                    dot: false,
                    connectNulls: true,
                    unit: " GB",
                  },
                  {
                    dataKey: "memoryRequests",
                    name: "Requests",
                    stroke: theme.palette.secondary.main,
                    dot: false,
                    connectNulls: true,
                    unit: " GB",
                  },
                  {
                    dataKey: "memoryLimits",
                    name: "Limits",
                    stroke: theme.palette.error.main,
                    dot: false,
                    connectNulls: true,
                    strokeDasharray: "0",
                    unit: " GB",
                  },
                ]}
              >
                <ChartTooltip
                  content={
                    <MetricsTooltip
                      title="Memory"
                      formatter={(value) => `${value.toFixed(2)} GB`}
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

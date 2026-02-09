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
import dayjs from "dayjs";
import { NoDataFound } from "@agent-management-platform/views";
import {
  Alert,
  Box,
  Card,
  CardContent,
  CardHeader,
  CircularProgress,
  Grid,
  IconButton,
  InputAdornment,
  MenuItem,
  Select,
  Skeleton,
  Stack,
  Typography,
  useTheme,
} from "@wso2/oxygen-ui";
import { Clock, RefreshCcw } from "@wso2/oxygen-ui-icons-react";
import { LineChart, ChartTooltip } from "@wso2/oxygen-ui-charts-react";
import type {
  MetricDataPoint,
  MetricsResponse,
} from "@agent-management-platform/types";

export interface TimeRangeOption {
  value: string;
  label: string;
}

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

  // Time and refresh controls
  timeRange?: string;
  timeRangeOptions?: TimeRangeOption[];
  onTimeRangeChange?: (timeRange: string) => void;
  onRefresh?: () => void;
  isRefreshing?: boolean;
}

export const MetricsView: React.FC<MetricsViewProps> = ({
  metrics,
  isLoading,
  error,
  timeRange,
  timeRangeOptions = [],
  onTimeRangeChange,
  onRefresh,
  isRefreshing = false,
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
    <Stack direction="column" gap={3}>
      {/* Filters and Controls */}
      {(timeRangeOptions.length > 0 || onRefresh) && (
        <Card variant="outlined">
          <CardContent>
            <Stack direction="row" spacing={2} alignItems="center" flexWrap="wrap">
              <Box sx={{ flexGrow: 1 }} />

              {/* Time Range Selector */}
              {timeRangeOptions.length > 0 && onTimeRangeChange && (
                <Select
                  size="small"
                  variant="outlined"
                  value={timeRange}
                  onChange={(e) => onTimeRangeChange(e.target.value)}
                  startAdornment={
                    <InputAdornment position="start">
                      <Clock size={16} />
                    </InputAdornment>
                  }
                  sx={{ minWidth: 150 }}
                >
                  {timeRangeOptions.map((opt) => (
                    <MenuItem key={opt.value} value={opt.value}>
                      {opt.label}
                    </MenuItem>
                  ))}
                </Select>
              )}

              {/* Refresh Button */}
              {onRefresh && (
                <IconButton
                  size="small"
                  disabled={isRefreshing}
                  onClick={onRefresh}
                  aria-label="Refresh"
                >
                  {isRefreshing ? (
                    <CircularProgress size={16} />
                  ) : (
                    <RefreshCcw size={16} />
                  )}
                </IconButton>
              )}
            </Stack>
          </CardContent>
        </Card>
      )}

      {/* Charts Grid */}
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
                yAxis={{ show: true, name: "GiB" }}
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
    </Stack>
  );
};

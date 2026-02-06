/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

import React, { useCallback, useMemo } from "react";
import { FadeIn, PageLayout } from "@agent-management-platform/views";
import { useParams, useSearchParams } from "react-router-dom";
import {
  TraceListTimeRange,
  getTimeRange,
} from "@agent-management-platform/types";
import {
  CircularProgress,
  IconButton,
  InputAdornment,
  MenuItem,
  Select,
  Stack,
} from "@mui/material";
import { Clock, RefreshCcw } from "@wso2/oxygen-ui-icons-react";
import { useGetAgentMetrics } from "@agent-management-platform/api-client";
import { MetricsView } from "./components/MetricsView/MetricsView";

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
  { value: TraceListTimeRange.THIRTY_DAYS, label: "30 Days" },
];

export const MetricsComponent: React.FC = () => {
  const { agentId, orgId, projectId, envId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();

  const timeRange = useMemo(
    () =>
      (searchParams.get("timeRange") as TraceListTimeRange) ||
      TraceListTimeRange.SEVEN_DAYS,
    [searchParams]
  );

  const timeRangeWindow = useMemo(() => getTimeRange(timeRange), [timeRange]);

  const metricsFilterRequest = useMemo(
    () => ({
      environmentName: envId ?? "",
      startTime: timeRangeWindow?.startTime ?? "",
      endTime: timeRangeWindow?.endTime ?? "",
    }),
    [envId, timeRangeWindow]
  );

  const {
    data: metrics,
    error,
    isLoading,
    isRefetching,
    refetch,
  } = useGetAgentMetrics(
    { agentName: agentId, orgName: orgId, projName: projectId },
    metricsFilterRequest,
    {
      enabled:
        !!agentId &&
        !!orgId &&
        !!projectId &&
        !!envId &&
        !!timeRangeWindow,
    }
  );

  const handleRefresh = useCallback(() => {
    refetch();
  }, [refetch]);

  return (
    <FadeIn>
      <PageLayout
        title="Metrics"
        actions={
          <Stack direction="row" gap={1} alignItems="center">
            <Select
              size="small"
              variant="outlined"
              value={timeRange}
              startAdornment={
                <InputAdornment position="start">
                  <Clock size={16} />
                </InputAdornment>
              }
              onChange={(e) => {
                const next = new URLSearchParams(searchParams);
                next.set("timeRange", e.target.value as TraceListTimeRange);
                setSearchParams(next);
              }}
            >
              {TIME_RANGE_OPTIONS.map((opt) => (
                <MenuItem key={opt.value} value={opt.value}>
                  {opt.label}
                </MenuItem>
              ))}
            </Select>
            <IconButton
              size="small"
              disabled={isRefetching}
              onClick={handleRefresh}
              aria-label="Refresh"
            >
              {isRefetching ? (
                <CircularProgress size={16} />
              ) : (
                <RefreshCcw size={16} />
              )}
            </IconButton>
          </Stack>
        }
        disableIcon
      >
        <MetricsView metrics={metrics} isLoading={isLoading} error={error} />
      </PageLayout>
    </FadeIn>
  );
};

export default MetricsComponent;

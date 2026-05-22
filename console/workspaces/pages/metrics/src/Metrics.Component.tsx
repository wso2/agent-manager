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
import { PageLayout, TimeRangeSelector, useTimeRangeParams } from "@agent-management-platform/views";
import { useParams, useSearchParams } from "react-router-dom";
import {
  TraceListTimeRange,
  getTimeRange,
} from "@agent-management-platform/types";
import { useGetAgentMetrics } from "@agent-management-platform/api-client";
import { MetricsView } from "./components/MetricsView/MetricsView";
import {
  CircularProgress,
  IconButton,
  Stack,
} from "@wso2/oxygen-ui";
import {
  RefreshCcw,
} from "@wso2/oxygen-ui-icons-react";

const TIME_RANGE_OPTIONS = [
  { value: TraceListTimeRange.TEN_MINUTES, label: "10 Minutes" },
  { value: TraceListTimeRange.THIRTY_MINUTES, label: "30 Minutes" },
  { value: TraceListTimeRange.ONE_HOUR, label: "1 Hour" },
  { value: TraceListTimeRange.SIX_HOURS, label: "6 Hours" },
  { value: TraceListTimeRange.TWELVE_HOURS, label: "12 Hours" },
  { value: TraceListTimeRange.ONE_DAY, label: "1 Day" },
  { value: TraceListTimeRange.SEVEN_DAYS, label: "7 Days" },
];

export const MetricsComponent: React.FC = () => {
  const { agentId, orgId, projectId, envId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();

  const {
    customStartTime,
    customEndTime,
    hasCustomRange,
    handleCustomRangeApply,
  } = useTimeRangeParams(searchParams, setSearchParams);

  const timeRange = useMemo(
    () =>
      hasCustomRange
        ? undefined
        : (Object.values(TraceListTimeRange) as string[]).includes(
            searchParams.get("timeRange") ?? "",
          )
          ? (searchParams.get("timeRange") as TraceListTimeRange)
          : TraceListTimeRange.ONE_HOUR,
    [searchParams, hasCustomRange],
  );

  const timeRangeWindow = useMemo(
    () => (timeRange ? getTimeRange(timeRange) : undefined),
    [timeRange],
  );

  const metricsFilterRequest = useMemo(
    () => ({
      environmentName: envId ?? "",
      startTime: hasCustomRange ? customStartTime! : (timeRangeWindow?.startTime ?? ""),
      endTime: hasCustomRange ? customEndTime! : (timeRangeWindow?.endTime ?? ""),
    }),
    [envId, hasCustomRange, customStartTime, customEndTime, timeRangeWindow],
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
        (hasCustomRange || !!timeRangeWindow),
    }
  );

  const handleRefresh = useCallback(() => {
    refetch();
  }, [refetch]);

  const handleTimeRangeChange = useCallback(
    (newTimeRange: string) => {
      const next = new URLSearchParams(searchParams);
      next.set("timeRange", newTimeRange as TraceListTimeRange);
      next.delete("startTime");
      next.delete("endTime");
      setSearchParams(next);
    },
    [searchParams, setSearchParams],
  );

  return (
      <PageLayout
        title="Metrics"
        disableIcon
        actions={
          <Stack direction="row" spacing={2} alignItems="center" flexWrap="wrap">
            <TimeRangeSelector
              preset={timeRange}
              customStart={customStartTime}
              customEnd={customEndTime}
              options={TIME_RANGE_OPTIONS}
              onPresetChange={handleTimeRangeChange}
              onCustomRangeApply={handleCustomRangeApply}
            />

            {/* Refresh Button */}
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
      >
        <MetricsView
          metrics={metrics}
          isLoading={isLoading}
          error={error}
        />
      </PageLayout>

  );
};

export default MetricsComponent;

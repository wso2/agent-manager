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
import { EnvironmentSelector } from "@agent-management-platform/shared-component";
import { NoDataFound, PageLayout, TimeRangeSelector, useTimeRangeParams } from "@agent-management-platform/views";
import { useParams, useSearchParams } from "react-router-dom";
import { TraceListTimeRange } from "@agent-management-platform/types";
import { useGetAgentMetrics, useListAgentDeployments, isObserverConfigured } from "@agent-management-platform/api-client";
import { MetricsView } from "./components/MetricsView/MetricsView";
import {
  Alert,
  CircularProgress,
  IconButton,
  Stack,
} from "@wso2/oxygen-ui";
import { PauseCircle, RefreshCcw } from "@wso2/oxygen-ui-icons-react";

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

  const { data: deployments } = useListAgentDeployments(
    { orgName: orgId, projName: projectId, agentName: agentId },
  );
  const isSuspended = deployments === undefined ? undefined : deployments[envId ?? ""]?.status === "suspended";

  const metricsFilterRequest = useMemo(
    () => ({
      environmentName: envId ?? "",
      ...(hasCustomRange ? { startTime: customStartTime!, endTime: customEndTime! } : {}),
    }),
    [envId, hasCustomRange, customStartTime, customEndTime],
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
      enabled: isSuspended === false && !!agentId && !!orgId &&
        !!projectId && !!envId && (hasCustomRange || !!timeRange),
      enableAutoRefresh: !hasCustomRange,
      timeRange: hasCustomRange ? undefined : timeRange,
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

  // Mirror Traces.Component: without a configured observer the request fails
  // as an opaque "Failed to fetch" snackbar, so surface an actionable empty
  // state instead.
  if (!isObserverConfigured()) {
    return (
      <PageLayout docs={["observability", "observeFirstAgent", "ampInstrumentation"]} title="System Metrics" disableIcon>
        <Alert severity="error" sx={{ mt: 2 }}>
          <strong>Observer not configured.</strong> Ask your platform
          administrator to set <code>AM_OBSERVER_PUBLIC_URL</code> on the
          agent-manager service.
        </Alert>
      </PageLayout>
    );
  }

  return (
    <PageLayout docs={["observability", "observeFirstAgent", "ampInstrumentation"]}
      title="System Metrics"
      disableIcon
      actions={
        <Stack direction="row" spacing={2} alignItems="center" flexWrap="wrap">
          <EnvironmentSelector />
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
      {isSuspended ? (
        <NoDataFound
          iconElement={PauseCircle}
          message="Environment Suspended"
          subtitle="Metrics are unavailable while the environment is suspended."
        />
      ) : (
        <MetricsView
          metrics={metrics}
          isLoading={isLoading}
          error={error}
        />
      )}
    </PageLayout>

  );
};

export default MetricsComponent;

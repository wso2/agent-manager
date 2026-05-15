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

import React, { useCallback, useMemo, useState } from "react";
import {
  DrawerContent,
  DrawerHeader,
  DrawerWrapper,
  PageLayout,
} from "@agent-management-platform/views";
import { useParams, useSearchParams } from "react-router-dom";
import {
  GetTraceListPathParams,
  TraceListTimeRange,
  getTimeRange,
  globalConfig,
} from "@agent-management-platform/types";
import {
  Workflow,
  Clock,
  RefreshCcw,
  SortAsc,
  SortDesc,
  Download,
} from "@wso2/oxygen-ui-icons-react";
import {
  useTraceList,
  useExportTraces,
  useGetAgent,
  useGetOrganization,
  useListEnvironments,
  type TraceListWithRange,
} from "@agent-management-platform/api-client";
import { TraceDetails, TracesView } from "./subComponents";
import {
  Alert,
  Button,
  CircularProgress,
  IconButton,
  InputAdornment,
  MenuItem,
  Select,
  Snackbar,
  Stack,
  Typography,
} from "@wso2/oxygen-ui";

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

export const TracesComponent: React.FC = () => {
  const { agentId, orgId, projectId, envId } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const { mutateAsync: exportTracesAsync, isPending: isExporting } =
    useExportTraces();

  const {
    data: orgData,
    isPending: isOrgPending,
    isSuccess: isOrgSuccess,
  } = useGetOrganization({ orgName: orgId ?? "" });
  const organization = orgData?.namespace;

  const {
    data: agentData,
    isPending: isAgentPending,
    isSuccess: isAgentSuccess,
  } = useGetAgent({
    orgName: orgId ?? "",
    projName: projectId ?? "",
    agentName: agentId ?? "",
  });
  const {
    data: environmentsData,
    isPending: isEnvPending,
    isSuccess: isEnvSuccess,
  } = useListEnvironments({
    orgName: orgId ?? "",
  });
  const matchedEnvironment = environmentsData?.find((e) => e.name === envId);
  const environmentName = matchedEnvironment?.name;
  const prereqsPending = isOrgPending || isAgentPending || isEnvPending;

  // Detect resolution mismatches only after queries have settled, so transient
  // "no data yet" states during loading aren't incorrectly shown as errors.
  const orgNotFound = isOrgSuccess && !organization;
  const agentNotFound = isAgentSuccess && !agentData?.uuid;
  const envNotFound =
    isEnvSuccess && environmentsData !== undefined && !environmentName;
  const [exportError, setExportError] = useState<string | null>(null);
  const [drawerFullscreen, setDrawerFullscreen] = useState(false);

  // Initialize state from URL search params with defaults.
  // Validate that both timestamps are parseable and start <= end before
  // activating custom-range mode, so malformed or inverted URLs are ignored.
  const [customStartTime, customEndTime, hasCustomRange] = useMemo((): [
    string | undefined,
    string | undefined,
    boolean,
  ] => {
    const startRaw = searchParams.get("startTime") || undefined;
    const endRaw = searchParams.get("endTime") || undefined;
    if (!startRaw || !endRaw) return [undefined, undefined, false];
    const startMs = Date.parse(startRaw);
    const endMs = Date.parse(endRaw);
    if (isNaN(startMs) || isNaN(endMs) || startMs > endMs)
      return [undefined, undefined, false];
    return [startRaw, endRaw, true];
  }, [searchParams]);

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

  const limit = useMemo(() => {
    const parsed = parseInt(searchParams.get("limit") || "10", 10);
    return Number.isFinite(parsed) && parsed > 0 ? parsed : 10;
  }, [searchParams]);

  const sortOrder = useMemo(() => {
    const raw = searchParams.get("sortOrder");
    return (raw === "asc" || raw === "desc") ? raw : "desc" as GetTraceListPathParams["sortOrder"];
  }, [searchParams]);
  const {
    data: traceData,
    isLoading,
    refetch,
    isRefetching,
    loadOlder,
    loadNewer,
    isLoadingOlder,
    isLoadingNewer,
  } = useTraceList(
    organization,
    projectId,
    agentId,
    environmentName,
    timeRange,
    limit,
    sortOrder,
    customStartTime,
    customEndTime,
  );

  // Resolved time range used by the TraceDetails drawer.
  // Prefer the concrete window captured during the last fetch (embedded in
  // traceData) so the drawer queries spans over the same bounds that produced
  // the selected trace, rather than recomputing from a relative preset.
  const resolvedTimeRange = useMemo(
    () => {
      const fetchedRange = (traceData as TraceListWithRange | undefined)?.fetchedRange;
      if (fetchedRange) return fetchedRange;
      if (hasCustomRange) return { startTime: customStartTime!, endTime: customEndTime! };
      return timeRange ? getTimeRange(timeRange) : undefined;
    },
    [traceData, hasCustomRange, customStartTime, customEndTime, timeRange],
  );

  const selectedTrace = useMemo(
    () => searchParams.get("selectedTrace"),
    [searchParams],
  );

  const handleTraceSelect = useCallback(
    (traceId: string) => {
      const next = new URLSearchParams(searchParams);
      next.set("selectedTrace", traceId);
      setSearchParams(next);
    },
    [searchParams, setSearchParams],
  );

  const handleCloseDrawer = useCallback(() => {
    const next = new URLSearchParams(searchParams);
    next.delete("selectedTrace");
    setSearchParams(next);
    setDrawerFullscreen(false);
  }, [searchParams, setSearchParams]);

  const handleExportTraces = useCallback(async () => {
    if (!organization || !projectId || !agentId || !environmentName) {
      setExportError("Missing required parameters for export");
      return;
    }

    try {
      setExportError(null);

      const range = hasCustomRange
        ? { startTime: customStartTime!, endTime: customEndTime! }
        : timeRange
          ? getTimeRange(timeRange)
          : null;
      if (!range) {
        setExportError("Invalid time range");
        return;
      }
      const { startTime, endTime } = range;

      const exportData = await exportTracesAsync({
        organization,
        project: projectId,
        component: agentId,
        environment: environmentName,
        startTime,
        endTime,
        sortOrder,
        limit,
      });

      // Create a blob from the JSON data
      const blob = new Blob([JSON.stringify(exportData, null, 2)], {
        type: "application/json",
      });

      // Create download link
      const url = window.URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = `traces-export-${new Date().toISOString().replace(/[:.]/g, "-")}.json`;
      document.body.appendChild(link);
      link.click();

      // Cleanup
      document.body.removeChild(link);
      window.URL.revokeObjectURL(url);
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error("Export failed:", error);
      setExportError(
        error instanceof Error ? error.message : "Failed to export traces",
      );
    }
  }, [
    organization,
    projectId,
    agentId,
    environmentName,
    timeRange,
    sortOrder,
    limit,
    exportTracesAsync,
    hasCustomRange,
    customStartTime,
    customEndTime,
  ]);

  const handleTimeRangeChange = useCallback(
    (newTimeRange: string) => {
      const next = new URLSearchParams(searchParams);
      next.set("timeRange", newTimeRange as TraceListTimeRange);
      // Clear custom range when switching to a preset
      next.delete("startTime");
      next.delete("endTime");
      setSearchParams(next);
    },
    [searchParams, setSearchParams],
  );

  const customRangeLabel = useMemo(() => {
    if (!hasCustomRange) return null;
    const fmt = (iso: string) =>
      new Date(iso).toLocaleString(undefined, {
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      });
    return `${fmt(customStartTime!)} – ${fmt(customEndTime!)}`;
  }, [hasCustomRange, customStartTime, customEndTime]);

  const handleSortOrderChange = useCallback(
    (newSortOrder: "asc" | "desc") => {
      const next = new URLSearchParams(searchParams);
      next.set("sortOrder", newSortOrder);
      setSearchParams(next);
    },
    [searchParams, setSearchParams],
  );

  const handleRefresh = useCallback(() => {
    refetch();
  }, [refetch]);

  const obsUrlMissing =
    !globalConfig.obsApiBaseUrl?.trim() ||
    globalConfig.obsApiBaseUrl.trim() === "$OBS_API_BASE_URL";

  if (obsUrlMissing) {
    return (
      <PageLayout title="Traces" disableIcon>
        <Alert severity="error" sx={{ mt: 2 }}>
          <strong>Traces service not configured.</strong> Set{" "}
          <code>OBS_API_BASE_URL</code> to the traces-observer-service URL. The
          agent-manager no longer serves trace routes.
        </Alert>
      </PageLayout>
    );
  }

  if (orgNotFound || agentNotFound || envNotFound) {
    return (
      <PageLayout title="Traces" disableIcon>
        {orgNotFound && (
          <Alert severity="error" sx={{ mt: 2 }}>
            <strong>Organization not found.</strong> No organization named{" "}
            <code>{orgId}</code> exists, or it has no organization identifier.
          </Alert>
        )}
        {agentNotFound && (
          <Alert severity="error" sx={{ mt: 2 }}>
            <strong>Agent not found.</strong> No agent named{" "}
            <code>{agentId}</code> exists in this project.
          </Alert>
        )}
        {envNotFound && (
          <Alert severity="error" sx={{ mt: 2 }}>
            <strong>Environment not found.</strong> <code>{envId}</code> does
            not match any environment in this organisation. Check the URL or
            verify the environment name.
          </Alert>
        )}
      </PageLayout>
    );
  }

  return (
    <>
      <PageLayout
        title="Traces"
        disableIcon
        actions={
          <Stack
            direction="row"
            spacing={2}
            alignItems="center"
            flexWrap="wrap"
          >
            {/* Time Range Selector */}
            {hasCustomRange ? (
              <Stack direction="row" spacing={0.5} alignItems="center">
                <Clock size={16} />
                <Typography variant="caption" color="text.secondary" noWrap>
                  {customRangeLabel}
                </Typography>
              </Stack>
            ) : (
              <Select
                size="small"
                variant="outlined"
                value={timeRange}
                onChange={(e) => handleTimeRangeChange(e.target.value)}
                startAdornment={
                  <InputAdornment position="start">
                    <Clock size={16} />
                  </InputAdornment>
                }
                sx={{ minWidth: 150 }}
              >
                {TIME_RANGE_OPTIONS.map((opt) => (
                  <MenuItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </MenuItem>
                ))}
              </Select>
            )}

            {/* Sort Toggle */}
            <IconButton
              size="small"
              onClick={() =>
                handleSortOrderChange(sortOrder === "desc" ? "asc" : "desc")
              }
              aria-label={
                sortOrder === "desc" ? "Sort ascending" : "Sort descending"
              }
            >
              {sortOrder === "desc" ? (
                <SortDesc size={16} />
              ) : (
                <SortAsc size={16} />
              )}
            </IconButton>

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

            {/* Export Button */}
            <Button
              size="small"
              variant="outlined"
              startIcon={
                isExporting ? (
                  <CircularProgress size={16} />
                ) : (
                  <Download size={16} />
                )
              }
              onClick={handleExportTraces}
              disabled={
                isExporting ||
                prereqsPending ||
                isLoading ||
                (traceData?.traces ?? []).length === 0
              }
            >
              Export
            </Button>
          </Stack>
        }
      >
        <TracesView
          traces={traceData?.traces ?? []}
          isLoading={prereqsPending || isLoading}
          selectedTrace={selectedTrace}
          sortOrder={sortOrder}
          isLoadingOlder={isLoadingOlder}
          isLoadingNewer={isLoadingNewer}
          onTraceSelect={handleTraceSelect}
          onLoadOlder={loadOlder}
          onLoadNewer={loadNewer}
        />
        <DrawerWrapper
          open={!!selectedTrace}
          disableScroll
          onClose={handleCloseDrawer}
          minWidth={"80vw"}
          fullscreen={drawerFullscreen}
        >
          <DrawerHeader
            title="Trace Details"
            icon={<Workflow size={24} />}
            onClose={handleCloseDrawer}
            isFullscreen={drawerFullscreen}
            onToggleFullscreen={() => setDrawerFullscreen(v => !v)}
          />
          <DrawerContent>
            {selectedTrace &&
            resolvedTimeRange &&
            organization &&
            projectId &&
            agentId &&
            environmentName ? (
              <TraceDetails
                traceId={selectedTrace}
                organization={organization}
                project={projectId}
                component={agentId}
                environment={environmentName!}
                startTime={resolvedTimeRange.startTime}
                endTime={resolvedTimeRange.endTime}
              />
            ) : null}
          </DrawerContent>
        </DrawerWrapper>
      </PageLayout>
      <Snackbar
        open={!!exportError}
        autoHideDuration={6000}
        onClose={() => setExportError(null)}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert onClose={() => setExportError(null)} severity="error">
          {exportError}
        </Alert>
      </Snackbar>
    </>
  );
};

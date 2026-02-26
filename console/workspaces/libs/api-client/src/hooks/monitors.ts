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

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuthHooks } from "@agent-management-platform/auth";
import {
  type CreateMonitorPathParams,
  type CreateMonitorRequest,
  type DeleteMonitorPathParams,
  type GetMonitorPathParams,
  type ListMonitorRunsPathParams,
  type ListMonitorsPathParams,
  type LogsResponse,
  type MonitorListResponse,
  type MonitorResponse,
  type MonitorRunListResponse,
  type MonitorRunLogsPathParams,
  type MonitorRunResponse,
  type MonitorRunScoresResponse,
  type MonitorRunPathParams,
  type MonitorScoresPathParams,
  type MonitorScoresQueryParams,
  type MonitorScoresResponse,
  type MonitorScoresTimeSeriesPathParams,
  type MonitorScoresTimeSeriesQueryParams,
  type RerunMonitorPathParams,
  type StartMonitorPathParams,
  type StopMonitorPathParams,
  type TimeSeriesResponse,
  type TraceScoresPathParams,
  type TraceScoresResponse,
  type UpdateMonitorPathParams,
  type UpdateMonitorRequest,
} from "@agent-management-platform/types";
import {
  createMonitor,
  deleteMonitor,
  getMonitor,
  getMonitorRunLogs,
  getMonitorRunScores,
  getMonitorScores,
  getMonitorScoresTimeSeries,
  getTraceScores,
  listMonitorRuns,
  listMonitors,
  rerunMonitor,
  startMonitor,
  stopMonitor,
  updateMonitor,
} from "../apis";

export function useListMonitors(params: ListMonitorsPathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<MonitorListResponse>({
    queryKey: ["monitors", params],
    queryFn: () => listMonitors(params, getToken),
    enabled: !!params.orgName && !!params.projName && !!params.agentName,
  });
}

export function useGetMonitor(params: GetMonitorPathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<MonitorResponse>({
    queryKey: ["monitor", params],
    queryFn: () => getMonitor(params, getToken),
    enabled:
      !!params.orgName &&
      !!params.projName &&
      !!params.agentName &&
      !!params.monitorName,
  });
}

export function useCreateMonitor(params: CreateMonitorPathParams) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<MonitorResponse, unknown, CreateMonitorRequest>({
    mutationFn: (body) => createMonitor(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["monitors"] });
    },
  });
}

export function useUpdateMonitor(params: UpdateMonitorPathParams) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<MonitorResponse, unknown, UpdateMonitorRequest>({
    mutationFn: (body) => updateMonitor(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["monitors"] });
      queryClient.invalidateQueries({ queryKey: ["monitor"] });
    },
  });
}

export function useDeleteMonitor() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<void, unknown, DeleteMonitorPathParams>({
    mutationFn: (mutationParams) => deleteMonitor(mutationParams, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["monitors"] });
    },
  });
}

export function useStopMonitor() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<MonitorResponse, unknown, StopMonitorPathParams>({
    mutationFn: async (mutationParams) => {
      const response = await stopMonitor(mutationParams, getToken);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["monitors"] }),
        queryClient.invalidateQueries({ queryKey: ["monitor"] }),
      ]);
      return response;
    },
  });
}

export function useStartMonitor() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<MonitorResponse, unknown, StartMonitorPathParams>({
    mutationFn: async (mutationParams) => {
      const response = await startMonitor(mutationParams, getToken);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["monitors"] }),
        queryClient.invalidateQueries({ queryKey: ["monitor"] }),
      ]);
      return response;
    },
  });
}

export function useListMonitorRuns(params: ListMonitorRunsPathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<MonitorRunListResponse>({
    queryKey: ["monitor-runs", params],
    queryFn: () => listMonitorRuns(params, getToken),
    enabled:
      !!params.orgName &&
      !!params.projName &&
      !!params.agentName &&
      !!params.monitorName,
  });
}

export function useRerunMonitor() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<MonitorRunResponse, unknown, RerunMonitorPathParams>({
    mutationFn: (params) => rerunMonitor(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["monitor-runs"] });
    },
  });
}

export function useMonitorRunLogs(params: MonitorRunLogsPathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<LogsResponse>({
    queryKey: ["monitor-run-logs", params],
    queryFn: () => getMonitorRunLogs(params, getToken),
    enabled:
      !!params.orgName &&
      !!params.projName &&
      !!params.agentName &&
      !!params.monitorName &&
      !!params.runId,
  });
}

export function useMonitorRunScores(params: MonitorRunPathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<MonitorRunScoresResponse>({
    queryKey: ["monitor-run-scores", params],
    queryFn: () => getMonitorRunScores(params, getToken),
    enabled:
      !!params.orgName &&
      !!params.projName &&
      !!params.agentName &&
      !!params.monitorName &&
      !!params.runId,
  });
}

export function useMonitorScores(
  params: MonitorScoresPathParams,
  query: MonitorScoresQueryParams
) {
  const { getToken } = useAuthHooks();
  return useQuery<MonitorScoresResponse>({
    queryKey: ["monitor-scores", params, query],
    queryFn: () => getMonitorScores(params, query, getToken),
    enabled:
      !!params.orgName &&
      !!params.projName &&
      !!params.agentName &&
      !!params.monitorName &&
      !!query.startTime &&
      !!query.endTime,
  });
}

export function useMonitorScoresTimeSeries(
  params: MonitorScoresTimeSeriesPathParams,
  query: MonitorScoresTimeSeriesQueryParams
) {
  const { getToken } = useAuthHooks();
  return useQuery<TimeSeriesResponse>({
    queryKey: ["monitor-scores-timeseries", params, query],
    queryFn: () => getMonitorScoresTimeSeries(params, query, getToken),
    enabled:
      !!params.orgName &&
      !!params.projName &&
      !!params.agentName &&
      !!params.monitorName &&
      !!query.startTime &&
      !!query.endTime &&
      !!query.evaluator,
  });
}

export function useTraceScores(params: TraceScoresPathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<TraceScoresResponse>({
    queryKey: ["trace-scores", params],
    queryFn: () => getTraceScores(params, getToken),
    enabled:
      !!params.orgName &&
      !!params.projName &&
      !!params.agentName &&
      !!params.traceId,
  });
}

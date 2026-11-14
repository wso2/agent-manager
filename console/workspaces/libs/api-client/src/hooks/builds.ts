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

import { useQueryClient } from "@tanstack/react-query";
import { buildAgent, getAgentBuilds, getAllAgentBuilds, getBuild, getBuildLogs } from "../apis";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useRef } from "react";
import type {
  BuildAgentPathParams,
  BuildAgentQuery,
  BuildLogEntry,
  BuildResponse,
  BuildsListResponse,
  GetAgentBuildsPathParams,
  GetAgentBuildsQuery,
  GetBuildLogsPathParams,
  GetBuildPathParams,
  BuildDetailsResponse,
} from "@agent-management-platform/types";
import { POLL_INTERVAL } from "../utils";
import { useApiMutation, useApiQuery } from "./react-query-notifications";

export function useBuildAgent() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    BuildResponse,
    unknown,
    { params: BuildAgentPathParams; query?: BuildAgentQuery }
  >({
    action: { verb: 'start', target: 'build' },
    mutationFn: ({ params, query }) => buildAgent(params, query, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["agent-builds"] });
      queryClient.invalidateQueries({ queryKey: ["build"] });
    },
    onError: () => {
      queryClient.invalidateQueries({ queryKey: ["agent-builds"] });
      queryClient.invalidateQueries({ queryKey: ["build"] });
    },
  });
}

export function useGetAgentBuilds(
  params: GetAgentBuildsPathParams,
  query?: GetAgentBuildsQuery,
  options?: { enabled?: boolean }
) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  const prevHasInProgressBuildRef = useRef<boolean>(false);

  return useApiQuery<BuildsListResponse>({
    queryKey: ["agent-builds", params, query],
    queryFn: () => getAgentBuilds(params, query, getToken),
    enabled:
      (options?.enabled ?? true) &&
      !!params.orgName &&
      !!params.projName &&
      !!params.agentName,
    refetchInterval: (queryState) => {
      // Check if any build is in progress
      const hasInProgressBuild =
        queryState?.state?.data?.builds?.some(
          (build: BuildDetailsResponse) =>
            build.status === "Pending" || build.status === "Running"
        ) ?? false;

      // Only invalidate when transitioning from true to false (build completed)
      if (prevHasInProgressBuildRef.current && !hasInProgressBuild) {
        queryClient.invalidateQueries({ queryKey: ["agent-deployments"] });
        queryClient.invalidateQueries({ queryKey: ["agent-configurations"] });
      }

      // Update the ref with current value
      prevHasInProgressBuildRef.current = hasInProgressBuild;

      return hasInProgressBuild ? POLL_INTERVAL : false;
    },
  });
}

// Number of most-recent builds polled to detect status changes. Status only ever
// changes on recent builds, so this bounded page is enough to know when the full
// history is worth refetching.
const RECENT_BUILDS_POLL_LIMIT = 20;

// Build history needs every build, not just the newest page the service returns
// by default. Use this instead of useGetAgentBuilds wherever the UI paginates the
// list itself, otherwise its pager can only ever walk the first page.
//
// The full history is NOT polled: it costs one request per 100 builds, so polling
// it every POLL_INTERVAL would mean 100 requests every 5s for an agent with 10k
// builds. Instead a small recent page is polled while a build is in flight, and
// the full history is refetched only when that page actually changes.
export function useGetAllAgentBuilds(
  params: GetAgentBuildsPathParams,
  options?: { enabled?: boolean }
) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  const prevStatusSignatureRef = useRef<string | null>(null);

  const enabled =
    (options?.enabled ?? true) &&
    !!params.orgName &&
    !!params.projName &&
    !!params.agentName;

  const allBuildsQueryKey = ["agent-builds", "all", params];

  // Status watcher: one bounded request per poll, regardless of history size.
  useApiQuery<BuildsListResponse>({
    queryKey: ["agent-builds", "recent", params],
    queryFn: () =>
      getAgentBuilds(
        params,
        { limit: RECENT_BUILDS_POLL_LIMIT, offset: 0 },
        getToken
      ),
    enabled,
    refetchInterval: (queryState) => {
      const recent = queryState?.state?.data?.builds;
      if (!recent) return false;

      // Refetch the full history only when something actually changed, rather
      // than on every tick — a build name appearing or a status moving on.
      const signature = recent
        .map((build: BuildDetailsResponse) => `${build.buildName}:${build.status}`)
        .join(",");
      const hasInProgressBuild = recent.some(
        (build: BuildDetailsResponse) =>
          build.status === "Pending" || build.status === "Running"
      );

      if (
        prevStatusSignatureRef.current !== null &&
        prevStatusSignatureRef.current !== signature
      ) {
        queryClient.invalidateQueries({ queryKey: allBuildsQueryKey });

        // A build reaching a terminal state can change what is deployable and
        // which config edits are allowed.
        if (!hasInProgressBuild) {
          queryClient.invalidateQueries({ queryKey: ["agent-deployments"] });
          queryClient.invalidateQueries({ queryKey: ["agent-configurations"] });
        }
      }
      prevStatusSignatureRef.current = signature;

      return hasInProgressBuild ? POLL_INTERVAL : false;
    },
  });

  return useApiQuery<BuildsListResponse>({
    queryKey: allBuildsQueryKey,
    queryFn: () => getAllAgentBuilds(params, getToken),
    enabled,
  });
}

export function useGetBuild(params: GetBuildPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<BuildDetailsResponse>({
    queryKey: ["build", params],
    queryFn: () => getBuild(params, getToken),
    enabled: !!params.orgName && !!params.projName && !!params.agentName && !!params.buildName,
    refetchInterval: (queryState) => {
      // Check if build is in progress
      const isBuildRunning =
        queryState?.state?.data &&
        (queryState.state.data.status === "Pending" ||
          queryState.state.data.status === "Running");
      return isBuildRunning ? POLL_INTERVAL : false;
    },
  });
}

export function useGetBuildLogs(
  params: GetBuildLogsPathParams,
  buildStatus?: string
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<BuildLogEntry[]>({
    queryKey: ["build-logs", params],
    queryFn: () => getBuildLogs(params, getToken),
    enabled: !!params.orgName && !!params.projName && !!params.agentName && !!params.buildName,
    refetchInterval:
      buildStatus === "Pending" || buildStatus === "Running"
        ? POLL_INTERVAL
        : false,
  });
}

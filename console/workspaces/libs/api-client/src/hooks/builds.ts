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

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { buildAgent, getAgentBuilds, getBuild, getBuildLogs } from "../apis";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useRef } from "react";
import {
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

export function useBuildAgent() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<
    BuildResponse,
    unknown,
    { params: BuildAgentPathParams; query?: BuildAgentQuery }
  >({
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
  query?: GetAgentBuildsQuery
) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  const prevHasInProgressBuildRef = useRef<boolean>(false);

  return useQuery<BuildsListResponse>({
    queryKey: ["agent-builds", params, query],
    queryFn: () => getAgentBuilds(params, query, getToken),
    enabled: !!params.orgName && !!params.projName && !!params.agentName,
    refetchInterval: (queryState) => {
      // Check if any build is in progress
      const hasInProgressBuild =
        queryState?.state?.data?.builds?.some(
          (build: BuildDetailsResponse) =>
            build.status === "BuildTriggered" ||
            build.status === "BuildInProgress"
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

export function useGetBuild(params: GetBuildPathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<BuildDetailsResponse>({
    queryKey: ["build", params],
    queryFn: () => getBuild(params, getToken),
    enabled: !!params.orgName && !!params.projName && !!params.agentName && !!params.buildName,
    refetchInterval: (queryState) => {
      // Check if build is in progress
      const isBuildInProgress =
        queryState?.state?.data &&
        (queryState.state.data.status === "BuildTriggered" ||
          queryState.state.data.status === "BuildInProgress");
      return isBuildInProgress ? POLL_INTERVAL : false;
    },
  });
}

export function useGetBuildLogs(params: GetBuildLogsPathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<BuildLogEntry[]>({
    queryKey: ["build-logs", params],
    queryFn: () => getBuildLogs(params, getToken),
    enabled: !!params.orgName && !!params.projName && !!params.agentName && !!params.buildName,
  });
}

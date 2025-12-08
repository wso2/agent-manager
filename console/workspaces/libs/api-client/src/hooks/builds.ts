import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { buildAgent, getAgentBuilds, getBuild, getBuildLogs } from '../apis';
import { useAuthHooks } from '@agent-management-platform/auth';
import { useRef } from 'react';
import {
  BuildAgentPathParams,
  BuildAgentQuery,
  BuildLogsResponse,
  BuildResponse,
  BuildsListResponse,
  GetAgentBuildsPathParams,
  GetAgentBuildsQuery,
  GetBuildLogsPathParams,
  GetBuildPathParams,
  BuildDetailsResponse,
} from '@agent-management-platform/types';
import { POLL_INTERVAL } from '../utils';

export function useBuildAgent() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<BuildResponse, unknown, 
  { params: BuildAgentPathParams; query?: BuildAgentQuery }>({
    mutationFn: ({ params, query }) => buildAgent(params, query, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agent-builds'] });
      queryClient.invalidateQueries({ queryKey: ['build'] });
    },
    onError: () => {
      queryClient.invalidateQueries({ queryKey: ['agent-builds'] });
      queryClient.invalidateQueries({ queryKey: ['build'] });
    },
  });
}

export function useGetAgentBuilds(params: GetAgentBuildsPathParams, query?: GetAgentBuildsQuery) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  const prevHasInProgressBuildRef = useRef<boolean>(false);

  return useQuery<BuildsListResponse>({
    queryKey: ['agent-builds', params, query],
    queryFn: () => getAgentBuilds(params, query, getToken),
    refetchInterval: (queryState) => {
      // Check if any build is in progress
      const hasInProgressBuild = queryState?.state?.data?.builds?.some(
        (build: BuildDetailsResponse) => build.status === 'BuildTriggered' || build.status === 'BuildInProgress'
      ) ?? false;
      
      // Only invalidate when transitioning from true to false (build completed)
      if (prevHasInProgressBuildRef.current && !hasInProgressBuild) {
        queryClient.invalidateQueries({ queryKey: ['agent-deployments'] });
        queryClient.invalidateQueries({ queryKey: ['agent-configurations'] });
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
    queryKey: ['build', params],
    queryFn: () => getBuild(params, getToken),
    refetchInterval: (queryState) => {
      // Check if build is in progress
      const isBuildInProgress = queryState?.state?.data && (
        queryState.state.data.status === 'BuildTriggered' || queryState.state.data.status === 'BuildInProgress'
      );
      return isBuildInProgress ? POLL_INTERVAL : false;
    },
  });
}

export function useGetBuildLogs(params: GetBuildLogsPathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<BuildLogsResponse>({
    queryKey: ['build-logs', params],
    queryFn: () => getBuildLogs(params, getToken),
  });
}



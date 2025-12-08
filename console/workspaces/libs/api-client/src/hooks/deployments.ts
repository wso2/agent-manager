import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { 
  deployAgent, 
  listAgentDeployments, 
  getAgentEndpoints, 
  getAgentConfigurations,
  listEnvironments,
  getDeploymentPipeline,
} from '../apis';
import { useAuthHooks } from '@agent-management-platform/auth';
import {
  DeployAgentPathParams,
  DeployAgentRequest,
  DeploymentListResponse,
  DeploymentResponse,
  ListAgentDeploymentsPathParams,
  GetAgentEndpointsPathParams,
  EndpointsResponse,
  EnvironmentQuery,
  GetAgentConfigurationsPathParams,
  ConfigurationResponse,
  ListEnvironmentsPathParams,
  EnvironmentListResponse,
  GetDeploymentPipelinePathParams,
  DeploymentPipelineResponse,
  DeploymentDetailsResponse,
} from '@agent-management-platform/types';
import { POLL_INTERVAL } from '../utils';

export function useDeployAgent() {
  const queryClient = useQueryClient();
  const { getToken } = useAuthHooks();
  return useMutation<DeploymentResponse, unknown, 
  { params: DeployAgentPathParams; body: DeployAgentRequest }>({
    mutationFn: ({ params, body }) => deployAgent(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agent-configurations'] });
      queryClient.invalidateQueries({ queryKey: ['agent-deployments'] });
    },
  });
}

export function useListAgentDeployments(
  params: ListAgentDeploymentsPathParams, 
  options?: { enabled?: boolean }
) {
  const { getToken } = useAuthHooks();
  
  return useQuery<DeploymentListResponse>({
    queryKey: ['agent-deployments', params.orgName, params.projName, params.agentName],
    queryFn: () => listAgentDeployments(params, getToken),
    enabled: options?.enabled ?? true,
    refetchInterval: (queryState) => {
      // Check if any deployment is in progress
      const hasInProgressDeployment = 
        queryState?.state?.data && 
        Object.values(queryState.state.data).some(
          (deployment: DeploymentDetailsResponse) => 
            deployment.status === 'in-progress'
        );
      return hasInProgressDeployment ? POLL_INTERVAL : false;
    },
  });
}

export function useGetAgentEndpoints(params: GetAgentEndpointsPathParams, query: EnvironmentQuery) {
  const { getToken } = useAuthHooks();
  return useQuery<EndpointsResponse>({
    queryKey: ['agent-endpoints', params, query],
    queryFn: () => getAgentEndpoints(params, query, getToken),
  });
}

export function useGetAgentConfigurations
(params: GetAgentConfigurationsPathParams, query: EnvironmentQuery) {
  const { getToken } = useAuthHooks();
  return useQuery<ConfigurationResponse>({
    queryKey: ['agent-configurations', params, query],
    queryFn: () => getAgentConfigurations(params, query, getToken),
  });
}

export function useListEnvironments(params: ListEnvironmentsPathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<EnvironmentListResponse>({
    queryKey: ['environments', params],
    queryFn: () => listEnvironments(params, getToken),
  });
}

export function useGetDeploymentPipeline(params: GetDeploymentPipelinePathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<DeploymentPipelineResponse>({
    queryKey: ['deployment-pipeline', params],
    queryFn: () => getDeploymentPipeline(params, getToken),
  });
}



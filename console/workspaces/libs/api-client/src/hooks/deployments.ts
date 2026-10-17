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

import { useQueryClient } from '@tanstack/react-query';
import {
  deployAgent,
  updateAgentDeploySettings,
  updateAgentConfigurations,
  regenerateTracingToken,
  listAgentDeployments,
  getAgentEndpoints,
  getAgentConfigurations,
  listEnvironments,
  getEnvironment,
  getDeploymentPipeline,
  listDeploymentPipelines,
  listDataPlanes,
  checkThunderUrlAvailability,
  updateDeploymentState,
  promoteAgent,
  createDeploymentPipeline,
  updateOrgDeploymentPipeline,
  deleteDeploymentPipeline,
  updateEnvironment,
  createEnvironment,
  deleteEnvironment,
} from '../apis';
import { useAuthHooks } from '@agent-management-platform/auth';
import type {
  DeployAgentPathParams,
  DeployAgentRequest,
  UpdateAgentDeploySettingsPathParams,
  UpdateAgentDeploySettingsRequest,
  UpdateAgentConfigurationsPathParams,
  UpdateAgentConfigurationsRequest,
  RegenerateTracingTokenPathParams,
  RegenerateTracingTokenRequest,
  RegenerateTracingTokenResponse,
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
  GetEnvironmentPathParams,
  GetDeploymentPipelinePathParams,
  DeploymentPipelineResponse,
  DeploymentPipelineListResponse,
  ListDeploymentPipelinesPathParams,
  ListDeploymentPipelinesQuery,
  ListDataPlanesPathParams,
  DataPlaneListResponse,
  DeploymentDetailsResponse,
  UpdateDeploymentStatePathParams,
  UpdateDeploymentStateRequest,
  UpdateDeploymentStateResponse,
  PromoteAgentPathParams,
  PromoteAgentRequest,
  PromoteAgentResponse,
  CreateDeploymentPipelinePathParams,
  CreateDeploymentPipelineRequest,
  UpdateOrgDeploymentPipelinePathParams,
  DeleteDeploymentPipelinePathParams,
  UpdateDeploymentPipelineRequest,
  UpdateEnvironmentPathParams,
  UpdateEnvironmentRequest,
  Environment,
  CreateEnvironmentRequest,
  CreateEnvironmentPathParams,
  DeleteEnvironmentPathParams,
  CheckThunderUrlAvailabilityPathParams,
  CheckThunderUrlAvailabilityQuery,
  ThunderUrlAvailabilityResponse,
} from '@agent-management-platform/types';
import { POLL_INTERVAL } from '../utils';
import { useApiMutation, useApiQuery } from './react-query-notifications';
import { invalidateA2AQueries } from './a2a-query-keys';

export function useDeployAgent() {
  const queryClient = useQueryClient();
  const { getToken } = useAuthHooks();
  return useApiMutation<DeploymentResponse, unknown,
  { params: DeployAgentPathParams; body: DeployAgentRequest }>({
    action: { verb: 'start', target: 'deployment' },
    mutationFn: ({ params, body }) => deployAgent(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agent'] });
      queryClient.invalidateQueries({ queryKey: ['agent-configurations'] });
      queryClient.invalidateQueries({ queryKey: ['agent-deployments'] });
    },
  });
}

export function useUpdateAgentDeploySettings() {
  const queryClient = useQueryClient();
  const { getToken } = useAuthHooks();
  return useApiMutation<void, unknown,
  { params: UpdateAgentDeploySettingsPathParams; body: UpdateAgentDeploySettingsRequest }>({
    action: { verb: 'update', target: 'deploy settings' },
    // In mutationFn, since onSuccess is not awaited; A2A settings re-queue the proxy publish.
    mutationFn: async ({ params, body }) => {
      await updateAgentDeploySettings(params, body, getToken);
      await invalidateA2AQueries(queryClient, { ...params, envId: body.environmentName });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agent'] });
      queryClient.invalidateQueries({ queryKey: ['agent-configurations'] });
      queryClient.invalidateQueries({ queryKey: ['agent-deployments'] });
    },
  });
}

export function useUpdateAgentConfigurations() {
  const queryClient = useQueryClient();
  const { getToken } = useAuthHooks();
  return useApiMutation<void, unknown,
  { params: UpdateAgentConfigurationsPathParams; body: UpdateAgentConfigurationsRequest }>({
    action: { verb: 'update', target: 'configurations' },
    mutationFn: ({ params, body }) => updateAgentConfigurations(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agent'] });
      queryClient.invalidateQueries({ queryKey: ['agent-configurations'] });
      queryClient.invalidateQueries({ queryKey: ['agent-deployments'] });
    },
  });
}

export function useRegenerateTracingToken() {
  const queryClient = useQueryClient();
  const { getToken } = useAuthHooks();
  return useApiMutation<RegenerateTracingTokenResponse, unknown,
  { params: RegenerateTracingTokenPathParams; body: RegenerateTracingTokenRequest }>({
    // The drawer pushes its own detailed success/error snackbars (expiry date, restart notice),
    // so suppress the generic ones here to avoid duplicate toasts.
    showSuccess: false,
    showError: false,
    mutationFn: ({ params, body }) => regenerateTracingToken(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agent-deployments'] });
    },
  });
}

export function useListAgentDeployments(
  params: ListAgentDeploymentsPathParams, 
  options?: { enabled?: boolean }
) {
  const { getToken } = useAuthHooks();
  
  return useApiQuery<DeploymentListResponse>({
    queryKey: ['agent-deployments', params.orgName, params.projName, params.agentName],
    queryFn: () => listAgentDeployments(params, getToken),
    enabled: options?.enabled ?? (!!params.orgName && !!params.projName && !!params.agentName),
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
  return useApiQuery<EndpointsResponse>({
    queryKey: ['agent-endpoints', params, query],
    queryFn: () => getAgentEndpoints(params, query, getToken),
    enabled: !!params.orgName && !!params.projName && !!params.agentName && !!query.environment,
  });
}

export function useGetAgentConfigurations
(params: GetAgentConfigurationsPathParams, query: EnvironmentQuery) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ConfigurationResponse>({
    queryKey: ['agent-configurations', params, query],
    queryFn: () => getAgentConfigurations(params, query, getToken),
    enabled: !!params.orgName && !!params.projName && !!params.agentName && !!query.environment,
  });
}

export function useListEnvironments(params: ListEnvironmentsPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<EnvironmentListResponse>({
    queryKey: ['environments', params],
    queryFn: () => listEnvironments(params, getToken),
    enabled: !!params.orgName,
  });
}

export function useGetEnvironment(params: GetEnvironmentPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<Environment>({
    queryKey: ['environment', params],
    queryFn: () => getEnvironment(params, getToken),
    enabled: !!params.orgName && !!params.envName,
  });
}

export function useGetDeploymentPipeline(params: GetDeploymentPipelinePathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<DeploymentPipelineResponse>({
    queryKey: ['deployment-pipeline', params],
    queryFn: () => getDeploymentPipeline(params, getToken),
    enabled: !!params.orgName && !!params.projName,
  });
}

export function useListDeploymentPipelines(
  params: ListDeploymentPipelinesPathParams,
  query?: ListDeploymentPipelinesQuery,
  options?: { enabled?: boolean },
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<DeploymentPipelineListResponse>({
    queryKey: ['deployment-pipelines', params.orgName, query],
    queryFn: () => listDeploymentPipelines(params, query, getToken),
    enabled: options?.enabled ?? !!params.orgName,
  });
}

export function useListDataPlanes(
  params: ListDataPlanesPathParams,
  options?: { enabled?: boolean },
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<DataPlaneListResponse>({
    queryKey: ['data-planes', params.orgName],
    queryFn: () => listDataPlanes(params, getToken),
    enabled: options?.enabled ?? !!params.orgName,
  });
}

// Advisory-only pre-flight check for the Create Environment drawer's Thunder
// URL handle field — silences the generic error snackbar since this fires on
// every keystroke and a transient failure shouldn't interrupt typing; the
// real enforcement is the 409 add-environment-thunder.sh's PUT gets back.
export function useCheckThunderUrlAvailability(
  params: CheckThunderUrlAvailabilityPathParams,
  query: CheckThunderUrlAvailabilityQuery,
  options?: { enabled?: boolean },
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ThunderUrlAvailabilityResponse>({
    queryKey: ['thunder-url-availability', params.orgName, query.handle],
    queryFn: () => checkThunderUrlAvailability(params, query, getToken),
    enabled: (options?.enabled ?? true) && !!query.handle,
    retry: false,
    silent: true,
  });
}

export function useUpdateDeploymentState() {
  const queryClient = useQueryClient();
  const { getToken } = useAuthHooks();
  return useApiMutation<UpdateDeploymentStateResponse, unknown,
  { params: UpdateDeploymentStatePathParams; body: UpdateDeploymentStateRequest }>({
    action: { verb: 'update', target: 'deployment state' },
    mutationFn: ({ params, body }) => updateDeploymentState(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agent-deployments'] });
    },
  });
}

export function usePromoteAgent() {
  const queryClient = useQueryClient();
  const { getToken } = useAuthHooks();
  return useApiMutation<PromoteAgentResponse, unknown,
  { params: PromoteAgentPathParams; body: PromoteAgentRequest }>({
    action: { verb: 'promote', target: 'agent' },
    mutationFn: ({ params, body }) => promoteAgent(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agent'] });
      queryClient.invalidateQueries({ queryKey: ['agent-configurations'] });
      queryClient.invalidateQueries({ queryKey: ['agent-deployments'] });
    },
  });
}

export function useCreateDeploymentPipeline() {
  const queryClient = useQueryClient();
  const { getToken } = useAuthHooks();
  return useApiMutation<DeploymentPipelineResponse, unknown,
  { params: CreateDeploymentPipelinePathParams; body: CreateDeploymentPipelineRequest }>({
    action: { verb: 'create', target: 'deployment pipeline' },
    mutationFn: ({ params, body }) => createDeploymentPipeline(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['deployment-pipelines'] });
    },
  });
}

export function useUpdateOrgDeploymentPipeline() {
  const queryClient = useQueryClient();
  const { getToken } = useAuthHooks();
  return useApiMutation<DeploymentPipelineResponse, unknown,
  { params: UpdateOrgDeploymentPipelinePathParams; body: UpdateDeploymentPipelineRequest }>({
    action: { verb: 'update', target: 'deployment pipeline' },
    mutationFn: ({ params, body }) => updateOrgDeploymentPipeline(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['deployment-pipelines'] });
    },
  });
}

export function useDeleteDeploymentPipeline() {
  const queryClient = useQueryClient();
  const { getToken } = useAuthHooks();
  return useApiMutation<void, unknown, DeleteDeploymentPipelinePathParams>({
    action: { verb: 'delete', target: 'deployment pipeline' },
    mutationFn: (params) => deleteDeploymentPipeline(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['deployment-pipelines'] });
    },
  });
}


export function useUpdateEnvironment() {
  const queryClient = useQueryClient();
  const { getToken } = useAuthHooks();
  return useApiMutation<Environment, unknown,
  { params: UpdateEnvironmentPathParams; body: UpdateEnvironmentRequest }>({
    action: { verb: 'update', target: 'environment' },
    mutationFn: ({ params, body }) => updateEnvironment(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['environments'] });
    },
  });
}

export function useCreateEnvironment() {
  const queryClient = useQueryClient();
  const { getToken } = useAuthHooks();
  return useApiMutation<Environment, unknown,
  { params: CreateEnvironmentPathParams; body: CreateEnvironmentRequest }>({
    action: { verb: 'create', target: 'environment' },
    mutationFn: ({ params, body }) => createEnvironment(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['environments'] });
    },
  });
}

export function useDeleteEnvironment() {
  const queryClient = useQueryClient();
  const { getToken } = useAuthHooks();
  return useApiMutation<void, unknown, DeleteEnvironmentPathParams>({
    action: { verb: 'delete', target: 'environment' },
    mutationFn: (params) => deleteEnvironment(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['environments'] });
    },
  });
}



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
  updateAgentDeploySettings,
  getAgentEndpoints,
  getAgentConfigurations,
} from '../apis';
import { useAuthHooks } from '@agent-management-platform/auth';
import type {
  UpdateAgentDeploySettingsPathParams,
  UpdateAgentDeploySettingsRequest,
  GetAgentEndpointsPathParams,
  EndpointsResponse,
  EnvironmentQuery,
  GetAgentConfigurationsPathParams,
  ConfigurationResponse,
} from '@agent-management-platform/types';
import { useApiMutation, useApiQuery } from './react-query-notifications';
import { invalidateA2AQueries } from './a2a-query-keys';

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

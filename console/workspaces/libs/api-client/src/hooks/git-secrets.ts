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

import { useQueryClient } from "@tanstack/react-query";
import {
  listGitSecrets,
  createGitSecret,
  getGitSecret,
  deleteGitSecret,
} from "../apis";
import {
  GitSecretListResponse,
  GitSecretResponse,
  CreateGitSecretRequest,
  ListGitSecretsPathParams,
  CreateGitSecretPathParams,
  GetGitSecretPathParams,
  DeleteGitSecretPathParams,
  ListGitSecretsQuery,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "./react-query-notifications";

/**
 * Hook to list all git secrets for an organization
 */
export function useListGitSecrets(
  params: ListGitSecretsPathParams,
  query?: ListGitSecretsQuery,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<GitSecretListResponse>({
    queryKey: ['git-secrets', params, query],
    queryFn: () => listGitSecrets(params, query, getToken),
    enabled: !!params.orgName,
  });
}

/**
 * Hook to get a single git secret by name
 */
export function useGetGitSecret(params: GetGitSecretPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<GitSecretResponse>({
    queryKey: ['git-secret', params],
    queryFn: () => getGitSecret(params, getToken),
    enabled: !!params.orgName && !!params.secretName,
  });
}

/**
 * Hook to create a new git secret
 */
export function useCreateGitSecret() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    GitSecretResponse,
    unknown,
    { params: CreateGitSecretPathParams; body: CreateGitSecretRequest }
  >({
    action: { verb: 'create', target: 'git secret' },
    mutationFn: ({ params, body }) => createGitSecret(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['git-secrets'] });
    },
  });
}

/**
 * Hook to delete a git secret
 */
export function useDeleteGitSecret() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, DeleteGitSecretPathParams>({
    action: { verb: 'delete', target: 'git secret' },
    mutationFn: (params) => deleteGitSecret(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['git-secrets'] });
    },
  });
}

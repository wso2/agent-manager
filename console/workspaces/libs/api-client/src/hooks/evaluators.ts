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
  type CreateCustomEvaluatorRequest,
  type CustomEvaluatorPathParams,
  type EvaluatorListQuery,
  type EvaluatorListResponse,
  type EvaluatorLLMProviderListResponse,
  type EvaluatorResponse,
  type GetEvaluatorPathParams,
  type ListEvaluatorLLMProvidersPathParams,
  type ListEvaluatorsPathParams,
  type UpdateCustomEvaluatorRequest,
} from "@agent-management-platform/types";
import {
  createCustomEvaluator,
  deleteCustomEvaluator,
  getCustomEvaluator,
  getEvaluator,
  listEvaluatorLLMProviders,
  listEvaluators,
  updateCustomEvaluator,
} from "../apis";

export function useListEvaluators(
  params: ListEvaluatorsPathParams,
  query?: EvaluatorListQuery
) {
  const { getToken } = useAuthHooks();
  return useQuery<EvaluatorListResponse>({
    queryKey: ["evaluators", params, query],
    queryFn: () => listEvaluators(params, query, getToken),
    enabled: !!params.orgName,
  });
}

export function useGetEvaluator(params: GetEvaluatorPathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<EvaluatorResponse>({
    queryKey: ["evaluator", params],
    queryFn: () => getEvaluator(params, getToken),
    enabled: !!params.orgName && !!params.evaluatorId,
  });
}

export function useListEvaluatorLLMProviders(
  params: ListEvaluatorLLMProvidersPathParams
) {
  const { getToken } = useAuthHooks();
  return useQuery<EvaluatorLLMProviderListResponse>({
    queryKey: ["evaluator-llm-providers", params],
    queryFn: () => listEvaluatorLLMProviders(params, getToken),
    enabled: !!params.orgName,
  });
}

export function useCreateCustomEvaluator(params: ListEvaluatorsPathParams) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<EvaluatorResponse, unknown, CreateCustomEvaluatorRequest>({
    mutationFn: (body) => createCustomEvaluator(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["evaluators"] });
    },
  });
}

export function useGetCustomEvaluator(params: CustomEvaluatorPathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<EvaluatorResponse>({
    queryKey: ["custom-evaluator", params],
    queryFn: () => getCustomEvaluator(params, getToken),
    enabled: !!params.orgName && !!params.identifier,
  });
}

export function useUpdateCustomEvaluator(params: CustomEvaluatorPathParams) {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<EvaluatorResponse, unknown, UpdateCustomEvaluatorRequest>({
    mutationFn: (body) => updateCustomEvaluator(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["evaluators"] });
      queryClient.invalidateQueries({ queryKey: ["custom-evaluator"] });
    },
  });
}

export function useDeleteCustomEvaluator() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<void, unknown, CustomEvaluatorPathParams>({
    mutationFn: (mutationParams) => deleteCustomEvaluator(mutationParams, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["evaluators"] });
    },
  });
}

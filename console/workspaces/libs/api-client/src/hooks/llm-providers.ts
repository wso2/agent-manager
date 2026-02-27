/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
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
  type CreateLLMDeploymentPathParams,
  type CreateLLMDeploymentRequest,
  type CreateLLMProviderPathParams,
  type CreateLLMProviderRequest,
  type CreateLLMProviderTemplatePathParams,
  type CreateLLMProviderTemplateRequest,
  type CreateLLMProxyPathParams,
  type CreateLLMProxyRequest,
  type DeleteLLMDeploymentPathParams,
  type DeleteLLMProviderPathParams,
  type DeleteLLMProviderTemplatePathParams,
  type DeleteLLMProxyPathParams,
  type GetLLMDeploymentPathParams,
  type GetLLMProviderPathParams,
  type GetLLMProviderTemplatePathParams,
  type GetLLMProxyPathParams,
  type ListLLMDeploymentsPathParams,
  type ListLLMProviderProxiesPathParams,
  type ListLLMProviderTemplatesPathParams,
  type ListLLMProvidersPathParams,
  type ListLLMProxiesPathParams,
  type LLMDeploymentListResponse,
  type LLMDeploymentResponse,
  type LLMProviderListResponse,
  type LLMProviderResponse,
  type LLMProviderTemplateListResponse,
  type LLMProviderTemplateResponse,
  type LLMProxyListResponse,
  type LLMProxyResponse,
  type RestoreLLMDeploymentPathParams,
  type UndeployLLMProviderPathParams,
  type UpdateLLMProviderCatalogPathParams,
  type UpdateLLMProviderCatalogRequest,
  type UpdateLLMProviderPathParams,
  type UpdateLLMProviderRequest,
  type UpdateLLMProviderTemplatePathParams,
  type UpdateLLMProviderTemplateRequest,
  type UpdateLLMProxyPathParams,
  type UpdateLLMProxyRequest,
} from "@agent-management-platform/types";
import {
  createLLMDeployment,
  createLLMProvider,
  createLLMProviderTemplate,
  createLLMProxy,
  deleteLLMDeployment,
  deleteLLMProvider,
  deleteLLMProviderTemplate,
  deleteLLMProxy,
  getLLMDeployment,
  getLLMProvider,
  getLLMProviderTemplate,
  getLLMProxy,
  listLLMDeployments,
  listLLMProviderProxies,
  listLLMProviders,
  listLLMProviderTemplates,
  listLLMProxies,
  restoreLLMDeployment,
  undeployLLMProvider,
  updateLLMProvider,
  updateLLMProviderCatalog,
  updateLLMProviderTemplate,
  updateLLMProxy,
} from "../apis";

interface PaginationQuery {
  limit?: number;
  offset?: number;
}

// Templates

export function useListLLMProviderTemplates(
  params: ListLLMProviderTemplatesPathParams,
  query?: PaginationQuery,
) {
  const { getToken } = useAuthHooks();
  return useQuery<LLMProviderTemplateListResponse>({
    queryKey: ["llm-provider-templates", params, query],
    queryFn: () => listLLMProviderTemplates(params, query, getToken),
    enabled: !!params.orgName,
  });
}

export function useGetLLMProviderTemplate(params: GetLLMProviderTemplatePathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<LLMProviderTemplateResponse>({
    queryKey: ["llm-provider-template", params],
    queryFn: () => getLLMProviderTemplate(params, getToken),
    enabled: !!params.orgName && !!params.templateId,
  });
}

export function useCreateLLMProviderTemplate() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<
    LLMProviderTemplateResponse,
    unknown,
    { params: CreateLLMProviderTemplatePathParams; body: CreateLLMProviderTemplateRequest }
  >({
    mutationFn: ({ params, body }) =>
      createLLMProviderTemplate(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["llm-provider-templates"] });
    },
  });
}

export function useUpdateLLMProviderTemplate() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<
    LLMProviderTemplateResponse,
    unknown,
    { params: UpdateLLMProviderTemplatePathParams; body: UpdateLLMProviderTemplateRequest }
  >({
    mutationFn: ({ params, body }) =>
      updateLLMProviderTemplate(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["llm-provider-templates"] });
      queryClient.invalidateQueries({ queryKey: ["llm-provider-template"] });
    },
  });
}

export function useDeleteLLMProviderTemplate() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<void, unknown, DeleteLLMProviderTemplatePathParams>({
    mutationFn: (params) => deleteLLMProviderTemplate(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["llm-provider-templates"] });
    },
  });
}

// Providers

export function useListLLMProviders(
  params: ListLLMProvidersPathParams,
  query?: PaginationQuery,
) {
  const { getToken } = useAuthHooks();
  return useQuery<LLMProviderListResponse>({
    queryKey: ["llm-providers", params, query],
    queryFn: () => listLLMProviders(params, query, getToken),
    enabled: !!params.orgName,
  });
}

export function useGetLLMProvider(params: GetLLMProviderPathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<LLMProviderResponse>({
    queryKey: ["llm-provider", params],
    queryFn: () => getLLMProvider(params, getToken),
    enabled: !!params.orgName && !!params.providerId,
  });
}

export function useCreateLLMProvider() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<
    LLMProviderResponse,
    unknown,
    { params: CreateLLMProviderPathParams; body: CreateLLMProviderRequest }
  >({
    mutationFn: ({ params, body }) => createLLMProvider(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["llm-providers"] });
    },
  });
}

export function useUpdateLLMProvider() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<
    LLMProviderResponse,
    unknown,
    { params: UpdateLLMProviderPathParams; body: UpdateLLMProviderRequest }
  >({
    mutationFn: ({ params, body }) => updateLLMProvider(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["llm-providers"] });
      queryClient.invalidateQueries({ queryKey: ["llm-provider"] });
    },
  });
}

export function useDeleteLLMProvider() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<void, unknown, DeleteLLMProviderPathParams>({
    mutationFn: (params) => deleteLLMProvider(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["llm-providers"] });
    },
  });
}

export function useUpdateLLMProviderCatalog() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<
    void,
    unknown,
    { params: UpdateLLMProviderCatalogPathParams; body: UpdateLLMProviderCatalogRequest }
  >({
    mutationFn: ({ params, body }) =>
      updateLLMProviderCatalog(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["llm-provider"] });
      queryClient.invalidateQueries({ queryKey: ["llm-providers"] });
    },
  });
}

// Proxies

export function useListLLMProviderProxies(
  params: ListLLMProviderProxiesPathParams,
  query?: PaginationQuery,
) {
  const { getToken } = useAuthHooks();
  return useQuery<LLMProxyListResponse>({
    queryKey: ["llm-provider-proxies", params, query],
    queryFn: () => listLLMProviderProxies(params, query, getToken),
    enabled: !!params.orgName && !!params.providerId,
  });
}

export function useListLLMProxies(
  params: ListLLMProxiesPathParams,
  query?: PaginationQuery,
) {
  const { getToken } = useAuthHooks();
  return useQuery<LLMProxyListResponse>({
    queryKey: ["llm-proxies", params, query],
    queryFn: () => listLLMProxies(params, query, getToken),
    enabled: !!params.orgName && !!params.projName,
  });
}

export function useGetLLMProxy(params: GetLLMProxyPathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<LLMProxyResponse>({
    queryKey: ["llm-proxy", params],
    queryFn: () => getLLMProxy(params, getToken),
    enabled: !!params.orgName && !!params.projName && !!params.proxyId,
  });
}

export function useCreateLLMProxy() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<
    LLMProxyResponse,
    unknown,
    { params: CreateLLMProxyPathParams; body: CreateLLMProxyRequest }
  >({
    mutationFn: ({ params, body }) => createLLMProxy(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["llm-proxies"] });
      queryClient.invalidateQueries({ queryKey: ["llm-provider-proxies"] });
    },
  });
}

export function useUpdateLLMProxy() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<
    LLMProxyResponse,
    unknown,
    { params: UpdateLLMProxyPathParams; body: UpdateLLMProxyRequest }
  >({
    mutationFn: ({ params, body }) => updateLLMProxy(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["llm-proxies"] });
      queryClient.invalidateQueries({ queryKey: ["llm-proxy"] });
      queryClient.invalidateQueries({ queryKey: ["llm-provider-proxies"] });
    },
  });
}

export function useDeleteLLMProxy() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<void, unknown, DeleteLLMProxyPathParams>({
    mutationFn: (params) => deleteLLMProxy(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["llm-proxies"] });
      queryClient.invalidateQueries({ queryKey: ["llm-provider-proxies"] });
    },
  });
}

// Deployments

export function useListLLMDeployments(
  params: ListLLMDeploymentsPathParams,
  query?: PaginationQuery,
) {
  const { getToken } = useAuthHooks();
  return useQuery<LLMDeploymentListResponse>({
    queryKey: ["llm-deployments", params, query],
    queryFn: () => listLLMDeployments(params, query, getToken),
    enabled: !!params.orgName && !!params.providerId,
  });
}

export function useGetLLMDeployment(params: GetLLMDeploymentPathParams) {
  const { getToken } = useAuthHooks();
  return useQuery<LLMDeploymentResponse>({
    queryKey: ["llm-deployment", params],
    queryFn: () => getLLMDeployment(params, getToken),
    enabled: !!params.orgName && !!params.providerId && !!params.deploymentId,
  });
}

export function useCreateLLMDeployment() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<
    LLMDeploymentResponse,
    unknown,
    { params: CreateLLMDeploymentPathParams; body: CreateLLMDeploymentRequest }
  >({
    mutationFn: ({ params, body }) =>
      createLLMDeployment(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["llm-deployments"] });
    },
  });
}

export function useUndeployLLMProvider() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<void, unknown, UndeployLLMProviderPathParams>({
    mutationFn: (params) => undeployLLMProvider(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["llm-deployments"] });
      queryClient.invalidateQueries({ queryKey: ["llm-provider"] });
    },
  });
}

export function useRestoreLLMDeployment() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<void, unknown, RestoreLLMDeploymentPathParams>({
    mutationFn: (params) => restoreLLMDeployment(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["llm-deployments"] });
      queryClient.invalidateQueries({ queryKey: ["llm-provider"] });
    },
  });
}

export function useDeleteLLMDeployment() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useMutation<void, unknown, DeleteLLMDeploymentPathParams>({
    mutationFn: (params) => deleteLLMDeployment(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["llm-deployments"] });
    },
  });
}


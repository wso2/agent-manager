/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useQueryClient } from "@tanstack/react-query";
import { useAuthHooks } from "@agent-management-platform/auth";
import type {
  CreateMCPProxyPathParams,
  DeleteMCPProxyPathParams,
  FetchMCPProxyServerInfoPathParams,
  GetMCPProxyPathParams,
  GetProjectMCPProxyPathParams,
  ListMCPProxiesPathParams,
  ListMCPProxiesQuery,
  MCPProxy,
  MCPProxyListResponse,
  MCPServerInfoFetchRequest,
  MCPServerInfoFetchResponse,
  ProjectMCPProxy,
  UpdateMCPProxyPathParams,
} from "@agent-management-platform/types";
import {
  createMCPProxy,
  deleteMCPProxy,
  documentFromProxy,
  fetchMCPProxyServerInfo,
  getMCPProxy,
  getProjectMCPProxy,
  listMCPProxies,
  proxyFromDocument,
  proxyListFromDocuments,
  updateDocumentFromProxy,
  updateMCPProxy,
} from "../apis/mcp-proxies";
import { useApiMutation, useApiQuery } from "../../shared/react-query-notifications";
import { useUIModulePresent } from "../../shared/useUIModulePresent";

export function useListMCPProxies(
  params: ListMCPProxiesPathParams,
  query?: ListMCPProxiesQuery,
) {
  const gatewayPresent = useUIModulePresent("ai-gateway");
  const { getToken } = useAuthHooks();
  return useApiQuery<MCPProxyListResponse>({
    queryKey: ["mcp-proxies", params, query],
    queryFn: () => listMCPProxies(params, query, getToken).then(proxyListFromDocuments),
    enabled: gatewayPresent && !!params.orgName,
  });
}

export function useCreateMCPProxy() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    MCPProxy,
    unknown,
    { params: CreateMCPProxyPathParams; body: MCPProxy }
  >({
    action: { verb: "create", target: "mcp server" },
    mutationFn: ({ params, body }) =>
      createMCPProxy(params, documentFromProxy(body), getToken).then(proxyFromDocument),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["mcp-proxies"] });
    },
  });
}

export function useGetMCPProxy(params: GetMCPProxyPathParams) {
  const gatewayPresent = useUIModulePresent("ai-gateway");
  const { getToken } = useAuthHooks();
  return useApiQuery<MCPProxy>({
    queryKey: ["mcp-proxy", params],
    queryFn: () => getMCPProxy(params, getToken).then(proxyFromDocument),
    enabled: gatewayPresent && !!params.orgName && !!params.proxyId,
  });
}

/** A project-owned proxy from its project route; off until org, project and proxy are known. */
export function useGetProjectMCPProxy(params: GetProjectMCPProxyPathParams) {
  const gatewayPresent = useUIModulePresent("ai-gateway");
  const { getToken } = useAuthHooks();
  return useApiQuery<ProjectMCPProxy>({
    queryKey: ["project-mcp-proxy", params],
    queryFn: () => getProjectMCPProxy(params, getToken),
    enabled: gatewayPresent && !!params.orgName && !!params.projName && !!params.proxyId,
  });
}

export function useUpdateMCPProxy() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    MCPProxy,
    unknown,
    { params: UpdateMCPProxyPathParams; body: MCPProxy }
  >({
    action: { verb: "update", target: "mcp server" },
    mutationFn: ({ params, body }) =>
      updateMCPProxy(params, updateDocumentFromProxy(params.proxyId, body), getToken)
        .then(proxyFromDocument),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["mcp-proxies"] });
      queryClient.invalidateQueries({ queryKey: ["mcp-proxy"] });
    },
  });
}

export function useDeleteMCPProxy() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, DeleteMCPProxyPathParams>({
    action: { verb: "delete", target: "mcp server" },
    mutationFn: (params) => deleteMCPProxy(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["mcp-proxies"] });
    },
  });
}

export function useFetchMCPProxyServerInfo() {
  const { getToken } = useAuthHooks();
  return useApiMutation<
    MCPServerInfoFetchResponse,
    unknown,
    {
      params: FetchMCPProxyServerInfoPathParams;
      body: MCPServerInfoFetchRequest;
      // Aborts the probe, so a superseded one frees its server-side slot.
      signal?: AbortSignal;
    }
  >({
    mutationFn: ({ params, body, signal }) =>
      fetchMCPProxyServerInfo(params, body, getToken, signal),
    showSuccess: false,
  });
}

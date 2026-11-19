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
  listMCPProxyScopes,
  createMCPProxyScope,
  updateMCPProxyScope,
  deleteMCPProxyScope,
} from "../apis/mcp-proxy-scopes";
import type {
  MCPProxyScopeListResponse,
  MCPProxyScopeRequest,
  MCPProxyScopeResponse,
  MCPProxyScopeUpdateRequest,
  MCPProxyScopesPathParams,
  MCPProxyScopePathParams,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "../../shared/react-query-notifications";
import { useUIModulePresent } from "../../shared/useUIModulePresent";

/**
 * Hook to list an MCP proxy's scopes
 */
export function useListMCPProxyScopes(
  params: MCPProxyScopesPathParams,
  options?: { enabled?: boolean },
) {
  const gatewayPresent = useUIModulePresent("ai-gateway");
  const { getToken } = useAuthHooks();
  return useApiQuery<MCPProxyScopeListResponse>({
    queryKey: ['mcp-proxy-scopes', params],
    queryFn: () => listMCPProxyScopes(params, getToken),
    enabled: gatewayPresent && (options?.enabled ?? true) && !!params.orgName && !!params.proxyId,
  });
}

/**
 * Hook to create an MCP proxy scope
 */
export function useCreateMCPProxyScope() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    MCPProxyScopeResponse, unknown, { params: MCPProxyScopesPathParams; body: MCPProxyScopeRequest }
  >({
    action: { verb: 'create', target: 'scope' },
    mutationFn: ({ params, body }) => createMCPProxyScope(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['mcp-proxy-scopes', params] });
    },
  });
}

/**
 * Hook to update an MCP proxy scope's description and/or tools
 */
export function useUpdateMCPProxyScope() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    MCPProxyScopeResponse,
    unknown,
    { params: MCPProxyScopePathParams; body: MCPProxyScopeUpdateRequest }
  >({
    action: { verb: 'update', target: 'scope' },
    mutationFn: ({ params, body }) => updateMCPProxyScope(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({
        queryKey: ['mcp-proxy-scopes', { orgName: params.orgName, proxyId: params.proxyId }],
      });
    },
  });
}

/**
 * Hook to delete an MCP proxy scope
 */
export function useDeleteMCPProxyScope() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, MCPProxyScopePathParams>({
    action: { verb: 'delete', target: 'scope' },
    mutationFn: (params) => deleteMCPProxyScope(params, getToken),
    onSuccess: (_data, params) => {
      queryClient.invalidateQueries({
        queryKey: ['mcp-proxy-scopes', { orgName: params.orgName, proxyId: params.proxyId }],
      });
    },
  });
}

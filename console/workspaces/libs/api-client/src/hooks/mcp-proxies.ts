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
  ProjectMCPProxyEnvironment,
  RefreshProjectMCPProxyCapabilitiesPathParams,
  ListMCPProxiesPathParams,
  ListMCPProxiesQuery,
  MCPProxy,
  MCPProxyListResponse,
  MCPServerInfoFetchRequest,
  MCPServerInfoFetchResponse,
  CreateProjectMCPProxyAPIKeyRequest,
  CreateProjectMCPProxyAPIKeyResponse,
  DeleteProjectMCPProxyPathParams,
  ListAPIKeysResponse,
  ProjectMCPProxy,
  ProjectMCPProxyAPIKeyPathParams,
  ProjectMCPProxyEnvironmentPathParams,
  ProjectMCPProxyEnvironmentRequest,
  ProjectMCPProxyRequest,
  RotateProjectMCPProxyAPIKeyRequest,
  RotateProjectMCPProxyAPIKeyResponse,
  UpdateMCPProxyPathParams,
} from "@agent-management-platform/types";
import {
  createMCPProxy,
  createProjectMCPProxyAPIKey,
  deleteMCPProxy,
  deleteProjectMCPProxy,
  deleteProjectMCPProxyEnvironment,
  fetchMCPProxyServerInfo,
  getMCPProxy,
  getProjectMCPProxy,
  refreshProjectMCPProxyCapabilities,
  listMCPProxies,
  listProjectMCPProxyAPIKeys,
  putProjectMCPProxyEnvironment,
  revokeProjectMCPProxyAPIKey,
  rotateProjectMCPProxyAPIKey,
  updateMCPProxy,
  upsertProjectMCPProxy,
} from "../apis";
import { useApiMutation, useApiQuery } from "./react-query-notifications";

export function useListMCPProxies(
  params: ListMCPProxiesPathParams,
  query?: ListMCPProxiesQuery,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<MCPProxyListResponse>({
    queryKey: ["mcp-proxies", params, query],
    queryFn: () => listMCPProxies(params, query, getToken),
    enabled: !!params.orgName,
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
    mutationFn: ({ params, body }) => createMCPProxy(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["mcp-proxies"] });
    },
  });
}

export function useGetMCPProxy(params: GetMCPProxyPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<MCPProxy>({
    queryKey: ["mcp-proxy", params],
    queryFn: () => getMCPProxy(params, getToken),
    enabled: !!params.orgName && !!params.proxyId,
  });
}

/** A project-owned proxy from its project route; off until org, project and proxy are known. */
export function useGetProjectMCPProxy(params: GetProjectMCPProxyPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ProjectMCPProxy>({
    queryKey: ["project-mcp-proxy", params],
    queryFn: () => getProjectMCPProxy(params, getToken),
    enabled: !!params.orgName && !!params.projName && !!params.proxyId,
  });
}

/**
 * Runs a project proxy binding's capability discovery again; its proxy reads refetch
 * with it Pending.
 */
export function useRefreshProjectMCPProxyCapabilities() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    ProjectMCPProxyEnvironment,
    unknown,
    RefreshProjectMCPProxyCapabilitiesPathParams
  >({
    action: { verb: "start", target: "tool discovery" },
    mutationFn: (params) => refreshProjectMCPProxyCapabilities(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["project-mcp-proxy"] });
    },
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
    mutationFn: ({ params, body }) => updateMCPProxy(params, body, getToken),
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

function useUpsertProjectMCPProxy(verb: "create" | "update") {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    ProjectMCPProxy,
    unknown,
    { params: DeleteProjectMCPProxyPathParams; body: ProjectMCPProxyRequest }
  >({
    action: { verb, target: "mcp server" },
    mutationFn: ({ params, body }) => upsertProjectMCPProxy(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["mcp-proxies"] });
      queryClient.invalidateQueries({ queryKey: ["project-mcp-proxy"] });
    },
  });
}

/** Registers a project-owned proxy (PUT on a new id). */
export function useCreateProjectMCPProxy() {
  return useUpsertProjectMCPProxy("create");
}

/** Updates a project-owned proxy; the body replaces its name, description, context and upstream. */
export function useUpdateProjectMCPProxy() {
  return useUpsertProjectMCPProxy("update");
}

/** Removes a project proxy from one environment. */
export function useDeleteProjectMCPProxyEnvironment() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, ProjectMCPProxyEnvironmentPathParams>({
    action: { verb: "remove", target: "mcp server environment" },
    mutationFn: (params) => deleteProjectMCPProxyEnvironment(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["project-mcp-proxy"] });
      queryClient.invalidateQueries({ queryKey: ["mcp-proxies"] });
    },
  });
}

export function useDeleteProjectMCPProxy() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, DeleteProjectMCPProxyPathParams>({
    action: { verb: "delete", target: "mcp server" },
    mutationFn: (params) => deleteProjectMCPProxy(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["mcp-proxies"] });
      queryClient.invalidateQueries({ queryKey: ["project-mcp-proxy"] });
    },
  });
}

/** Replaces a project proxy's binding in one environment; refreshes the proxy and lists after. */
export function usePutProjectMCPProxyEnvironment() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    ProjectMCPProxyEnvironment,
    unknown,
    {
      params: ProjectMCPProxyEnvironmentPathParams;
      body: ProjectMCPProxyEnvironmentRequest;
    }
  >({
    action: { verb: "update", target: "mcp server" },
    mutationFn: ({ params, body }) =>
      putProjectMCPProxyEnvironment(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["project-mcp-proxy"] });
      queryClient.invalidateQueries({ queryKey: ["mcp-proxies"] });
    },
  });
}

const PROJECT_MCP_PROXY_API_KEY_QUERY_KEY = "project-mcp-proxy-api-keys";

export function useListProjectMCPProxyAPIKeys(
  params: ProjectMCPProxyEnvironmentPathParams,
  options?: { enabled?: boolean },
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<ListAPIKeysResponse>({
    queryKey: [PROJECT_MCP_PROXY_API_KEY_QUERY_KEY, params],
    queryFn: () => listProjectMCPProxyAPIKeys(params, getToken),
    enabled:
      (options?.enabled ?? true) &&
      !!(params.orgName && params.projName && params.proxyId && params.envName),
  });
}

export function useCreateProjectMCPProxyAPIKey() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    CreateProjectMCPProxyAPIKeyResponse,
    unknown,
    {
      params: ProjectMCPProxyEnvironmentPathParams;
      body: CreateProjectMCPProxyAPIKeyRequest;
    }
  >({
    action: { verb: "create", target: "MCP server api key" },
    mutationFn: ({ params, body }) =>
      createProjectMCPProxyAPIKey(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [PROJECT_MCP_PROXY_API_KEY_QUERY_KEY],
      });
    },
  });
}

export function useRotateProjectMCPProxyAPIKey() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    RotateProjectMCPProxyAPIKeyResponse,
    unknown,
    {
      params: ProjectMCPProxyAPIKeyPathParams;
      body: RotateProjectMCPProxyAPIKeyRequest;
    }
  >({
    action: { verb: "rotate", target: "MCP server api key" },
    mutationFn: ({ params, body }) =>
      rotateProjectMCPProxyAPIKey(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [PROJECT_MCP_PROXY_API_KEY_QUERY_KEY],
      });
    },
  });
}

export function useRevokeProjectMCPProxyAPIKey() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, ProjectMCPProxyAPIKeyPathParams>({
    action: { verb: "revoke", target: "MCP server api key" },
    mutationFn: (params) => revokeProjectMCPProxyAPIKey(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: [PROJECT_MCP_PROXY_API_KEY_QUERY_KEY],
      });
    },
  });
}

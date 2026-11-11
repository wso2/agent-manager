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

import { type QueryClient, useQueryClient } from "@tanstack/react-query";
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
  DeleteProjectMCPProxyPathParams,
  ProjectMCPProxy,
  ProjectMCPProxyEnvironmentPathParams,
  ProjectMCPProxyEnvironmentRequest,
  ProjectMCPProxyRequest,
  UpdateMCPProxyPathParams,
} from "@agent-management-platform/types";
import {
  createMCPProxy,
  deleteMCPProxy,
  deleteProjectMCPProxy,
  deleteProjectMCPProxyEnvironment,
  fetchMCPProxyServerInfo,
  getMCPProxy,
  getProjectMCPProxy,
  refreshProjectMCPProxyCapabilities,
  listAllMCPProxies,
  listMCPProxies,
  putProjectMCPProxyEnvironment,
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

/** Every proxy the list returns, across its pages; see listAllMCPProxies. */
export function useListAllMCPProxies(
  params: ListMCPProxiesPathParams,
  query?: Omit<ListMCPProxiesQuery, "limit" | "offset">,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<MCPProxyListResponse>({
    queryKey: ["mcp-proxies", params, "all", query],
    queryFn: () => listAllMCPProxies(params, query, getToken),
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

/** Refreshes what a project proxy change shows in: the proxy's page and the MCP server lists. */
function invalidateProjectMCPProxies(queryClient: QueryClient) {
  queryClient.invalidateQueries({ queryKey: ["project-mcp-proxy"] });
  queryClient.invalidateQueries({ queryKey: ["mcp-proxies"] });
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
    // Registering only creates, so it cannot replace a server the project already has.
    mutationFn: ({ params, body }) =>
      upsertProjectMCPProxy(params, body, getToken, { createOnly: verb === "create" }),
    onSuccess: () => {
      invalidateProjectMCPProxies(queryClient);
    },
  });
}

/** Registers a project-owned proxy (a create-only PUT: an id already in the project is refused). */
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
      invalidateProjectMCPProxies(queryClient);
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
      invalidateProjectMCPProxies(queryClient);
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
      invalidateProjectMCPProxies(queryClient);
    },
  });
}

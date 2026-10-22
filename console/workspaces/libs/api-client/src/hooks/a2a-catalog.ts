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

import { keepPreviousData } from "@tanstack/react-query";
import { useAuthHooks } from "@agent-management-platform/auth";
import type {
  A2ACatalogAgent,
  A2ACatalogEntryPathParams,
  A2ACatalogListPathParams,
  A2ACatalogListQuery,
  A2ACatalogListResponse,
  A2AConnectPathParams,
  A2AConnectResponse,
} from "@agent-management-platform/types";
import { useApiQuery } from "./react-query-notifications";
import { getA2ACatalogEntry, getA2AConnect, listA2ACatalog } from "../apis/a2a-catalog";
import { nullIfNotFound } from "../utils";

export const A2A_CATALOG_STALE_MS = 30_000;

export const a2aCatalogListQueryKey = (
  params: A2ACatalogListPathParams,
  query: A2ACatalogListQuery,
) => ["a2a-agents", params, query] as const;

export const a2aCatalogEntryQueryKey = (params: A2ACatalogEntryPathParams) =>
  ["a2a-catalog-entry", params] as const;

export const a2aConnectQueryKey = (params: A2AConnectPathParams) =>
  ["a2a-connect", params] as const;

export function useListA2ACatalog(params: A2ACatalogListPathParams, query: A2ACatalogListQuery) {
  const { getToken } = useAuthHooks();
  return useApiQuery<A2ACatalogListResponse>({
    queryKey: a2aCatalogListQueryKey(params, query),
    queryFn: () => listA2ACatalog(params, query, getToken),
    enabled: !!params.orgName,
    placeholderData: keepPreviousData,
    staleTime: A2A_CATALOG_STALE_MS,
  });
}

/** Resolves to null when the agent is not in the catalogue (a 404). */
export function useGetA2ACatalogEntry(params: A2ACatalogEntryPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<A2ACatalogAgent | null>({
    queryKey: a2aCatalogEntryQueryKey(params),
    queryFn: () => nullIfNotFound(getA2ACatalogEntry(params, getToken)),
    enabled: !!(params.orgName && params.projName && params.agentName),
    staleTime: A2A_CATALOG_STALE_MS,
    retry: false,
    silent: true,
  });
}

export function useGetA2AConnect(params: A2AConnectPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<A2AConnectResponse>({
    queryKey: a2aConnectQueryKey(params),
    queryFn: () => getA2AConnect(params, getToken),
    enabled: !!(params.orgName && params.projName && params.agentName && params.envId),
  });
}

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

import type {
  AgentRegistryEntryDetail,
  AgentRegistryListResponse,
  GetAgentRegistryEntryPathParams,
  ListAgentRegistryPathParams,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";
import {
  getAgentRegistryEntry,
  listAgentRegistry,
  listUnmanagedAgents,
} from "../apis";
import { useApiQuery } from "./react-query-notifications";

/**
 * How long a fetched registry list is reused before it is refetched. Longer than the
 * console's default: one list call reads every agent and deployment in the organization.
 */
const REGISTRY_LIST_STALE_MS = 60_000;

/** Every managed agent in the organization, across all projects, with its registry record. */
export function useListAgentRegistry(params: ListAgentRegistryPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentRegistryListResponse>({
    queryKey: ["agent-registry", params],
    queryFn: () => listAgentRegistry(params, getToken),
    enabled: !!params.orgName,
    staleTime: REGISTRY_LIST_STALE_MS,
  });
}

/** Every unmanaged agent: the Shadow Agent Discovery findings not onboarded yet. */
export function useListUnmanagedAgents(params: ListAgentRegistryPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentRegistryListResponse>({
    // Under the findings' prefix, so a Shadow Agent Discovery change (scan, onboard,
    // delete) refreshes it.
    queryKey: ["shadow-agents", "findings", params.orgName, "agent-registry"],
    queryFn: () => listUnmanagedAgents(params, getToken),
    enabled: !!params.orgName,
    staleTime: REGISTRY_LIST_STALE_MS,
  });
}

/** One managed agent's registry record; null when there is no such agent. */
export function useGetAgentRegistryEntry(params: GetAgentRegistryEntryPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentRegistryEntryDetail | null>({
    // Under the list's prefix, so invalidating ["agent-registry"] refreshes both.
    queryKey: ["agent-registry", "entry", params],
    queryFn: () => getAgentRegistryEntry(params, getToken),
    enabled: !!params.orgName && !!params.projName && !!params.agentName,
  });
}

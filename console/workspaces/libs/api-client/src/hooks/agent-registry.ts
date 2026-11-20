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
  AgentRegistryEntry,
  AgentRegistryListResponse,
  GetAgentRegistryEntryPathParams,
  GetUnregisteredAgentPathParams,
  ListAgentRegistryPathParams,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";
import { getAgentRegistryEntry, getUnregisteredAgent, listAgentRegistry } from "../apis";
import { useApiQuery } from "./react-query-notifications";

/** Every agent in the organization, across all projects, with its registry record. */
export function useListAgentRegistry(params: ListAgentRegistryPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentRegistryListResponse>({
    queryKey: ["agent-registry", params],
    queryFn: () => listAgentRegistry(params, getToken),
    enabled: !!params.orgName,
  });
}

/** One registered agent's registry record; null when there is no such agent. */
export function useGetAgentRegistryEntry(params: GetAgentRegistryEntryPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentRegistryEntry | null>({
    // Under the list's prefix, so invalidating ["agent-registry"] refreshes both.
    queryKey: ["agent-registry", "entry", params],
    queryFn: () => getAgentRegistryEntry(params, getToken),
    enabled: !!params.orgName && !!params.projName && !!params.agentName,
  });
}

/** One unregistered (shadow) agent by its finding ID; null when there is no such agent. */
export function useGetUnregisteredAgent(params: GetUnregisteredAgentPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentRegistryEntry | null>({
    queryKey: ["agent-registry", "unregistered", params],
    queryFn: () => getUnregisteredAgent(params, getToken),
    enabled: !!params.orgName && !!params.findingId,
  });
}

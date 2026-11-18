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
  AgentRegistryListResponse,
  ListAgentRegistryPathParams,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";
import { listAgentRegistry } from "../apis";
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

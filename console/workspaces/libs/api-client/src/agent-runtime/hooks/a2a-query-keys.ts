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

import type { QueryClient } from "@tanstack/react-query";
import type { AgentCardPathParams } from "@agent-management-platform/types";

// Card and proxy share this prefix so one invalidation refetches both.
const a2aEnvQueryKey = (p: AgentCardPathParams) =>
  ["a2a", p.orgName, p.projName, p.agentName, p.envId] as const;

export const agentCardQueryKey = (p: AgentCardPathParams) => [...a2aEnvQueryKey(p), "card"] as const;

export const a2aProxyQueryKey = (p: AgentCardPathParams) => [...a2aEnvQueryKey(p), "proxy"] as const;

/** A card or proxy change also changes the other, so both are refetched. */
export async function invalidateA2AQueries(
  queryClient: QueryClient,
  params: AgentCardPathParams,
): Promise<void> {
  await queryClient.invalidateQueries({ queryKey: a2aEnvQueryKey(params) });
}

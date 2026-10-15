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
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useAuthHooks } from "@agent-management-platform/auth";
import { listAvailableMCPPolicies } from "../apis";
import { toPolicyDefinition, type GuardrailDefinition } from "./guardrails";
import { useApiQuery } from "./react-query-notifications";

/** An MCP policy as the policy picker renders it; `version` is the major ("v1"). */
export type MCPPolicyDefinition = GuardrailDefinition;

export interface MCPPoliciesCatalogResponse {
  count: number;
  data: MCPPolicyDefinition[];
}

/**
 * Fetches the MCP policies available on the organization's active gateways.
 * agent-manager-service returns them display-ready: major version, inline parameter
 * schema, and policy hub names, descriptions and categories, with only policies that
 * apply to MCP included.
 */
export function useMCPPoliciesCatalog(orgName?: string) {
  const { getToken } = useAuthHooks();

  return useApiQuery<MCPPoliciesCatalogResponse>({
    queryKey: ["MCP policies catalog", orgName],
    enabled: Boolean(orgName),
    queryFn: async () => {
      if (!orgName) {
        throw new Error("Organization name is required to list MCP policies.");
      }

      const available = await listAvailableMCPPolicies({ orgName }, getToken);
      const data = (available.list ?? []).map(toPolicyDefinition);
      return { count: data.length, data };
    },
  });
}

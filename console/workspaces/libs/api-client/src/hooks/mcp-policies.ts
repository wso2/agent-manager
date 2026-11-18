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
import type { PolicyCatalogResponse } from "@agent-management-platform/types";
import { listAvailableMCPPolicies } from "../apis";
import {
  toGuardrailDefinition,
  type GuardrailDefinition,
  type GuardrailsCatalogResponse,
} from "./guardrails";
import { normalizePolicyVersionToMajor } from "../utils/policyVersion";
import { useApiQuery } from "./react-query-notifications";

export { normalizePolicyVersionToMajor };

export interface MCPPolicyDefinition extends GuardrailDefinition {
  /**
   * Major version (e.g. `v1`) stored on a proxy policy; `version` is the exact
   * gateway version.
   */
  savedVersion: string;
}

export interface MCPPoliciesCatalogResponse
  extends Omit<GuardrailsCatalogResponse, "data"> {
  data: MCPPolicyDefinition[];
}

/** Maps the agent-manager-service policy catalog response for MCP pickers. */
export function toMCPCatalog(
  resp: PolicyCatalogResponse,
): MCPPoliciesCatalogResponse {
  return {
    count: resp.count,
    data: (resp.list ?? []).map((item) => ({
      ...toGuardrailDefinition(item),
      savedVersion: normalizePolicyVersionToMajor(item.version),
    })),
    hubAvailable: resp.hubAvailable,
    availabilityScope: resp.availabilityScope,
  };
}

export function useMCPPoliciesCatalog(orgName?: string, proxyId?: string) {
  const { getToken } = useAuthHooks();
  // Route params fall back to "" when missing; that means no proxy, not a proxy
  // named "".
  const scopedProxyId = proxyId || undefined;

  return useApiQuery<MCPPoliciesCatalogResponse>({
    queryKey: ["MCP policies catalog", orgName, scopedProxyId],
    enabled: Boolean(orgName),
    queryFn: async () => {
      if (!orgName) {
        throw new Error("Organization name is required to list MCP policies.");
      }
      const token = await getToken();
      const resp = await listAvailableMCPPolicies(
        { orgName },
        { proxyId: scopedProxyId },
        async () => token,
      );
      return toMCPCatalog(resp);
    },
  });
}

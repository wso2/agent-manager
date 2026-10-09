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

import { useAuthHooks } from "@agent-management-platform/auth";
import { listAvailableLLMPolicies } from "../apis";
import { useApiQuery } from "./react-query-notifications";

export interface GuardrailDefinition {
  name: string;
  version: string;
  displayName: string;
  description: string;
  provider: string;
  categories: string[];
  isLatest: boolean;
  /**
   * JSON-Schema of the user-configurable parameters, reported inline by the gateway
   * manifest. PolicySelectorDrawer renders the parameter form directly from it.
   */
  parameters: Record<string, unknown>;
  systemParameters: Record<string, unknown>;
  /**
   * Gateway config keys the policy reads with no default. Non-empty means an
   * operator must set them on the gateway for the policy to work; the console shows
   * the policy with a warning. Whether they are set is not known.
   */
  requiredSystemConfig: string[];
}

// Hidden from the generic LLM policy picker: configured by a dedicated LLM provider
// tab (security, rate limiting, cost) instead. Which policies apply to LLM at all
// (e.g. MCP-only ones) is decided by agent-manager-service, and policies that need
// gateway configuration are shown with a warning rather than hidden (see
// GuardrailDefinition.requiredSystemConfig).
const LLM_MANAGED_POLICY_NAMES = new Set([
  "api-key-auth",
  "basic-auth",
  "jwt-auth",
  "advanced-ratelimit",
  "basic-ratelimit",
  "token-based-ratelimit",
  "llm-cost-based-ratelimit",
  "llm-cost",
  "subscription-validation",
]);

/** Filters the LLM policy catalog for the generic policy picker. */
export function filterGuardrailPolicies(
  policies: GuardrailDefinition[],
): GuardrailDefinition[] {
  return policies.filter((p) => !LLM_MANAGED_POLICY_NAMES.has(p.name));
}

export interface GuardrailsCatalogResponse {
  count: number;
  data: GuardrailDefinition[];
}

/** One policy as the LLM and MCP policy listing endpoints return it. */
export interface AvailablePolicyItem {
  name: string;
  version: string;
  displayName?: string;
  description?: string;
  categories?: string[];
  requiredSystemConfig?: string[];
  parameters?: Record<string, unknown>;
  systemParameters?: Record<string, unknown>;
}

/**
 * Maps a policy listing item to the shape the policy picker renders. The service
 * already resolves display names (gateway, then hub, then the raw name); the name
 * fallback here only guards against an item without one.
 */
export function toPolicyDefinition(p: AvailablePolicyItem): GuardrailDefinition {
  return {
    name: p.name,
    version: p.version,
    displayName: p.displayName || p.name,
    description: p.description ?? "",
    provider: "gateway",
    categories: p.categories ?? [],
    isLatest: true,
    parameters: p.parameters ?? {},
    systemParameters: p.systemParameters ?? {},
    requiredSystemConfig: p.requiredSystemConfig ?? [],
  };
}

/**
 * Fetches the LLM policies available on the gateways (scoped to `providerId`'s
 * gateways when given). agent-manager-service returns them display-ready: the
 * gateway's exact version and inline parameter schema, enriched with policy hub
 * names, descriptions and categories, with MCP-only policies already excluded.
 */
export function useLLMPoliciesCatalog(
  orgName?: string,
  enabled = true,
  providerId?: string,
) {
  const { getToken } = useAuthHooks();

  return useApiQuery<GuardrailsCatalogResponse>({
    queryKey: ["LLM gateway policies catalog", orgName, providerId],
    enabled: enabled && Boolean(orgName),
    queryFn: async () => {
      if (!orgName) {
        throw new Error("Organization name is required to list LLM policies.");
      }

      const available = await listAvailableLLMPolicies(
        { orgName },
        { providerId },
        getToken,
      );
      const data = (available.list ?? []).map(toPolicyDefinition);
      return { count: data.length, data };
    },
  });
}

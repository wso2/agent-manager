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
import {
  type PolicyCatalogAvailabilityScope,
  type PolicyCatalogGatewayRef,
  type PolicyCatalogItem,
  type PolicyCatalogResponse,
  type PolicyCatalogSource,
  type PolicyCatalogStatus,
} from "@agent-management-platform/types";
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
  /** Where the policy comes from: the public hub or an organization-managed custom policy. */
  source: PolicyCatalogSource;
  /** Whether the policy is installed on the gateways it would be deployed to. */
  status: PolicyCatalogStatus;
  /** Gateways the policy is missing on; set only when `status` is `not_installed`. */
  missingOn?: PolicyCatalogGatewayRef[];
  /**
   * Present only for installed policies (see useLLMPoliciesCatalog). When set,
   * PolicySelectorDrawer renders the parameter form directly from these.
   */
  parameters?: Record<string, unknown>;
  systemParameters?: Record<string, unknown>;
}

// Tier 1: Always hidden — infra/auth/MCP policies irrelevant to LLM governance,
// plus policies not available in the supported gateway version.
const NON_GUARDRAIL_POLICY_EXCLUDELIST = new Set([
  "api-key-auth",
  "basic-auth",
  "jwt-auth",
  "cors",
  "advanced-ratelimit",
  "basic-ratelimit",
  // Rate limiting policies managed via the Rate Limiting tab
  "token-based-ratelimit",
  "llm-cost-based-ratelimit",
  "llm-cost",
  "mcp-acl-list",
  "mcp-auth",
  "mcp-authz",
  "mcp-rewrite",
  "mcp-ratelimit",
  "respond",
  "redirect",
  "semantic-tool-filtering",
  // Not available in gateway v1.0.0
  // Infra/mediation policies that only become visible once the picker reads the
  // full, unfiltered gateway manifest instead of the hub's categories=Guardrails,AI
  // filter (useLLMPoliciesCatalog) — none of these are LLM guardrails.
  "analytics-header-filter",
  "backend-jwt",
  "dynamic-endpoint",
  "host-rewrite",
  "json-xml-mediator",
  "log-message",
  // Found live against a real gateway manifest (2026-07-17): more mediation
  // policies not covered by the upstream build-manifest.yaml audit above.
  "remove-headers",
  "request-rewrite",
  "set-headers",
  "subscription-validation",
]);

/**
 * Filters the raw policy catalog for display in the guardrail selector.
 *
 * Always hidden: infra/auth/MCP policies (NON_GUARDRAIL_POLICY_EXCLUDELIST).
 * Not-installed policies must also be categorised "Guardrails" to be shown.
 * Everything else is shown, including guardrails that need external system
 * configuration on the gateway (AWS Bedrock, Azure Content Safety, …).
 */
export function filterGuardrailPolicies(
  policies: GuardrailDefinition[],
): GuardrailDefinition[] {
  return policies.filter((p) => {
    // The hub tags about twenty non-guardrail policies "AI"; a policy no gateway
    // has installed is only worth offering when the hub files it under Guardrails.
    if (p.status === "not_installed" && !p.categories.includes("Guardrails"))
      return false;
    return !NON_GUARDRAIL_POLICY_EXCLUDELIST.has(p.name);
  });
}

export interface GuardrailsCatalogResponse {
  count: number;
  data: GuardrailDefinition[];
  /** False when the policy hub is disabled or unreachable (installed policies still list). */
  hubAvailable: boolean;
  availabilityScope: PolicyCatalogAvailabilityScope;
}

export function toGuardrailDefinition(
  item: PolicyCatalogItem,
): GuardrailDefinition {
  return {
    name: item.name,
    version: item.version,
    displayName: item.displayName || item.name,
    description: item.description ?? "",
    provider: item.source,
    source: item.source,
    status: item.status,
    missingOn: item.missingOn,
    categories: item.categories ?? [],
    isLatest: true,
    // Left undefined for not-installed items (no schema available). An inline,
    // possibly empty, object is what tells PolicySelectorDrawer the schema is known.
    parameters: item.parameters,
    systemParameters: item.systemParameters,
  };
}

/** Maps the agent-manager-service policy catalog response 1:1 for the picker. */
export function toGuardrailCatalog(
  resp: PolicyCatalogResponse,
): GuardrailsCatalogResponse {
  return {
    count: resp.count,
    data: (resp.list ?? []).map(toGuardrailDefinition),
    hubAvailable: resp.hubAvailable,
    availabilityScope: resp.availabilityScope,
  };
}

/**
 * Fetches the merged LLM policy catalog from agent-manager-service: the public hub's
 * policies plus organization-managed custom ones, each flagged installed or
 * not-installed against the gateways the provider is deployed to (or all managed
 * gateways when no provider is given). The browser never calls the policy hub.
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
      const token = await getToken();
      const resp = await listAvailableLLMPolicies(
        { orgName },
        { providerId },
        async () => token,
      );
      return toGuardrailCatalog(resp);
    },
  });
}

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
import { globalConfig, type GuardrailCapabilities } from "@agent-management-platform/types";
import { useApiQuery } from "./react-query-notifications";

export interface GuardrailDefinition {
  name: string;
  version: string;
  displayName: string;
  description: string;
  provider: string;
  categories: string[];
  isLatest: boolean;
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
  "respond",
  "semantic-tool-filtering",
  // Not available in gateway v1.0.0
  "prompt-compressor",
]);

// Tier 2: Hidden by default — require external system config.
// Shown only when the corresponding capability flag is enabled in runtime config.
// Typed as Record<keyof GuardrailCapabilities, ...> so adding a new capability flag
// without a corresponding policy entry (or vice versa) is a compile error.
const CAPABILITY_POLICY_MAP: Record<keyof GuardrailCapabilities, string[]> = {
  awsBedrock:         ["aws-bedrock-guardrail"],
  azureContentSafety: ["azure-content-safety-content-moderation"],
  graniteGuardian:    ["granite-guardian-prompt-injection"],
  nemoGuard:          ["nvidia-nemoguard-content-safety"],
  semanticGuardrails: ["semantic-prompt-guard", "semantic-cache"],
};

const ALL_CAPABILITY_GATED_POLICIES = new Set(
  Object.values(CAPABILITY_POLICY_MAP).flat(),
);

/**
 * Filters the raw policy catalog for display in the guardrail selector.
 *
 * Tier 1 — always hidden: infra/auth/MCP policies.
 * Tier 2 — hidden by default: policies requiring external system config;
 *           shown when the corresponding capability flag is true in `capabilities`.
 * Tier 3 — always shown: OOTB policies with no external dependencies.
 */
export function filterGuardrailPolicies(
  policies: GuardrailDefinition[],
  capabilities?: GuardrailCapabilities,
): GuardrailDefinition[] {
  const enabledCapabilityPolicies = new Set(
    (Object.entries(CAPABILITY_POLICY_MAP) as [keyof GuardrailCapabilities, string[]][])
      .filter(([key]) => capabilities?.[key])
      .flatMap(([, names]) => names),
  );

  return policies.filter((p) => {
    if (NON_GUARDRAIL_POLICY_EXCLUDELIST.has(p.name)) return false;
    if (ALL_CAPABILITY_GATED_POLICIES.has(p.name)) return enabledCapabilityPolicies.has(p.name);
    return true;
  });
}

export interface GuardrailsCatalogResponse {
  count: number;
  data: GuardrailDefinition[];
}

export function useGuardrailsCatalog() {
  const url = globalConfig.guardrailsCatalogUrl;
  const { getToken } = useAuthHooks();

  return useApiQuery<GuardrailsCatalogResponse>({
    queryKey: ["Guardrails catalog", url],
    enabled: Boolean(url),
    queryFn: async () => {
      if (!url) {
        throw new Error("Guardrails catalog URL is not configured.");
      }

      const token = await getToken();
      const res = await fetch(url, {
        headers: token
          ? { Authorization: `Bearer ${token}` }
          : undefined,
      });
      if (!res.ok) {
        const text = await res.text().catch(() => "");
        throw new Error(
          text || `Failed to fetch guardrails catalog: ${res.status}`,
        );
      }
      return (await res.json()) as GuardrailsCatalogResponse;
    },
  });
}

/**
 * Fetches a single guardrail policy definition (YAML) by name and version.
 *
 * The definition endpoint returns YAML content which should be
 * parsed by the consumer (e.g. with `parsePolicyYaml`).
 *
 * URL pattern:
 * `{guardrailsDefinitionBaseUrl}/{name}/versions/{version}/definition`
 */
export function useGuardrailPolicyDefinition(
  name: string | undefined,
  version: string | undefined,
) {
  const baseUrl = globalConfig.guardrailsDefinitionBaseUrl;
  const { getToken } = useAuthHooks();
  const enabled = Boolean(baseUrl && name && version);

  return useApiQuery<string>({
    queryKey: [
      "Guardrail policy definition", baseUrl, name, version,
    ],
    enabled,
    queryFn: async () => {
      if (!baseUrl || !name || !version) {
        throw new Error(
          "Guardrails definition base URL, policy name,"
          + " and version are required.",
        );
      }

      const token = await getToken();
      const url =
        `${baseUrl}/${encodeURIComponent(name)}`
        + `/versions/${encodeURIComponent(version)}`
        + `/definition`;
      const res = await fetch(url, {
        headers: token
          ? { Authorization: `Bearer ${token}` }
          : undefined,
      });
      if (!res.ok) {
        const errText = await res.text().catch(() => "");
        throw new Error(
          errText
          || `Failed to fetch policy definition: ${res.status}`,
        );
      }
      return res.text();
    },
  });
}


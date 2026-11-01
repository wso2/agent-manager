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
  globalConfig,
  type GuardrailCapabilities,
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
  /**
   * Present only for gateway-manifest-sourced policies (see useLLMPoliciesCatalog).
   * When set, PolicySelectorDrawer renders the parameter form directly from these
   * instead of fetching+parsing a YAML definition from the policy hub.
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
  // Bundled since gateway 20261031-r1. mtls-auth and oauth2-token-exchange are
  // auth. The typesafe-jev-* policies need the typesafe_jev_v0 system config,
  // which AM does not provision; exposing them means adding that config plus a
  // CAPABILITY_POLICY_MAP flag.
  "mtls-auth",
  "oauth2-token-exchange",
  "typesafe-jev-content-safety",
  "typesafe-jev-mcp-tool-intent-verification",
  "typesafe-jev-mcp-tool-result-screening",
  "typesafe-jev-model-routing",
  "typesafe-jev-tool-filtering",
]);

// Tier 2: Hidden by default — require external system config.
// Shown only when the corresponding capability flag is enabled in runtime config.
// Typed as Record<keyof GuardrailCapabilities, ...> so adding a new capability flag
// without a corresponding policy entry (or vice versa) is a compile error.
const CAPABILITY_POLICY_MAP: Record<keyof GuardrailCapabilities, string[]> = {
  awsBedrock: ["aws-bedrock-guardrail"],
  azureContentSafety: ["azure-content-safety-content-moderation"],
  graniteGuardian: ["granite-guardian-prompt-injection"],
  nemoGuard: ["nvidia-nemoguard-content-safety"],
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
    (
      Object.entries(CAPABILITY_POLICY_MAP) as [
        keyof GuardrailCapabilities,
        string[],
      ][]
    )
      .filter(([key]) => capabilities?.[key])
      .flatMap(([, names]) => names),
  );

  return policies.filter((p) => {
    if (NON_GUARDRAIL_POLICY_EXCLUDELIST.has(p.name)) return false;
    if (ALL_CAPABILITY_GATED_POLICIES.has(p.name))
      return enabledCapabilityPolicies.has(p.name);
    return true;
  });
}

export interface GuardrailsCatalogResponse {
  count: number;
  data: GuardrailDefinition[];
}

export function useGuardrailsCatalog(enabled = true) {
  const url = globalConfig.guardrailsCatalogUrl;
  const { getToken } = useAuthHooks();

  return useApiQuery<GuardrailsCatalogResponse>({
    queryKey: ["Guardrails catalog", url],
    enabled: enabled && Boolean(url),
    queryFn: async () => {
      if (!url) {
        throw new Error("Guardrails catalog URL is not configured.");
      }

      const token = await getToken();
      const res = await fetch(url, {
        headers: token ? { Authorization: `Bearer ${token}` } : undefined,
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
    queryKey: ["Guardrail policy definition", baseUrl, name, version],
    enabled,
    queryFn: async () => {
      if (!baseUrl || !name || !version) {
        throw new Error(
          "Guardrails definition base URL, policy name," +
            " and version are required.",
        );
      }

      const token = await getToken();
      const url =
        `${baseUrl}/${encodeURIComponent(name)}` +
        `/versions/${encodeURIComponent(version)}` +
        `/definition`;
      const res = await fetch(url, {
        headers: token ? { Authorization: `Bearer ${token}` } : undefined,
      });
      if (!res.ok) {
        const errText = await res.text().catch(() => "");
        throw new Error(
          errText || `Failed to fetch policy definition: ${res.status}`,
        );
      }
      return res.text();
    },
  });
}

/**
 * Best-effort lookup of {policyName → displayName} from the public policy hub.
 *
 * Used purely to give WSO2's built-in policies a friendly name when the gateway's
 * own manifest omits `displayName` (which, today, it does for all but one). The hub
 * is NEVER consulted for availability or version — only for a nicer label. Any
 * failure (hub unconfigured, unreachable, or a bad response) resolves to an empty
 * map so the picker degrades to the raw kebab-case name instead of erroring. Keyed
 * by name only: display names are stable across patch versions, and the hub's coarse
 * `major.minor` version wouldn't match the gateway's exact build version anyway.
 *
 * Bounded by an AbortController timeout: a hub that hangs (unreachable host, stalled
 * TLS) rather than failing fast must NOT block the authoritative gateway list this
 * runs in parallel with — on timeout the abort rejects the fetch and we fall back.
 */
const HUB_DISPLAY_NAME_TIMEOUT_MS = 3000;

async function fetchHubPolicyDisplayNames(
  token: string | undefined,
): Promise<Map<string, string>> {
  const displayNames = new Map<string, string>();
  const url = globalConfig.guardrailsCatalogUrl;
  if (!url) return displayNames;
  const controller = new AbortController();
  const timer = setTimeout(
    () => controller.abort(),
    HUB_DISPLAY_NAME_TIMEOUT_MS,
  );
  try {
    const res = await fetch(url, {
      headers: token ? { Authorization: `Bearer ${token}` } : undefined,
      signal: controller.signal,
    });
    if (!res.ok) return displayNames;
    const catalog = (await res.json()) as GuardrailsCatalogResponse;
    for (const policy of catalog.data ?? []) {
      if (policy.displayName) displayNames.set(policy.name, policy.displayName);
    }
  } catch {
    // Hub is a best-effort name source only (includes AbortError on timeout);
    // ignore and fall back to kebab names.
  } finally {
    clearTimeout(timer);
  }
  return displayNames;
}

/**
 * Fetches available LLM guardrail policies from the gateway's own reported manifest
 * (via agent-manager-service), instead of the external policy hub. Every policy
 * carries its full JSON-Schema `parameters`/`systemParameters` inline, so the picker
 * never needs a second round-trip to fetch a YAML definition, and a custom policy
 * appears the moment its gateway redeploys — no policy-hub publishing step involved.
 *
 * The gateway is the sole source of truth for which policies exist and at which
 * version. The policy hub is consulted only as a best-effort source of friendly
 * display names for policies whose gateway manifest left `displayName` empty (see
 * fetchHubPolicyDisplayNames) — a hybrid that keeps user-authored custom policies
 * working (they fall through to their own name) without depending on the hub for
 * availability.
 */
export function useLLMPoliciesCatalog(
  orgName?: string,
  enabled = true,
  providerId?: string,
) {
  const { getToken } = useAuthHooks();

  return useApiQuery<GuardrailsCatalogResponse>({
    queryKey: [
      "LLM gateway policies catalog",
      orgName,
      providerId,
      globalConfig.guardrailsCatalogUrl,
    ],
    enabled: enabled && Boolean(orgName),
    queryFn: async () => {
      if (!orgName) {
        throw new Error("Organization name is required to list LLM policies.");
      }

      const token = await getToken();
      // Run both in parallel: the hub lookup adds no latency beyond the slower call,
      // and a hub failure never blocks the (authoritative) gateway list.
      const [available, hubDisplayNames] = await Promise.all([
        listAvailableLLMPolicies({ orgName }, { providerId }, async () => token),
        fetchHubPolicyDisplayNames(token),
      ]);
      const data: GuardrailDefinition[] = (available.list ?? []).map((p) => ({
        name: p.name,
        // Always the gateway's exact build version — never the hub's coarser one.
        version: p.version,
        // Priority: the gateway's own displayName (authoritative; the only source
        // for a user-authored custom policy) → the policy hub's friendly name (for
        // WSO2 built-ins the gateway left blank) → the raw kebab-case name.
        displayName: p.displayName || hubDisplayNames.get(p.name) || p.name,
        description: p.description ?? "",
        provider: "gateway",
        categories: [],
        isLatest: true,
        // Never undefined: an inline (possibly empty) object is what signals to
        // PolicySelectorDrawer that this policy's schema is already known, so it
        // can skip the hub YAML-definition fetch entirely.
        parameters: p.parameters ?? {},
        systemParameters: p.systemParameters ?? {},
      }));
      return { count: data.length, data };
    },
  });
}

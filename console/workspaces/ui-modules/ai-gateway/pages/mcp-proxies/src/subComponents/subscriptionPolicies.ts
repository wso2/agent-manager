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
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */


import type {
  MCPProxyEndpoint,
  MCPProxyPolicy,
  SubscriptionPlan,
} from "@agent-management-platform/types";

// The gateway policies that enforce subscription plans on an MCP server, as the API control plane
// attaches them. subscription-validation v2 checks the subscription and records the plan; the
// rate-limit policies after it enforce the plan's conditions.
export const SUBSCRIPTION_VALIDATION = "subscription-validation";
export const REQUEST_RATELIMIT = "subscription-request-ratelimit";
export const TOOL_RATELIMIT = "subscription-tool-ratelimit";

/**
 * Condition types an MCP server counts, and the policy that enforces each. Only tool calls: on
 * an MCP server every protocol message (initialize, tools/list, pings) is a request, so a plan's
 * request limits are not enforced there, as a plan's token limits are not.
 */
const ENFORCERS: Record<string, string> = {
  "tool-call-count": TOOL_RATELIMIT,
};

/** Whether any of the plans has request limits, which an MCP server doesn't enforce. */
export const hasRequestLimits = (plans: readonly Pick<SubscriptionPlan, "conditions">[]) =>
  plans.some((p) => (p.conditions ?? []).some((c) => c.type === "request-count"));

/** Built-in condition types, in the order they are listed, with their fixed IDs and names. */
export const CONDITION_TYPES = [
  { type: "request-count", id: "requests", name: "Request count" },
  { type: "tool-call-count", id: "tool-calls", name: "Tool call count" },
  { type: "token-count", id: "tokens", name: "Token count" },
] as const;

const DEFAULT_PARAMS: Record<string, Record<string, unknown>> = {
  [SUBSCRIPTION_VALIDATION]: { subscriptionKeyHeader: "Subscription-Key" },
};

const policyOf = (name: string, version: string): MCPProxyPolicy => ({
  name,
  version,
  params: { ...DEFAULT_PARAMS[name] },
});

const majorOf = (version: string) => version.replace(/^v?(\d+).*$/, "v$1");

/** Whether the policies hold this one at the same major version. */
export const hasPolicy = (policies: readonly MCPProxyPolicy[], wanted: MCPProxyPolicy) =>
  policies.some((p) => p.name === wanted.name && majorOf(p.version) === majorOf(wanted.version));

/**
 * The policies an MCP server needs to enforce these plans: subscription-validation v2 for any plan,
 * then the rate limit of each condition type the plans hold that an MCP server counts.
 */
export function requiredPolicies(
  plans: readonly Pick<SubscriptionPlan, "conditions">[],
): MCPProxyPolicy[] {
  if (plans.length === 0) return [];
  const types = new Set(plans.flatMap((p) => (p.conditions ?? []).map((c) => c.type)));
  const enforcers = Object.entries(ENFORCERS)
    .filter(([type]) => types.has(type as SubscriptionPlan["conditions"][number]["type"]))
    .map(([, name]) => policyOf(name, "v1"));
  return [policyOf(SUBSCRIPTION_VALIDATION, "v2"), ...enforcers];
}

/** The required policies that one or more endpoints lack, once each. */
export function missingPolicies(
  required: readonly MCPProxyPolicy[],
  endpoints: readonly Pick<MCPProxyEndpoint, "policies">[],
): MCPProxyPolicy[] {
  return required.filter((policy) =>
    endpoints.some((endpoint) => !hasPolicy(endpoint.policies ?? [], policy)),
  );
}

/**
 * The policies with the required ones added: subscription-validation first (an older version of
 * it is replaced in place), the rate limits after everything else, so they run after it.
 */
export function withPoliciesAdded(
  policies: readonly MCPProxyPolicy[],
  required: readonly MCPProxyPolicy[],
): MCPProxyPolicy[] {
  let next = [...policies];
  for (const policy of required) {
    if (hasPolicy(next, policy)) continue;
    if (policy.name === SUBSCRIPTION_VALIDATION) {
      const at = next.findIndex((p) => p.name === SUBSCRIPTION_VALIDATION);
      next = at >= 0 ? next.map((p, i) => (i === at ? policy : p)) : [policy, ...next];
    } else {
      next.push(policy);
    }
  }
  return next;
}

/** Whether one plan condition is enforced on an endpoint, and by which policy. */
export type EnforcementRow = {
  type: string;
  id: string;
  name: string;
  policy: string | null;
  status: "enforced" | "notEnforced" | "notApplicable";
};

/**
 * What the offered plans need from an endpoint's policies: one row per condition type they hold.
 * A condition is enforced only while subscription-validation v2 is there too.
 */
export function planEnforcement(
  plans: readonly Pick<SubscriptionPlan, "conditions">[],
  policies: readonly MCPProxyPolicy[],
): { validationMissing: boolean; rows: EnforcementRow[]; enforced: number; applicable: number } {
  const validationMissing = !hasPolicy(policies, policyOf(SUBSCRIPTION_VALIDATION, "v2"));
  const types = new Set(plans.flatMap((p) => (p.conditions ?? []).map((c) => c.type)));
  const rows: EnforcementRow[] = CONDITION_TYPES.filter((c) => types.has(c.type)).map((c) => {
    const policy = ENFORCERS[c.type] ?? null;
    if (!policy) return { ...c, policy, status: "notApplicable" };
    const attached = hasPolicy(policies, policyOf(policy, "v1")) && !validationMissing;
    return { ...c, policy, status: attached ? "enforced" : "notEnforced" };
  });
  const applicable = rows.filter((r) => r.status !== "notApplicable");
  return {
    validationMissing,
    rows,
    enforced: applicable.filter((r) => r.status === "enforced").length,
    applicable: applicable.length,
  };
}

/** A policy as people read it: `subscription-validation v2`. */
export const policyLabel = (policy: Pick<MCPProxyPolicy, "name" | "version">) =>
  `${policy.name} ${majorOf(policy.version)}`;

/** Display names of the subscription policies, for prompts that list them. */
const POLICY_TITLES: Record<string, string> = {
  [SUBSCRIPTION_VALIDATION]: "Subscription Validation",
  [TOOL_RATELIMIT]: "Subscription Tool Rate Limit",
  [REQUEST_RATELIMIT]: "Subscription Request Rate Limit",
};

/** One-line summaries of the subscription policies, for prompts that list them. */
const POLICY_SUMMARIES: Record<string, string> = {
  [SUBSCRIPTION_VALIDATION]: "Checks the caller holds an active subscription",
  [TOOL_RATELIMIT]: "Limits tool calls to the plan's quota",
  [REQUEST_RATELIMIT]: "Limits requests to the plan's quota",
};

/** A policy as a list row shows it: its display name, major version and a one-line summary. */
export function policyDetails(policy: Pick<MCPProxyPolicy, "name" | "version">): {
  title: string;
  version: string;
  summary?: string;
} {
  return {
    title: POLICY_TITLES[policy.name] ?? policy.name,
    version: majorOf(policy.version),
    summary: POLICY_SUMMARIES[policy.name],
  };
}

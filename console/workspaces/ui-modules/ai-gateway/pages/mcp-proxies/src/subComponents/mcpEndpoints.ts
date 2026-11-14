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
  MCPProxyCapabilities,
  MCPProxyEndpoint,
  MCPProxyPolicy,
  MCPServerInfoFetchResponse,
  UpstreamAuth,
} from "@agent-management-platform/types";
import type { EndpointDraft } from "./EndpointFormFields";
import {
  CAPABILITY_SECTION_KEY,
  getCapabilityId,
  type CapabilityKind,
} from "@agent-management-platform/shared-component";
import { ACL_POLICY_NAME, REWRITE_POLICY_NAME } from "../constants";

// Default for a brand-new endpoint (no prior config) — matches upstream's
// long-standing default (AddMCPProxyForm on main hardcodes the same shape):
// every new proxy requires an X-API-Key header until an admin deliberately
// changes it.
export const DEFAULT_ENDPOINT_SECURITY = {
  enabled: true,
  apiKey: {
    enabled: true,
    key: "X-API-Key",
    in: "header" as const,
  },
};

// The auth-type rule lives in shared-component so the agent creation and
// configuration pages derive their runtime variables from the same definition
// this page renders. Re-exported here for existing importers.
export {
  CAPABILITY_SECTION_KEY,
  getCapabilityId,
  isToolBlockedByAcl,
  parseAclSection,
  type CapabilityKind,
} from "@agent-management-platform/shared-component";
export {
  getAuthenticationTypeLabel,
  isAPIKeySecurityEnabled,
  resolveAuthenticationType,
  type AuthenticationType,
} from "@agent-management-platform/shared-component";

function collectCapabilityIds(
  kind: CapabilityKind,
  entries: Record<string, unknown>[] | undefined,
): Set<string> {
  const ids = new Set<string>();
  (entries ?? []).forEach((raw) => {
    const id = getCapabilityId(kind, raw);
    if (id) ids.add(id);
  });
  return ids;
}

// The backend identifier a Rewrite policy entry targets (its explicit `target`,
// falling back to the client-facing name/uri).
function rewriteEntryId(
  kind: CapabilityKind,
  entry: Record<string, unknown>,
): string | null {
  const target =
    typeof entry.target === "string" && entry.target.trim()
      ? entry.target.trim()
      : null;
  if (target) return target;
  return getCapabilityId(kind, entry);
}

// Converts stored per-environment capabilities into the shape returned by the
// fetch-server-info API, so an already-configured endpoint can be rendered and
// edited with the same components as a freshly fetched one.
export function capabilitiesToFetchedInfo(
  capabilities: MCPProxyCapabilities | undefined,
): MCPServerInfoFetchResponse {
  return {
    tools: capabilities?.tools ?? [],
    resources: capabilities?.resources ?? [],
    prompts: capabilities?.prompts ?? [],
  };
}

/**
 * Converts a native backend endpoint into the form's editable draft (1:1). The draft's
 * client `id` carries the endpoint handle so the save path can preserve the endpoint's
 * identity. Auth values are never returned by the API, so the draft carries an empty
 * `authValue` (treated as "keep existing"). Its environment list is the flat set of bound
 * environment UUIDs.
 */
export function endpointToDraft(endpoint: MCPProxyEndpoint): EndpointDraft {
  const gatewayByEnv: Record<string, string> = {};
  (endpoint.environments ?? []).forEach((env) => {
    if (env.gatewayId) gatewayByEnv[env.environmentUuid] = env.gatewayId;
  });
  return {
    id: endpoint.id,
    name: endpoint.name ?? "",
    url: endpoint.upstream?.main?.url ?? "",
    authHeader: endpoint.upstream?.main?.auth?.header ?? "",
    authValue: "",
    resilienceTimeout: endpoint.resilience?.timeout ?? "",
    resilienceIdleTimeout: endpoint.resilience?.idleTimeout ?? "",
    environments: (endpoint.environments ?? []).map(
      (env) => env.environmentUuid,
    ),
    gatewayByEnv,
    // The saved versions ride along with the saved capabilities until the endpoint is
    // fetched again, so saving it unchanged keeps them.
    fetchedInfo: {
      ...capabilitiesToFetchedInfo(endpoint.capabilities),
      supportedVersions: endpoint.mcpSpecVersions,
    },
  };
}

function buildUpstreamAuth(endpoint: EndpointDraft): UpstreamAuth | undefined {
  const header = endpoint.authHeader.trim();
  if (!header) return undefined;
  const value = endpoint.authValue.trim();
  // Omit the value when the user left the masked credential untouched, mirroring
  // the Connection tab: the backend keeps the stored value when none is supplied.
  return value
    ? { type: "api-key", header, value }
    : { type: "api-key", header };
}

/**
 * Removes Rewrite and Manage Tools policy entries that reference tools, resources
 * or prompts absent from `capabilities`. Other policy entries and policies are left
 * untouched. Returns `undefined` when no policies remain.
 */
export function prunePoliciesForCapabilities(
  policies: MCPProxyPolicy[] | undefined,
  capabilities: MCPProxyCapabilities | undefined,
): MCPProxyPolicy[] | undefined {
  if (!policies || policies.length === 0) return policies;

  const validIds: Record<CapabilityKind, Set<string>> = {
    tool: collectCapabilityIds("tool", capabilities?.tools),
    resource: collectCapabilityIds("resource", capabilities?.resources),
    prompt: collectCapabilityIds("prompt", capabilities?.prompts),
  };
  const kinds: CapabilityKind[] = ["tool", "resource", "prompt"];

  const next: MCPProxyPolicy[] = [];
  for (const policy of policies) {
    if (policy.name === REWRITE_POLICY_NAME) {
      const pruned = pruneRewritePolicy(policy, validIds, CAPABILITY_SECTION_KEY, kinds);
      if (pruned) next.push(pruned);
    } else if (policy.name === ACL_POLICY_NAME) {
      const pruned = pruneAclPolicy(policy, validIds, CAPABILITY_SECTION_KEY, kinds);
      if (pruned) next.push(pruned);
    } else {
      next.push(policy);
    }
  }

  return next.length > 0 ? next : undefined;
}

function pruneRewritePolicy(
  policy: MCPProxyPolicy,
  validIds: Record<CapabilityKind, Set<string>>,
  sectionKey: Record<CapabilityKind, string>,
  kinds: CapabilityKind[],
): MCPProxyPolicy | null {
  const params = (policy.params ?? {}) as Record<string, unknown>;
  const nextParams: Record<string, unknown> = { ...params };
  let hasAny = false;

  for (const kind of kinds) {
    const entries = params[sectionKey[kind]];
    if (!Array.isArray(entries)) continue;
    const kept = (entries as Record<string, unknown>[]).filter((entry) => {
      const id = rewriteEntryId(kind, entry);
      return id != null && validIds[kind].has(id);
    });
    if (kept.length > 0) {
      nextParams[sectionKey[kind]] = kept;
      hasAny = true;
    } else {
      delete nextParams[sectionKey[kind]];
    }
  }

  if (!hasAny) return null;
  return { ...policy, params: nextParams };
}

function pruneAclPolicy(
  policy: MCPProxyPolicy,
  validIds: Record<CapabilityKind, Set<string>>,
  sectionKey: Record<CapabilityKind, string>,
  kinds: CapabilityKind[],
): MCPProxyPolicy | null {
  const params = (policy.params ?? {}) as Record<string, unknown>;
  const nextParams: Record<string, unknown> = { ...params };
  let hasAny = false;

  for (const kind of kinds) {
    const section = params[sectionKey[kind]] as
      | Record<string, unknown>
      | undefined;
    if (!section) continue;
    // Drop the whole section when the new capabilities have nothing of this kind.
    if (validIds[kind].size === 0) {
      delete nextParams[sectionKey[kind]];
      continue;
    }
    const exceptions = Array.isArray(section.exceptions)
      ? (section.exceptions as unknown[]).filter(
          (entry): entry is string =>
            typeof entry === "string" && validIds[kind].has(entry.trim()),
        )
      : [];
    nextParams[sectionKey[kind]] = { ...section, exceptions };
    hasAny = true;
  }

  if (!hasAny) return null;
  return { ...policy, params: nextParams };
}

/**
 * Longest handle we derive from a blank-name endpoint. The backend composes the deployed
 * gateway artifact's name/displayName as `{proxyHandle}-{endpointHandle}-{envUUID}`, which
 * must stay within the gateway's 100-char displayName limit. A raw URL host
 *  slugifies to ~70 chars and blows that budget once the proxy handle and
 * 32-char environment suffix are added — deploying then fails gateway-side. Bounding the
 * derived handle keeps the blank-name case well clear; the backend still validates the
 * full composite for callers that bypass this derivation.
 */
const MAX_DERIVED_ENDPOINT_HANDLE_LENGTH = 30;

/**
 * Derives an endpoint handle (id) from its name, then the fetched server name, then the URL
 * host, and finally a positional `endpoint-N`. The result is slugified to the `[a-z0-9-]`
 * handle charset and bounded to MAX_DERIVED_ENDPOINT_HANDLE_LENGTH so the backend's composite
 * artifact handle stays within the gateway's length limit.
 *
 * When `usedHandles` is supplied, the derived handle is de-duplicated against it (two new
 * endpoints sharing a name or URL host would otherwise collide, which the backend rejects
 * via UNIQUE(mcp_proxy_uuid, handle)): a numeric suffix is appended until unique, and the
 * chosen handle is added to the set so subsequent calls see it.
 */
export function deriveEndpointHandle(
  draft: Pick<EndpointDraft, "name" | "url" | "serverName">,
  index: number,
  usedHandles?: Set<string>,
): string {
  const positional = `endpoint-${index + 1}`;
  const base =
    boundHandle(
      slugify(draft.name ?? "") ||
        slugify(draft.serverName ?? "") ||
        slugify(hostFromUrl(draft.url)),
    ) || positional;

  if (!usedHandles) return base;

  let handle = base;
  let suffix = 2;
  while (usedHandles.has(handle)) {
    // Reserve room for the suffix so the collision-resolved handle still honours
    // MAX_DERIVED_ENDPOINT_HANDLE_LENGTH (a bare `${base}-${suffix}` would overrun it).
    const suffixStr = `-${suffix}`;
    const truncatedBase =
      boundHandle(base, MAX_DERIVED_ENDPOINT_HANDLE_LENGTH - suffixStr.length) ||
      positional;
    handle = `${truncatedBase}${suffixStr}`;
    suffix += 1;
  }
  usedHandles.add(handle);
  return handle;
}

/**
 * Converts an edited draft into a native backend endpoint (1:1). When an `existing`
 * endpoint is supplied (edit path), its policies, security and tool-scope bindings are
 * preserved — policies pruned to the endpoint's surviving capabilities — and its handle is
 * reused. Capabilities follow the draft's fetched info. The endpoint's environment
 * bindings are the draft's flat env-UUID list; `deploymentStatus` is response-only and
 * never sent.
 */
export function draftToEndpoint(
  draft: EndpointDraft,
  index: number,
  existing?: MCPProxyEndpoint,
  usedHandles?: Set<string>,
): MCPProxyEndpoint {
  const auth = buildUpstreamAuth(draft);
  const capabilities: MCPProxyCapabilities = {
    tools: draft.fetchedInfo.tools,
    resources: draft.fetchedInfo.resources,
    prompts: draft.fetchedInfo.prompts,
  };
  const name = (draft.name ?? "").trim();

  // An edited endpoint keeps its handle; register it so freshly-derived handles for
  // new endpoints in the same batch don't collide with it.
  let id: string;
  if (existing?.id !== undefined) {
    id = existing.id;
    usedHandles?.add(id);
  } else {
    id = deriveEndpointHandle(draft, index, usedHandles);
  }

  const trimmedResilienceTimeout = draft.resilienceTimeout.trim();
  const trimmedResilienceIdleTimeout = draft.resilienceIdleTimeout.trim();

  return {
    id,
    name: name || undefined,
    upstream: {
      ...existing?.upstream,
      main: {
        url: draft.url,
        auth,
      },
    },
    resilience:
      trimmedResilienceTimeout || trimmedResilienceIdleTimeout
        ? {
            timeout: trimmedResilienceTimeout || undefined,
            idleTimeout: trimmedResilienceIdleTimeout || undefined,
          }
        : undefined,
    capabilities,
    // The MCP versions this endpoint's server reported; left out when it reported none,
    // so the proxy's mcpSpecVersion applies.
    mcpSpecVersions: draft.fetchedInfo.supportedVersions?.length
      ? draft.fetchedInfo.supportedVersions
      : undefined,
    policies: prunePoliciesForCapabilities(existing?.policies, capabilities),
    security: existing?.security ?? DEFAULT_ENDPOINT_SECURITY,
    environments: draft.environments.map((environmentUuid) => ({
      environmentUuid,
      gatewayId: draft.gatewayByEnv?.[environmentUuid],
    })),
  };
}

function slugify(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

// Truncates an already-slugified handle to `max` (default MAX_DERIVED_ENDPOINT_HANDLE_LENGTH),
// stripping any hyphen the cut leaves dangling so the result stays a clean `[a-z0-9-]` handle.
// Callers reserving room for a de-dup suffix pass a smaller `max` so the suffixed handle still
// honours MAX_DERIVED_ENDPOINT_HANDLE_LENGTH.
function boundHandle(
  handle: string,
  max: number = MAX_DERIVED_ENDPOINT_HANDLE_LENGTH,
): string {
  if (handle.length <= max) return handle;
  return handle.slice(0, max).replace(/-+$/g, "");
}

function hostFromUrl(url: string): string {
  try {
    return new URL(url).hostname;
  } catch {
    return "";
  }
}

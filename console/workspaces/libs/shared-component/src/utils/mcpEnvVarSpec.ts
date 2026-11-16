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

import type {
  MCPProxy,
  MCPProxyEndpoint,
  ProjectMCPProxy,
} from "@agent-management-platform/types";
import {
  resolveAuthenticationType,
  type AuthenticationType,
} from "./mcpEndpointSecurity";

/**
 * One row in an "Environment Variables References" table: a variable the
 * platform injects into the agent deployment, with the (possibly
 * user-overridden) name the agent code reads.
 */
export interface EnvVarReferenceRow {
  /** Canonical key identifying the variable. */
  key: string;
  /** The (possibly user-overridden) environment variable name. */
  name: string;
  /** Human-readable description of what the variable holds. */
  description?: string;
}

/**
 * The env var keys a user can name for a tool binding. `apikey` only applies to
 * API-key-secured endpoints — see {@link resolveMCPEnvVarSpec}.
 */
export const ENV_VAR_KEYS = ["url", "apikey"] as const;

export type EnvVarKey = (typeof ENV_VAR_KEYS)[number];

/**
 * The OAuth (AgentID) variables the platform injects for an identity-secured MCP
 * endpoint. Their names are fixed — only their values vary per environment — so
 * they are reference-only and never user-editable.
 */
export const AGENTID_ENV_VAR_ROWS: readonly EnvVarReferenceRow[] = [
  Object.freeze({
    key: "clientId",
    name: "AMP_AGENTID_CLIENT_ID",
    description: "This agent's OAuth2 client ID for this environment",
  }),
  Object.freeze({
    key: "clientSecret",
    name: "AMP_AGENTID_CLIENT_SECRET",
    description: "This agent's OAuth2 client secret for this environment",
  }),
  Object.freeze({
    key: "tokenEndpoint",
    name: "AMP_AGENTID_TOKEN_ENDPOINT",
    description: "Token endpoint to call with a client_credentials grant, passing this tool's URL as the resource parameter",
  }),
  Object.freeze({
    key: "scopes",
    name: "AMP_AGENTID_SCOPES",
    description: "Space-separated scopes to request for this tool's actions",
  }),
];
Object.freeze(AGENTID_ENV_VAR_ROWS);

/**
 * A project proxy's environments in the endpoint shape the resolvers below read,
 * one per environment. A project proxy's org GET carries no endpoints, so its
 * security comes from the project route; this lets the same rules judge both
 * kinds of proxy. Project bindings secure with an API key, identity (OAuth, met by
 * agents with their AgentID) or nothing, so `enabled`, `apiKey` and `identity` are
 * carried over. A
 * binding's capabilities are carried over as an endpoint's, so its tools list the
 * same way an org proxy's do.
 */
export function projectMCPProxyEndpoints(
  proxy: Pick<ProjectMCPProxy, "environments"> | null | undefined,
): MCPProxyEndpoint[] {
  return (proxy?.environments ?? []).map((environment) => ({
    id: environment.environmentName,
    security: environment.security
      ? {
          enabled: environment.security.enabled,
          apiKey: environment.security.apiKey,
          identity: environment.security.identity,
        }
      : undefined,
    capabilities: environment.capabilities,
    capabilityDiscovery: environment.capabilityDiscovery,
    environments: [{ environmentUuid: environment.environmentUuid }],
  }));
}

/**
 * The endpoint bound to `environmentUuid`, or undefined when the proxy has none.
 * Mirrors the server's per-environment resolution (`resolveMCPEndpointForEnv`),
 * which is the model the agent-creation flow follows because it targets exactly
 * one environment.
 *
 * Callers must branch on the *endpoint* rather than on its `security`, because a
 * bound endpoint with no security config means "unsecured", which is a different
 * answer from "no endpoint here".
 */
export function resolveMCPEndpointForEnvironment(
  proxy: Pick<MCPProxy, "endpoints"> | null | undefined,
  environmentUuid: string | undefined,
): MCPProxyEndpoint | undefined {
  if (!proxy?.endpoints?.length || !environmentUuid) return undefined;
  return proxy.endpoints.find((candidate) =>
    (candidate.environments ?? []).some(
      (binding) => binding.environmentUuid === environmentUuid,
    ),
  );
}

/**
 * The proxy's authentication type collapsed across all of its endpoints. This is
 * the environment-agnostic rule, used when a single binding maps every
 * environment to one proxy at once (the agent-configuration flow).
 *
 * A proxy counts as OAuth only when every endpoint is; otherwise any endpoint
 * needing an API key keeps the API key field available, since a mixed-security
 * proxy must still be nameable. A proxy whose endpoints are all unsecured (or a
 * mix of unsecured and OAuth) needs no API key at all.
 */
export function resolveProxyAuthenticationType(
  proxy: Pick<MCPProxy, "endpoints"> | null | undefined,
): AuthenticationType {
  const endpoints = proxy?.endpoints ?? [];
  if (endpoints.length === 0) return "";
  const types = endpoints.map((endpoint) => resolveAuthenticationType(endpoint));
  if (types.every((type) => type === "identity")) return "identity";
  if (types.some((type) => type === "apiKey")) return "apiKey";
  return "";
}

/** Which env vars a tool binding exposes, given how the endpoint is secured. */
export interface MCPEnvVarSpec {
  /** Keys the user can name. OAuth endpoints still need the URL. */
  editableKeys: EnvVarKey[];
  /** Fixed, platform-injected variables to show for reference only. */
  referenceRows: EnvVarReferenceRow[];
}

/**
 * The single source of truth for "which runtime variables does this MCP tool
 * binding need?". Both the agent-creation and agent-configuration flows derive
 * their field set from here so the two cannot drift apart again (issue #1597).
 *
 * Only an API-key endpoint has a key for the user to name. An OAuth (AgentID)
 * endpoint mints its credentials server-side and gets the fixed AMP_AGENTID_*
 * variables instead; an unsecured endpoint needs no credential at all. Offering
 * the API key field in either case produces an environment variable the platform
 * injects permanently empty. The URL is needed in all three cases.
 */
export function resolveMCPEnvVarSpec(
  authenticationType: AuthenticationType,
): MCPEnvVarSpec {
  switch (authenticationType) {
    case "apiKey":
      return { editableKeys: [...ENV_VAR_KEYS], referenceRows: [] };
    case "identity":
      return { editableKeys: ["url"], referenceRows: [...AGENTID_ENV_VAR_ROWS] };
    default:
      return { editableKeys: ["url"], referenceRows: [] };
  }
}

/**
 * The env var spec for a proxy considered across *all* of its endpoints at
 * once — used by the agent-configuration flow, which binds every environment
 * to the same proxy in a single call.
 *
 * Deriving this from {@link resolveProxyAuthenticationType} would collapse a
 * mixed proxy to one label first and lose information: a proxy with one
 * API-key endpoint and one OAuth endpoint resolves to `"apiKey"` (so the field
 * stays nameable), but feeding that single label into
 * {@link resolveMCPEnvVarSpec} would then drop the AgentID reference rows the
 * OAuth endpoint still needs. This resolver unions each endpoint's actual
 * requirement instead: the API key field is offered if any endpoint needs one,
 * and the AgentID rows are shown if any endpoint uses OAuth — independently of
 * each other.
 */
export function resolveProxyEnvVarSpec(
  proxy: Pick<MCPProxy, "endpoints"> | null | undefined,
): MCPEnvVarSpec {
  const types = (proxy?.endpoints ?? []).map((endpoint) =>
    resolveAuthenticationType(endpoint),
  );
  const needsAPIKey = types.some((type) => type === "apiKey");
  const needsIdentity = types.some((type) => type === "identity");
  return {
    editableKeys: needsAPIKey ? [...ENV_VAR_KEYS] : ["url"],
    referenceRows: needsIdentity ? [...AGENTID_ENV_VAR_ROWS] : [],
  };
}

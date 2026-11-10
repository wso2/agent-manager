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
  MCPEndpointConfig,
  MCPProxy,
  ProjectMCPProxy,
  ProjectMCPProxyEnvironment,
  ProjectMCPProxyEnvironmentRequest,
  ProjectMCPProxyEnvironmentSecurity,
  ProjectMCPProxyRequest,
} from "@agent-management-platform/types";
import type { EndpointDraft } from "./EndpointFormFields";

/** The longest proxy id the service takes. */
export const MAX_PROXY_ID_LENGTH = 100;

/** A kebab-case proxy id from a display name, as the service takes it. */
export function toProjectProxyId(name: string): string {
  return name
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, MAX_PROXY_ID_LENGTH)
    .replace(/-+$/g, "");
}

/** The upsert body that keeps a project proxy as it is; an edit changes only its own field. */
export function proxyRequestFrom(
  proxy: ProjectMCPProxy,
): ProjectMCPProxyRequest {
  const request: ProjectMCPProxyRequest = {
    name: proxy.name,
    version: proxy.version,
    context: proxy.context,
  };
  if (proxy.description) request.description = proxy.description;
  if (proxy.mcpSpecVersion) request.mcpSpecVersion = proxy.mcpSpecVersion;
  if (proxy.source) request.source = proxy.source;
  if (proxy.upstream) request.upstream = proxy.upstream;
  return request;
}

/** The header a binding's API key is read from when none is set (the service's default). */
export const DEFAULT_API_KEY_HEADER = "X-API-Key";

/** Whether the binding asks callers for an API key. */
export function bindingRequiresAPIKey(
  security: ProjectMCPProxyEnvironmentSecurity | undefined,
): boolean {
  if (!security) return true; // no security reads back as the default, an API key
  if (security.enabled === false) return false;
  return security.apiKey?.enabled !== false;
}

/**
 * The PUT body that keeps a binding as it is. The binding PUT replaces the whole
 * configuration, so every save starts from this and changes only its own field.
 * The upstream credential is sent without its value, which keeps the stored one.
 */
export function bindingRequestFrom(
  env: ProjectMCPProxyEnvironment,
): ProjectMCPProxyEnvironmentRequest {
  const request: ProjectMCPProxyEnvironmentRequest = {
    policies: env.policies ?? [],
  };
  if (env.gatewayId) request.gatewayId = env.gatewayId;
  if (env.releaseBindingName)
    request.releaseBindingName = env.releaseBindingName;
  if (env.security) {
    request.security = bindingRequiresAPIKey(env.security)
      ? {
          enabled: true,
          apiKey: {
            enabled: true,
            key: env.security.apiKey?.key || DEFAULT_API_KEY_HEADER,
            in: env.security.apiKey?.in ?? "header",
          },
        }
      : { enabled: false };
  }
  if (env.upstreamAuth?.type === "api-key" && env.upstreamAuth.header) {
    request.upstreamAuth = { type: "api-key", header: env.upstreamAuth.header };
  }
  // Capabilities a caller gave are sent back, as omitting them hands them to discovery.
  // Discovered ones are left out, which keeps them and keeps discovery on.
  if (env.capabilitiesSource === "caller" && env.capabilities) {
    request.capabilities = env.capabilities;
  }
  return request;
}

/**
 * A binding seen as an org endpoint config, so the org MCP server tabs (Overview, Connection,
 * Manage Tools, Rewrite, Policies) can show and edit it. A project proxy registered by URL calls
 * that URL in every environment; one with a source has no URL to show.
 */
export function bindingEndpointConfig(
  proxy: ProjectMCPProxy,
  env: ProjectMCPProxyEnvironment,
): MCPEndpointConfig {
  const auth =
    env.upstreamAuth?.type === "api-key" && env.upstreamAuth.header
      ? { type: "api-key" as const, header: env.upstreamAuth.header }
      : undefined;
  return {
    upstream: { main: { url: proxy.upstream?.url ?? "", auth } },
    policies: env.policies ?? [],
    capabilities: env.capabilities,
    mcpSpecVersions: proxy.mcpSpecVersion ? [proxy.mcpSpecVersion] : undefined,
    security: bindingRequiresAPIKey(env.security)
      ? {
          enabled: true,
          apiKey: {
            enabled: true,
            key: env.security?.apiKey?.key || DEFAULT_API_KEY_HEADER,
            in: env.security?.apiKey?.in ?? "header",
          },
        }
      : { enabled: false },
  };
}

/** What an org tab's edit of a binding's endpoint config changes on the project proxy. */
export interface BindingConfigChange {
  /** The binding PUT body with the edit applied. */
  binding: ProjectMCPProxyEnvironmentRequest;
  /**
   * The new URL of a proxy registered by URL, when the edit changed it. The URL is the
   * proxy's, so every environment takes it.
   */
  proxyURL?: string;
}

/**
 * Applies an org tab's edit of a binding's endpoint config. Policies, capabilities and the
 * upstream credential belong to the binding; the URL belongs to a proxy registered by URL.
 * Timeouts are not kept on bindings and are dropped.
 */
export function applyBindingConfigChange(
  proxy: ProjectMCPProxy,
  env: ProjectMCPProxyEnvironment,
  fields: Partial<MCPEndpointConfig>,
): BindingConfigChange {
  const binding = bindingRequestFrom(env);
  const change: BindingConfigChange = { binding };
  if (fields.policies !== undefined) binding.policies = fields.policies;
  if ("capabilities" in fields) {
    if (fields.capabilities) binding.capabilities = fields.capabilities;
    else delete binding.capabilities;
  }
  if ("upstream" in fields) {
    const main = fields.upstream?.main;
    const auth = main?.auth;
    if (auth?.header) {
      binding.upstreamAuth = {
        type: "api-key",
        header: auth.header,
        ...(auth.value ? { value: auth.value } : {}),
      };
    } else {
      delete binding.upstreamAuth;
    }
    const url = main?.url?.trim();
    if (proxy.upstream && url && url !== proxy.upstream.url) {
      change.proxyURL = url;
    }
  }
  return change;
}

/** The project proxy as the org Overview tab reads it. */
export function proxyAsOrgProxy(proxy: ProjectMCPProxy): MCPProxy {
  return {
    id: proxy.id,
    name: proxy.name,
    version: proxy.version,
    description: proxy.description,
    context: proxy.context,
    mcpSpecVersion: proxy.mcpSpecVersion,
    projectName: proxy.projectName,
    createdAt: proxy.createdAt,
    updatedAt: proxy.updatedAt,
  };
}

/**
 * A project proxy registered by URL as the org edit form's one endpoint: its URL, the
 * credential and tools of its first environment, and every environment it is deployed to.
 */
export function projectProxyEndpointDraft(
  proxy: ProjectMCPProxy,
): EndpointDraft {
  const first = proxy.environments[0];
  const gatewayByEnv: Record<string, string> = {};
  for (const env of proxy.environments) {
    if (env.gatewayId) gatewayByEnv[env.environmentUuid] = env.gatewayId;
  }
  return {
    id: "primary",
    name: "",
    url: proxy.upstream?.url ?? "",
    authHeader:
      first?.upstreamAuth?.type === "api-key"
        ? (first.upstreamAuth.header ?? "")
        : "",
    authValue: "",
    resilienceTimeout: "",
    resilienceIdleTimeout: "",
    environments: proxy.environments.map((env) => env.environmentUuid),
    gatewayByEnv,
    fetchedInfo: {
      tools: first?.capabilities?.tools ?? [],
      resources: first?.capabilities?.resources ?? [],
      prompts: first?.capabilities?.prompts ?? [],
      supportedVersions: proxy.mcpSpecVersion
        ? [proxy.mcpSpecVersion]
        : undefined,
    },
  };
}

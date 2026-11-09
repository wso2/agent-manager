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
  MCPProxyListItem,
  ProjectMCPProxy,
  ProjectMCPProxyEnvironment,
  ProjectMCPProxyEnvironmentRequest,
  ProjectMCPProxyEnvironmentSecurity,
  ProjectMCPProxyRequest,
} from "@agent-management-platform/types";

/** The longest proxy id the service takes. */
export const MAX_PROXY_ID_LENGTH = 100;

/**
 * Where a listed proxy's upstream is: a project component endpoint, or an external server
 * (an org server, or a project server registered by URL).
 */
export function proxySourceLabel(proxy: MCPProxyListItem): string {
  if (!proxy.source) return "External";
  return `${proxy.source.componentName} / ${proxy.source.endpointName}`;
}

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
export function proxyRequestFrom(proxy: ProjectMCPProxy): ProjectMCPProxyRequest {
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
  if (env.releaseBindingName) request.releaseBindingName = env.releaseBindingName;
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
  return request;
}

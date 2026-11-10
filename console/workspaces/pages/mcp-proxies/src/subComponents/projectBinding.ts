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

/** An environment a new project server could not be deployed to, and why. */
export interface BindingFailure {
  environment: string;
  message: string;
}

/** The navigation state the register form hands the server's page. */
export interface ProjectProxyViewState {
  bindingFailures?: BindingFailure[];
}

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
  if (env.resilience) request.resilience = env.resilience;
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
    resilience: env.resilience,
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
 * Applies an org tab's edit of a binding's endpoint config. Policies, capabilities, timeouts and
 * the upstream credential belong to the binding; the URL belongs to a proxy registered by URL.
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
  if ("resilience" in fields) {
    if (fields.resilience) binding.resilience = fields.resilience;
    else delete binding.resilience;
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
    resilienceTimeout: first?.resilience?.timeout ?? "",
    resilienceIdleTimeout: first?.resilience?.idleTimeout ?? "",
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

/**
 * The PUT body of an environment the edit form keeps. It keeps the binding's own credential and
 * timeouts, which may differ from the first environment's the form loaded, and replaces each only
 * when the user changed it in the form.
 */
export function keptBindingRequest(
  env: ProjectMCPProxyEnvironment,
  initial: EndpointDraft,
  draft: EndpointDraft,
  edited: Pick<ProjectMCPProxyEnvironmentRequest, "upstreamAuth" | "resilience">,
): ProjectMCPProxyEnvironmentRequest {
  const request = bindingRequestFrom(env);
  if (credentialEdited(initial, draft)) {
    if (edited.upstreamAuth) request.upstreamAuth = edited.upstreamAuth;
    else delete request.upstreamAuth;
  }
  if (timeoutsEdited(initial, draft)) {
    if (edited.resilience) request.resilience = edited.resilience;
    else delete request.resilience;
  }
  return request;
}

/** Whether the form's credential was changed: a new header, or a value typed. */
export function credentialEdited(
  initial: EndpointDraft,
  draft: EndpointDraft,
): boolean {
  return draft.authHeader.trim() !== initial.authHeader.trim() || !!draft.authValue;
}

/** Whether the form's request or idle timeout was changed. */
export function timeoutsEdited(
  initial: EndpointDraft,
  draft: EndpointDraft,
): boolean {
  return (
    draft.resilienceTimeout.trim() !== initial.resilienceTimeout.trim() ||
    draft.resilienceIdleTimeout.trim() !== initial.resilienceIdleTimeout.trim()
  );
}

/** Whether two URLs share scheme, host and port; a URL that does not parse matches nothing. */
export function sameOrigin(a: string | undefined, b: string | undefined): boolean {
  try {
    return !!a && !!b && new URL(a.trim()).origin === new URL(b.trim()).origin;
  } catch {
    return false;
  }
}

/** One request of a URL server's edit save, in the order they are sent. */
export type URLProxySaveStep =
  | { kind: "remove"; envName: string }
  | { kind: "binding"; envName: string; body: ProjectMCPProxyEnvironmentRequest }
  | { kind: "proxy"; url: string };

/** What the edit form sets on every environment, when the user changed it. */
export interface URLProxyEdits {
  url: string;
  upstreamAuth?: ProjectMCPProxyEnvironmentRequest["upstreamAuth"];
  resilience?: ProjectMCPProxyEnvironmentRequest["resilience"];
}

/**
 * The requests that save an edit of a server registered by URL, checked before any is sent so a
 * save that cannot succeed writes nothing. Unassigned environments go first. When the URL moves
 * to another host, the service refuses it while a credential is stored, and a stored credential
 * must not reach the new host: each kept one is cleared before the URL changes and set again,
 * with a value typed now, after it.
 */
export function planURLProxySave(
  proxy: ProjectMCPProxy,
  initial: EndpointDraft,
  draft: EndpointDraft,
  edits: URLProxyEdits,
  added: { envName: string; gatewayId?: string }[],
): URLProxySaveStep[] {
  const kept = new Set(draft.environments);
  const keptEnvs = proxy.environments.filter((env) => kept.has(env.environmentUuid));
  const auth = edits.upstreamAuth;
  // A credential is never read back, so a new environment can only take one typed now.
  if (added.length > 0 && auth?.header && !auth.value) {
    throw new Error(
      "Enter the credential value again to deploy the server to a new environment.",
    );
  }
  // A changed header with no value keeps each environment's stored value; one with none
  // stored has nothing to keep.
  if (
    credentialEdited(initial, draft) &&
    auth?.header &&
    !auth.value &&
    keptEnvs.some((env) => !env.upstreamAuth)
  ) {
    throw new Error(
      "Enter the credential value: an environment the server is deployed to has none stored.",
    );
  }
  const kepts = keptEnvs.map((env) => ({
    env,
    body: keptBindingRequest(env, initial, draft, edits),
  }));
  const movesHost = !sameOrigin(proxy.upstream?.url, edits.url);
  if (movesHost && kepts.some(({ body }) => body.upstreamAuth && !body.upstreamAuth.value)) {
    throw new Error(
      "Enter the credential value again: a stored credential is not sent to a new host.",
    );
  }

  const steps: URLProxySaveStep[] = proxy.environments
    .filter((env) => !kept.has(env.environmentUuid))
    .map((env) => ({ kind: "remove", envName: env.environmentName }));
  if (movesHost) {
    for (const env of keptEnvs) {
      if (!env.upstreamAuth) continue;
      const cleared = bindingRequestFrom(env);
      delete cleared.upstreamAuth;
      steps.push({ kind: "binding", envName: env.environmentName, body: cleared });
    }
  }
  steps.push({ kind: "proxy", url: edits.url });
  for (const { env, body } of kepts) {
    steps.push({ kind: "binding", envName: env.environmentName, body });
  }
  for (const { envName, gatewayId } of added) {
    steps.push({
      kind: "binding",
      envName,
      body: {
        ...(gatewayId ? { gatewayId } : {}),
        ...(auth ? { upstreamAuth: auth } : {}),
        ...(edits.resilience ? { resilience: edits.resilience } : {}),
      },
    });
  }
  return steps;
}

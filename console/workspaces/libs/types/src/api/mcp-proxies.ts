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

import type { ListQuery, OrgPathParams, OrgProjPathParams } from "./common";
import type {
  APIKeySecurity,
  CreateLLMAPIKeyRequest,
  CreateLLMAPIKeyResponse,
  IdentitySecurity,
  Resilience,
  RotateLLMAPIKeyRequest,
  RotateLLMAPIKeyResponse,
  SecurityConfig,
  UpstreamAuth,
  UpstreamConfig,
} from "./llm-providers";

export interface MCPProxyCapabilities {
  tools?: Record<string, unknown>[];
  resources?: Record<string, unknown>[];
  prompts?: Record<string, unknown>[];
}

export interface MCPProxyPolicy {
  name: string;
  version: string;
  displayName?: string;
  executionCondition?: string;
  params?: Record<string, unknown>;
}

export interface MCPPolicyAvailableItem {
  name: string;
  version: string;
}

export interface MCPPolicyAvailabilityResponse {
  count: number;
  list: MCPPolicyAvailableItem[];
}

/**
 * MCPEndpointConfig is the deployable configuration of a single MCP proxy endpoint:
 * upstream (URL + auth), policies, capabilities and security. It is the flat config
 * carried on each MCPProxyEndpoint. Environment binding and per-environment deployment
 * status live on MCPEndpointEnvironment, not here. Scopes gating individual tools are
 * managed separately, at the proxy level (see MCPProxyScopeResponse).
 */
export interface MCPEndpointConfig {
  upstream?: UpstreamConfig;
  resilience?: Resilience;
  policies?: MCPProxyPolicy[];
  capabilities?: MCPProxyCapabilities;
  /** MCP versions this endpoint's server supports, from its fetch-server-info answer. */
  mcpSpecVersions?: string[];
  security?: SecurityConfig;
  /**
   * Agent Manager's discovery of these capabilities from the MCP server. Only on a
   * project proxy's binding (carried over by projectMCPProxyEndpoints), and only
   * while its capabilities are not the caller's.
   */
  capabilityDiscovery?: MCPCapabilityDiscoveryStatus;
}

/**
 * Agent Manager's discovery of a project MCP proxy binding's capabilities from the
 * MCP server, through the binding's gateway. Pending while queued or retrying (lastError
 * says why the last attempt failed); Failed once given up on until the next deploy or
 * refresh.
 */
export interface MCPCapabilityDiscoveryStatus {
  status: "Pending" | "Discovered" | "Failed";
  lastError?: string;
  discoveredAt?: string;
}

/**
 * MCPEndpointEnvironment is one endpoint→environment binding. deploymentStatus is
 * response-only: it reports whether this environment's gateway artifact is currently
 * deployed ("Deployed") or not ("Undeployed"). Computed on read; never sent.
 *
 * gatewayId is the egress gateway to deploy this binding to. Optional: inferred
 * server-side when the environment has exactly one egress-capable gateway, required
 * (400s) when it has more than one. Placement is fixed once deployed — the server
 * rejects a different gatewayId on update. Create/Update responses do NOT echo it;
 * only GET/list derives it as the gateway the binding is actually deployed to.
 */
export interface MCPEndpointEnvironment {
  environmentUuid: string;
  deploymentStatus?: "Deployed" | "Undeployed";
  gatewayId?: string;
}

/**
 * MCPProxyEndpoint is one deployable endpoint of an MCP proxy. Its id is unique within the
 * parent proxy. The endpoint's flat config (upstream/policies/capabilities/security) applies
 * to every environment it is bound to via environments; within a proxy an environment maps to
 * at most one endpoint.
 */
export interface MCPProxyEndpoint extends MCPEndpointConfig {
  id: string;
  name?: string;
  environments: MCPEndpointEnvironment[];
}

/**
 * MCPProxy is an org-level grouping. Name/version/context/vhost/mcpSpecVersion are shared
 * metadata; the deployable config lives on each endpoint in endpoints. The proxy itself
 * deploys nothing to any gateway.
 */
export interface MCPProxy {
  id: string;
  inCatalog?: boolean;
  name: string;
  version: string;
  description?: string;
  createdBy?: string;
  context?: string;
  vhost?: string;
  mcpSpecVersion?: string;
  /**
   * Absent on a project-owned proxy (projectName set): its per-environment data is on the
   * project route, see ProjectMCPProxy.
   */
  endpoints?: MCPProxyEndpoint[];
  /** Set for a project-owned proxy (read only on the org routes). Absent for an org proxy. */
  projectName?: string;
  createdAt?: string;
  updatedAt?: string;
}

/** A project proxy binding's security: an API key, identity, or none (enabled false). */
export interface ProjectMCPProxyEnvironmentSecurity {
  enabled?: boolean;
  apiKey?: APIKeySecurity;
  identity?: IdentitySecurity;
}

/**
 * The credential the gateway adds when it calls a project proxy's upstream, as returned:
 * never the value.
 */
export interface ProjectMCPProxyEnvironmentUpstreamAuth {
  type: "api-key" | "none";
  header?: string;
}

/**
 * The body of PUT .../projects/{proj}/mcp-proxies/{proxyId}/environments/{envName}. It replaces the
 * binding's configuration: an omitted field resets to its default. An api-key upstreamAuth sent
 * without a value keeps the stored one.
 */
export interface ProjectMCPProxyEnvironmentRequest {
  gatewayId?: string;
  releaseBindingName?: string;
  security?: ProjectMCPProxyEnvironmentSecurity;
  policies?: MCPProxyPolicy[];
  upstreamAuth?: ProjectMCPProxyEnvironmentUpstreamAuth & { value?: string };
  /**
   * The tools, prompts and resources the server offers, stored as given. Omitted, Agent
   * Manager discovers them through the binding's gateway and keeps what it discovered.
   */
  capabilities?: MCPProxyCapabilities;
  /** The binding's request and idle timeouts; omitted, the gateway's defaults. */
  resilience?: Resilience;
}

/** One environment binding of a project-owned MCP proxy, as the project route reports it. */
export interface ProjectMCPProxyEnvironment {
  environmentName: string;
  environmentUuid: string;
  context: string;
  deploymentStatus?: "Deploying" | "Deployed" | "Failed";
  deploymentStatusReason?: string;
  gatewayId?: string;
  releaseBindingName?: string;
  publicUrl?: string;
  security?: ProjectMCPProxyEnvironmentSecurity;
  policies?: MCPProxyPolicy[];
  upstreamAuth?: ProjectMCPProxyEnvironmentUpstreamAuth;
  /**
   * The binding's tools, prompts and resources: the ones given on the binding PUT, or the
   * ones Agent Manager discovered from the MCP server (see capabilitiesSource). Absent
   * when there are none yet.
   */
  capabilities?: MCPProxyCapabilities;
  /** Where capabilities came from; absent with no capabilities. */
  capabilitiesSource?: "caller" | "discovered";
  /** The capability discovery, when one was queued and the capabilities are not the caller's. */
  capabilityDiscovery?: MCPCapabilityDiscoveryStatus;
  /** The binding's request and idle timeouts; absent when the gateway's defaults apply. */
  resilience?: Resilience;
  updatedAt?: string;
}

/** The OpenChoreo component endpoint a project MCP proxy fronts. */
export interface MCPProxySource {
  componentName: string;
  endpointName: string;
}

/** The public MCP server URL a project proxy registered by URL fronts. */
export interface ProjectMCPProxyUpstream {
  url: string;
  /**
   * Request only. On a URL change to another origin, the environments whose stored upstream
   * credential the update clears, rather than refuse the change. A binding with a stored
   * credential that is not named refuses it with 409: the caller's view is out of date.
   */
  resetBindingAuthFor?: string[];
}

/**
 * The body of PUT /orgs/{org}/projects/{proj}/mcp-proxies/{proxyId}. Exactly one of source
 * (a project component endpoint) and upstream (a public URL) is set, fixed after create.
 */
export interface ProjectMCPProxyRequest {
  name: string;
  description?: string;
  version?: string;
  context?: string;
  mcpSpecVersion?: string;
  source?: MCPProxySource;
  upstream?: ProjectMCPProxyUpstream;
}

/** A project-owned MCP proxy (GET /orgs/{org}/projects/{proj}/mcp-proxies/{proxyId}). */
export interface ProjectMCPProxy {
  id: string;
  name: string;
  description?: string;
  version: string;
  context: string;
  mcpSpecVersion?: string;
  projectName: string;
  /** Set for a proxy that fronts a project component endpoint. */
  source?: MCPProxySource;
  /** Set for a proxy registered by a public URL. */
  upstream?: ProjectMCPProxyUpstream;
  environments: ProjectMCPProxyEnvironment[];
  createdBy?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface MCPProxyListItem {
  id?: string;
  name?: string;
  version?: string;
  description?: string;
  createdBy?: string;
  context?: string;
  status?: "pending" | "deployed" | "failed";
  mcpSpecVersion?: string;
  /** Set for a project-scoped proxy: the project that owns it. Absent for an org proxy. */
  projectName?: string;
  /** The component endpoint a project-scoped proxy fronts. Absent for an org proxy. */
  source?: MCPProxySource;
  /** The environments the proxy is bound to. */
  environments?: MCPProxyListEnvironment[];
  createdAt?: string;
  updatedAt?: string;
}

/** One environment a listed MCP proxy is bound to. */
export interface MCPProxyListEnvironment {
  environmentUuid: string;
  /** Set for a project-scoped proxy, whose binding is named after its environment. */
  environmentName?: string;
}

export interface MCPProxyPagination {
  count: number;
  limit: number;
  offset: number;
}

export interface MCPProxyListResponse {
  count: number;
  list: MCPProxyListItem[];
  pagination: MCPProxyPagination;
}

export interface MCPServerInfoFetchRequest {
  url?: string;
  proxyId?: string;
  auth?: UpstreamAuth;
}

export interface MCPServerInfoFetchResponse {
  serverInfo?: Record<string, unknown>;
  tools?: Record<string, unknown>[];
  resources?: Record<string, unknown>[];
  prompts?: Record<string, unknown>[];
  supportedVersions?: string[];
}

export type CreateMCPProxyPathParams = OrgPathParams;
export type DeleteMCPProxyPathParams = OrgPathParams & { proxyId: string };
export type GetMCPProxyPathParams = OrgPathParams & { proxyId: string };
export type GetProjectMCPProxyPathParams = OrgProjPathParams & { proxyId: string };
export type RefreshProjectMCPProxyCapabilitiesPathParams = GetProjectMCPProxyPathParams & {
  envName: string;
};
export type UpdateMCPProxyPathParams = OrgPathParams & { proxyId: string };
export type ListMCPProxiesPathParams = OrgPathParams;
export type ListAvailableMCPPoliciesPathParams = OrgPathParams;
export type ListMCPProxiesQuery = ListQuery & {
  /**
   * OpenChoreo project name. When set, the list also includes that project's
   * project-scoped MCP proxies alongside the org-level ones.
   */
  project?: string;
};
export type FetchMCPProxyServerInfoPathParams = OrgPathParams;

export type ProjectMCPProxyEnvironmentPathParams = GetProjectMCPProxyPathParams & {
  envName: string | undefined;
};
export type ProjectMCPProxyAPIKeyPathParams = ProjectMCPProxyEnvironmentPathParams & {
  keyName: string;
};
export type DeleteProjectMCPProxyPathParams = GetProjectMCPProxyPathParams;
export type CreateProjectMCPProxyAPIKeyRequest = CreateLLMAPIKeyRequest;
export type CreateProjectMCPProxyAPIKeyResponse = CreateLLMAPIKeyResponse;
export type RotateProjectMCPProxyAPIKeyRequest = RotateLLMAPIKeyRequest;
export type RotateProjectMCPProxyAPIKeyResponse = RotateLLMAPIKeyResponse;

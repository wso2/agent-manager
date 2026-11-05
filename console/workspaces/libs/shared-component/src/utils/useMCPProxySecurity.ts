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

import { useMemo } from "react";
import {
  useGetMCPProxy,
  useGetProjectMCPProxy,
} from "@agent-management-platform/api-client";
import type { MCPProxy } from "@agent-management-platform/types";
import {
  projectMCPProxyEndpoints,
  resolveMCPEndpointForEnvironment,
  resolveMCPEnvVarSpec,
  resolveProxyAuthenticationType,
  resolveProxyEnvVarSpec,
  type MCPEnvVarSpec,
} from "./mcpEnvVarSpec";
import {
  resolveAuthenticationType,
  type AuthenticationType,
} from "./mcpEndpointSecurity";

export interface UseMCPProxySecurityParams {
  orgName?: string;
  /** The MCP proxy the tool binding points at. Empty disables the fetch. */
  proxyId?: string | null;
  /**
   * Scope the answer to one environment's endpoint. Omit (or pass undefined,
   * which happens when the environment carries no uuid) to fall back to the
   * environment-agnostic every-endpoint rule.
   */
  environmentUuid?: string;
}

export interface UseMCPProxySecurityResult {
  /** How the resolved endpoint is secured: API key, OAuth, or none. */
  authenticationType: AuthenticationType;
  /** True when the resolved endpoint is secured with OAuth (AgentID). */
  usesIdentitySecurity: boolean;
  /** Which env vars to offer and which to show for reference. */
  spec: MCPEnvVarSpec;
  /** The proxy fetch is still in flight, so `spec` is not yet trustworthy. */
  isLoading: boolean;
  /** The proxy fetch failed, so the security kind is unknown. */
  isError: boolean;
  /**
   * `spec` reflects the proxy's real security. False while loading, on a failed
   * fetch, and when the proxy came back empty.
   *
   * Callers MUST gate on this rather than reading `authenticationType`
   * directly. An unresolved fetch yields `""`, which is indistinguishable from a
   * genuinely unsecured endpoint — and acting on it hides the API key field and
   * drops the variable from the payload, the same silent-empty-credential
   * failure as issue #1597 with the polarity reversed. There is no safe guess
   * here: assuming "apiKey" reintroduces #1597 for OAuth proxies. Surface the
   * error instead.
   */
  isResolved: boolean;
}

export interface UseMCPProxyEndpointsResult {
  /**
   * The endpoints the security rules judge from: an org proxy's own, or a project
   * proxy's environments in the same shape (see projectMCPProxyEndpoints).
   * Undefined until there is an answer, and while no proxy is selected.
   */
  proxy: Pick<MCPProxy, "endpoints"> | undefined;
  isLoading: boolean;
  isError: boolean;
}

/**
 * The endpoints of an MCP proxy, as far as how it is secured goes. An org proxy's
 * are on its org GET. A project proxy (its org GET has `projectName` set) carries
 * no endpoints there; its per-environment bindings are read from the project
 * route instead, under the project the org GET names, so a caller needs no
 * project context of its own.
 */
export function useMCPProxyEndpoints({
  orgName,
  proxyId,
}: Pick<UseMCPProxySecurityParams, "orgName" | "proxyId">): UseMCPProxyEndpointsResult {
  const {
    data: orgProxy,
    isLoading: isOrgLoading,
    isError: isOrgError,
  } = useGetMCPProxy({
    orgName,
    proxyId: proxyId ?? "",
  });

  // Only a project proxy asks its project route; for an org proxy the query stays
  // disabled (no project, no proxy id).
  const ownerProject = proxyId ? orgProxy?.projectName : undefined;
  const {
    data: projectProxy,
    isLoading: isProjectLoading,
    isError: isProjectError,
  } = useGetProjectMCPProxy({
    orgName,
    projName: ownerProject ?? "",
    proxyId: ownerProject ? (proxyId ?? "") : "",
  });

  const proxy = useMemo<Pick<MCPProxy, "endpoints"> | undefined>(() => {
    if (!proxyId || !orgProxy) return undefined;
    if (!ownerProject) return orgProxy;
    if (!projectProxy) return undefined;
    return { endpoints: projectMCPProxyEndpoints(projectProxy) };
  }, [proxyId, orgProxy, ownerProject, projectProxy]);

  return {
    proxy,
    isLoading: isOrgLoading || (Boolean(ownerProject) && isProjectLoading),
    isError: isOrgError || (Boolean(ownerProject) && isProjectError),
  };
}

/**
 * Resolves how an MCP proxy's endpoint is secured and, from that, which runtime
 * variables a tool binding needs. `MCPProxyListItem` carries no security data, so
 * the full proxy has to be fetched — this hook is the one place that does it.
 *
 * An org proxy's security is on its endpoints; a project proxy's is on its
 * project route's environments (see useMCPProxyEndpoints).
 *
 * When `environmentUuid` is supplied and the proxy has an endpoint (or, for a
 * project proxy, an environment) bound to it, the answer is that one's security,
 * matching the server's per-environment resolution. Otherwise it falls back to
 * the every-endpoint rule, which is the sound answer when one binding covers all
 * environments at once, and also the safe answer when the environment simply has
 * no uuid to match on.
 */
export function useMCPProxySecurity({
  orgName,
  proxyId,
  environmentUuid,
}: UseMCPProxySecurityParams): UseMCPProxySecurityResult {
  const { proxy, isLoading, isError } = useMCPProxyEndpoints({ orgName, proxyId });

  // Resolved once, then reused: authenticationType needs it to derive one
  // label, and — for the fallback case — spec needs the whole proxy anyway to
  // union each endpoint's requirement rather than derive from that one label.
  const scopedEndpoint = useMemo(
    () => (proxy ? resolveMCPEndpointForEnvironment(proxy, environmentUuid) : undefined),
    [proxy, environmentUuid],
  );

  const authenticationType = useMemo<AuthenticationType>(() => {
    if (!proxy) return "";
    if (scopedEndpoint) return resolveAuthenticationType(scopedEndpoint);
    return resolveProxyAuthenticationType(proxy);
  }, [proxy, scopedEndpoint]);

  const spec = useMemo<MCPEnvVarSpec>(() => {
    if (!proxy) return resolveMCPEnvVarSpec("");
    // Scoped to one endpoint: a single label is the whole answer.
    if (scopedEndpoint) return resolveMCPEnvVarSpec(authenticationType);
    // Environment-agnostic: union every endpoint's requirement instead of
    // collapsing to one label first, or a proxy mixing an API-key endpoint
    // with an OAuth endpoint would lose the OAuth reference rows.
    return resolveProxyEnvVarSpec(proxy);
  }, [proxy, scopedEndpoint, authenticationType]);

  // No proxy selected yet means nothing to wait for, whatever the query says.
  const hasProxy = Boolean(proxyId);
  // A proxy that fetched successfully but reports zero endpoints (or a project
  // proxy with no environments bound yet) is not a state this hook can make a
  // security determination from — treat it the same as an unresolved fetch
  // rather than silently defaulting to None.
  const hasEndpoints = Boolean(proxy?.endpoints?.length);

  return {
    authenticationType,
    usesIdentitySecurity: authenticationType === "identity",
    spec,
    isLoading: hasProxy && isLoading,
    isError: hasProxy && isError,
    // React Query reports isLoading false once a fetch errors, so `proxy` being
    // present (with endpoints to judge) is the only thing that proves the
    // answer is real.
    isResolved: !hasProxy || hasEndpoints,
  };
}

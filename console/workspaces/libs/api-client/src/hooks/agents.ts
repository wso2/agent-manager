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

import { useMemo } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useParams } from "react-router-dom";
import {
  createAgent, deleteAgent, getAgent, listAgents, listOrgAgents, generateAgentToken, updateAgent,
  updateAgentBuildParameters, getAgentRoles, getAgentGroups, getAgentIdentity,
  provisionAgentIdentity, regenerateAgentIdentitySecret, retryAgentIdentityProvisioning,
  revokeAgentIdentitySecret,
} from "../apis";
import { SLOW_POLL_INTERVAL } from "../utils";
import type {
  AgentListResponse,
  AgentResponse,
  AgentSummary,
  AgentSummaryListResponse,
  CreateAgentPathParams,
  CreateAgentRequest,
  DeleteAgentPathParams,
  GetAgentPathParams,
  ListAgentsPathParams,
  ListAgentsQuery,
  ListOrgAgentsPathParams,
  UpdateAgentPathParams,
  UpdateAgentRequest,
  UpdateAgentBuildParametersPathParams,
  UpdateAgentBuildParametersRequest,
  GenerateAgentTokenPathParams,
  GenerateAgentTokenQuery,
  TokenRequest,
  TokenResponse,
  GetAgentRolesPathParams,
  GetAgentRolesQuery,
  AgentRolesResponse,
  GetAgentGroupsPathParams,
  GetAgentGroupsQuery,
  AgentGroupsResponse,
  GetAgentIdentityPathParams,
  GetAgentIdentityQuery,
  AgentIdentityEnvironmentView,
  ProvisionAgentIdentityPathParams,
  ProvisionAgentIdentityQuery,
  RegenerateAgentIdentitySecretPathParams,
  RetryAgentIdentityProvisioningPathParams,
  AgentIdentityActionRequest,
  AgentRegenerateSecretResponse,
  RevokeAgentIdentitySecretPathParams,
  RevokeAgentIdentitySecretQuery,
  AgentRevokeSecretResponse,
} from "@agent-management-platform/types";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "./react-query-notifications";

export function useListAgents(
  params: ListAgentsPathParams,
  query?: ListAgentsQuery,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentListResponse>({
    queryKey: ['agents', params, query],
    queryFn: () => listAgents(params, query, getToken),
    enabled: !!params.orgName && !!params.projName,
  });
}

// Org-wide, lightweight (name + displayName only) agent listing across all
// projects — for pickers/selectors, not the per-project ListAgents table.
export function useListOrgAgents(params: ListOrgAgentsPathParams) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentSummaryListResponse>({
    queryKey: ['org-agents', params],
    queryFn: () => listOrgAgents(params, getToken),
    enabled: !!params.orgName,
  });
}

// Agent names are only unique within a project, so any lookup keyed off name
// alone risks colliding across projects — always pair it with projectName.
const orgAgentSummaryKey = (projectName: string, name: string) => `${projectName}::${name}`;

export interface OrgAgentDisplayResolver {
  // Resolves an agent's real display name; falls back to the raw name when
  // it's missing from the org-wide list (still loading, or not an agent at
  // all — e.g. a monitor).
  resolveAgentName: (projectName: string, name: string) => string;
  // Same fallback contract as resolveAgentName, but for the owning project.
  resolveProjectName: (projectName: string, name: string) => string;
}

// Single source of truth for resolving a raw (projectName, name) pair to its
// real display name and project display name, backed by the org-wide agent
// list. Every consumer that needs to show a display name instead of a raw
// name/project should go through this, rather than re-deriving the lookup.
export function useOrgAgentDisplayNames(params: ListOrgAgentsPathParams): OrgAgentDisplayResolver {
  const { data } = useListOrgAgents(params);
  const byKey = useMemo(() => {
    const map = new Map<string, AgentSummary>();
    for (const agent of data?.agents ?? []) {
      map.set(orgAgentSummaryKey(agent.projectName, agent.name), agent);
    }
    return map;
  }, [data]);

  return useMemo(
    () => ({
      resolveAgentName: (projectName: string, name: string) =>
        byKey.get(orgAgentSummaryKey(projectName, name))?.displayName ?? name,
      resolveProjectName: (projectName: string, name: string) =>
        byKey.get(orgAgentSummaryKey(projectName, name))?.projectDisplayName ?? projectName,
    }),
    [byKey],
  );
}

export function useGetAgent(params: GetAgentPathParams) {
    const { getToken } = useAuthHooks();
    return useApiQuery<AgentResponse>({
        queryKey: ['agent', params],
        queryFn: () => getAgent(params, getToken),
        enabled: !!params.orgName && !!params.projName && !!params.agentName,
    });
}

/**
 * The agent named in the current URL (/org/:orgId/project/:projectId/agents/:agentId).
 * Shares the query cache with every other useGetAgent call for that agent, so
 * using it in many places (for example plugin activation hooks) costs one request.
 * Disabled, with no data, on pages that have no agent in the URL.
 */
export function useRouteAgent() {
  const { orgId, projectId, agentId } = useParams();
  return useGetAgent({ orgName: orgId, projName: projectId, agentName: agentId });
}

export function useCreateAgent() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentResponse,
    unknown,
    { params: CreateAgentPathParams; body: CreateAgentRequest }
  >({
    action: { verb: 'create', target: 'agent' },
    mutationFn: ({ params, body }) => createAgent(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agents'] });
      queryClient.invalidateQueries({ queryKey: ['org-agents'] });
    },
  });
}

export function useUpdateAgent() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentResponse,
    unknown,
    { params: UpdateAgentPathParams; body: UpdateAgentRequest }
  >({
    action: { verb: 'update', target: 'agent' },
    mutationFn: ({ params, body }) => updateAgent(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agents'] });
      queryClient.invalidateQueries({ queryKey: ['agent'] });
      queryClient.invalidateQueries({ queryKey: ['org-agents'] });
    },
  });
}

export function useUpdateAgentBuildParameters() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentResponse,
    unknown,
    { params: UpdateAgentBuildParametersPathParams; body: UpdateAgentBuildParametersRequest }
  >({
    action: { verb: 'update', target: 'agent build parameters' },
    mutationFn: ({ params, body }) => updateAgentBuildParameters(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agents'] });
      queryClient.invalidateQueries({ queryKey: ['agent'] });
    },
  });
}

export function useDeleteAgent() {
    const { getToken } = useAuthHooks();
    const queryClient = useQueryClient();
    return useApiMutation<void, unknown, DeleteAgentPathParams>({
      action: { verb: 'delete', target: 'agent' },
        mutationFn: (params) => deleteAgent(params, getToken),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['agents'] });
            queryClient.invalidateQueries({ queryKey: ['org-agents'] });
        },
    });
}


// Lazy, cache-backed token generation. It mints only when `enabled` is set true (explicit intent)
// and every automatic refetch trigger is disabled, so it never silently re-mints on
// mount/refocus/remount (#1140). Caching (staleTime/gcTime Infinity) keeps the minted token visible
// across the drawer's remounts within a session; callers force a new token with refetch().
export function useGenerateAgentToken(
  params: GenerateAgentTokenPathParams,
  body?: TokenRequest,
  query?: GenerateAgentTokenQuery,
  enabled: boolean = false,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<TokenResponse>({
    // Duration is intentionally NOT in the key: changing it must not auto-mint. refetch() re-runs
    // queryFn with the latest duration on explicit (re)generate.
    queryKey: ["agent-token", params.agentName, params.projName, params.orgName, query?.environment],
    queryFn: () => generateAgentToken(params, body, query, getToken),
    enabled,
    retry: false,
    staleTime: Infinity,
    gcTime: Infinity,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
}

// --- Agent identity: roles/groups (read-only) ---

export function useGetAgentRoles(
  params: GetAgentRolesPathParams,
  query: GetAgentRolesQuery,
  options?: { enabled?: boolean },
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentRolesResponse>({
    queryKey: ['agent-roles', params, query],
    queryFn: () => getAgentRoles(params, query, getToken),
    enabled: (options?.enabled ?? true)
      && !!params.orgName && !!params.projName && !!params.agentName && !!query.environment,
  });
}

export function useGetAgentGroups(
  params: GetAgentGroupsPathParams,
  query: GetAgentGroupsQuery,
  options?: { enabled?: boolean },
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentGroupsResponse>({
    queryKey: ['agent-groups', params, query],
    queryFn: () => getAgentGroups(params, query, getToken),
    enabled: (options?.enabled ?? true)
      && !!params.orgName && !!params.projName && !!params.agentName && !!query.environment,
  });
}

// --- Agent identity: AgentID lifecycle (per environment) ---

export function useGetAgentIdentity(
  params: GetAgentIdentityPathParams,
  query?: GetAgentIdentityQuery,
) {
  const { getToken } = useAuthHooks();
  return useApiQuery<AgentIdentityEnvironmentView[]>({
    queryKey: ['agent-identity', params, query],
    queryFn: () => getAgentIdentity(params, query, getToken),
    enabled: !!params.orgName && !!params.projName && !!params.agentName,
    // Provisioning happens in the background (write-ahead PENDING, then a
    // best-effort attempt) — poll while any binding is still settling, and
    // stop automatically once every binding has completed or failed.
    refetchInterval: (q) => {
      const views = q.state.data;
      const stillProvisioning = views?.some(
        (v) => v.status === 'pending' || v.status === 'in_progress',
      );
      return stillProvisioning ? SLOW_POLL_INTERVAL : false;
    },
  });
}

interface AgentIdentityBindingParams {
  orgId: string;
  projectId: string;
  agentId: string;
  envId: string;
}

/**
 * Shared "is this environment's AgentID binding usable" read. Several
 * consumers across pages (the overview's roles/groups list, the identity
 * regenerate button, the identity claim/reveal UI) each need to know whether
 * provisioning has completed — this centralizes that definition instead of
 * every consumer re-deriving `status === "completed"` from its own copy of
 * the binding.
 */
export function useAgentIdentityBinding({
  orgId, projectId, agentId, envId,
}: AgentIdentityBindingParams) {
  const { data: identityViews, isLoading, isError, error } = useGetAgentIdentity(
    { orgName: orgId, projName: projectId, agentName: agentId },
    { environment: envId },
  );
  const binding = identityViews?.[0];

  return {
    binding,
    provisioned: binding?.status === "completed",
    isLoading,
    isError,
    error,
  };
}

export function useProvisionAgentIdentity() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentIdentityEnvironmentView,
    unknown,
    { params: ProvisionAgentIdentityPathParams; query: ProvisionAgentIdentityQuery }
  >({
    action: { verb: 'create', target: 'agent identity' },
    mutationFn: ({ params, query }) => provisionAgentIdentity(params, query, getToken),
    // A fixed, humanized message rather than the raw backend error: the
    // realistic failure here is the environment's ThunderID instance being
    // briefly unreachable, not something the exact backend wording helps a
    // user act on. The Retry button on a failed attempt uses the backend's
    // stored lastError instead, which is a persisted, already-user-facing
    // diagnostic rather than a raw transport error.
    errorMessage: "Couldn't create the Agent ID for this environment. Check that the environment is reachable, then try again.",
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['agent-identity', params] });
    },
  });
}

export function useRegenerateAgentIdentitySecret() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentRegenerateSecretResponse,
    unknown,
    { params: RegenerateAgentIdentitySecretPathParams; body: AgentIdentityActionRequest }
  >({
    action: { verb: 'rotate', target: 'agent identity secret' },
    mutationFn: ({ params, body }) => regenerateAgentIdentitySecret(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['agent-identity', params] });
    },
  });
}

export function useRetryAgentIdentityProvisioning() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentIdentityEnvironmentView,
    unknown,
    { params: RetryAgentIdentityProvisioningPathParams; body: AgentIdentityActionRequest }
  >({
    action: { verb: 'rerun', target: 'agent identity provisioning' },
    mutationFn: ({ params, body }) => retryAgentIdentityProvisioning(params, body, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['agent-identity', params] });
    },
  });
}

export function useRevokeAgentIdentitySecret() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    AgentRevokeSecretResponse,
    unknown,
    { params: RevokeAgentIdentitySecretPathParams; query: RevokeAgentIdentitySecretQuery }
  >({
    action: { verb: 'revoke', target: 'agent identity secret' },
    mutationFn: ({ params, query }) => revokeAgentIdentitySecret(params, query, getToken),
    onSuccess: (_data, { params }) => {
      queryClient.invalidateQueries({ queryKey: ['agent-identity', params] });
    },
  });
}


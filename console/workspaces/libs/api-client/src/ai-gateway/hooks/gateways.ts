/**
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAuthHooks } from "@agent-management-platform/auth";
import { useApiMutation, useApiQuery } from "../../shared/react-query-notifications";
import { useUIModulePresent } from "../../shared/useUIModulePresent";
import { POLL_INTERVAL } from "../../shared/utils";
import type {
  CreateSelfHostedGatewayPathParams,
  DeleteGatewayPathParams,
  DeleteIdentityProviderPathParams,
  DiscoverOidcPathParams,
  DiscoverOidcQuery,
  GatewayEnvironmentResponse,
  GatewayListResponse,
  GatewayResponse,
  GatewayTokenListResponse,
  GatewayTokenResponse,
  GetGatewayPathParams,
  IdentityProvider,
  IdentityProviderListResponse,
  ListGatewaysPathParams,
  ListGatewaysQuery,
  ListIdentityProvidersPathParams,
  OidcDiscoveryResponse,
  RevokeSelfHostedGatewayTokenPathParams,
  SelfHostedGatewayRequest,
  SelfHostedGatewayTokensPathParams,
  SelfHostedGatewayUpdateRequest,
  UpdateSelfHostedGatewayPathParams,
  UpsertIdentityProviderPathParams,
  UpsertIdentityProviderRequest,
} from "@agent-management-platform/types";
import {
  createSelfHostedGateway,
  deleteGateway,
  deleteIdentityProvider,
  discoverOidc,
  gatewayEnvironment,
  gatewayFromDocument,
  gatewayListFromDocuments,
  getGateway,
  listGatewayTokens,
  listGateways,
  listIdentityProviders,
  revokeGatewayToken,
  rotateGatewayToken,
  updateSelfHostedGateway,
  upsertIdentityProvider,
} from "../apis/gateways";
import { listEnvironments } from "../../agent-runtime/apis/deployments";

/**
 * The organization's environments, which every gateway serves. A caller who cannot read them still
 * sees its gateways, without environments; any other failure fails the query.
 */
async function gatewayEnvironments(
  orgName: string | undefined,
  getToken: () => Promise<string>,
): Promise<GatewayEnvironmentResponse[]> {
  try {
    return (await listEnvironments({ orgName }, getToken)).map(gatewayEnvironment);
  } catch (err) {
    if ((err as { code?: string } | null)?.code === "FORBIDDEN") return [];
    throw err;
  }
}

export function useListGateways(
  params: ListGatewaysPathParams,
  query?: ListGatewaysQuery,
) {
  const gatewayPresent = useUIModulePresent("ai-gateway");
  const { getToken } = useAuthHooks();
  return useApiQuery<GatewayListResponse>({
    queryKey: ["gateways", params, query],
    queryFn: async () => {
      const [list, environments] = await Promise.all([
        listGateways(params, query, getToken),
        gatewayEnvironments(params.orgName, getToken),
      ]);
      return gatewayListFromDocuments(list, environments);
    },
    enabled: gatewayPresent && !!params.orgName,
  });
}

/** A gateway is active once it has connected to the control plane. */
export function isGatewayActive(gateway: GatewayResponse | undefined): boolean {
  return (
    gateway?.status === "ACTIVE" ||
    (gateway as { isActive?: boolean } | undefined)?.isActive === true
  );
}

/**
 * Reads one gateway. With `pollUntilActive`, it refetches every POLL_INTERVAL until
 * the gateway connects, then refreshes the gateway list so it shows the new status.
 */
export function useGetGateway(
  params: GetGatewayPathParams,
  options?: { pollUntilActive?: boolean },
) {
  const gatewayPresent = useUIModulePresent("ai-gateway");
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  const pollUntilActive = options?.pollUntilActive ?? false;
  const query = useApiQuery<GatewayResponse>({
    queryKey: ["gateway", params],
    queryFn: async () => {
      const [doc, environments] = await Promise.all([
        getGateway(params, getToken),
        gatewayEnvironments(params.orgName, getToken),
      ]);
      return gatewayFromDocument(doc, environments);
    },
    enabled: gatewayPresent && !!params.orgName && !!params.gatewayId,
    refetchInterval: (q) =>
      pollUntilActive && !isGatewayActive(q.state.data) ? POLL_INTERVAL : false,
  });

  const active = isGatewayActive(query.data);
  useEffect(() => {
    if (pollUntilActive && active) {
      void queryClient.invalidateQueries({ queryKey: ["gateways"] });
    }
  }, [pollUntilActive, active, queryClient]);

  return query;
}

export function useListIdentityProviders(
  params: ListIdentityProvidersPathParams,
) {
  const gatewayPresent = useUIModulePresent("ai-gateway");
  const { getToken } = useAuthHooks();
  return useApiQuery<IdentityProviderListResponse>({
    queryKey: ["identity-providers", params],
    queryFn: () => listIdentityProviders(params, getToken),
    enabled: gatewayPresent && !!params.orgName,
  });
}

/**
 * Resolves issuer + JWKS URI from an OIDC discovery URL to auto-fill the Add
 * Identity Provider dialog. Triggered on demand (button click); notifications
 * are suppressed so the dialog can surface success/failure inline.
 */
export function useDiscoverOidc(params: DiscoverOidcPathParams) {
  const { getToken } = useAuthHooks();
  return useApiMutation<OidcDiscoveryResponse, unknown, DiscoverOidcQuery>({
    mutationFn: (query) => discoverOidc(params, query, getToken),
    showSuccess: false,
    showError: false,
  });
}

/**
 * Upserts a gateway identity provider via the REST API. Used by the managed
 * (cloud) path of the Add Identity Provider dialog, where the server also patches
 * the gateway runtime config; the self-hosted path renders the script instead.
 */
export function useUpsertIdentityProvider() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    IdentityProvider,
    unknown,
    { params: UpsertIdentityProviderPathParams; body: UpsertIdentityProviderRequest }
  >({
    action: { verb: 'create', target: 'identity provider' },
    mutationFn: ({ params, body }) => upsertIdentityProvider(params, body, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["identity-providers"] });
    },
  });
}

export function useDeleteIdentityProvider() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, DeleteIdentityProviderPathParams>({
    action: { verb: 'remove', target: 'identity provider' },
    mutationFn: (params) => deleteIdentityProvider(params, getToken),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["identity-providers"] });
    },
  });
}

function invalidateGatewayQueries(queryClient: ReturnType<typeof useQueryClient>) {
  queryClient.invalidateQueries({ queryKey: ["gateways"] });
  queryClient.invalidateQueries({ queryKey: ["gateway"] });
}

/** Deletes a self-hosted gateway; a managed one is read-only. */
export function useDeleteGateway() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, DeleteGatewayPathParams>({
    action: { verb: 'delete', target: 'gateway' },
    mutationFn: (params) => deleteGateway(params, getToken),
    onSuccess: () => invalidateGatewayQueries(queryClient),
  });
}

/** Tokens change nothing a gateway query returns, so they have a root key of their own. */
const selfHostedGatewayTokensKey = ({ orgName, gatewayId }: SelfHostedGatewayTokensPathParams) => [
  "self-hosted-gateway-tokens",
  { orgName, gatewayId },
];

export function useCreateSelfHostedGateway() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    GatewayResponse,
    unknown,
    { params: CreateSelfHostedGatewayPathParams; body: SelfHostedGatewayRequest }
  >({
    action: { verb: 'create', target: 'self-hosted gateway' },
    mutationFn: async ({ params, body }) =>
      gatewayFromDocument(await createSelfHostedGateway(params, body, getToken)),
    onSuccess: () => invalidateGatewayQueries(queryClient),
  });
}

export function useUpdateSelfHostedGateway() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<
    GatewayResponse,
    unknown,
    { params: UpdateSelfHostedGatewayPathParams; body: SelfHostedGatewayUpdateRequest }
  >({
    action: { verb: 'update', target: 'self-hosted gateway' },
    mutationFn: async ({ params, body }) =>
      gatewayFromDocument(await updateSelfHostedGateway(params, body, getToken)),
    onSuccess: () => invalidateGatewayQueries(queryClient),
  });
}

/** With `enabled: false` nothing loads on mount; `refetch` still fetches on demand. */
export function useSelfHostedGatewayTokens(
  params: SelfHostedGatewayTokensPathParams,
  { enabled = true }: { enabled?: boolean } = {},
) {
  const gatewayPresent = useUIModulePresent("ai-gateway");
  const { getToken } = useAuthHooks();
  return useApiQuery<GatewayTokenListResponse>({
    queryKey: selfHostedGatewayTokensKey(params),
    queryFn: () => listGatewayTokens(params, getToken),
    enabled: gatewayPresent && enabled && !!params.orgName && !!params.gatewayId,
  });
}

export function useCreateSelfHostedGatewayToken() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<GatewayTokenResponse, unknown, SelfHostedGatewayTokensPathParams>({
    action: { verb: 'create', target: 'registration token' },
    mutationFn: (params) => rotateGatewayToken(params, getToken),
    onSuccess: (_data, params) => {
      queryClient.invalidateQueries({ queryKey: selfHostedGatewayTokensKey(params) });
    },
  });
}

export function useRevokeSelfHostedGatewayToken() {
  const { getToken } = useAuthHooks();
  const queryClient = useQueryClient();
  return useApiMutation<void, unknown, RevokeSelfHostedGatewayTokenPathParams>({
    action: { verb: 'revoke', target: 'registration token' },
    mutationFn: (params) => revokeGatewayToken(params, getToken),
    onSuccess: (_data, params) => {
      queryClient.invalidateQueries({ queryKey: selfHostedGatewayTokensKey(params) });
    },
  });
}
